package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const sentinelOrgID = "org_anonymous"

var anonAdjectives = []string{
	"bright", "calm", "cool", "crisp", "early", "fair", "fast", "fine",
	"fresh", "glad", "gold", "keen", "kind", "late", "lean", "light",
	"live", "mild", "neat", "new", "next", "open", "pale", "pure",
	"rare", "real", "rich", "safe", "sharp", "slim", "soft", "still",
	"sure", "tall", "tidy", "true", "vast", "warm", "wide", "wise",
}

var anonNouns = []string{
	"apex", "arch", "beam", "bell", "bolt", "cape", "card", "clay",
	"coin", "core", "cove", "dawn", "dome", "dune", "edge", "fern",
	"ford", "gate", "glen", "grid", "halo", "haze", "helm", "hill",
	"iris", "isle", "jade", "lake", "lane", "lark", "leaf", "link",
	"loom", "luna", "mast", "mesa", "mint", "mist", "moon", "moss",
	"nest", "node", "opal", "orb", "palm", "path", "peak", "pine",
	"pond", "reed", "reef", "ring", "rock", "rose", "sage", "sand",
	"silk", "snow", "star", "stem", "tide", "vale", "vine", "wave",
}

var allowedAnonExtensions = map[string]bool{
	".html": true, ".htm": true,
	".css":   true,
	".js":    true, ".mjs": true, ".jsx": true, ".ts": true, ".tsx": true,
	".json":  true,
	".png":   true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true, ".ico": true, ".avif": true,
	".woff":  true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".pdf":   true,
	".mp4":   true, ".webm": true, ".ogg": true, ".mp3": true, ".wav": true, ".m4a": true,
	".wasm":  true,
	".txt":   true, ".xml": true, ".csv": true,
	".map":   true,
	".glb":   true, ".gltf": true,
}

func generateAnonSlug() string {
	adj := anonAdjectives[randInt(len(anonAdjectives))]
	noun := anonNouns[randInt(len(anonNouns))]

	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		suffix[i] = chars[randInt(len(chars))]
	}

	return adj + "-" + noun + "-" + string(suffix)
}

func randInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(err)
	}
	return int(n.Int64())
}

func validateAnonFileTypes(files []fileMeta) error {
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(f.Name))
		}
		if ext == "" {
			continue // extensionless files (e.g., LICENSE) are fine
		}
		if !allowedAnonExtensions[ext] {
			return fmt.Errorf("file type %s is not allowed for anonymous publishes", ext)
		}
	}
	return nil
}

type anonPublishResponse struct {
	SiteURL    string `json:"siteUrl"`
	Slug       string `json:"slug"`
	ClaimToken string `json:"claimToken"`
	ClaimURL   string `json:"claimUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

func (app *application) publishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	prepared, err := app.prepareUpload(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	needsCleanup := app.store != nil
	defer func() {
		if needsCleanup {
			cleanupPreparedUpload(prepared)
		}
	}()

	if err := validateUpload(prepared.request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := validateAnonFileTypes(prepared.request.Files); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	slug := generateAnonSlug()

	var storagePrefix string
	if app.store != nil && prepared.normalizedTo != "" {
		storagePrefix = "anon/" + slug
		if err := app.uploadToR2WithPrefix(r.Context(), prepared, storagePrefix); err != nil {
			log.Printf("r2 upload (anonymous): %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store uploaded files"})
			return
		}
	} else if prepared.normalizedTo != "" {
		storagePrefix = prepared.normalizedTo
		needsCleanup = false
	}

	now := time.Now().UTC()
	claimToken := generateToken(32)
	expiresAt := now.Add(24 * time.Hour)

	tx, err := app.db.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		log.Printf("begin tx (anonymous publish): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	defer tx.Rollback(r.Context())

	projectID := generateID("proj")
	projectName := strings.TrimSpace(prepared.request.Name)

	if _, err := tx.Exec(r.Context(), `
		insert into projects (id, org_id, slug, name, is_public, created_at, updated_at)
		values ($1, $2, $3, $4, true, $5, $5)
	`, projectID, sentinelOrgID, slug, projectName, now); err != nil {
		log.Printf("insert project (anonymous): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	totalSize := int64(0)
	for _, f := range prepared.request.Files {
		totalSize += f.Size
	}

	deployID := generateID("dep")
	if _, err := tx.Exec(r.Context(), `
		insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at)
		values ($1, $2, 'validated', $3, $4, $5, $6)
	`, deployID, projectID, totalSize, len(prepared.request.Files), storagePrefix, now); err != nil {
		log.Printf("insert deploy (anonymous): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if _, err := tx.Exec(r.Context(), `
		update projects set current_deploy_id = $2, updated_at = $3 where id = $1
	`, projectID, deployID, now); err != nil {
		log.Printf("set current deploy (anonymous): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	anonID := generateID("anon")
	if _, err := tx.Exec(r.Context(), `
		insert into anonymous_deploys (id, slug, deploy_id, project_id, claim_token_hash, expires_at, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, anonID, slug, deployID, projectID, hashToken(claimToken), expiresAt, now); err != nil {
		log.Printf("insert anonymous_deploys: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("commit (anonymous publish): %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	siteURL := app.contentBaseURL + "/" + slug
	claimURL := app.frontendOrigin + "/claim/" + slug

	writeJSON(w, http.StatusOK, anonPublishResponse{
		SiteURL:    siteURL,
		Slug:       slug,
		ClaimToken: claimToken,
		ClaimURL:   claimURL,
		ExpiresAt:  expiresAt.Format(time.RFC3339),
	})
}
