package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samsolomon/protopen/api/migrate"
)

const (
	demoUserEmail = "sam@protopen.dev"
	demoUserName  = "Sam Solomon"
	demoUsername   = "sam"
	demoPassword  = "protopen-demo"
	sessionCookie = "protopen_session"
)

type application struct {
	db             *pgxpool.Pool
	store          *objectStore
	ingestRoot     string
	contentBaseURL string
	frontendOrigin string
	appOrigin      string
	contentOrigin  string
	cookieDomain   string

	mailer *emailClient

	authLimiter *rateLimiter
	adminEmails []string
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
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	UpdatedAt     string  `json:"updatedAt"`
	DeployCount   int     `json:"deployCount"`
	LiveURL       string  `json:"liveUrl"`
	IsPublic      bool    `json:"isPublic"`
	GitBranch     *string `json:"gitBranch,omitempty"`
	GitCommitHash *string `json:"gitCommitHash,omitempty"`
	GitRemoteURL  *string `json:"gitRemoteURL,omitempty"`
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
	ID      string
	OrgID   string
	Name    string
	Slug    string
	OrgSlug string
	Deploys int
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
	databaseURL := getenv("DATABASE_URL", "postgres://protopen:protopen@localhost:5432/protopen?sslmode=disable")
	ingestRoot := getenv("INGEST_ROOT", filepath.Join(".data", "ingest"))
	listenAddr := getenv("PORT", "")
	appListenAddr := getenv("APP_LISTEN_ADDR", ":8080")
	contentListenAddr := getenv("CONTENT_LISTEN_ADDR", ":8081")
	contentBaseURL := strings.TrimRight(getenv("PUBLIC_CONTENT_URL", "http://127.0.0.1:8081"), "/")
	frontendOrigin := strings.TrimRight(getenv("FRONTEND_ORIGIN", "http://localhost:5173"), "/")
	appOrigin := getenv("APP_ORIGIN", "")
	contentOrigin := getenv("CONTENT_ORIGIN", "")

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

	var mailer *emailClient
	if apiKey := getenv("RESEND_API_KEY", ""); apiKey != "" {
		mailer = newEmailClient(apiKey, getenv("RESEND_FROM_ADDRESS", ""))
		log.Printf("email sending enabled via Resend")
	} else {
		log.Printf("email sending disabled (no RESEND_API_KEY)")
	}

	var adminEmails []string
	if raw := getenv("ADMIN_EMAILS", ""); raw != "" {
		for _, e := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				adminEmails = append(adminEmails, trimmed)
			}
		}
	}

	app := &application{
		db:             db,
		store:          store,
		ingestRoot:     ingestRoot,
		contentBaseURL: contentBaseURL,
		frontendOrigin: frontendOrigin,
		appOrigin:      appOrigin,
		contentOrigin:  contentOrigin,
		cookieDomain:   cookieDomain,
		mailer:         mailer,
		adminEmails:    adminEmails,
	}
	app.authLimiter = newRateLimiter(10, 15*time.Minute)

	if getenv("SEED_DEMO", "") != "" {
		if err := app.seedDemoData(ctx); err != nil {
			log.Fatalf("seed demo data: %v", err)
		}
	}

	app.authLimiter.startCleanup(ctx)
	app.startCleanupLoop(ctx)

	appMux := http.NewServeMux()
	appMux.HandleFunc("/healthz", app.healthzHandler)
	appMux.HandleFunc("/api/session", app.sessionHandler)
	appMux.HandleFunc("/api/sign-in", app.rateLimit(app.authLimiter, app.signInHandler))
	appMux.HandleFunc("/api/sign-up", app.rateLimit(app.authLimiter, app.signUpHandler))
	appMux.HandleFunc("/api/sign-out", app.signOutHandler)
	appMux.HandleFunc("/api/sites", app.sitesHandler)
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
	appMux.HandleFunc("/api/admin/sites", app.adminSitesHandler)
	appMux.HandleFunc("/api/admin/sites/", app.adminSiteByIDHandler)
	appMux.HandleFunc("/api/auth/device", app.rateLimit(app.authLimiter, app.deviceCodeHandler))
	appMux.HandleFunc("/api/auth/device/", app.deviceCodePollHandler)
	serveFrontend(appMux, frontendOrigin, contentSecurityHeaders(http.HandlerFunc(app.serveSiteHandler)))

	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/", app.serveSiteHandler)

	if listenAddr != "" {
		addr := ":" + listenAddr
		contentHost := strings.TrimPrefix(contentOrigin, "https://")
		contentHost = strings.TrimPrefix(contentHost, "http://")
		appHandler := appSecurityHeaders(withCORS(frontendOrigin, appMux))
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

	// Local dev mode: two separate servers
	appServer := &http.Server{
		Addr:              appListenAddr,
		Handler:           appSecurityHeaders(withCORS(frontendOrigin, appMux)),
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
