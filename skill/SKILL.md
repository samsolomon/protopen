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

## Constraints

- Must contain `index.html` at the root or in a subdirectory
- 20 MB per file, 100 MB total per deploy
- 5 anonymous publishes per hour per IP
- Anonymous sites expire in 24 hours unless claimed
- Allowed file types: HTML, CSS, JS/TS, JSON, images, fonts, PDF, video, audio, WASM, text, source maps, 3D models
