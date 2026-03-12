# Velori

Velori is a hosted static prototype platform for non-engineers.

This repo starts with two apps:

- `web/` - React dashboard for uploads and project management
- `api/` - Go API for health checks, projects, and mock uploads

## Build plan

Implementation milestones and checklists live in `BUILD_PLAN.md`.

## MCP

This project includes a root `.mcp.json` with the `shadcn` MCP server configured.

After restarting your MCP client, you should be able to browse and install shadcn registry items.

Note: the current frontend is not fully set up for shadcn component installs yet because Tailwind and `components.json` are not configured.

## Quick start

### Database

```bash
docker compose up -d postgres
```

The API defaults to `postgres://velori:velori@localhost:5432/velori?sslmode=disable`.

Make sure Docker Desktop or another Docker daemon is running first.

### Web

```bash
cd web
npm install
npm run dev
```

### API

```bash
cd api
go run .
```

### Tests

```bash
cd api
go test ./...
```

The web app expects the API at `http://localhost:8080`.

Local hosted prototype content is served separately at `http://localhost:8081`.

The API runs migrations automatically on startup and seeds a demo user plus sample projects for local development.

Local auth now uses email/password plus an HTTP-only session cookie on the app origin. Use the seeded demo account:

- `sam@velori.dev`
- `velori-demo`

The current upload flow now sends real multipart file uploads to the API for both folder and zip selections. Folder uploads are normalized into a temporary ingest directory, and zip uploads are extracted server-side into that same staging area before validation.

Use `INGEST_ROOT` to override the local staging directory. By default it uses `api/.data/ingest` when the API is started from `api/`.

Successful staged deploys are now served from the content origin at URLs like `http://localhost:8081/~sam/product-teardown`.
