// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chai2010/webp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/jackc/pgx/v5"
)

const (
	thumbnailHeaderName        = "X-Protopen-Thumbnail-Token"
	thumbnailFilename          = "thumbnail.webp"
	thumbnailViewportWidth     = 1280
	thumbnailViewportHeight    = 800
	thumbnailQuality           = 75
	thumbnailMaxConcurrency    = 2
	thumbnailFailureCap        = 3
	thumbnailCaptureTimeout    = 10 * time.Second
	thumbnailBackstopInterval  = 10 * time.Minute
	thumbnailBackstopBatchSize = 20
)

type thumbnailer struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	token       string
	sem         chan struct{}
	inflight    sync.Map // deployID -> struct{}
	failures    sync.Map // deployID -> int (process-lifetime retry count)
}

func newThumbnailer(parent context.Context, token, chromiumPath string) *thumbnailer {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.Flag("hide-scrollbars", "true"),
	)
	if chromiumPath != "" {
		opts = append(opts, chromedp.ExecPath(chromiumPath))
	}
	allocCtx, cancel := chromedp.NewExecAllocator(parent, opts...)
	return &thumbnailer{
		allocCtx:    allocCtx,
		allocCancel: cancel,
		token:       token,
		sem:         make(chan struct{}, thumbnailMaxConcurrency),
	}
}

func (t *thumbnailer) close() {
	if t.allocCancel != nil {
		t.allocCancel()
	}
}

func (t *thumbnailer) failureCount(deployID string) int {
	v, ok := t.failures.Load(deployID)
	if !ok {
		return 0
	}
	n, _ := v.(int)
	return n
}

