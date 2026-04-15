---
name: velori
description: >
  Deploy static sites and prototypes to the web instantly. Use when asked to
  "deploy this", "publish this", "host this", "share this on the web",
  "make this live", "put this online", "deploy to velori", or "create a live URL".
  Outputs a live, shareable URL at sites.velori.dev.
---

# Velori

**Skill version: 1.0.0**

Deploy any folder with an `index.html` to a live URL instantly.

To install or update: `curl -fsSL https://app.velori.dev/install.sh | sh`

## Requirements

- The `velori` CLI (installed by the install script above)
- For anonymous deploys without the CLI: `curl` and `zip`

## Login

When the user asks to log in or authenticate, run this command:

```bash
velori login
```

This opens the browser automatically. The user approves access, and the token is saved. Do not ask the user to copy or paste a token — the command handles everything.

If the user already has a token: `velori login --token vtk_...`

Without authentication, deploys are anonymous and expire in 24 hours.

## Deploy a site

```bash
velori deploy {file-or-dir}
```

Requires authentication. Outputs the live URL.

### Deploy with a name

```bash
velori deploy {file-or-dir} --name "My Prototype"
```

### Additional flags

- `--label` — deploy label (e.g. "sprint 4 demo")
- `--private` — make the site private
- `--org` — deploy to an organization
- `--json` — machine-readable JSON output

### Anonymous deploy (no account)

Use the publish script for quick, anonymous sharing (expires in 24 hours):

```bash
./scripts/publish.sh {file-or-dir}
```

## Manage sites

```bash
velori list                                    # List all sites
velori deploys {name}                          # List deploy history
velori rollback {name} {deploy-id}             # Rollback to a previous deploy
velori visibility {name} public|private        # Set site visibility
```

## Token permissions

API tokens can deploy, delete sites, change visibility, rollback, and manage tokens. User management and account settings require signing in to the dashboard.

## Constraints

- Must contain `index.html` at the root or in a subdirectory
- 250 MB per file, 250 MB total per deploy
- 5 anonymous publishes per hour per IP
- Anonymous sites expire in 24 hours unless claimed
- Allowed file types: HTML, CSS, JS/TS, JSON, images, fonts, PDF, video, audio, WASM, text, source maps, 3D models

## Building for Velori

Velori runs everything in the browser — no server, no backend, no infrastructure.

- **Simple data**: hardcode it in JS arrays or objects. Most prototypes don't need a database.
- **Queryable data**: use [sql.js](https://github.com/sql-js/sql.js/) (SQLite compiled to WASM). Load from CDN and seed data in JS — do not bundle `.db` files.
- **SPA routing**: Velori falls back to `index.html` for unmatched paths, so client-side routers work.

### sql.js quick start

```html
<script src="https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.12.0/sql-wasm.js"></script>
<script>
initSqlJs({ locateFile: f => `https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.12.0/${f}` }).then(SQL => {
  const db = new SQL.Database();
  db.run("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, price REAL)");
  db.run("INSERT INTO items VALUES (1,'Widget',9.99), (2,'Gadget',24.99)");
  const results = db.exec("SELECT * FROM items");
  console.log(results);
});
</script>
```
