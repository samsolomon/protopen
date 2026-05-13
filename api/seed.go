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

	orgID, err := ensurePersonalOrg(ctx, tx, userID, username, demoUserName)
	if err != nil {
		return err
	}

	teammates := []struct {
		email, name, username, role string
	}{
		{"jane@velori.dev", "Jane Chen", "jane", roleAdmin},
		{"alex@velori.dev", "Alex Rivera", "alex", roleMember},
		{"morgan@velori.dev", "Morgan Lee", "morgan", roleMember},
	}
	for _, t := range teammates {
		tmID, _, tmErr := ensureUser(ctx, tx, t.email, t.name, t.username, demoPassword)
		if tmErr != nil {
			return tmErr
		}
		if _, tmErr = ensurePersonalOrg(ctx, tx, tmID, t.username, t.name); tmErr != nil {
			return tmErr
		}
		if _, tmErr = tx.Exec(ctx, `
			insert into org_members (id, org_id, user_id, role, created_at)
			values ($1, $2, $3, $4, now())
			on conflict (org_id, user_id) do nothing
		`, generateID("mem"), orgID, tmID, t.role); tmErr != nil {
			return tmErr
		}
	}

	var siteCount int
	if err := tx.QueryRow(ctx, `select count(*) from sites where org_id = $1 and deleted_at is null`, orgID).Scan(&siteCount); err != nil {
		return err
	}

	if siteCount == 0 {
		seedSites := []struct {
			name, slug string
			deploys    int
			commits    []seedCommit
		}{
			{"Product Teardown", "product-teardown", 4, []seedCommit{
				{"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0", "main", "Initial prototype layout", "Jane Chen", false},
				{"b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1", "main", "Add feature cards and hero section", "Alex Rivera", false},
				{"c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2", "feature/docs", "Add docs page", "Jane Chen", false},
				{"d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3", "main", "Polish colors and typography", "Morgan Lee", true},
			}},
			{"AI Signup Flow", "ai-signup-flow", 3, []seedCommit{
				{"e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4", "main", "Scaffold signup form", "Alex Rivera", false},
				{"f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5", "feature/validation", "Add email validation and error states", "Jane Chen", false},
				{"a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6", "main", "Merge validation, add success screen", "Alex Rivera", false},
			}},
			{"Pricing Page", "pricing-page", 5, []seedCommit{
				{"1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b", "main", "Initial pricing grid layout", "Morgan Lee", false},
				{"2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c", "feature/toggle", "Add monthly/annual toggle", "Jane Chen", false},
				{"3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d", "feature/toggle", "Animate toggle transition", "Jane Chen", true},
				{"4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e", "main", "Add enterprise tier and CTA", "Alex Rivera", false},
				{"5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f", "main", "Final copy pass", "Morgan Lee", false},
			}},
			{"Mobile Nav", "mobile-nav", 2, []seedCommit{
				{"6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a", "feature/hamburger", "Add hamburger menu prototype", "Alex Rivera", false},
				{"7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b", "feature/hamburger", "Slide-in animation and backdrop", "Alex Rivera", true},
			}},
			{"Dashboard Widgets", "dashboard-widgets", 3, []seedCommit{
				{"8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c", "main", "Chart widget with sample data", "Morgan Lee", false},
				{"9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d", "feature/stats", "Add stat cards and KPI row", "Jane Chen", false},
				{"0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e", "main", "Responsive grid and dark mode tokens", "Morgan Lee", false},
			}},
		}

		for _, sp := range seedSites {
			if err := insertSeedSite(ctx, tx, orgID, username, app.contentBaseURL, sp.name, sp.slug, sp.deploys, app.ingestRoot, sp.commits); err != nil {
				return err
			}
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
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into users (id, email, auth_ref, username, name, password_hash, created_at, email_verified_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, strings.ToLower(strings.TrimSpace(email)), "demo-auth", username, name, hashedPassword, now, now); err != nil {
		return "", "", err
	}

	return id, username, nil
}

func ensurePersonalOrg(ctx context.Context, tx pgx.Tx, userID string, username string, name string) (string, error) {
	var orgID string
	err := tx.QueryRow(ctx, `
		select o.id from organizations o
		join org_members m on m.org_id = o.id
		where m.user_id = $1 and o.is_personal = true
	`, userID).Scan(&orgID)
	if err == nil {
		return orgID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	orgID = generateID("org")
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into organizations (id, slug, name, is_personal, created_at, updated_at)
		values ($1, $2, $3, true, $4, $4)
	`, orgID, username, name, now); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		insert into org_members (id, org_id, user_id, role, created_at)
		values ($1, $2, $3, 'admin', $4)
	`, generateID("mem"), orgID, userID, now); err != nil {
		return "", err
	}
	return orgID, nil
}

type seedCommit struct {
	hash, branch, message, author string
	dirty                         bool
}

func insertSeedSite(ctx context.Context, tx pgx.Tx, orgID string, orgSlug string, contentBaseURL string, name string, slug string, deployCount int, ingestRoot string, commits []seedCommit) error {
	siteID := generateID("site")
	now := time.Now().UTC().Add(-time.Duration(deployCount) * time.Hour)
	if _, err := tx.Exec(ctx, `
		insert into sites (id, org_id, slug, name, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $5)
	`, siteID, orgID, slug, name, now); err != nil {
		return err
	}

	seedRemoteURL := "https://github.com/velori-team/" + slug

	var latestDeployID string
	for index := 0; index < deployCount; index++ {
		deployID := generateID("dep")
		deployTime := now.Add(time.Duration(index) * time.Hour)
		seedRoot, err := createSeedDeployFiles(ingestRoot, orgSlug, slug, index)
		if err != nil {
			return err
		}
		commit := commits[index%len(commits)]
		if _, err := tx.Exec(ctx, `
			insert into deploys (id, site_id, status, size_bytes, file_count, storage_prefix, created_at,
				git_commit_hash, git_branch, git_commit_message, git_dirty, git_author, git_remote_url)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`, deployID, siteID, "seeded", int64(250000+index*12000), 5+index, seedRoot, deployTime,
			commit.hash, commit.branch, commit.message, commit.dirty, commit.author, seedRemoteURL); err != nil {
			return err
		}
		latestDeployID = deployID
	}

	if _, err := tx.Exec(ctx, `
		update sites
		set current_deploy_id = $2, updated_at = $3
		where id = $1
	`, siteID, latestDeployID, now.Add(time.Duration(deployCount-1)*time.Hour)); err != nil {
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
	accentLight := []string{"#fff3ec", "#ecfdf5", "#eff6ff", "#fef9ee"}[version%4]
	title := nameFromSlug(slug)

	stylesCSS := fmt.Sprintf(`:root { color-scheme: light; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: Inter, system-ui, -apple-system, sans-serif; background: #fafafa; color: #18181b; -webkit-font-smoothing: antialiased; }
nav { border-bottom: 1px solid #e4e4e7; background: white; }
.nav-inner { max-width: 960px; margin: 0 auto; padding: 0 24px; height: 56px; display: flex; align-items: center; gap: 24px; }
.nav-brand { font-weight: 700; font-size: 15px; letter-spacing: -0.01em; }
.nav-links { display: flex; gap: 20px; flex: 1; }
.nav-links a { color: #71717a; text-decoration: none; font-size: 14px; font-weight: 500; }
.nav-links a:hover { color: #18181b; }
.btn { display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; font-size: 14px; font-weight: 500; text-decoration: none; transition: all 0.15s; cursor: pointer; border: none; }
.btn-sm { padding: 6px 14px; background: #f4f4f5; color: #18181b; }
.btn-sm:hover { background: #e4e4e7; }
.btn-primary { padding: 10px 20px; background: %s; color: white; font-weight: 600; }
.btn-primary:hover { opacity: 0.9; }
.btn-ghost { padding: 10px 20px; color: #71717a; }
.btn-ghost:hover { color: #18181b; }
.hero { max-width: 960px; margin: 0 auto; padding: 80px 24px 64px; text-align: center; }
.eyebrow { color: %s; text-transform: uppercase; letter-spacing: 0.12em; font-size: 12px; font-weight: 700; margin-bottom: 16px; }
h1 { font-size: 52px; font-weight: 700; letter-spacing: -0.03em; line-height: 1.1; margin-bottom: 20px; }
.subtitle { color: #71717a; font-size: 18px; line-height: 1.6; max-width: 480px; margin: 0 auto 32px; }
.hero-actions { display: flex; gap: 12px; justify-content: center; }
.features { max-width: 960px; margin: 0 auto; padding: 0 24px 80px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px; }
.feature { background: white; border: 1px solid #e4e4e7; border-radius: 16px; padding: 28px; }
.feature-icon { width: 40px; height: 40px; border-radius: 10px; display: flex; align-items: center; justify-content: center; font-size: 18px; color: %s; margin-bottom: 16px; }
h3 { font-size: 16px; font-weight: 600; margin-bottom: 8px; }
.feature p { color: #71717a; font-size: 14px; line-height: 1.6; }
footer { border-top: 1px solid #e4e4e7; padding: 24px; text-align: center; color: #a1a1aa; font-size: 13px; }
@media (max-width: 640px) { h1 { font-size: 32px; } .features { grid-template-columns: 1fr; } }
`, accent, accent, accent)

	indexHTML := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>%s</title>
    <style>%s</style>
  </head>
  <body>
    <nav>
      <div class="nav-inner">
        <span class="nav-brand">%s</span>
        <div class="nav-links">
          <a href="#">Features</a>
          <a href="#">Pricing</a>
          <a href="docs">Docs</a>
        </div>
        <a href="#" class="btn btn-sm">Sign in</a>
      </div>
    </nav>
    <section class="hero">
      <p class="eyebrow">Version %d</p>
      <h1>Ship prototypes<br>your team will love</h1>
      <p class="subtitle">Share interactive prototypes with your team. Get feedback, iterate, and ship faster.</p>
      <div class="hero-actions">
        <a href="#" class="btn btn-primary">Get started</a>
        <a href="docs" class="btn btn-ghost">Read the docs</a>
      </div>
    </section>
    <section class="features">
      <div class="feature">
        <div class="feature-icon" style="background:%s">&#9672;</div>
        <h3>Instant deploys</h3>
        <p>Push your prototype and get a live URL in seconds. No build step required.</p>
      </div>
      <div class="feature">
        <div class="feature-icon" style="background:%s">&#9830;</div>
        <h3>Team feedback</h3>
        <p>Invite your team to review prototypes and leave comments directly on the page.</p>
      </div>
      <div class="feature">
        <div class="feature-icon" style="background:%s">&#9733;</div>
        <h3>Version history</h3>
        <p>Every deploy is preserved. Compare versions side by side to track progress.</p>
      </div>
    </section>
    <footer>
      <p>%s &middot; Seed site v%d</p>
    </footer>
  </body>
</html>
`, title, stylesCSS, title, version+1, accentLight, accentLight, accentLight, title, version+1)

	docsHTML := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head><meta charset="utf-8" /><meta name="viewport" content="width=device-width, initial-scale=1" /><title>%s Docs</title>
  <style>* { margin: 0; padding: 0; box-sizing: border-box; } body { font-family: Inter, system-ui, sans-serif; background: #fafafa; color: #18181b; -webkit-font-smoothing: antialiased; } main { max-width: 640px; margin: 0 auto; padding: 64px 24px; } h1 { font-size: 32px; font-weight: 700; letter-spacing: -0.02em; margin-bottom: 12px; } p { color: #71717a; line-height: 1.7; } a { color: %s; font-weight: 500; }</style></head>
  <body><main><h1>%s</h1><p>This page verifies directory index serving. <a href="/">Back to home</a></p></main></body>
</html>
`, slug, accent, nameFromSlug(slug)+" Docs")

	if err := os.WriteFile(filepath.Join(seedRoot, "index.html"), []byte(indexHTML), 0o644); err != nil {
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
