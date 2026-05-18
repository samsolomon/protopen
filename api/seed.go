// SPDX-License-Identifier: AGPL-3.0-only

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
		{"jane@protopen.dev", "Jane Chen", "jane", roleAdmin},
		{"alex@protopen.dev", "Alex Rivera", "alex", roleMember},
		{"morgan@protopen.dev", "Morgan Lee", "morgan", roleMember},
	}
	teammateIDs := make(map[string]string, len(teammates))
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
		teammateIDs[t.username] = tmID
	}

	var siteCount int
	if err := tx.QueryRow(ctx, `select count(*) from sites where org_id = $1 and deleted_at is null`, orgID).Scan(&siteCount); err != nil {
		return err
	}

	// Distribute the seed sites across demo + teammates so the dashboard's
	// My/All scope tabs and the author chip both have content out of the box.
	seedSites := []struct {
		name, slug, createdBy string
		deploys               int
		commits               []seedCommit
	}{
		{"Product Teardown", "product-teardown", userID, 4, []seedCommit{
			{"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0", "main", "Initial prototype layout", "Jane Chen", false},
			{"b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1", "main", "Add feature cards and hero section", "Alex Rivera", false},
			{"c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2", "feature/docs", "Add docs page", "Jane Chen", false},
			{"d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3", "main", "Polish colors and typography", "Morgan Lee", true},
		}},
		{"AI Signup Flow", "ai-signup-flow", teammateIDs["alex"], 3, []seedCommit{
			{"e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4", "main", "Scaffold signup form", "Alex Rivera", false},
			{"f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5", "feature/validation", "Add email validation and error states", "Jane Chen", false},
			{"a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6", "main", "Merge validation, add success screen", "Alex Rivera", false},
		}},
		{"Pricing Page", "pricing-page", teammateIDs["jane"], 5, []seedCommit{
			{"1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b", "main", "Initial pricing grid layout", "Morgan Lee", false},
			{"2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c", "feature/toggle", "Add monthly/annual toggle", "Jane Chen", false},
			{"3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d", "feature/toggle", "Animate toggle transition", "Jane Chen", true},
			{"4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e", "main", "Add enterprise tier and CTA", "Alex Rivera", false},
			{"5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f", "main", "Final copy pass", "Morgan Lee", false},
		}},
		{"Mobile Nav", "mobile-nav", userID, 2, []seedCommit{
			{"6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a", "feature/hamburger", "Add hamburger menu prototype", "Alex Rivera", false},
			{"7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b", "feature/hamburger", "Slide-in animation and backdrop", "Alex Rivera", true},
		}},
		{"Dashboard Widgets", "dashboard-widgets", teammateIDs["morgan"], 3, []seedCommit{
			{"8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c", "main", "Chart widget with sample data", "Morgan Lee", false},
			{"9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d", "feature/stats", "Add stat cards and KPI row", "Jane Chen", false},
			{"0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e", "main", "Responsive grid and dark mode tokens", "Morgan Lee", false},
		}},
	}

	if siteCount == 0 {
		for _, sp := range seedSites {
			if err := insertSeedSite(ctx, tx, app.ingestRoot, orgID, username, sp.name, sp.slug, sp.createdBy, sp.deploys, sp.commits); err != nil {
				return err
			}
		}
	} else {
		// Existing dev DBs predate the ownership migration, so seed-known
		// sites can be sitting in the orphan state (created_by NULL). Heal
		// those rows in-place; never overwrite a row a real user authored.
		for _, sp := range seedSites {
			if _, err := tx.Exec(ctx, `
				update sites
				set created_by = $3
				where org_id = $1 and slug = $2 and created_by is null and deleted_at is null
			`, orgID, sp.slug, sp.createdBy); err != nil {
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
			hashed, hashErr := hashPassword(password)
			if hashErr != nil {
				return "", "", hashErr
			}
			if _, execErr := tx.Exec(ctx, `update users set password_hash = $2 where id = $1`, id, hashed); execErr != nil {
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
		generated, hashErr := hashPassword(password)
		if hashErr != nil {
			return "", "", hashErr
		}
		hashedPassword = generated
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
	var isPersonal bool
	err := tx.QueryRow(ctx, `
		select o.id, o.is_personal from organizations o
		join org_members m on m.org_id = o.id
		where m.user_id = $1 and o.slug = $2
	`, userID, username).Scan(&orgID, &isPersonal)
	if err == nil {
		if !isPersonal {
			// Heal seed-owned orgs whose is_personal was set false by older
			// seed code, so the API's personal-org lookup finds them again.
			// Scope is intentional: only orgs the seed itself touches.
			if _, err := tx.Exec(ctx, `update organizations set is_personal = true where id = $1`, orgID); err != nil {
				return "", err
			}
		}
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

// seedDeployAccents cycles through accent colors for the seed deploy HTML
// (createSeedDeployFiles) so each deploy gets a recognisably different look.
var seedDeployAccents = []string{"#ff8f52", "#0fb381", "#5ba8ff", "#f4b942"}

func insertSeedSite(ctx context.Context, tx pgx.Tx, ingestRoot string, orgID string, orgSlug string, name string, slug string, createdByUserID string, deployCount int, commits []seedCommit) error {
	siteID := generateID("site")
	now := time.Now().UTC().Add(-time.Duration(deployCount) * time.Hour)
	if _, err := tx.Exec(ctx, `
		insert into sites (id, org_id, slug, name, created_at, updated_at, created_by)
		values ($1, $2, $3, $4, $5, $5, $6)
	`, siteID, orgID, slug, name, now, createdByUserID); err != nil {
		return err
	}

	seedRemoteURL := "https://github.com/protopen-team/" + slug

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

	return nil
}

func createSeedDeployFiles(ingestRoot string, username string, slug string, version int) (string, error) {
	seedRoot := filepath.Join(ingestRoot, "seed", username, slug, strconv.Itoa(version), "site")
	if err := os.MkdirAll(filepath.Join(seedRoot, "docs"), 0o755); err != nil {
		return "", err
	}

	accent := seedDeployAccents[version%len(seedDeployAccents)]
	accentLight := []string{"#fff3ec", "#ecfdf5", "#eff6ff", "#fef9ee"}[version%4]
	title := nameFromSlug(slug)

	indexHTML := renderSeedIndex(slug, title, version, accent, accentLight)

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

func renderSeedIndex(slug, title string, version int, accent, accentLight string) string {
	baseCSS := commonSeedCSS(accent)
	body, extraCSS := renderSeedBody(slug, title, version, accent, accentLight)
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>%s</title>
    <style>%s
%s</style>
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
%s
    <footer>
      <p>%s &middot; Seed site v%d</p>
    </footer>
  </body>
</html>
`, title, baseCSS, extraCSS, title, body, title, version+1)
}

func commonSeedCSS(accent string) string {
	return fmt.Sprintf(`:root { color-scheme: light; }
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: Inter, system-ui, -apple-system, sans-serif; background: #fafafa; color: #18181b; -webkit-font-smoothing: antialiased; }
nav { border-bottom: 1px solid #e4e4e7; background: white; }
.nav-inner { max-width: 1120px; margin: 0 auto; padding: 0 24px; height: 56px; display: flex; align-items: center; gap: 24px; }
.nav-brand { font-weight: 700; font-size: 15px; letter-spacing: -0.01em; }
.nav-links { display: flex; gap: 20px; flex: 1; }
.nav-links a { color: #71717a; text-decoration: none; font-size: 14px; font-weight: 500; }
.btn { display: inline-flex; align-items: center; justify-content: center; border-radius: 8px; font-size: 14px; font-weight: 500; text-decoration: none; cursor: pointer; border: none; }
.btn-sm { padding: 6px 14px; background: #f4f4f5; color: #18181b; }
.btn-primary { padding: 12px 22px; background: %s; color: white; font-weight: 600; font-size: 15px; }
.btn-ghost { padding: 12px 22px; color: #71717a; }
.eyebrow { color: %s; text-transform: uppercase; letter-spacing: 0.12em; font-size: 12px; font-weight: 700; margin-bottom: 16px; }
footer { border-top: 1px solid #e4e4e7; padding: 24px; text-align: center; color: #a1a1aa; font-size: 13px; }
h1 { font-weight: 700; letter-spacing: -0.03em; line-height: 1.05; }
h3 { font-size: 16px; font-weight: 600; margin-bottom: 8px; }
`, accent, accent)
}

func renderSeedBody(slug, title string, version int, accent, accentLight string) (string, string) {
	switch slug {
	case "product-teardown":
		return seedBodyTeardown(version, accent, accentLight), seedCSSTeardown
	case "ai-signup-flow":
		return seedBodySignup(version, accent, accentLight), seedCSSSignup
	case "pricing-page":
		return seedBodyPricing(version, accent, accentLight), seedCSSPricing
	case "mobile-nav":
		return seedBodyMobileNav(version, accent, accentLight), seedCSSMobileNav
	case "dashboard-widgets":
		return seedBodyDashboard(version, accent, accentLight), seedCSSDashboard
	default:
		return seedBodyGeneric(title, version, accentLight), ""
	}
}

const seedCSSTeardown = `.teardown { max-width: 1120px; margin: 0 auto; padding: 56px 24px 80px; display: grid; grid-template-columns: 1.2fr 1fr; gap: 56px; align-items: center; }
.teardown h1 { font-size: 56px; margin-bottom: 20px; }
.teardown .lede { color: #52525b; font-size: 18px; line-height: 1.5; margin-bottom: 28px; max-width: 460px; }
.teardown ol { list-style: none; counter-reset: pin; display: grid; gap: 14px; }
.teardown ol li { counter-increment: pin; position: relative; padding-left: 48px; font-size: 15px; color: #3f3f46; line-height: 1.5; }
.teardown ol li::before { content: counter(pin); position: absolute; left: 0; top: -2px; width: 32px; height: 32px; border-radius: 50%; background: var(--accent); color: white; font-weight: 700; font-size: 14px; display: flex; align-items: center; justify-content: center; }
.teardown ol li b { color: #18181b; font-weight: 600; }
.mockup { position: relative; background: white; border-radius: 18px; box-shadow: 0 30px 60px -20px rgba(24,24,27,0.25), 0 0 0 1px #e4e4e7; padding: 18px; }
.mockup-bar { display: flex; gap: 6px; margin-bottom: 14px; }
.mockup-bar span { width: 11px; height: 11px; border-radius: 50%; background: #e4e4e7; }
.mockup-bar span:first-child { background: #ff6058; } .mockup-bar span:nth-child(2) { background: #ffbd2e; } .mockup-bar span:nth-child(3) { background: #28c941; }
.mockup-screen { background: #fafafa; border-radius: 10px; padding: 28px 24px; }
.mockup-screen .mh { font-size: 22px; font-weight: 700; margin-bottom: 8px; }
.mockup-screen .ms { color: #71717a; font-size: 13px; margin-bottom: 20px; }
.mockup-fields { display: grid; gap: 10px; margin-bottom: 18px; }
.mockup-fields div { height: 38px; background: white; border: 1px solid #e4e4e7; border-radius: 8px; }
.mockup-cta { height: 40px; background: var(--accent); border-radius: 8px; }
.pin { position: absolute; width: 26px; height: 26px; border-radius: 50%; background: var(--accent); color: white; font-weight: 700; font-size: 13px; display: flex; align-items: center; justify-content: center; box-shadow: 0 4px 10px rgba(0,0,0,0.18); }
@media (max-width: 720px) { .teardown { grid-template-columns: 1fr; } }
`

func seedBodyTeardown(version int, accent, accentLight string) string {
	return fmt.Sprintf(`    <section class="teardown" style="--accent:%s">
      <div>
        <p class="eyebrow">Teardown &middot; v%d</p>
        <h1>Anatomy of a great onboarding screen</h1>
        <p class="lede">Five details that turn a generic signup into a moment users remember. Here's what we found pulling apart the best ones.</p>
        <ol>
          <li><b>Single decisive headline.</b> One promise, eight words or fewer, never two.</li>
          <li><b>One input at a time.</b> Email first, defer everything else until after the magic happens.</li>
          <li><b>Inline reassurance.</b> A 12px line of trust copy beats a full-width disclaimer banner.</li>
          <li><b>Primary action carries the brand.</b> Bright, weighty, unmistakable from across the room.</li>
        </ol>
      </div>
      <div class="mockup">
        <div class="mockup-bar"><span></span><span></span><span></span></div>
        <div class="mockup-screen">
          <div class="mh">Welcome back</div>
          <div class="ms">Pick up where you left off in seconds.</div>
          <div class="mockup-fields"><div></div><div></div></div>
          <div class="mockup-cta"></div>
        </div>
        <span class="pin" style="top:62px;right:-14px">1</span>
        <span class="pin" style="top:160px;right:-14px">2</span>
        <span class="pin" style="top:248px;right:-14px">3</span>
        <span class="pin" style="bottom:32px;right:-14px;background:%s;color:#18181b">4</span>
      </div>
    </section>
`, accent, version+1, accentLight)
}

const seedCSSSignup = `.signup { max-width: 1120px; margin: 0 auto; padding: 56px 24px 80px; display: grid; grid-template-columns: 1fr 1fr; gap: 64px; align-items: center; }
.signup-copy h1 { font-size: 56px; margin-bottom: 20px; }
.signup-copy .lede { color: #52525b; font-size: 18px; line-height: 1.55; margin-bottom: 28px; max-width: 440px; }
.signup-bullets { display: grid; gap: 14px; }
.signup-bullets div { display: flex; gap: 12px; align-items: flex-start; font-size: 15px; color: #3f3f46; }
.signup-bullets span.dot { flex: 0 0 22px; height: 22px; border-radius: 50%; background: var(--accent-light); color: var(--accent); display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 13px; margin-top: 1px; }
.signup-card { background: white; border-radius: 18px; padding: 32px; box-shadow: 0 24px 50px -20px rgba(24,24,27,0.18), 0 0 0 1px #e4e4e7; }
.steps { display: flex; align-items: center; gap: 8px; margin-bottom: 26px; }
.step { flex: 1; height: 4px; border-radius: 2px; background: #e4e4e7; }
.step.active { background: var(--accent); }
.signup-card h2 { font-size: 24px; font-weight: 700; letter-spacing: -0.02em; margin-bottom: 6px; }
.signup-card .sub { color: #71717a; font-size: 14px; margin-bottom: 24px; }
.field { margin-bottom: 14px; }
.field label { display: block; font-size: 13px; font-weight: 600; color: #3f3f46; margin-bottom: 6px; }
.field input { display: block; width: 100%; height: 42px; padding: 0 14px; border: 1px solid #d4d4d8; border-radius: 8px; font: inherit; font-size: 14px; background: #fafafa; }
.field input.has-value { background: white; color: #18181b; }
.signup-cta { width: 100%; margin-top: 6px; }
.divider { display: flex; align-items: center; gap: 12px; color: #a1a1aa; font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; margin: 22px 0 16px; }
.divider::before, .divider::after { content: ""; flex: 1; height: 1px; background: #e4e4e7; }
.oauth { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.oauth button { height: 40px; border: 1px solid #d4d4d8; background: white; border-radius: 8px; font-weight: 600; font-size: 13px; color: #18181b; }
@media (max-width: 760px) { .signup { grid-template-columns: 1fr; } }
`

func seedBodySignup(version int, accent, accentLight string) string {
	steps := []string{"active", "active", ""}
	if version >= 1 {
		steps[2] = "active"
	}
	return fmt.Sprintf(`    <section class="signup" style="--accent:%s; --accent-light:%s">
      <div class="signup-copy">
        <p class="eyebrow">Get started &middot; v%d</p>
        <h1>Create your account in 30 seconds</h1>
        <p class="lede">Drop in your email, name your workspace, invite the team. We'll handle the rest, including the welcome confetti.</p>
        <div class="signup-bullets">
          <div><span class="dot">&check;</span>No credit card, no trial countdown, no friction.</div>
          <div><span class="dot">&check;</span>Magic-link sign-in works on every device you own.</div>
          <div><span class="dot">&check;</span>Cancel in two clicks, keep your data forever.</div>
        </div>
      </div>
      <div class="signup-card">
        <div class="steps"><div class="step %s"></div><div class="step %s"></div><div class="step %s"></div></div>
        <h2>Welcome to Protopen</h2>
        <p class="sub">Step 2 of 3 &middot; tell us where to send the invite.</p>
        <div class="field"><label>Work email</label><input class="has-value" value="jane@protopen.dev" readonly></div>
        <div class="field"><label>Workspace name</label><input class="has-value" value="Protopen Team" readonly></div>
        <a class="btn btn-primary signup-cta">Continue &rarr;</a>
        <div class="divider">or continue with</div>
        <div class="oauth"><button>&#xf09b; GitHub</button><button>&#xf1a0; Google</button></div>
      </div>
    </section>
`, accent, accentLight, version+1, steps[0], steps[1], steps[2])
}

const seedCSSPricing = `.pricing-hero { max-width: 760px; margin: 0 auto; padding: 64px 24px 32px; text-align: center; }
.pricing-hero h1 { font-size: 56px; margin-bottom: 16px; }
.pricing-hero p { color: #52525b; font-size: 18px; line-height: 1.55; max-width: 520px; margin: 0 auto 24px; }
.toggle { display: inline-flex; padding: 4px; background: #f4f4f5; border-radius: 999px; font-size: 13px; font-weight: 600; }
.toggle button { padding: 8px 18px; border: none; background: transparent; color: #71717a; border-radius: 999px; cursor: pointer; font-weight: 600; }
.toggle button.on { background: white; color: #18181b; box-shadow: 0 1px 2px rgba(0,0,0,0.08); }
.tiers { max-width: 1120px; margin: 0 auto; padding: 24px 24px 80px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; }
.tier { background: white; border-radius: 18px; padding: 32px 28px; border: 1px solid #e4e4e7; display: flex; flex-direction: column; }
.tier.featured { border: 2px solid var(--accent); transform: translateY(-8px); box-shadow: 0 24px 50px -20px rgba(24,24,27,0.18); position: relative; }
.tier .badge { position: absolute; top: -12px; left: 50%; transform: translateX(-50%); background: var(--accent); color: white; font-size: 11px; font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; padding: 5px 12px; border-radius: 999px; }
.tier h3 { font-size: 17px; font-weight: 600; margin-bottom: 8px; color: #18181b; }
.tier .price { display: flex; align-items: baseline; gap: 4px; margin: 10px 0 6px; }
.tier .price b { font-size: 44px; font-weight: 700; letter-spacing: -0.02em; }
.tier .price span { color: #71717a; font-size: 14px; }
.tier .blurb { color: #71717a; font-size: 14px; margin-bottom: 22px; line-height: 1.5; }
.tier ul { list-style: none; display: grid; gap: 10px; margin-bottom: 28px; flex: 1; }
.tier li { font-size: 14px; color: #3f3f46; padding-left: 24px; position: relative; line-height: 1.5; }
.tier li::before { content: "\2713"; position: absolute; left: 0; top: 0; color: var(--accent); font-weight: 700; }
.tier .cta { display: block; text-align: center; padding: 12px; border-radius: 8px; font-weight: 600; font-size: 14px; }
.tier .cta-primary { background: var(--accent); color: white; }
.tier .cta-ghost { background: #f4f4f5; color: #18181b; }
@media (max-width: 840px) { .tiers { grid-template-columns: 1fr; } .tier.featured { transform: none; } }
`

func seedBodyPricing(version int, accent, accentLight string) string {
	_ = accentLight
	annualOn, monthlyOn := "on", ""
	if version%2 == 1 {
		annualOn, monthlyOn = "", "on"
	}
	prices := [3]string{"$0", "$24", "$96"}
	if monthlyOn == "on" {
		prices = [3]string{"$0", "$29", "$119"}
	}
	return fmt.Sprintf(`    <section class="pricing-hero" style="--accent:%s">
      <p class="eyebrow">Pricing &middot; v%d</p>
      <h1>Simple, transparent pricing</h1>
      <p>Start free, upgrade only when your team starts shipping. Cancel anytime, your prototypes stay yours.</p>
      <div class="toggle"><button class="%s">Annual &mdash; save 20%%</button><button class="%s">Monthly</button></div>
    </section>
    <section class="tiers" style="--accent:%s">
      <div class="tier">
        <h3>Hobby</h3>
        <div class="price"><b>%s</b><span>/month</span></div>
        <p class="blurb">For tinkerers and weekend prototypes.</p>
        <ul><li>3 active sites</li><li>Community support</li><li>Custom subdomains</li></ul>
        <a class="cta cta-ghost">Get started</a>
      </div>
      <div class="tier featured">
        <span class="badge">Most popular</span>
        <h3>Pro</h3>
        <div class="price"><b>%s</b><span>/month</span></div>
        <p class="blurb">For designers shipping every week.</p>
        <ul><li>Unlimited sites</li><li>Custom domains &amp; SSL</li><li>Password-protected previews</li><li>Priority support</li></ul>
        <a class="cta cta-primary">Start free trial</a>
      </div>
      <div class="tier">
        <h3>Team</h3>
        <div class="price"><b>%s</b><span>/month</span></div>
        <p class="blurb">For teams collaborating in real time.</p>
        <ul><li>Everything in Pro</li><li>Shared workspaces</li><li>Role-based access</li><li>SAML SSO</li></ul>
        <a class="cta cta-ghost">Contact sales</a>
      </div>
    </section>
`, accent, version+1, annualOn, monthlyOn, accent, prices[0], prices[1], prices[2])
}

const seedCSSMobileNav = `.mobile { max-width: 1120px; margin: 0 auto; padding: 56px 24px 80px; display: grid; grid-template-columns: 1.1fr 1fr; gap: 56px; align-items: center; }
.mobile h1 { font-size: 54px; margin-bottom: 20px; }
.mobile .lede { color: #52525b; font-size: 18px; line-height: 1.55; margin-bottom: 28px; max-width: 460px; }
.mobile-pills { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 28px; }
.mobile-pills span { padding: 6px 14px; background: white; border: 1px solid #e4e4e7; border-radius: 999px; font-size: 13px; font-weight: 600; color: #3f3f46; }
.mobile-pills span.on { background: var(--accent); color: white; border-color: var(--accent); }
.phone-wrap { display: flex; justify-content: center; }
.phone { width: 320px; height: 620px; background: #18181b; border-radius: 44px; padding: 14px; box-shadow: 0 40px 60px -20px rgba(24,24,27,0.35); position: relative; }
.phone::before { content: ""; position: absolute; top: 14px; left: 50%; transform: translateX(-50%); width: 110px; height: 28px; background: #18181b; border-radius: 0 0 18px 18px; z-index: 2; }
.phone-screen { width: 100%; height: 100%; background: white; border-radius: 32px; overflow: hidden; position: relative; }
.status-bar { height: 36px; display: flex; align-items: center; justify-content: space-between; padding: 0 22px 0 28px; font-size: 13px; font-weight: 700; color: #18181b; }
.status-bar .right { display: flex; gap: 6px; align-items: center; }
.mobile-header { display: flex; align-items: center; justify-content: space-between; padding: 12px 20px; border-bottom: 1px solid #f4f4f5; }
.mobile-header b { font-size: 17px; font-weight: 700; }
.hamburger { display: grid; gap: 4px; }
.hamburger span { width: 22px; height: 2px; background: var(--accent); border-radius: 1px; }
.drawer { position: absolute; inset: 76px 0 0 0; background: rgba(255,255,255,0.98); padding: 18px 0; }
.drawer-item { display: flex; align-items: center; gap: 14px; padding: 14px 24px; font-size: 16px; font-weight: 500; color: #18181b; border-bottom: 1px solid #f4f4f5; }
.drawer-item.active { background: var(--accent-light); color: var(--accent); font-weight: 700; }
.drawer-icon { width: 28px; height: 28px; border-radius: 8px; background: #f4f4f5; }
.drawer-item.active .drawer-icon { background: var(--accent); }
.drawer-footer { position: absolute; left: 0; right: 0; bottom: 0; padding: 18px 24px; border-top: 1px solid #f4f4f5; background: white; }
.drawer-footer .row { display: flex; align-items: center; gap: 12px; }
.avatar { width: 36px; height: 36px; border-radius: 50%; background: var(--accent); color: white; font-weight: 700; font-size: 13px; display: flex; align-items: center; justify-content: center; }
.drawer-footer .who { font-size: 14px; font-weight: 600; }
.drawer-footer .who small { display: block; color: #71717a; font-weight: 500; }
@media (max-width: 760px) { .mobile { grid-template-columns: 1fr; } }
`

func seedBodyMobileNav(version int, accent, accentLight string) string {
	state := "Closed"
	hamburgerOn := ""
	if version >= 1 {
		state = "Open"
		hamburgerOn = "on"
	}
	return fmt.Sprintf(`    <section class="mobile" style="--accent:%s; --accent-light:%s">
      <div>
        <p class="eyebrow">Mobile nav &middot; v%d</p>
        <h1>Navigation that fits in one thumb.</h1>
        <p class="lede">A slide-in drawer that respects the bottom safe area, animates with a real spring, and surfaces the user's identity right where the thumb already lives.</p>
        <div class="mobile-pills"><span class="%s">Drawer %s</span><span>Spring 220/22</span><span>Safe-area aware</span><span>Reduced-motion ready</span></div>
        <a class="btn btn-primary">View the demo &rarr;</a>
      </div>
      <div class="phone-wrap">
        <div class="phone"><div class="phone-screen">
          <div class="status-bar"><span>9:41</span><span class="right">&#x25CF;&#x25CF;&#x25CF; &#x25A1;</span></div>
          <div class="mobile-header"><b>Protopen</b><div class="hamburger"><span></span><span></span><span></span></div></div>
          <div class="drawer">
            <div class="drawer-item active"><div class="drawer-icon"></div>Sites</div>
            <div class="drawer-item"><div class="drawer-icon"></div>Deploys</div>
            <div class="drawer-item"><div class="drawer-icon"></div>Team</div>
            <div class="drawer-item"><div class="drawer-icon"></div>Settings</div>
            <div class="drawer-item"><div class="drawer-icon"></div>Help &amp; docs</div>
          </div>
          <div class="drawer-footer"><div class="row"><div class="avatar">JC</div><div class="who">Jane Chen<small>jane@protopen.dev</small></div></div></div>
        </div></div>
      </div>
    </section>
`, accent, accentLight, version+1, hamburgerOn, state)
}

const seedCSSDashboard = `.dash { max-width: 1120px; margin: 0 auto; padding: 40px 24px 80px; }
.dash-head { display: flex; align-items: flex-end; justify-content: space-between; margin-bottom: 24px; gap: 24px; flex-wrap: wrap; }
.dash-head h1 { font-size: 38px; }
.dash-head p { color: #71717a; font-size: 14px; margin-top: 6px; }
.dash-head .range { display: inline-flex; padding: 4px; background: white; border: 1px solid #e4e4e7; border-radius: 10px; font-size: 13px; font-weight: 600; }
.dash-head .range button { padding: 6px 12px; border: none; background: transparent; color: #71717a; border-radius: 7px; cursor: pointer; font-weight: 600; }
.dash-head .range button.on { background: var(--accent); color: white; }
.kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 24px; }
.kpi { background: white; border-radius: 14px; padding: 20px; border: 1px solid #e4e4e7; }
.kpi .label { color: #71717a; font-size: 12px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; }
.kpi .value { font-size: 30px; font-weight: 700; letter-spacing: -0.02em; margin-top: 6px; }
.kpi .delta { display: inline-flex; align-items: center; gap: 4px; margin-top: 6px; font-size: 12px; font-weight: 700; padding: 2px 8px; border-radius: 999px; background: var(--accent-light); color: var(--accent); }
.kpi .delta.neg { background: #fef2f2; color: #b91c1c; }
.dash-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 16px; }
.panel { background: white; border-radius: 14px; padding: 20px; border: 1px solid #e4e4e7; }
.panel h3 { display: flex; justify-content: space-between; align-items: center; }
.panel h3 span { color: #71717a; font-size: 12px; font-weight: 500; }
.chart { height: 200px; margin-top: 16px; display: flex; align-items: flex-end; gap: 8px; }
.bar { flex: 1; background: var(--accent-light); border-radius: 6px 6px 0 0; position: relative; }
.bar::after { content: ""; position: absolute; left: 0; right: 0; bottom: 0; background: var(--accent); border-radius: 6px 6px 0 0; }
.list { margin-top: 14px; display: grid; gap: 12px; }
.list .row { display: flex; align-items: center; justify-content: space-between; font-size: 14px; }
.list .row .left { display: flex; align-items: center; gap: 10px; }
.list .row .swatch { width: 10px; height: 10px; border-radius: 3px; background: var(--accent); }
.list .row b { font-weight: 600; }
.list .row span { color: #71717a; font-size: 13px; }
@media (max-width: 880px) { .kpis { grid-template-columns: repeat(2, 1fr); } .dash-grid { grid-template-columns: 1fr; } }
`

func seedBodyDashboard(version int, accent, accentLight string) string {
	heights := []int{38, 54, 46, 72, 60, 84, 68, 92, 78, 96, 82, 100}
	if version%3 == 1 {
		heights = []int{42, 48, 64, 58, 76, 70, 88, 82, 94, 86, 100, 92}
	} else if version%3 == 2 {
		heights = []int{60, 72, 56, 88, 74, 96, 80, 100, 86, 92, 78, 84}
	}
	var bars strings.Builder
	for _, h := range heights {
		fmt.Fprintf(&bars, `<div class="bar" style="height:%d%%"><div style="height:%d%%"></div></div>`, h, h-12)
	}
	return fmt.Sprintf(`    <section class="dash" style="--accent:%s; --accent-light:%s">
      <div class="dash-head">
        <div>
          <p class="eyebrow">Dashboard &middot; v%d</p>
          <h1>Welcome back, Jane</h1>
          <p>Your team shipped 12 prototypes this week. Here's how they're performing.</p>
        </div>
        <div class="range"><button>7d</button><button class="on">30d</button><button>90d</button></div>
      </div>
      <div class="kpis">
        <div class="kpi"><div class="label">Active sites</div><div class="value">128</div><span class="delta">&uarr; 12%%</span></div>
        <div class="kpi"><div class="label">Deploys this week</div><div class="value">47</div><span class="delta">&uarr; 8%%</span></div>
        <div class="kpi"><div class="label">Avg. preview time</div><div class="value">2.3s</div><span class="delta neg">&darr; 0.4s</span></div>
        <div class="kpi"><div class="label">Team members</div><div class="value">14</div><span class="delta">&uarr; 2</span></div>
      </div>
      <div class="dash-grid">
        <div class="panel">
          <h3>Deploys over time<span>Last 30 days</span></h3>
          <div class="chart">%s</div>
        </div>
        <div class="panel">
          <h3>Most active sites<span>This week</span></h3>
          <div class="list">
            <div class="row"><div class="left"><span class="swatch"></span><b>Pricing page</b></div><span>18 deploys</span></div>
            <div class="row"><div class="left"><span class="swatch" style="background:#0fb381"></span><b>Signup flow</b></div><span>11 deploys</span></div>
            <div class="row"><div class="left"><span class="swatch" style="background:#f4b942"></span><b>Mobile nav</b></div><span>8 deploys</span></div>
            <div class="row"><div class="left"><span class="swatch" style="background:#ff8f52"></span><b>Teardown v3</b></div><span>5 deploys</span></div>
          </div>
        </div>
      </div>
    </section>
`, accent, accentLight, version+1, bars.String())
}

func seedBodyGeneric(title string, version int, accentLight string) string {
	return fmt.Sprintf(`    <section class="hero" style="max-width:960px;margin:0 auto;padding:80px 24px 64px;text-align:center;">
      <p class="eyebrow">Version %d</p>
      <h1 style="font-size:52px;margin-bottom:20px;">Ship prototypes<br>your team will love</h1>
      <p style="color:#71717a;font-size:18px;line-height:1.6;max-width:480px;margin:0 auto 32px;">Share interactive prototypes with your team. Get feedback, iterate, and ship faster.</p>
      <div style="display:flex;gap:12px;justify-content:center;">
        <a href="#" class="btn btn-primary">Get started</a>
        <a href="docs" class="btn btn-ghost">Read the docs</a>
      </div>
      <p style="margin-top:48px;color:#a1a1aa;font-size:13px;">%s &middot; %s</p>
    </section>
`, version+1, title, accentLight)
}

