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

	"github.com/samsolomon/velori/api/migrate"
)

const (
	demoUserEmail = "sam@velori.dev"
	demoUserName  = "Sam Solomon"
	demoUsername   = "sam"
	demoPassword  = "velori-demo"
	sessionCookie = "velori_session"
)

type application struct {
	db             *pgxpool.Pool
	store          *objectStore
	ingestRoot     string
	contentBaseURL string
	frontendOrigin string
	appOrigin      string
	contentOrigin  string
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

func main() {
	ctx := context.Background()
	databaseURL := getenv("DATABASE_URL", "postgres://velori:velori@localhost:5432/velori?sslmode=disable")
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

	app := &application{
		db:             db,
		store:          store,
		ingestRoot:     ingestRoot,
		contentBaseURL: contentBaseURL,
		frontendOrigin: frontendOrigin,
		appOrigin:      appOrigin,
		contentOrigin:  contentOrigin,
	}
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
	appMux.HandleFunc("/api/tokens", app.tokensHandler)
	appMux.HandleFunc("/api/tokens/", app.tokenByIDHandler)
	serveFrontend(appMux)

	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/", app.serveProjectHandler)

	if listenAddr != "" {
		addr := ":" + listenAddr
		contentHost := strings.TrimPrefix(contentOrigin, "https://")
		contentHost = strings.TrimPrefix(contentHost, "http://")
		appHandler := withCORS(frontendOrigin, appMux)
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

		log.Printf("velori listening on %s (app: %s, content: %s)", addr, appOrigin, contentOrigin)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		return
	}

	// Local dev mode: two separate servers
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
