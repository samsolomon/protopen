# Protopen

A self-hostable playpen for static prototypes. Deploy a folder, get a live URL, and gather feedback through comments. Make it public to test designs with customers, or keep it private for you and your team. Protopen is built for product teams working with agents.

**License:** [AGPL-3.0](LICENSE) &middot; **Self-hosting:** [docs/self-hosting.md](docs/self-hosting.md) &middot; **Contributing:** [CONTRIBUTING.md](CONTRIBUTING.md)

## Why

Coding agents—Claude Code, Codex, Cursor—can rapidly turn out static sites and prototypes. Protopen gives you a way to share them and gather feedback. It's the final step: deploy your prototype and get a live URL in seconds, then manage what you've shipped from the web dashboard.

## CLI

```bash
cd cli && make build                        # build the binary
./protopen login                            # browser-based device-code flow (default)
./protopen login --token ptk_...            # or paste a token from Settings > API Tokens
./protopen deploy ./my-site                 # → live URL on stdout
./protopen deploy ./dist --json             # → structured JSON
./protopen list                             # your sites
./protopen deploys <site-name>              # deploy history for a site
./protopen rollback <site-name> <deploy-id> # revert
./protopen visibility <name> public|private
./protopen comments <name>                  # comments left on a site
./protopen version                          # print CLI version
./protopen help                             # full command list
```

Output is intentionally minimal so commands compose cleanly into pipelines: the live URL on stdout, git/branch metadata on stderr. Every data command — `deploy`, `list`, `deploys`, `rollback`, `visibility`, `comments`, `token` — takes `--json` for machine-readable output.

Config lives at `~/.protopen/config.json`. Resolution order: `--flag` > `PROTOPEN_TOKEN` / `PROTOPEN_URL` / `PROTOPEN_ORG` env > config file > defaults.

## MCP server

For agents that speak the [Model Context Protocol](https://modelcontextprotocol.io) (Claude Desktop, MCP-aware editors), `protopen mcp` runs an MCP server over stdio. It's the same actions as the CLI, exposed as MCP tools, sharing `~/.protopen/config.json` — so `protopen login` is the only setup step for either surface.

| Action         | CLI                                  | MCP tool        |
| -------------- | ------------------------------------ | --------------- |
| Deploy         | `protopen deploy <path>`             | `deploy`        |
| List sites     | `protopen list`                      | `list_sites`    |
| List deploys   | `protopen deploys <name>`            | `list_deploys`  |
| Rollback       | `protopen rollback <name> <id>`      | `rollback`      |
| Read comments  | `protopen comments <name>`           | `list_comments` |
| Set visibility | `protopen visibility <name> <vis>`   | `set_visibility` |

Example Claude Desktop config (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "protopen": {
      "command": "/usr/local/bin/protopen",
      "args": ["mcp"]
    }
  }
}
```

The `deploy` tool requires absolute paths because stdio MCP has no shared working directory with the agent.

## Use with agents

Any agent that can run a shell command can deploy via the CLI — Claude Code, Cursor, Codex, in-house scripts, post-commit hooks.

Example agent instruction:

> When the user asks to share, preview, or publish what we just built, run `protopen deploy <path>` and return the URL it prints.

Or a one-shot bash snippet you can hand off:

```bash
URL=$(protopen deploy ./dist --json | jq -r .liveUrl)
echo "Preview at: $URL"
```

For agents that speak MCP natively, point them at `protopen mcp` (above). For everything else, the HTTP API is documented at [docs/api-reference.md](docs/api-reference.md).

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

Dashboard at <http://localhost:5173>, API at <http://localhost:8080>, deployed sites served from <http://127.0.0.1:8081>. The Postgres container binds to host port `5433` (not the usual 5432) so it coexists with a Homebrew or Postgres.app server already running locally; the API reads `.env` from the repo root automatically, so no `source .env` is needed.

A demo account, three teammates, and five example sites are seeded on first boot (toggled by `SEED_DEMO` in `.env.example`, defaulted on):

- email: `demo@protopen.dev`
- password: `protopen-demo`

Sign in, then visit the dashboard to see the seeded sites. Thumbnails are captured by the runtime backstop once `THUMBNAILS_ENABLED=1` (also defaulted on) and a Chromium binary is available — on macOS the standard Google Chrome install is auto-detected, on Linux any `chromium-browser` / `google-chrome` on `PATH` works, otherwise set `CHROMIUM_PATH`.

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
- Password auth with optional email verification; email via Resend or SMTP
- Figma-style comments on live sites — pinned threads, `@mentions`, resolve
- In-app and email notifications with per-user preferences; reply to a
  comment straight from your inbox

## Tests

See [CONTRIBUTING.md#running-tests](CONTRIBUTING.md#running-tests).

## Production / self-hosting

See [docs/self-hosting.md](docs/self-hosting.md) — Cloudflare R2, DNS setup, email (Resend or SMTP), and deploy targets (Fly, Railway, bare VM, Docker).

## License

[AGPL-3.0-only](LICENSE). Network-use disclosure: if you run a modified copy of protopen as a hosted service, you must offer the modified source to your users.
