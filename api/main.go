package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	demoUserEmail = "sam@velori.dev"
	demoUserName  = "Sam Solomon"
	demoUsername  = "sam"
	demoPassword  = "velori-demo"
	sessionCookie = "velori_session"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type application struct {
	db             *pgxpool.Pool
	ingestRoot     string
	contentBaseURL string
	frontendOrigin string
}

type sessionUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

type project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	UpdatedAt   string `json:"updatedAt"`
	DeployCount int    `json:"deployCount"`
	LiveURL     string `json:"liveUrl"`
}

type uploadRequest struct {
	Name  string     `json:"name"`
	Mode  string     `json:"mode"`
	Files []fileMeta `json:"files"`
}

type fileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

type preparedUpload struct {
	request      uploadRequest
	ingestRoot   string
	normalizedTo string
}

func main() {
	ctx := context.Background()
	databaseURL := getenv("DATABASE_URL", "postgres://velori:velori@localhost:5432/velori?sslmode=disable")
	ingestRoot := getenv("INGEST_ROOT", filepath.Join(".data", "ingest"))
	appListenAddr := getenv("APP_LISTEN_ADDR", ":8080")
	contentListenAddr := getenv("CONTENT_LISTEN_ADDR", ":8081")
	contentBaseURL := strings.TrimRight(getenv("PUBLIC_CONTENT_URL", "http://127.0.0.1:8081"), "/")
	frontendOrigin := strings.TrimRight(getenv("FRONTEND_ORIGIN", "http://localhost:5173"), "/")

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("create database pool: %v", err)
	}
	defer db.Close()

	if err := runMigrations(ctx, db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := os.MkdirAll(ingestRoot, 0o755); err != nil {
		log.Fatalf("create ingest root: %v", err)
	}

	app := &application{db: db, ingestRoot: ingestRoot, contentBaseURL: contentBaseURL, frontendOrigin: frontendOrigin}
	if err := app.seedDemoData(ctx); err != nil {
		log.Fatalf("seed demo data: %v", err)
	}

	appMux := http.NewServeMux()
	appMux.HandleFunc("/healthz", app.healthzHandler)
	appMux.HandleFunc("/api/session", app.sessionHandler)
	appMux.HandleFunc("/api/sign-in", app.signInHandler)
	appMux.HandleFunc("/api/sign-up", app.signUpHandler)
	appMux.HandleFunc("/api/sign-out", app.signOutHandler)
	appMux.HandleFunc("/api/projects", app.projectsHandler)
	appMux.HandleFunc("/api/projects/", app.projectByIDHandler)
	appMux.HandleFunc("/api/uploads", app.uploadsHandler)

	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/", app.serveProjectHandler)

	appServer := &http.Server{
		Addr:              appListenAddr,
		Handler:           withCORS(frontendOrigin, appMux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	contentServer := &http.Server{
		Addr:              contentListenAddr,
		Handler:           contentSecurityHeaders(contentMux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("velori app api listening on http://localhost%s", appListenAddr)
		if err := appServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		log.Printf("velori content serving on http://localhost%s", contentListenAddr)
		if err := contentServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if err := <-errCh; err != nil {
		log.Fatal(err)
	}
}

func (app *application) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (app *application) sessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (app *application) signInHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload authRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth payload"})
		return
	}

	user, err := app.authenticateUser(r.Context(), payload.Email, payload.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
		return
	}

	if err := app.createSession(w, r.Context(), user.ID); err != nil {
		log.Printf("create session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (app *application) signUpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload authRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth payload"})
		return
	}

	user, err := app.registerUser(r.Context(), payload)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	if err := app.createSession(w, r.Context(), user.ID); err != nil {
		log.Printf("create session: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (app *application) signOutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(sessionCookie)
	if err == nil && strings.TrimSpace(cookie.Value) != "" {
		hashed := hashToken(cookie.Value)
		_, _ = app.db.Exec(r.Context(), `delete from sessions where token_hash = $1`, hashed)
	}

	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projects, err := app.listProjects(r.Context(), user.Email)
	if err != nil {
		log.Printf("list projects: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load projects"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (app *application) projectByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	projectID := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || strings.Contains(projectID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project id"})
		return
	}

	deleted, err := app.deleteProject(r.Context(), user.Email, projectID)
	if err != nil {
		log.Printf("delete project: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete project"})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (app *application) serveProjectHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.NotFound(w, r)
		return
	}

	username, slug, assetPath, ok := parseProjectPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	deployment, err := app.lookupLiveDeploy(r.Context(), username, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("lookup live deploy: %v", err)
		http.Error(w, "could not load project", http.StatusInternalServerError)
		return
	}

	resolvedPath, fallbackToIndex, err := resolveAssetPath(deployment.siteRoot, assetPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if fallbackToIndex {
		resolvedPath = filepath.Join(deployment.siteRoot, "index.html")
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

func (app *application) uploadsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	prepared, err := app.prepareUpload(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := validateUpload(prepared.request); err != nil {
		cleanupPreparedUpload(prepared)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		cleanupPreparedUpload(prepared)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	result, err := app.upsertProjectFromUpload(r.Context(), user.Email, prepared)
	if err != nil {
		cleanupPreparedUpload(prepared)
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "multiple existing projects") {
			status = http.StatusConflict
		}

		log.Printf("upsert upload: %v", err)
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "Upload staged and recorded.",
		"status":  "queued",
		"ingest":  prepared.normalizedTo,
		"project": result,
	})
}

func (app *application) prepareUpload(w http.ResponseWriter, r *http.Request) (preparedUpload, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return app.prepareMultipartUpload(w, r)
	}

	var payload uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return preparedUpload{}, fmt.Errorf("invalid upload payload")
	}

	return preparedUpload{request: payload}, nil
}

func (app *application) prepareMultipartUpload(w http.ResponseWriter, r *http.Request) (prepared preparedUpload, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, 110*1024*1024)
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		return preparedUpload{}, fmt.Errorf("invalid upload payload")
	}

	payload := uploadRequest{
		Name: strings.TrimSpace(r.FormValue("name")),
		Mode: strings.TrimSpace(r.FormValue("mode")),
	}

	paths := []string{}
	if rawPaths := strings.TrimSpace(r.FormValue("paths")); rawPaths != "" {
		if err := json.Unmarshal([]byte(rawPaths), &paths); err != nil {
			return preparedUpload{}, fmt.Errorf("invalid upload path manifest")
		}
	}

	headers := fileHeadersFromForm(r.MultipartForm)
	if len(paths) > 0 && len(paths) != len(headers) {
		return preparedUpload{}, fmt.Errorf("file path manifest does not match upload count")
	}

	ingestID := generateID("ingest")
	uploadRoot := filepath.Join(app.ingestRoot, ingestID)
	rawRoot := filepath.Join(uploadRoot, "raw")
	normalizedRoot := filepath.Join(uploadRoot, "normalized")
	defer func() {
		if err != nil && uploadRoot != "" {
			_ = os.RemoveAll(uploadRoot)
		}
	}()

	if err := os.MkdirAll(rawRoot, 0o755); err != nil {
		return preparedUpload{}, err
	}
	if err := os.MkdirAll(normalizedRoot, 0o755); err != nil {
		return preparedUpload{}, err
	}

	payload.Files = make([]fileMeta, 0, len(headers))
	for index, header := range headers {
		path := header.Filename
		if len(paths) > 0 {
			path = paths[index]
		}

		normalizedPath, err := normalizeUploadPath(path)
		if err != nil {
			return preparedUpload{}, err
		}

		rawFilePath := filepath.Join(rawRoot, fmt.Sprintf("%04d-%s", index, filepath.Base(normalizedPath)))
		if err := writeMultipartFile(header, rawFilePath); err != nil {
			return preparedUpload{}, err
		}

		if payload.Mode == "zip" {
			if err := extractZipToDir(rawFilePath, normalizedRoot); err != nil {
				return preparedUpload{}, err
			}
		} else {
			destinationPath := filepath.Join(normalizedRoot, filepath.FromSlash(normalizedPath))
			if err := copyFile(rawFilePath, destinationPath); err != nil {
				return preparedUpload{}, err
			}
		}

		payload.Files = append(payload.Files, fileMeta{
			Name: header.Filename,
			Size: header.Size,
			Path: normalizedPath,
		})
	}

	if payload.Mode == "zip" {
		if _, err := collectFiles(normalizedRoot); err != nil {
			return preparedUpload{}, err
		}
	}

	siteRoot, err := detectSiteRoot(normalizedRoot)
	if err != nil {
		return preparedUpload{}, err
	}

	normalizedFiles, err := collectFiles(siteRoot)
	if err != nil {
		return preparedUpload{}, err
	}
	payload.Files = normalizedFiles

	prepared = preparedUpload{request: payload, ingestRoot: uploadRoot, normalizedTo: siteRoot}
	return prepared, nil
}

func cleanupPreparedUpload(prepared preparedUpload) {
	if prepared.ingestRoot == "" {
		return
	}
	_ = os.RemoveAll(prepared.ingestRoot)
}

func writeMultipartFile(header *multipart.FileHeader, destination string) error {
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, file)
	return err
}

func copyFile(source string, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, input)
	return err
}

