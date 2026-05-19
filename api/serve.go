// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed comment_runtime.js
var commentRuntimeJS string

// commentRuntimeScriptTag is injected before </body> on every deployed-site
// HTML response (by default). The runtime is served separately so it can be
// cached and inspected as a normal JS file in DevTools.
const commentRuntimeScriptTag = `<script src="/__protopen/comment-runtime.js" defer data-site-org="%s" data-site-slug="%s" data-api-base="%s"></script>`

const privateSiteHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Private Site</title></head>
<body style="font-family:system-ui;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0">
<p>This site is private. <a href="%s">Sign in</a> to view it.</p>
</body></html>`

func (app *application) serveSiteHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.NotFound(w, r)
		return
	}

	if r.URL.Path == "/__protopen/comment-runtime.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write([]byte(commentRuntimeJS))
		return
	}

	orgSlug, slug, deployID, assetPath, ok := parseSitePath(r.URL.Path)
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
		http.Error(w, "could not load site", http.StatusInternalServerError)
		return
	}

	// For private sites, deny access if not authenticated or not in org.
	// The thumbnail renderer bypasses this check via an internal token so
	// it can capture private prototypes from the content origin.
	if !deployment.isPublic && !app.checkThumbnailToken(r) {
		user, _ := app.requireSessionUser(r)
		if user.ID == "" || !userInOrg(user, orgSlug) {
			w.Header().Set("Cache-Control", "private, no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, privateSiteHTML, app.frontendOrigin)
			return
		}
	}

	// Inject the comment runtime into HTML by default. Skip injection for:
	//   - Thumbnail capture requests (carry the internal token).
	//   - Explicit opt-out via ?protopen-comments=0 (ad-hoc embeds, screenshots).
	shouldInject := !app.checkThumbnailToken(r) && r.URL.Query().Get("protopen-comments") != "0"
	target := w
	var injector *commentScriptWriter
	if shouldInject {
		apiBase := app.appBaseURL
		if apiBase == "" {
			apiBase = "http://" + r.Host
		}
		injector = &commentScriptWriter{
			ResponseWriter: w,
			scriptTag:      fmt.Sprintf(commentRuntimeScriptTag, html.EscapeString(orgSlug), html.EscapeString(slug), html.EscapeString(apiBase)),
		}
		target = injector
	}

	if app.store != nil {
		app.serveFromR2(target, r, deployment.siteRoot, assetPath)
	} else {
		app.serveFromFilesystem(target, r, deployment.siteRoot, assetPath)
	}

	if injector != nil {
		injector.finalize()
	}
}

// commentScriptWriter buffers an HTML response body so the comment-runtime
// <script> tag can be injected before </body>. Non-HTML responses pass through
// untouched. scriptTag is computed per-request with the org/site slugs.
type commentScriptWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	status      int
	isHTML      bool
	headerSent  bool
	contentType string
	scriptTag   string
}

func (w *commentScriptWriter) WriteHeader(status int) {
	if w.headerSent {
		return
	}
	w.status = status
	w.contentType = w.ResponseWriter.Header().Get("Content-Type")
	if strings.HasPrefix(w.contentType, "text/html") {
		w.isHTML = true
		return
	}
	w.ResponseWriter.WriteHeader(status)
	w.headerSent = true
}

func (w *commentScriptWriter) Write(p []byte) (int, error) {
	if w.status == 0 && w.contentType == "" {
		w.contentType = w.ResponseWriter.Header().Get("Content-Type")
		if strings.HasPrefix(w.contentType, "text/html") {
			w.isHTML = true
			w.status = http.StatusOK
		}
	}
	if w.isHTML {
		return w.buf.Write(p)
	}
	if !w.headerSent {
		w.ResponseWriter.WriteHeader(http.StatusOK)
		w.headerSent = true
	}
	return w.ResponseWriter.Write(p)
}

func (w *commentScriptWriter) finalize() {
	if !w.isHTML {
		return
	}
	body := injectCommentScript(w.buf.Bytes(), w.scriptTag)
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(len(body)))
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(status)
	w.ResponseWriter.Write(body)
}

func injectCommentScript(body []byte, scriptTag string) []byte {
	script := []byte(scriptTag)
	lower := bytes.ToLower(body)
	if i := bytes.LastIndex(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:i]...)
		out = append(out, script...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, script...)
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
		log.Printf("serve site file: %v", err)
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

func parseSitePath(rawPath string) (orgSlug string, slug string, deployID string, assetPath string, ok bool) {
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
		from sites p
		join organizations o on o.id = p.org_id
		join deploys d on d.id = $3 and d.site_id = p.id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
	`, orgSlug, slug, deployID).Scan(&d.siteID, &d.deployID, &d.siteRoot, &d.isPublic, &d.siteName)
	if err != nil {
		return liveDeploy{}, err
	}
	return d, nil
}

func (app *application) lookupLiveDeploy(ctx context.Context, orgSlug string, slug string) (liveDeploy, error) {
	var d liveDeploy
	err := app.db.QueryRow(ctx, `
		select p.id, d.id, d.storage_prefix, p.is_public, p.name
		from sites p
		join organizations o on o.id = p.org_id
		join deploys d on d.id = p.current_deploy_id
		where o.slug = $1 and p.slug = $2 and p.deleted_at is null
	`, orgSlug, slug).Scan(&d.siteID, &d.deployID, &d.siteRoot, &d.isPublic, &d.siteName)
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

