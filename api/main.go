// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samsolomon/protopen/api/migrate"
)

const (
	demoUserEmail = "demo@protopen.dev"
	demoUserName  = "Demo User"
	demoUsername  = "demo"
	demoPassword  = "protopen-demo"
	sessionCookie = "protopen_session"
)

type application struct {
	db             *pgxpool.Pool
	store          *objectStore
	ingestRoot     string
	contentBaseURL string
	appBaseURL     string
	frontendOrigin string
	appOrigin      string
	contentOrigin  string
	cookieDomain   string

	mailer atomic.Pointer[emailClient]

	authLimiter        *rateLimiter
	commentLimiter     *rateLimiter
	deviceTokens       *deviceTokenStore
	adminEmails        []string
	trustedProxyHeader string

	thumbnailer  atomic.Pointer[thumbnailer]
	thumbnailMu  sync.Mutex
	thumbnailEnv thumbnailEnv
}

type sessionUser struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	Username        string     `json:"username"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt,omitempty"`
	Orgs            []orgInfo  `json:"orgs"`
	IsAdmin         bool       `json:"isAdmin,omitempty"`
	isBearerToken   bool
}

type orgInfo struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	IsPersonal bool   `json:"isPersonal"`
	Role       string `json:"role"`
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

type site struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Slug             string         `json:"slug"`
	OrgSlug          string         `json:"orgSlug"`
	UpdatedAt        string         `json:"updatedAt"`
	DeployCount      int            `json:"deployCount"`
	OpenCommentCount int            `json:"openCommentCount"`
	LiveURL          string         `json:"liveUrl"`
	IsPublic         bool           `json:"isPublic"`
	GitBranch        *string        `json:"gitBranch,omitempty"`
	GitCommitHash    *string        `json:"gitCommitHash,omitempty"`
	GitRemoteURL     *string        `json:"gitRemoteURL,omitempty"`
	CreatedBy        *authorSummary `json:"createdBy"`
}

type authorSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type uploadRequest struct {
	Name             string     `json:"name"`
	Mode             string     `json:"mode"`
	Label            string     `json:"label,omitempty"`
	Files            []fileMeta `json:"files"`
	GitCommitHash    string     `json:"gitCommitHash,omitempty"`
	GitBranch        string     `json:"gitBranch,omitempty"`
	GitCommitMessage string     `json:"gitCommitMessage,omitempty"`
	GitDirty         *bool      `json:"gitDirty,omitempty"`
	GitAuthor        string     `json:"gitAuthor,omitempty"`
	GitRemoteURL     string     `json:"gitRemoteURL,omitempty"`
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

type siteRecord struct {
	ID        string
	OrgID     string
	Name      string
	Slug      string
	OrgSlug   string
	Deploys   int
	CreatedBy *string
}

type liveDeploy struct {
	siteID   string
	deployID string
	siteRoot string
	isPublic bool
	siteName string
}

func main() {
	ctx := context.Background()

	// Load .env from the working directory (or nearest parent) before reading
	// any env. Values already set in the process environment win, so
	// shell-exported overrides still take precedence.
	if err := loadDotenv(); err != nil {
		log.Fatalf("load .env: %v", err)
	}

	databaseURL := getenv("DATABASE_URL", "postgres://protopen:protopen@localhost:5433/protopen?sslmode=disable")
	ingestRoot := getenv("INGEST_ROOT", filepath.Join(".data", "ingest"))
	listenAddr := getenv("PORT", "")
	appListenAddr := getenv("APP_LISTEN_ADDR", ":8080")
	contentListenAddr := getenv("CONTENT_LISTEN_ADDR", ":8081")
	contentBaseURL := strings.TrimRight(getenv("PUBLIC_CONTENT_URL", "http://127.0.0.1:8081"), "/")
	frontendOrigin := strings.TrimRight(getenv("FRONTEND_ORIGIN", "http://localhost:5173"), "/")
	appOrigin := strings.TrimRight(getenv("APP_ORIGIN", ""), "/")
	contentOrigin := strings.TrimRight(getenv("CONTENT_ORIGIN", ""), "/")

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("create database pool: %v", err)
	}
	defer db.Close()

	if err := migrate.Run(ctx, db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := os.MkdirAll(ingestRoot, 0o755); err != nil {
		log.Fatalf("create ingest root: %v", err)
	}

	var store *objectStore
	if bucket := getenv("R2_BUCKET_NAME", ""); bucket != "" {
		store = newObjectStore(
			getenv("R2_ACCOUNT_ID", ""),
			getenv("R2_ACCESS_KEY_ID", ""),
			getenv("R2_SECRET_ACCESS_KEY", ""),
			bucket,
		)
		log.Printf("using R2 object storage (bucket: %s)", bucket)
	} else {
		log.Printf("using local filesystem storage (%s)", ingestRoot)
	}

	cookieDomain := getenv("COOKIE_DOMAIN", "")

	var adminEmails []string
	if raw := getenv("ADMIN_EMAILS", ""); raw != "" {
		for _, e := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				adminEmails = append(adminEmails, trimmed)
			}
		}
	}

	trustedProxyHeader := http.CanonicalHeaderKey(getenv("TRUSTED_PROXY_HEADER", ""))

	app := &application{
		db:                 db,
		store:              store,
		ingestRoot:         ingestRoot,
		contentBaseURL:     contentBaseURL,
		frontendOrigin:     frontendOrigin,
		appOrigin:          appOrigin,
		contentOrigin:      contentOrigin,
		cookieDomain:       cookieDomain,
		adminEmails:        adminEmails,
		trustedProxyHeader: trustedProxyHeader,
	}
	app.authLimiter = newRateLimiter(10, 15*time.Minute)
	app.commentLimiter = newRateLimiter(20, time.Minute)
	app.deviceTokens = newDeviceTokenStore()

	if getenv("SEED_DEMO", "") != "" {
		// Refuse to seed in any non-local environment. The demo password is
		// public knowledge (see README), so seeding into a real deployment
		// would create a known-credential backdoor.
		if appOrigin != "" && !strings.HasPrefix(appOrigin, "http://localhost") && !strings.HasPrefix(appOrigin, "http://127.") {
			log.Fatalf("SEED_DEMO refused: APP_ORIGIN=%q is not localhost. The seeded credentials are publicly known.", appOrigin)
		}
		if err := app.seedDemoData(ctx); err != nil {
			log.Fatalf("seed demo data: %v", err)
		}
	}

	app.thumbnailEnv = resolveThumbnailEnv()
	if app.thumbnailEnv.available {
		log.Printf("thumbnail capture available (chromium: %s)", app.thumbnailEnv.chromiumPath)
	} else if app.thumbnailEnv.reason != "" {
		log.Printf("thumbnail capture unavailable: %s", app.thumbnailEnv.reason)
	}

	if err := app.initThumbnailState(ctx); err != nil {
		log.Fatalf("init thumbnail state: %v", err)
	}

	if err := app.initEmailState(ctx); err != nil {
		log.Fatalf("init email state: %v", err)
	}

	app.authLimiter.startCleanup(ctx)
	app.commentLimiter.startCleanup(ctx)
	app.startCleanupLoop(ctx)
	app.startThumbnailBackstopLoop(ctx)

	appMux := http.NewServeMux()
	appMux.HandleFunc("/healthz", app.healthzHandler)
	appMux.HandleFunc("/api/session", app.sessionHandler)
	appMux.HandleFunc("/api/sign-in", app.rateLimit(app.authLimiter, app.signInHandler))
	appMux.HandleFunc("/api/sign-up", app.rateLimit(app.authLimiter, app.signUpHandler))
	appMux.HandleFunc("/api/sign-out", app.signOutHandler)
	appMux.HandleFunc("/api/sites", app.sitesHandler)
	appMux.HandleFunc("/api/sites/by-slug/", app.commentContextHandler)
	appMux.HandleFunc("/api/sites/", app.siteByIDHandler)
	appMux.HandleFunc("/api/uploads", app.uploadsHandler)
	appMux.HandleFunc("/api/tokens", app.tokensHandler)
	appMux.HandleFunc("/api/tokens/", app.tokenByIDHandler)
	appMux.HandleFunc("/api/account/", app.accountHandler)
	appMux.HandleFunc("/api/orgs", app.orgsHandler)
	appMux.HandleFunc("/api/orgs/", app.orgByIDHandler)
	appMux.HandleFunc("/api/verify-email", app.verifyEmailHandler)
	appMux.HandleFunc("/api/resend-verification", app.rateLimit(app.authLimiter, app.resendVerificationHandler))
	appMux.HandleFunc("/api/forgot-password", app.rateLimit(app.authLimiter, app.forgotPasswordHandler))
	appMux.HandleFunc("/api/reset-password", app.resetPasswordHandler)
	appMux.HandleFunc("/api/admin/users", app.adminUsersHandler)
	appMux.HandleFunc("/api/admin/users/", app.adminUserByIDHandler)
	appMux.HandleFunc("/api/admin/settings", app.instanceSettingsHandler)
	appMux.HandleFunc("/api/admin/settings/email-test", app.emailTestHandler)
	appMux.HandleFunc("/api/comments/", app.commentByIDHandler)
	appMux.HandleFunc("/api/notifications", app.notificationsHandler)
	appMux.HandleFunc("/api/notifications/", app.notificationByIDHandler)
	appMux.HandleFunc("/api/auth/device", app.rateLimit(app.authLimiter, app.deviceCodeHandler))
	appMux.HandleFunc("/api/auth/device/", app.rateLimit(app.authLimiter, app.deviceCodePollHandler))
	serveFrontend(appMux, frontendOrigin, contentSecurityHeaders(http.HandlerFunc(app.serveSiteHandler)))

	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/", app.serveSiteHandler)

	if listenAddr != "" {
		addr := ":" + listenAddr
		contentHost := strings.TrimPrefix(contentOrigin, "https://")
		contentHost = strings.TrimPrefix(contentHost, "http://")
		if app.appBaseURL == "" {
			app.appBaseURL = appOrigin
		}
		allowedOrigins := []string{frontendOrigin, appOrigin, contentOrigin}
		appHandler := appSecurityHeaders(withCORS(allowedOrigins, verifyOrigin(allowedOrigins, limitJSONBody(appMux))))
		contentHandler := contentSecurityHeaders(contentMux)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.Split(r.Host, ":")[0]
			if contentHost != "" && host == contentHost {
				contentHandler.ServeHTTP(w, r)
			} else {
				appHandler.ServeHTTP(w, r)
			}
		})

		server := &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		}

		log.Printf("protopen listening on %s (app: %s, content: %s)", addr, appOrigin, contentOrigin)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}

	// Local dev mode: serve content on app port so session cookie works for private sites
	if os.Getenv("PUBLIC_CONTENT_URL") == "" {
		app.contentBaseURL = "http://localhost" + appListenAddr
	}
	// Compute app's base URL so the injected runtime knows where to call
	// the API from. In production this is appOrigin; in dev we point at the
	// app port directly.
	if appOrigin != "" {
		app.appBaseURL = appOrigin
	} else {
		app.appBaseURL = "http://localhost" + appListenAddr
	}

	// Local dev mode: two separate servers
	allowedOrigins := []string{frontendOrigin, appOrigin, contentOrigin, "http://localhost" + contentListenAddr, "http://127.0.0.1" + contentListenAddr}
	appServer := &http.Server{
		Addr:              appListenAddr,
		Handler:           appSecurityHeaders(withCORS(allowedOrigins, verifyOrigin(allowedOrigins, limitJSONBody(appMux)))),
		ReadHeaderTimeout: 5 * time.Second,
	}

	contentServer := &http.Server{
		Addr:              contentListenAddr,
		Handler:           contentSecurityHeaders(contentMux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("protopen app api listening on http://localhost%s", appListenAddr)
		if err := appServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		log.Printf("protopen content serving on http://localhost%s", contentListenAddr)
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
