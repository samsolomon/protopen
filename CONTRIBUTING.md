# Contributing to Protopen

Thanks for your interest. Protopen is AGPL-3.0; by contributing you agree to license your contribution under the same terms.

## Local setup

The quickstart in [README.md#quickstart-local-dev](README.md#quickstart-local-dev) walks through everything you need. R2 and Resend can stay empty for local work; see `.env.example` for what each var falls back to.

## Running tests

```bash
(cd api && go test ./...)         # Go API
(cd cli && go test ./...)         # Go CLI
(cd web && npm test)              # React/Vitest
```

Type-check the web side without building:

```bash
(cd web && npx tsc --noEmit)
```

Smoke tests exercise the running API end-to-end (need Postgres + API up):

```bash
./scripts/local-smoke-test.sh
./scripts/zip-smoke-test.sh
```

## Commit and PR style

- One logical change per commit; keep them reviewable.
- Subject line: short imperative ("Add X", "Fix Y", "Remove Z"), under 70 chars.
- Body (when needed): the **why**, not the **what**. The diff already says what changed.
- Run the relevant test suites before opening a PR.
- API changes must update [docs/api-reference.md](docs/api-reference.md) — that file is the authoritative reference.

## Filing issues

- Bugs: include reproduction steps, expected vs actual behavior, and the protopen version (`./protopen version` or commit SHA).
- Feature requests: describe the use case first, then the proposed shape. Keep scope to what one PR can land.
- Security: please email rather than opening a public issue.

## Project structure

```
api/        Go API (single binary, embeds the web dist)
  migrate/    SQL migrations
cli/        Go CLI
web/        React dashboard (Vite)
docs/       Self-hosting and API reference
scripts/    Smoke tests
```

Migrations run automatically on API startup. To write a new migration, add `api/migrate/migrations/NNN_short_description.sql` — the runner picks them up in filename order.
