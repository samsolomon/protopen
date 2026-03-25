package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

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

func ensureDemoUser(ctx context.Context, tx pgx.Tx) (string, string, error) {
	return ensureUser(ctx, tx, demoUserEmail, demoUserName, demoUsername, demoPassword)
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
