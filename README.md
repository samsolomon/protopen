# Protopen

A self-hostable deploy target for static sites. One command, folder in, live URL out. Built for CLI workflows and coding agents.

**License:** [AGPL-3.0](LICENSE) &middot; **Self-hosting:** [docs/self-hosting.md](docs/self-hosting.md) &middot; **Contributing:** [CONTRIBUTING.md](CONTRIBUTING.md)

## Why

Coding agents (Claude Code, Cursor, custom scripts) generate static sites and prototypes constantly. Protopen exists to be the deploy step at the end of that loop: any process that can shell out gets a live URL in one call. The web dashboard is for managing what you've already shipped — the CLI is the primary interface.

## CLI

```bash
cd cli && make build                        # build the binary
./protopen login                            # paste an API token from Settings > API Tokens
./protopen deploy ./my-site                 # → live URL on stdout
./protopen deploy ./dist --json             # → structured JSON
./protopen list                             # your sites
./protopen rollback <site-name> <deploy-id> # revert
./protopen visibility <name> public|private
./protopen help                             # full command list
```

Output is intentionally minimal so commands compose cleanly into pipelines: the live URL on stdout, git/branch metadata on stderr. Every command takes `--json` for machine-readable output.

Config lives at `~/.protopen/config.json`. Resolution order: `--flag` > `PROTOPEN_TOKEN` / `PROTOPEN_URL` / `PROTOPEN_ORG` env > config file > defaults.

## Use with agents

Give your agent the CLI in its toolbox. Any agent that can run a shell command can deploy.

Example agent instruction:

> When the user asks to share, preview, or publish what we just built, run `protopen deploy <path>` and return the URL it prints.

Or a one-shot bash snippet you can hand off:

```bash
URL=$(protopen deploy ./dist --json | jq -r .liveUrl)
echo "Preview at: $URL"
```

Works with anything that can call a binary — Claude Code, Cursor, Codex, in-house scripts, post-commit hooks. The CLI is the universal interface; the API is documented at [docs/api-reference.md](docs/api-reference.md) if you'd rather talk HTTP directly.

## Quickstart (local dev)

```bash
git clone https://github.com/samsolomon/protopen.git
cd protopen
cp .env.example .env
docker compose up -d postgres
npm install
(cd api && go run .) &
npm run dev:web
```

Dashboard at <http://localhost:5173>, API at <http://localhost:8080>, deployed sites served from <http://127.0.0.1:8081>.

Demo account seeded automatically when `SEED_DEMO` is set (see `.env.example`):

- email: `sam@protopen.dev`
- password: `protopen-demo`

## What's in this repo

- `cli/` — Go CLI, the primary interface
- `api/` — Go API: auth, uploads, deploys, content serving
- `web/` — React dashboard for managing deployed sites

## Capabilities

- Single-command deploy from a folder or zip
- Stable site URLs across redeploys; versioned URLs for any previous deploy
- Rollback to any previous deploy in one command
- Public / private sites (private requires sign-in)
- Local-filesystem storage for dev, Cloudflare R2 for production
- Password auth with optional email verification (Resend)

## Tests

See [CONTRIBUTING.md#running-tests](CONTRIBUTING.md#running-tests).

## Production / self-hosting

See [docs/self-hosting.md](docs/self-hosting.md) — Cloudflare R2, wildcard DNS, Resend, and deploy targets (Fly, Railway, bare VM, Docker).

## License

[AGPL-3.0-only](LICENSE). Network-use disclosure: if you run a modified copy of protopen as a hosted service, you must offer the modified source to your users.
