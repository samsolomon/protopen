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

To install or update: `npx skills add samsolomon/velori --skill velori -g`

## Requirements

- Required binaries: `curl`, `zip`
- Optional environment variable: `$VELORI_TOKEN`

## Deploy a site

```bash
./scripts/publish.sh {file-or-dir}
```

Outputs the live URL (e.g. `https://sites.velori.dev/bright-canvas-a7k2`).

Without an API token this creates an **anonymous site** that expires in 24 hours.
With a saved token (`$VELORI_TOKEN`), the site is permanent.

**File structure:** Place `index.html` at the root of the directory you publish. The directory's contents become the site root.

## Deploy with a name

```bash
./scripts/publish.sh {file-or-dir} --name "My Prototype"
```

## Authenticated deploy (permanent)

```bash
export VELORI_TOKEN=vtk_...
./scripts/publish.sh {file-or-dir} --name "My Project"
```

Or pass inline: `./scripts/publish.sh {dir} --api-key vtk_...`

## Token permissions

API tokens (`$VELORI_TOKEN`) can **publish and read only**. Deleting projects, changing settings, and managing teams require signing in to the dashboard.

## Constraints

- Must contain `index.html` at the root or in a subdirectory
- 20 MB per file, 100 MB total per deploy
- 5 anonymous publishes per hour per IP
- Anonymous sites expire in 24 hours unless claimed
- Allowed file types: HTML, CSS, JS/TS, JSON, images, fonts, PDF, video, audio, WASM, text, source maps, 3D models

## Building for Velori

Velori is **static-only** — no server-side code runs. Everything must work in the browser.

- **Simple data**: hardcode it in JS arrays or objects. Most prototypes don't need a database.
- **Queryable data**: use [sql.js](https://github.com/nicolewindows/sql.js/) (SQLite compiled to WASM). Load from CDN and seed data in JS — do not bundle `.db` files.
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
