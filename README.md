# Protopen

Protopen is a static site and prototype hosting platform. Signed-in users can deploy from the dashboard or CLI.

This repo has three main apps:

- `web/` - React dashboard for auth, uploads, sites, and settings
- `api/` - Go API for auth, uploads, sites/deploys, and content serving
- `cli/` - Command-line tool for deploying from the terminal

## Docs

- Implementation milestones and checklists live in `BUILD_PLAN.md`.

## Current capabilities

- upload a folder or zip from the dashboard
- redeploy the same site name and keep a stable live URL
- deploy from the CLI with saved config or environment variables
- rollback to any previous deploy and toggle site visibility (public/private)

## MCP

This repo includes a root `.mcp.json` with the `shadcn` MCP server configured.

The web app is already configured for shadcn and Tailwind via `web/components.json` and `web/src/index.css`, so registry installs should work after restarting your MCP client.

## Quick start

### Database

```bash
docker compose up -d postgres
```

The default local database URL is `postgres://protopen:protopen@localhost:5432/protopen?sslmode=disable`.

### Web

Install dependencies from the repo root, then start the Vite app:

```bash
npm install
npm run dev:web
```

The dashboard runs at `http://localhost:5173` and expects the API at `http://localhost:8080`.

### API

```bash
cd api
go run .
```

Local defaults:

- app/API server: `http://localhost:8080`
- content server: `http://127.0.0.1:8081`
- storage: local filesystem under `.data/ingest`

The API runs migrations automatically on startup and seeds a demo user plus sample projects for local development.

Use the seeded demo account:

- `sam@protopen.dev`
- `protopen-demo`

### Environment

Overrides are documented in `.env.example`.

- Leave `PORT` empty for local split mode (`:8080` app/API and `:8081` content).
- Set `PORT` for single-server production mode.
- Leave `R2_*` empty to use local filesystem storage.
- Set `R2_*` and `R2_BUCKET_NAME` to use Cloudflare R2 object storage.

## CLI

Build the CLI:

```bash
cd cli
make build
```

Authenticate with an API token from the dashboard under Settings > API Tokens:

```bash
./protopen login
```

Deploy a folder or zip:

```bash
./protopen deploy ./my-site
./protopen deploy ./dist --name "My Prototype" --label "v2"
./protopen deploy ./site.zip
```

Config is stored at `~/.protopen/config.json`.

Resolution order is:

- flags
- environment variables: `PROTOPEN_TOKEN`, `PROTOPEN_URL`, `PROTOPEN_ORG`
- config file
- defaults

Useful commands:

- `./protopen list`
- `./protopen deploys <project-name>`
- `./protopen rollback <project-name> <deploy-id>`
- `./protopen visibility <project-name> <public|private>`
- `./protopen token`
- `./protopen logout`
- `./protopen config show`

Run `./protopen help` for the full command list.

## Tests

Run API tests:

```bash
cd api
go test ./...
```

## Smoke tests

With Postgres and the API running locally:

```bash
./scripts/local-smoke-test.sh
./scripts/zip-smoke-test.sh
```

These cover:

- signed-in upload and stable URL redeploy
- zip upload serving
