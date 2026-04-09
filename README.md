# Velori

Velori is a static site and prototype hosting platform. Signed-in users can deploy from the dashboard or CLI, and anonymous publishes can be claimed into a permanent account later.

This repo has three main apps:

- `web/` - React dashboard for auth, uploads, projects, notifications, settings, and CLI docs
- `api/` - Go API for auth, uploads, anonymous publish/claim, comments, notifications, and content serving
- `cli/` - Command-line tool for deploying from the terminal

## Docs

- Implementation milestones and checklists live in `BUILD_PLAN.md`.
- Anonymous publish and claim examples live in `SKILL.md`.

## Current capabilities

- upload a folder or zip from the dashboard
- redeploy the same project name and keep a stable live URL
- deploy from the CLI with saved config or environment variables
- publish a temporary anonymous site, then claim it into an account
- add comments with `@mentions` and receive notifications in the dashboard

## MCP

This repo includes a root `.mcp.json` with the `shadcn` MCP server configured.

The web app is already configured for shadcn and Tailwind via `web/components.json` and `web/src/index.css`, so registry installs should work after restarting your MCP client.

## Quick start

### Database

```bash
docker compose up -d postgres
```

The default local database URL is `postgres://velori:velori@localhost:5432/velori?sslmode=disable`.

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

- `sam@velori.dev`
- `velori-demo`

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
./velori login
```

Deploy a folder or zip:

```bash
./velori deploy ./my-site
./velori deploy ./dist --name "My Prototype" --label "v2"
./velori deploy ./site.zip
```

Config is stored at `~/.velori/config.json`.

Resolution order is:

- flags
- environment variables: `VELORI_TOKEN`, `VELORI_URL`, `VELORI_ORG`
- config file
- defaults

Useful commands:

- `./velori list`
- `./velori deploys <project-name>`
- `./velori rollback <project-name> <deploy-id>`
- `./velori visibility <project-name> <public|private>`
- `./velori token`
- `./velori logout`
- `./velori config show`

Run `./velori help` for the full command list.

## Anonymous publish API

Publish a folder without creating an account first:

```bash
curl -X POST http://localhost:8080/api/v1/publish \
  -F "name=my-prototype" \
  -F "files=@index.html"
```

The response includes a live `siteUrl`, a `claimToken`, and a `claimUrl`.

- anonymous sites expire after 24 hours unless claimed
- anonymous publish is rate limited to 5 requests per hour per IP

See `SKILL.md` for full examples, including zip upload and claim requests.

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
./scripts/anon-smoke-test.sh
```

These cover:

- signed-in upload and stable URL redeploy
- zip upload serving
- anonymous publish, claim, badge injection, and publish rate limiting