func (t *thumbnailer) capture(ctx context.Context, deployURL string) ([]byte, error) {
	browserCtx, browserCancel := chromedp.NewContext(t.allocCtx)
	defer browserCancel()
	captureCtx, timeoutCancel := context.WithTimeout(browserCtx, thumbnailCaptureTimeout)
	defer timeoutCancel()

	// chromedp.ListenTarget invokes the callback from the browser's event
	// goroutine, so we use atomic to publish the last document status to
	// the goroutine that reads it after chromedp.Run returns.
	var mainStatus atomic.Int64
	mainStatus.Store(200)
	chromedp.ListenTarget(captureCtx, func(ev interface{}) {
		e, ok := ev.(*network.EventResponseReceived)
		if !ok || e.Response == nil {
			return
		}
		if e.Type == network.ResourceTypeDocument {
			mainStatus.Store(e.Response.Status)
		}
	})

	var pngBytes []byte
	err := chromedp.Run(captureCtx,
		network.Enable(),
		network.SetExtraHTTPHeaders(network.Headers{thumbnailHeaderName: t.token}),
		emulation.SetDeviceMetricsOverride(thumbnailViewportWidth, thumbnailViewportHeight, 1, false),
		chromedp.Navigate(deployURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.CaptureScreenshot(&pngBytes),
	)
	if err != nil {
		return nil, err
	}

	if status := mainStatus.Load(); status >= 400 {
		return nil, fmt.Errorf("deploy returned HTTP %d", status)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	var out bytes.Buffer
	if err := webp.Encode(&out, img, &webp.Options{Quality: thumbnailQuality}); err != nil {
		return nil, fmt.Errorf("encode webp: %w", err)
	}
	return out.Bytes(), nil
}

// writeDeployArtifact and openDeployArtifact dispatch between the R2 object
// store and the local filesystem. In filesystem mode storage_prefix is an
// absolute path on disk; in R2 mode it is an object-key prefix.
func (app *application) writeDeployArtifact(ctx context.Context, storagePrefix, filename, contentType string, body []byte) (string, error) {
	storedPath := storagePrefix + "/" + filename
	if app.store != nil {
		return storedPath, app.store.upload(ctx, storedPath, bytes.NewReader(body), contentType)
	}
	if err := os.MkdirAll(storagePrefix, 0o755); err != nil {
		return "", err
	}
	return storedPath, os.WriteFile(storedPath, body, 0o644)
}

func (app *application) openDeployArtifact(ctx context.Context, path string) (io.ReadCloser, error) {
	if app.store != nil {
		body, _, err := app.store.download(ctx, path)
		return body, err
	}
	return os.Open(path)
}

func (app *application) captureDeployThumbnail(ctx context.Context, deployID, deployURL, storagePrefix string) {
	if app.thumbnailer == nil || deployID == "" || deployURL == "" || storagePrefix == "" {
		return
	}
	t := app.thumbnailer

	if _, loaded := t.inflight.LoadOrStore(deployID, struct{}{}); loaded {
		return
	}
	defer t.inflight.Delete(deployID)

	if t.failureCount(deployID) >= thumbnailFailureCap {
		return
	}

	select {
	case t.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-t.sem }()

	body, err := t.capture(ctx, deployURL)
	if err != nil {
		log.Printf("thumbnail capture deploy=%s: %v", deployID, err)
		t.failures.Store(deployID, t.failureCount(deployID)+1)
		return
	}

	storedPath, err := app.writeDeployArtifact(ctx, storagePrefix, thumbnailFilename, "image/webp", body)
	if err != nil {
		log.Printf("thumbnail write deploy=%s: %v", deployID, err)
		return
	}

	if _, err := app.db.Exec(ctx, `update deploys set thumbnail_path = $1 where id = $2 and thumbnail_path is null`, storedPath, deployID); err != nil {
		log.Printf("thumbnail db update deploy=%s: %v", deployID, err)
		return
	}
	t.failures.Delete(deployID)
}

func (app *application) checkThumbnailToken(r *http.Request) bool {
	if app.thumbnailer == nil {
		return false
	}
	got := r.Header.Get(thumbnailHeaderName)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(app.thumbnailer.token)) == 1
}

func (app *application) startThumbnailBackstopLoop(ctx context.Context) {
	if app.thumbnailer == nil {
		return
	}
	ticker := time.NewTicker(thumbnailBackstopInterval)
	go func() {
		// Run once immediately so a server restart doesn't make the dashboard
		// wait a full tick for any thumbnails the previous process missed.
		app.runThumbnailBackstop(ctx)
		for {
			select {
			case <-ticker.C:
				app.runThumbnailBackstop(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

type pendingThumbnail struct {
	deployID, storagePrefix, orgSlug, siteSlug string
}

func (app *application) runThumbnailBackstop(ctx context.Context) {
	rows, err := app.db.Query(ctx, `
		select d.id, d.storage_prefix, o.slug, s.slug
		from deploys d
		join sites s on s.current_deploy_id = d.id
		join organizations o on o.id = s.org_id
		where d.thumbnail_path is null
		  and s.deleted_at is null
		  and d.created_at > now() - interval '7 days'
		order by d.created_at desc
		limit $1
	`, thumbnailBackstopBatchSize)
	if err != nil {
		log.Printf("thumbnail backstop query: %v", err)
		return
	}
	defer rows.Close()

	var batch []pendingThumbnail
	for rows.Next() {
		var p pendingThumbnail
		if err := rows.Scan(&p.deployID, &p.storagePrefix, &p.orgSlug, &p.siteSlug); err != nil {
			log.Printf("thumbnail backstop scan: %v", err)
			continue
		}
		batch = append(batch, p)
	}

	// Fan out so the global semaphore actually throttles to its capacity
	// rather than serializing through this loop.
	var wg sync.WaitGroup
	for _, p := range batch {
		wg.Add(1)
		go func(p pendingThumbnail) {
			defer wg.Done()
			app.captureDeployThumbnail(ctx, p.deployID, app.buildLiveURL(p.orgSlug, p.siteSlug), p.storagePrefix)
		}(p)
	}
	wg.Wait()
}

func (app *application) siteThumbnailHandler(w http.ResponseWriter, r *http.Request, siteID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var (
		isPublic      bool
		orgID         string
		thumbnailPath *string
		deployID      string
	)
	err := app.db.QueryRow(r.Context(), `
		select s.is_public, s.org_id, d.thumbnail_path, d.id
		from sites s
		join deploys d on d.id = s.current_deploy_id
		where s.id = $1 and s.deleted_at is null
	`, siteID).Scan(&isPublic, &orgID, &thumbnailPath, &deployID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		log.Printf("thumbnail lookup site=%s: %v", siteID, err)
		http.Error(w, "could not load thumbnail", http.StatusInternalServerError)
		return
	}

	if !isPublic {
		user, _ := app.requireSessionUser(r)
		if user.ID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if _, ok := orgRole(user, orgID); !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
			return
		}
	}

	if thumbnailPath == nil || *thumbnailPath == "" {
		http.NotFound(w, r)
		return
	}

	etag := fmt.Sprintf("\"%s\"", deployID)
	if ifNoneMatchHas(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "private, max-age=300, must-revalidate")
	w.Header().Set("ETag", etag)

	if r.Method == http.MethodHead {
		return
	}

	body, err := app.openDeployArtifact(r.Context(), *thumbnailPath)
	if err != nil {
		if isNotFound(err) {
			http.NotFound(w, r)
			return
		}
		log.Printf("thumbnail download site=%s: %v", siteID, err)
		http.Error(w, "could not load thumbnail", http.StatusInternalServerError)
		return
	}
	defer body.Close()
	io.Copy(w, body)
}

// ifNoneMatchHas parses a comma-separated If-None-Match list and reports
// whether any entry matches etag exactly. strings.Contains alone would
// falsely match etags that are substrings of others (a real risk with short
// hex IDs).
func ifNoneMatchHas(header, etag string) bool {
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

func resolveChromiumPath() string {
	if p := os.Getenv("CHROMIUM_PATH"); p != "" {
		return p
	}
	for _, candidate := range []string{"chromium-browser", "chromium", "google-chrome", "google-chrome-stable"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
