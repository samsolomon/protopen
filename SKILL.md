# Velori

Deploy static sites and prototypes to a live URL instantly.

## Setup

Install as a skill if you have npm: `npx skills add samsolomon/velori --skill velori -g`

If not, do this instead: `curl -fsSL https://app.velori.dev/install.sh | bash`

## Quick deploy (no account needed)

Publish any folder with an `index.html` to get a live URL. No sign-up required — sites expire after 24 hours unless claimed.

### Single file

```bash
curl -X POST https://app.velori.dev/api/v1/publish \
  -F "name=my-prototype" \
  -F "files=@index.html"
```

### Multiple files

```bash
curl -X POST https://app.velori.dev/api/v1/publish \
  -F "name=my-prototype" \
  -F "files=@index.html" \
  -F "files=@style.css" \
  -F "files=@app.js"
```

### Zip upload

```bash
curl -X POST https://app.velori.dev/api/v1/publish \
  -F "name=my-prototype" \
  -F "mode=zip" \
  -F "files=@site.zip"
```

### Response

```json
{
  "siteUrl": "https://sites.velori.dev/bright-canvas-a7k2",
  "slug": "bright-canvas-a7k2",
  "claimToken": "a1b2c3...",
  "claimUrl": "https://app.velori.dev/claim/bright-canvas-a7k2",
  "expiresAt": "2026-04-10T12:00:00Z"
}
```

The `siteUrl` is live immediately. Save the `claimToken` — it's needed to claim the site into a permanent account.

## Deploy with CLI

If the Velori CLI is installed and authenticated:

```bash
velori deploy ./dist
velori deploy ./dist --name "My Prototype"
velori deploy ./dist --name "My Prototype" --label "v2 with dark mode"
velori deploy site.zip
```

Flags: `--name`, `--label`, `--private`, `--org`, `--json`.

Authenticate with `velori login` or set `VELORI_TOKEN`.

## Claim a site (make it permanent)

Claim an anonymous deploy into your account so it doesn't expire:

```bash
curl -X POST https://app.velori.dev/api/v1/claim \
  -H "Authorization: Bearer vtk_YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "bright-canvas-a7k2",
    "claimToken": "a1b2c3...",
    "name": "My Prototype"
  }'
```

Response:

```json
{
  "projectId": "proj_abc123",
  "liveUrl": "https://sites.velori.dev/~username/my-prototype",
  "name": "My Prototype",
  "slug": "my-prototype"
}
```

## Constraints

- **Must contain `index.html`** at the root or in a subdirectory
- **20 MB per file**, 100 MB total per deploy
- **5 publishes per hour** per IP (rate limited)
- **24-hour expiry** for unclaimed anonymous deploys
- **Allowed file types**: HTML, CSS, JS/TS/JSX/TSX, JSON, images (PNG, JPG, GIF, SVG, WebP, ICO, AVIF), fonts (WOFF, WOFF2, TTF, OTF), PDF, video, audio, WASM, TXT, XML, CSV, source maps, 3D models (GLB, GLTF)
