package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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
)

const (
	demoUserEmail = "sam@velori.dev"
	demoUserName  = "Sam Solomon"
	demoUsername  = "sam"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type application struct {
	db            *pgxpool.Pool
	ingestRoot    string
	publicBaseURL string
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
	publicBaseURL := strings.TrimRight(getenv("PUBLIC_APP_URL", "https://velori.dev"), "/")

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

	app := &application{db: db, ingestRoot: ingestRoot, publicBaseURL: publicBaseURL}
	if err := app.seedDemoData(ctx); err != nil {
		log.Fatalf("seed demo data: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.healthzHandler)
	mux.HandleFunc("/api/projects", app.projectsHandler)
	mux.HandleFunc("/api/uploads", app.uploadsHandler)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("velori api listening on http://localhost:8080")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (app *application) healthzHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (app *application) projectsHandler(w http.ResponseWriter, r *http.Request) {
	projects, err := app.listProjects(r.Context(), demoUserEmail)
	if err != nil {
		log.Printf("list projects: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load projects"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
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

	result, err := app.upsertProjectFromUpload(r.Context(), demoUserEmail, prepared)
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
		normalizedFiles, err := collectFiles(normalizedRoot)
		if err != nil {
			return preparedUpload{}, err
		}
		payload.Files = normalizedFiles
	}

	prepared = preparedUpload{request: payload, ingestRoot: uploadRoot, normalizedTo: normalizedRoot}
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
		if err := insertSeedProject(ctx, tx, userID, username, app.publicBaseURL, "Product Teardown", "product-teardown", 4); err != nil {
			return err
		}
		if err := insertSeedProject(ctx, tx, userID, username, app.publicBaseURL, "AI Signup Flow", "ai-signup-flow", 2); err != nil {
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
		entry.LiveURL = fmt.Sprintf("%s/~%s/%s", app.publicBaseURL, username, entry.Slug)
		projects = append(projects, entry)
	}

	return projects, rows.Err()
}

func (app *application) upsertProjectFromUpload(ctx context.Context, email string, prepared preparedUpload) (project, error) {
	tx, err := app.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return project{}, err
	}
	defer tx.Rollback(ctx)

	payload := prepared.request

	userID, username, err := ensureUserByEmail(ctx, tx, email)
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
		LiveURL:     fmt.Sprintf("%s/~%s/%s", app.publicBaseURL, username, entry.Slug),
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
	return ensureUser(ctx, tx, demoUserEmail, demoUserName, demoUsername)
}

func ensureUserByEmail(ctx context.Context, tx pgx.Tx, email string) (string, string, error) {
	if email == demoUserEmail {
		return ensureDemoUser(ctx, tx)
	}

	return "", "", fmt.Errorf("unknown user %q", email)
}

func ensureUser(ctx context.Context, tx pgx.Tx, email string, name string, username string) (string, string, error) {
	var id string
	err := tx.QueryRow(ctx, `select id from users where email = $1`, email).Scan(&id)
	if err == nil {
		return id, username, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}

	id = generateID("usr")
	if _, err := tx.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, created_at)
		values ($1, $2, $3, $4, $5, $6)
	`, id, email, "demo-auth", username, name, time.Now().UTC()); err != nil {
		return "", "", err
	}

	return id, username, nil
}

func insertSeedProject(ctx context.Context, tx pgx.Tx, userID string, username string, baseURL string, name string, slug string, deployCount int) error {
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
		if _, err := tx.Exec(ctx, `
			insert into deploys (id, project_id, status, size_bytes, file_count, storage_prefix, created_at)
			values ($1, $2, $3, $4, $5, $6, $7)
		`, deployID, projectID, "seeded", int64(250000+index*12000), 5+index, fmt.Sprintf("seed/%s/%s", projectID, deployID), deployTime); err != nil {
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

	_ = username
	_ = baseURL
	return nil
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

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

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
