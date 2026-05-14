# Security Policy

## Reporting a vulnerability

Please **don't** open a public GitHub issue for security reports. Instead, email **security@protopen.dev** (or my GitHub-listed contact if that bounces) with:

- A description of the issue and its impact
- Steps to reproduce
- The protopen version (commit SHA or release tag) and how you were running it

I'll acknowledge within 72 hours, ask follow-up questions if needed, and credit you in the release notes when the fix ships unless you'd rather stay anonymous.

## Scope

In scope:

- The `api/` Go service (auth, uploads, sites, tokens, content serving)
- The `cli/` Go binary
- The `web/` React dashboard
- Migrations and seed data

Out of scope:

- Self-hosted instances configured insecurely by the operator (missing `COOKIE_DOMAIN`, public `ADMIN_EMAILS` lists, exposed Postgres, etc.)
- Findings in third-party dependencies — please report those upstream
- Denial of service via volumetric attacks — the project assumes operators put a CDN/WAF in front
- Anything requiring a stolen API token or admin credentials

## Supported versions

Only `main` is supported for security fixes. There are no LTS releases.