func normalizeUploadPath(value string) (string, error) {
	cleaned := filepath.ToSlash(strings.TrimSpace(value))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = path.Clean(cleaned)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("invalid file path")
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("unsafe file path: %s", value)
	}
	return cleaned, nil
}

func extractZipToDir(zipPath string, destinationRoot string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("invalid zip upload")
	}
	defer reader.Close()

	const maxArchiveFiles = 5000
	const maxArchiveBytes = int64(100 * 1024 * 1024)

	var totalBytes int64
	fileCount := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip uploads cannot contain symlinks")
		}

		normalizedPath, err := normalizeUploadPath(file.Name)
		if err != nil {
			return err
		}

		fileCount++
		if fileCount > maxArchiveFiles {
			return fmt.Errorf("zip upload contains too many files")
		}

		totalBytes += int64(file.UncompressedSize64)
		if totalBytes > maxArchiveBytes {
			return fmt.Errorf("zip upload exceeds the 100MB extracted size limit")
		}

		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(normalizedPath))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}

		input, err := file.Open()
		if err != nil {
			return err
		}

		output, err := os.Create(destinationPath)
		if err != nil {
			input.Close()
			return err
		}

		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inputErr != nil {
			return inputErr
		}
	}

	return nil
}

func collectFiles(root string) ([]fileMeta, error) {
	files := make([]fileMeta, 0)
	err := filepath.WalkDir(root, func(pathname string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, pathname)
		if err != nil {
			return err
		}

		normalizedPath, err := normalizeUploadPath(relativePath)
		if err != nil {
			return err
		}

		files = append(files, fileMeta{
			Name: filepath.Base(normalizedPath),
			Size: info.Size(),
			Path: normalizedPath,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i int, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func detectSiteRoot(root string) (string, error) {
	var candidates []string
	err := filepath.WalkDir(root, func(pathname string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(entry.Name(), "index.html") {
			return nil
		}
		candidates = append(candidates, filepath.Dir(pathname))
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no index.html found - Velori hosts built static sites only")
	}

	sort.Slice(candidates, func(i int, j int) bool {
		depthI := strings.Count(filepath.ToSlash(candidates[i]), "/")
		depthJ := strings.Count(filepath.ToSlash(candidates[j]), "/")
		if depthI == depthJ {
			return candidates[i] < candidates[j]
		}
		return depthI < depthJ
	})

	return candidates[0], nil
}

func fileHeadersFromForm(form *multipart.Form) []*multipart.FileHeader {
	if form == nil || len(form.File) == 0 {
		return nil
	}

	keys := make([]string, 0, len(form.File))
	for key := range form.File {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	headers := make([]*multipart.FileHeader, 0)
	for _, key := range keys {
		headers = append(headers, form.File[key]...)
	}

	return headers
}

func (app *application) seedDemoData(ctx context.Context) error {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	userID, username, err := ensureDemoUser(ctx, tx)
	if err != nil {
		return err
	}

	var projectCount int
	if err := tx.QueryRow(ctx, `select count(*) from projects where user_id = $1 and deleted_at is null`, userID).Scan(&projectCount); err != nil {
		return err
	}

	if projectCount == 0 {
		if err := insertSeedProject(ctx, tx, userID, username, app.contentBaseURL, "Product Teardown", "product-teardown", 4, app.ingestRoot); err != nil {
			return err
		}
		if err := insertSeedProject(ctx, tx, userID, username, app.contentBaseURL, "AI Signup Flow", "ai-signup-flow", 2, app.ingestRoot); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (app *application) listProjects(ctx context.Context, email string) ([]project, error) {
	rows, err := app.db.Query(ctx, `
		select
			p.id,
			p.name,
			p.slug,
			p.updated_at,
			coalesce(count(d.id), 0) as deploy_count,
			u.username
		from projects p
		join users u on u.id = p.user_id
		left join deploys d on d.project_id = p.id
		where u.email = $1 and p.deleted_at is null
		group by p.id, u.username
		order by p.updated_at desc
	`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []project
	for rows.Next() {
		var (
			entry       project
			updatedAt   time.Time
			username    string
			deployCount int64
		)

		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Slug, &updatedAt, &deployCount, &username); err != nil {
			return nil, err
		}

		entry.DeployCount = int(deployCount)
		entry.UpdatedAt = relativeTime(updatedAt)
		entry.LiveURL = fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, username, entry.Slug)
		projects = append(projects, entry)
	}

	return projects, rows.Err()
}

func (app *application) deleteProject(ctx context.Context, email string, projectID string) (bool, error) {
	commandTag, err := app.db.Exec(ctx, `
		update projects
		set deleted_at = now(), current_deploy_id = null, updated_at = now()
		where id = $1
		  and deleted_at is null
		  and user_id = (select id from users where email = $2)
	`, projectID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return false, err
	}

	return commandTag.RowsAffected() > 0, nil
}

func (app *application) upsertProjectFromUpload(ctx context.Context, email string, prepared preparedUpload) (project, error) {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return project{}, err
	}
	defer tx.Rollback(ctx)

	payload := prepared.request

	userID, username, err := lookupUserByEmail(ctx, tx, email)
	if err != nil {
		return project{}, err
	}

	matches, err := exactNameMatches(ctx, tx, userID, strings.TrimSpace(payload.Name))
	if err != nil {
		return project{}, err
	}

	if len(matches) > 1 {
		return project{}, fmt.Errorf("multiple existing projects match %q", payload.Name)
	}

	totalSize := int64(0)
	for _, file := range payload.Files {
		totalSize += file.Size
	}

	now := time.Now().UTC()
	var entry projectRecord
	if len(matches) == 1 {
		entry = matches[0]
		if _, err := tx.Exec(ctx, `update projects set updated_at = $2 where id = $1`, entry.ID, now); err != nil {
			return project{}, err
		}
	} else {
		slug, err := uniqueSlug(ctx, tx, userID, slugify(payload.Name))
		if err != nil {
			return project{}, err
		}

		entry = projectRecord{
			ID:       generateID("proj"),
			UserID:   userID,
			Name:     strings.TrimSpace(payload.Name),
			Slug:     slug,
			Username: username,
		}

		if _, err := tx.Exec(ctx, `
			insert into projects (id, user_id, slug, name, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $5)
		`, entry.ID, entry.UserID, entry.Slug, entry.Name, now); err != nil {
			return project{}, err
		}
	}

	deployID := generateID("dep")
	storagePrefix := fmt.Sprintf("dev/%s/%s", entry.ID, deployID)
	if prepared.normalizedTo != "" {
		storagePrefix = prepared.normalizedTo
	}
	if _, err := tx.Exec(ctx, `
		insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, deployID, entry.ID, "validated", totalSize, len(payload.Files), storagePrefix, now); err != nil {
		return project{}, err
	}

	if _, err := tx.Exec(ctx, `
		update projects
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, entry.ID, deployID, now); err != nil {
		return project{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return project{}, err
	}

	return project{
		ID:          entry.ID,
		Name:        entry.Name,
		Slug:        entry.Slug,
		UpdatedAt:   relativeTime(now),
		DeployCount: countForMatch(matches),
		LiveURL:     fmt.Sprintf("%s/~%s/%s", app.contentBaseURL, username, entry.Slug),
	}, nil
}

func validateUpload(payload uploadRequest) error {
	cleanName := strings.TrimSpace(payload.Name)
	if cleanName == "" {
		return fmt.Errorf("upload name is required")
	}

	if len(payload.Files) == 0 {
		return fmt.Errorf("select a folder or zip to deploy")
	}

	const maxDeploySize = 100 * 1024 * 1024
	const maxFileSize = 20 * 1024 * 1024

	totalSize := int64(0)
	hasIndexHTML := false
	hasZip := false

	for _, file := range payload.Files {
		totalSize += file.Size

		if file.Size > maxFileSize {
			return fmt.Errorf("%s exceeds the 20MB per-file limit", file.Name)
		}

		path := strings.ToLower(strings.TrimSpace(file.Path))
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if strings.HasSuffix(name, ".zip") {
			hasZip = true
		}

		if name == "index.html" || strings.HasSuffix(path, "/index.html") {
			hasIndexHTML = true
		}
	}

	if totalSize > maxDeploySize {
		return fmt.Errorf("deploy exceeds the 100MB upload limit")
	}

	if payload.Mode == "zip" {
		if hasZip {
			if len(payload.Files) != 1 {
				return fmt.Errorf("zip uploads must contain exactly one .zip file")
			}
			return nil
		}

		if !hasIndexHTML {
			return fmt.Errorf("no index.html found in extracted zip - Velori hosts built static sites only")
		}
		return nil
	}

	if !hasIndexHTML {
		return fmt.Errorf("no index.html found - Velori hosts built static sites only")
	}

	return nil
}

type projectRecord struct {
	ID       string
	UserID   string
	Name     string
	Slug     string
	Username string
	Deploys  int
}

type liveDeploy struct {
	siteRoot string
}

func parseProjectPath(rawPath string) (username string, slug string, assetPath string, ok bool) {
	trimmed := strings.Trim(rawPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "~") {
		return "", "", "", false
	}

	username = strings.TrimPrefix(parts[0], "~")
	slug = parts[1]
	if username == "" || slug == "" {
		return "", "", "", false
	}

	if len(parts) > 2 {
		assetPath = strings.Join(parts[2:], "/")
	}

	return username, slug, assetPath, true
}

func (app *application) lookupLiveDeploy(ctx context.Context, username string, slug string) (liveDeploy, error) {
	var siteRoot string
	err := app.db.QueryRow(ctx, `
		select d.storage_prefix
		from projects p
		join users u on u.id = p.user_id
		join deploys d on d.id = p.current_deploy_id
		where u.username = $1 and p.slug = $2 and p.deleted_at is null
	`, username, slug).Scan(&siteRoot)
	if err != nil {
		return liveDeploy{}, err
	}

	return liveDeploy{siteRoot: siteRoot}, nil
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
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}

	http.ServeContent(w, r, filepath.Base(filePath), info.ModTime(), file)
	return nil
}

func exactNameMatches(ctx context.Context, tx pgx.Tx, userID string, name string) ([]projectRecord, error) {
	rows, err := tx.Query(ctx, `
		select p.id, p.user_id, p.name, p.slug, u.username, coalesce(count(d.id), 0) as deploy_count
		from projects p
		join users u on u.id = p.user_id
		left join deploys d on d.project_id = p.id
		where p.user_id = $1 and p.name = $2 and p.deleted_at is null
		group by p.id, u.username
		order by p.updated_at desc
	`, userID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []projectRecord
	for rows.Next() {
		var entry projectRecord
		var deployCount int64
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Name, &entry.Slug, &entry.Username, &deployCount); err != nil {
			return nil, err
		}
		entry.Deploys = int(deployCount)
		matches = append(matches, entry)
	}

	return matches, rows.Err()
}

func (app *application) requireSessionUser(r *http.Request) (sessionUser, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return sessionUser{}, fmt.Errorf("missing session")
	}

	hashed := hashToken(cookie.Value)
	var user sessionUser
	err = app.db.QueryRow(r.Context(), `
		select u.id, u.email, u.name, u.username
		from sessions s
		join users u on u.id = s.user_id
		where s.token_hash = $1 and s.expires_at > now()
	`, hashed).Scan(&user.ID, &user.Email, &user.Name, &user.Username)
	if err != nil {
		return sessionUser{}, err
	}

	return user, nil
}

func (app *application) authenticateUser(ctx context.Context, email string, password string) (sessionUser, error) {
	var user sessionUser
	var passwordHash string
	err := app.db.QueryRow(ctx, `
		select id, email, name, username, password_hash
		from users
		where email = $1
	`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email, &user.Name, &user.Username, &passwordHash)
	if err != nil {
		return sessionUser{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return sessionUser{}, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

func (app *application) registerUser(ctx context.Context, payload authRequest) (sessionUser, error) {
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	password := strings.TrimSpace(payload.Password)
	name := strings.TrimSpace(payload.Name)
	if email == "" || password == "" || name == "" {
		return sessionUser{}, fmt.Errorf("name, email, and password are required")
	}
	if len(password) < 8 {
		return sessionUser{}, fmt.Errorf("password must be at least 8 characters")
	}

	username, err := uniqueUsername(ctx, app.db, email)
	if err != nil {
		return sessionUser{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return sessionUser{}, err
	}

	user := sessionUser{ID: generateID("usr"), Email: email, Name: name, Username: username}
	_, err = app.db.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, password_hash, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, user.ID, user.Email, "local-password", user.Username, user.Name, string(passwordHash), time.Now().UTC())
	if err != nil {
		if strings.Contains(err.Error(), "users_email_key") || strings.Contains(err.Error(), "duplicate") {
			return sessionUser{}, fmt.Errorf("an account with that email already exists")
		}
		return sessionUser{}, err
	}

	return user, nil
}

func (app *application) createSession(w http.ResponseWriter, ctx context.Context, userID string) error {
	token := generateToken(32)
	_, err := app.db.Exec(ctx, `
		insert into sessions (id, user_id, token_hash, created_at, expires_at)
		values ($1, $2, $3, $4, $5)
	`, generateID("ses"), userID, hashToken(token), time.Now().UTC(), time.Now().UTC().Add(30*24*time.Hour))
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 60 * 60,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func uniqueUsername(ctx context.Context, db *pgxpool.Pool, email string) (string, error) {
	base := slugify(strings.Split(email, "@")[0])
	if base == "" {
		base = "user"
	}
	username := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := db.QueryRow(ctx, `select exists(select 1 from users where username = $1)`, username).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return username, nil
		}
		username = base + strconv.Itoa(attempt)
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateToken(byteLength int) string {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func uniqueSlug(ctx context.Context, tx pgx.Tx, userID string, base string) (string, error) {
	if base == "" {
		base = "prototype"
	}

	slug := base
	for attempt := 2; ; attempt++ {
		var exists bool
		if err := tx.QueryRow(ctx, `
			select exists(
				select 1 from projects where user_id = $1 and slug = $2 and deleted_at is null
			)
		`, userID, slug).Scan(&exists); err != nil {
			return "", err
		}

		if !exists {
			return slug, nil
		}

		slug = base + "-" + strconv.Itoa(attempt)
	}
}

func ensureDemoUser(ctx context.Context, tx pgx.Tx) (string, string, error) {
	return ensureUser(ctx, tx, demoUserEmail, demoUserName, demoUsername, demoPassword)
}

func lookupUserByEmail(ctx context.Context, tx pgx.Tx, email string) (string, string, error) {
	var (
		id       string
		username string
	)
	err := tx.QueryRow(ctx, `select id, username from users where email = $1`, strings.ToLower(strings.TrimSpace(email))).Scan(&id, &username)
	if err != nil {
		return "", "", err
	}

	return id, username, nil
}

func ensureUser(ctx context.Context, tx pgx.Tx, email string, name string, username string, password string) (string, string, error) {
	var id string
	var passwordHash string
	err := tx.QueryRow(ctx, `select id, password_hash from users where email = $1`, email).Scan(&id, &passwordHash)
	if err == nil {
		if passwordHash == "" && password != "" {
			hashed, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				return "", "", hashErr
			}
			if _, execErr := tx.Exec(ctx, `update users set password_hash = $2 where id = $1`, id, string(hashed)); execErr != nil {
				return "", "", execErr
			}
		}
		return id, username, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}

	id = generateID("usr")
	hashedPassword := ""
	if password != "" {
		generated, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return "", "", hashErr
		}
		hashedPassword = string(generated)
	}
	if _, err := tx.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, password_hash, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, id, strings.ToLower(strings.TrimSpace(email)), "demo-auth", username, name, hashedPassword, time.Now().UTC()); err != nil {
		return "", "", err
	}

	return id, username, nil
}

func insertSeedProject(ctx context.Context, tx pgx.Tx, userID string, username string, contentBaseURL string, name string, slug string, deployCount int, ingestRoot string) error {
	projectID := generateID("proj")
	now := time.Now().UTC().Add(-time.Duration(deployCount) * time.Hour)
	if _, err := tx.Exec(ctx, `
		insert into projects (id, user_id, slug, name, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $5)
	`, projectID, userID, slug, name, now); err != nil {
		return err
	}

	var latestDeployID string
	for index := 0; index < deployCount; index++ {
		deployID := generateID("dep")
		deployTime := now.Add(time.Duration(index) * time.Hour)
		seedRoot, err := createSeedDeployFiles(ingestRoot, username, slug, index)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at)
			values ($1, $2, $3, $4, $5, $6, $7)
		`, deployID, projectID, "seeded", int64(250000+index*12000), 5+index, seedRoot, deployTime); err != nil {
			return err
		}
		latestDeployID = deployID
	}

	if _, err := tx.Exec(ctx, `
		update projects
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, projectID, latestDeployID, now.Add(time.Duration(deployCount-1)*time.Hour)); err != nil {
		return err
	}

	_ = contentBaseURL
	return nil
}

func createSeedDeployFiles(ingestRoot string, username string, slug string, version int) (string, error) {
	seedRoot := filepath.Join(ingestRoot, "seed", username, slug, strconv.Itoa(version), "site")
	if err := os.MkdirAll(filepath.Join(seedRoot, "docs"), 0o755); err != nil {
		return "", err
	}

	accent := []string{"#ff8f52", "#0fb381", "#5ba8ff", "#f4b942"}[version%4]
	title := nameFromSlug(slug)
	indexHTML := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>%s</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <main>
      <p class="eyebrow">Velori seed project</p>
      <h1>%s</h1>
      <p>Version %d of the locally seeded demo deploy.</p>
      <a href="docs">Open docs route</a>
    </main>
  </body>
</html>
`, title, title, version+1)

	stylesCSS := fmt.Sprintf(`:root { color-scheme: light; }
body { margin: 0; font-family: Inter, system-ui, sans-serif; background: linear-gradient(180deg, #f6f8fb, #eef2f7); color: #13202b; }
main { max-width: 720px; margin: 80px auto; padding: 32px; border-radius: 24px; background: white; box-shadow: 0 24px 70px rgba(18, 24, 40, 0.08); }
.eyebrow { color: %s; text-transform: uppercase; letter-spacing: 0.12em; font-size: 12px; font-weight: 700; }
h1 { margin: 8px 0 12px; font-size: 48px; }
a { color: %s; font-weight: 600; }
`, accent, accent)

	docsHTML := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head><meta charset="utf-8" /><meta name="viewport" content="width=device-width, initial-scale=1" /><title>%s Docs</title></head>
  <body><main><h1>%s docs route</h1><p>This page verifies directory index serving.</p></main></body>
</html>
`, slug, slug)

	if err := os.WriteFile(filepath.Join(seedRoot, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(seedRoot, "styles.css"), []byte(stylesCSS), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(seedRoot, "docs", "index.html"), []byte(docsHTML), 0o644); err != nil {
		return "", err
	}

	return seedRoot, nil
}

func nameFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func runMigrations(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `
		create table if not exists schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return err
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}

	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			versions = append(versions, entry.Name())
		}
	}
	sort.Strings(versions)

	for _, version := range versions {
		var alreadyApplied bool
		if err := db.QueryRow(ctx, `select exists(select 1 from schema_migrations where version = $1)`, version).Scan(&alreadyApplied); err != nil {
			return err
		}
		if alreadyApplied {
			continue
		}

		body, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return err
		}

		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}

		statements := strings.Split(string(body), ";")
		for _, statement := range statements {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("apply migration %s: %w", version, err)
			}
		}

		if _, err := tx.Exec(ctx, `insert into schema_migrations (version) values ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	return nil
}

func relativeTime(value time.Time) string {
	delta := time.Since(value)
	switch {
	case delta < time.Minute:
		return "Just now"
	case delta < time.Hour:
		minutes := int(delta.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case delta < 24*time.Hour:
		hours := int(delta.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case delta < 48*time.Hour:
		return "Yesterday"
	default:
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}
}

func countForMatch(matches []projectRecord) int {
	if len(matches) == 1 {
		return matches[0].Deploys + 1
	}
	return 1
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".zip", "")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")

	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		isLetter := char >= 'a' && char <= 'z'
		isNumber := char >= '0' && char <= '9'

		switch {
		case isLetter || isNumber:
			builder.WriteRune(char)
			lastDash = false
		case !lastDash:
			builder.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "prototype"
	}

	return slug
}

func generateID(prefix string) string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(buffer)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func withCORS(frontendOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == frontendOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func contentSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
