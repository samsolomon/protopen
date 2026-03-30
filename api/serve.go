package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const privateSiteHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Private Site</title></head>
<body style="font-family:system-ui;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0">
<p>This site is private. <a href="%s">Sign in</a> to view it.</p>
</body></html>`

func (app *application) serveProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.NotFound(w, r)
		return
	}

	orgSlug, slug, deployID, assetPath, ok := parseProjectPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var deployment liveDeploy
	var err error
	if deployID != "" {
		deployment, err = app.lookupDeployByID(r.Context(), orgSlug, slug, deployID)
	} else {
		deployment, err = app.lookupLiveDeploy(r.Context(), orgSlug, slug)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("lookup deploy: %v", err)
		http.Error(w, "could not load project", http.StatusInternalServerError)
		return
	}

	// Always attempt auth so comment widget knows who the user is.
	// For private projects, deny access if not authenticated or not in org.
	user, _ := app.requireSessionUser(r)
	if !deployment.isPublic {
		if user.ID == "" || !userInOrg(user, orgSlug) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, privateSiteHTML, app.frontendOrigin)
			return
		}
	}

	deploys := app.listToolbarDeploys(r.Context(), orgSlug, slug)
	baseURL := fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, orgSlug, slug)
	snippet := frameSnippet(deployment.projectName, app.frontendOrigin, deploys, deployment.deployID, baseURL, deployment.projectID, assetPath, user.ID, user.Name, orgSlug)
	fw := newFrameWriter(w, snippet)
	defer fw.Close()

	if app.store != nil {
		app.serveFromR2(fw, r, deployment.siteRoot, assetPath)
	} else {
		app.serveFromFilesystem(fw, r, deployment.siteRoot, assetPath)
	}
}

func (app *application) serveFromR2(w http.ResponseWriter, r *http.Request, prefix string, assetPath string) {
	prefix = strings.TrimSuffix(prefix, "/")

	// Determine the R2 key to fetch
	key := prefix + "/index.html"
	if assetPath != "" {
		normalized, err := normalizeUploadPath(assetPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		key = prefix + "/" + normalized
	}

	// Try exact key
	body, contentType, err := app.store.download(r.Context(), key)
	if err != nil && !isNotFound(err) {
		log.Printf("r2 download error: %v", err)
		http.Error(w, "could not serve file", http.StatusInternalServerError)
		return
	}

	if err == nil {
		defer body.Close()
		serveR2Response(w, key, contentType, body)
		return
	}

	if assetPath != "" && path.Ext(assetPath) == "" {
		dirKey := key + "/index.html"
		body, contentType, err = app.store.download(r.Context(), dirKey)
		if err == nil {
			defer body.Close()
			serveR2Response(w, dirKey, contentType, body)
			return
		}

		indexKey := prefix + "/index.html"
		body, contentType, err = app.store.download(r.Context(), indexKey)
		if err == nil {
			defer body.Close()
			w.Header().Set("Cache-Control", "no-store")
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			} else {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
			}
			io.Copy(w, body)
			return
		}
	}

	http.NotFound(w, r)
}

func serveR2Response(w http.ResponseWriter, key string, contentType string, body io.Reader) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else if ct := mime.TypeByExtension(filepath.Ext(key)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	if strings.HasSuffix(key, "index.html") {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}

	io.Copy(w, body)
}

func (app *application) serveFromFilesystem(w http.ResponseWriter, r *http.Request, siteRoot string, assetPath string) {
	resolvedPath, fallbackToIndex, err := resolveAssetPath(siteRoot, assetPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if fallbackToIndex {
		resolvedPath = filepath.Join(siteRoot, "index.html")
	}

	if err := serveFile(w, r, resolvedPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		log.Printf("serve project file: %v", err)
		http.Error(w, "could not serve file", http.StatusInternalServerError)
	}
}

func userInOrg(user sessionUser, orgSlug string) bool {
	for _, org := range user.Orgs {
		if org.Slug == orgSlug {
			return true
		}
	}
	return false
}

func parseProjectPath(rawPath string) (orgSlug string, slug string, deployID string, assetPath string, ok bool) {
	trimmed := strings.Trim(rawPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "~") {
		return "", "", "", "", false
	}

	orgSlug = strings.TrimPrefix(parts[0], "~")
	slug = parts[1]
	if orgSlug == "" || slug == "" {
		return "", "", "", "", false
	}

	if len(parts) > 2 && parts[2] == "_v" {
		if len(parts) < 4 || parts[3] == "" {
			return "", "", "", "", false
		}
		deployID = parts[3]
		if len(parts) > 4 {
			assetPath = strings.Join(parts[4:], "/")
		}
	} else if len(parts) > 2 {
		assetPath = strings.Join(parts[2:], "/")
	}

	return orgSlug, slug, deployID, assetPath, true
}

func (app *application) lookupDeployByID(ctx context.Context, orgSlug string, slug string, deployID string) (liveDeploy, error) {
	var d liveDeploy
	err := app.db.QueryRow(ctx, `
		select p.id, d.id, d.storage_prefix, p.is_public, p.name
		from projects p
		join organizations o on o.id = p.org_id
		join deploys d on d.id = $3 and d.project_id = p.id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
	`, orgSlug, slug, deployID).Scan(&d.projectID, &d.deployID, &d.siteRoot, &d.isPublic, &d.projectName)
	if err != nil {
		return liveDeploy{}, err
	}
	return d, nil
}

type toolbarDeploy struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Time          string  `json:"time"`
	IsCurrent     bool    `json:"isCurrent"`
	GitCommitHash *string `json:"gitCommitHash,omitempty"`
	GitBranch     *string `json:"gitBranch,omitempty"`
}

func (app *application) listToolbarDeploys(ctx context.Context, orgSlug string, slug string) []toolbarDeploy {
	rows, err := app.db.Query(ctx, `
		select d.id, d.label, d.created_at, (d.id = p.current_deploy_id) as is_current,
			d.git_commit_hash, d.git_branch
		from deploys d
		join projects p on p.id = d.project_id
		join organizations o on o.id = p.org_id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
		order by d.created_at asc
		limit 50
	`, orgSlug, slug)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var deploys []toolbarDeploy
	for rows.Next() {
		var d toolbarDeploy
		var label *string
		var createdAt time.Time
		if err := rows.Scan(&d.ID, &label, &createdAt, &d.IsCurrent, &d.GitCommitHash, &d.GitBranch); err != nil {
			return nil
		}
		if label != nil {
			d.Label = *label
		}
		d.Time = relativeTime(createdAt)
		deploys = append(deploys, d)
	}
	return deploys
}

func (app *application) lookupLiveDeploy(ctx context.Context, orgSlug string, slug string) (liveDeploy, error) {
	var d liveDeploy
	err := app.db.QueryRow(ctx, `
		select p.id, d.id, d.storage_prefix, p.is_public, p.name
		from projects p
		join organizations o on o.id = p.org_id
		join deploys d on d.id = p.current_deploy_id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
	`, orgSlug, slug).Scan(&d.projectID, &d.deployID, &d.siteRoot, &d.isPublic, &d.projectName)
	if err != nil {
		return liveDeploy{}, err
	}
	return d, nil
}

func resolveAssetPath(siteRoot string, requested string) (string, bool, error) {
	if requested == "" {
		return filepath.Join(siteRoot, "index.html"), false, nil
	}

	normalized, err := normalizeUploadPath(requested)
	if err != nil {
		return "", false, err
	}

	resolved := filepath.Join(siteRoot, filepath.FromSlash(normalized))
	relativePath, err := filepath.Rel(siteRoot, resolved)
	if err != nil {
		return "", false, err
	}
	if strings.HasPrefix(relativePath, "..") || relativePath == "." {
		return "", false, fs.ErrNotExist
	}

	info, err := os.Stat(resolved)
	if err == nil && !info.IsDir() {
		return resolved, false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}

	directoryIndex := filepath.Join(resolved, "index.html")
	if directoryInfo, directoryErr := os.Stat(directoryIndex); directoryErr == nil && !directoryInfo.IsDir() {
		return directoryIndex, false, nil
	} else if directoryErr != nil && !errors.Is(directoryErr, fs.ErrNotExist) {
		return "", false, directoryErr
	}

	if path.Ext(normalized) != "" {
		return "", false, fs.ErrNotExist
	}

	return filepath.Join(siteRoot, "index.html"), true, nil
}

func serveFile(w http.ResponseWriter, r *http.Request, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if contentType := mime.TypeByExtension(filepath.Ext(filePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	if strings.EqualFold(filepath.Base(filePath), "index.html") {
		w.Header().Set("Cache-Control", "no-store")
		io.Copy(w, file)
		return nil
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeContent(w, r, filepath.Base(filePath), info.ModTime(), file)
	return nil
}
