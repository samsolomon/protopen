# Protopen

A self-hostable static site and prototype hosting platform. Deploy a folder or zip from a dashboard or CLI and get a live URL.

**License:** [AGPL-3.0](LICENSE) &middot; **Self-hosting:** [docs/self-hosting.md](docs/self-hosting.md) &middot; **Contributing:** [CONTRIBUTING.md](CONTRIBUTING.md)

## What's in this repo

- `api/` — Go API: auth, uploads, sites/deploys, content serving
- `web/` — React dashboard
- `cli/` — Go CLI for deploying from the terminal

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

The dashboard runs at <http://localhost:5173>, talks to the API at <http://localhost:8080>, and serves deployed sites from <http://127.0.0.1:8081>.

A demo account is seeded automatically when `SEED_DEMO` is set (see `.env.example`):

- email: `sam@protopen.dev`
- password: `protopen-demo`

## Capabilities

- Upload a folder or zip from the dashboard or CLI
- Stable site URLs across redeploys, with versioned deploy paths
- Roll back to any previous deploy
- Toggle site visibility (public / private — private sites require sign-in)
- Auth: password-based with optional email verification (Resend)
- Storage: local filesystem for dev, Cloudflare R2 for production

## CLI

After building (`cd cli && make build`):

```bash
./protopen login                                   # paste an API token from Settings > API Tokens
./protopen deploy ./my-site                        # deploy a folder
./protopen deploy ./site.zip --name "Prototype"    # deploy a zip with a custom name
./protopen list                                    # list your sites
./protopen rollback <site-name> <deploy-id>        # roll back
./protopen help                                    # full command list
```

Config lives at `~/.protopen/config.json`. Resolution order: `--flag` > `PROTOPEN_TOKEN` / `PROTOPEN_URL` / `PROTOPEN_ORG` env > config file > defaults.

## Tests

See [CONTRIBUTING.md#running-tests](CONTRIBUTING.md#running-tests).

## Production / self-hosting

See [docs/self-hosting.md](docs/self-hosting.md) for the full guide — Cloudflare R2 setup, wildcard DNS, Resend email, deploy targets (Fly, Railway, bare VM, Docker).

## License

[AGPL-3.0-only](LICENSE). Network-use disclosure: if you run a modified copy of protopen as a hosted service, you must offer the modified source to your users.
