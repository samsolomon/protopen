package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

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
