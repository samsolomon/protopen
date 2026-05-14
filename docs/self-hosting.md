# Self-hosting Protopen

This guide walks you through running protopen in production. R2 and Resend are optional — protopen falls back to local-filesystem storage and a no-op mailer when their env vars are empty, so you can stand up a working instance with only Postgres and an API host.

## 1. Prerequisites

- A domain you control (we'll use `example.com` below)
- A Postgres 14+ database (managed or self-run)
- A host that can run a Go binary (Fly, Railway, Render, Hetzner, a $5 VPS — anything works)
- Optional: a Cloudflare account for R2 object storage and DNS
- Optional: a Resend account for transactional email

## 2. Cloudflare R2 (object storage)

Without R2, protopen stores uploads on the local filesystem at `INGEST_ROOT` (`.data/ingest` by default). That's fine for a single-host setup but doesn't survive container restarts and doesn't scale horizontally. For production, use R2.

1. In the Cloudflare dashboard, **R2 → Create bucket**. Name it (e.g. `protopen-sites`).
2. **R2 → Manage R2 API Tokens → Create API Token**. Permission: **Object Read & Write**, scoped to your bucket. Save the **Account ID**, **Access Key ID**, and **Secret Access Key**.
3. Set the four R2 env vars in your deployment:
   ```
   R2_BUCKET_NAME=protopen-sites
   R2_ACCOUNT_ID=...
   R2_ACCESS_KEY_ID=...
   R2_SECRET_ACCESS_KEY=...
   ```
4. For serving site content, point `PUBLIC_CONTENT_URL` at a URL that fronts the bucket. The simplest option is **R2 → Settings → enable the public r2.dev URL** and use that. The recommended option is a [custom domain bound to the bucket](https://developers.cloudflare.com/r2/buckets/public-buckets/#custom-domains) (e.g. `sites.example.com`).

## 3. DNS

Protopen serves site content from URLs like `https://sites.example.com/~{org}/{site}`. Point the relevant records at your content host or R2 custom domain:

- `app.example.com` (or your apex) → your protopen API host
- `sites.example.com` → your content host (R2 custom domain or content origin)

If you want both behind one domain (single-server production mode), set `PORT` and protopen will serve both on the same port. Otherwise use split mode: API on `APP_LISTEN_ADDR`, content on `CONTENT_LISTEN_ADDR`.

## 4. Resend (transactional email)

Email is **optional**. With `RESEND_API_KEY` empty, protopen still works — auth is password-based, not magic-link. The only difference is that email-verification sends are skipped (verification is non-blocking) and password-reset emails won't go out.

To enable email:

1. Create a [Resend](https://resend.com) account and verify a sending domain (e.g. `example.com`).
2. Create an API key with **Sending access**.
3. Set the env vars:
   ```
   RESEND_API_KEY=re_...
   RESEND_FROM_ADDRESS=Protopen <noreply@example.com>
   ```

## 5. Database

Protopen runs migrations automatically on startup. Migrations are versioned files in `api/migrate/migrations/` and picked up in filename order on each boot — nothing to run by hand.

## 6. Deploy

The API is a single Go binary that embeds the built web dist (see `api/frontend.go`'s `//go:embed all:dist`). One process serves both the dashboard and content.

### Docker Compose (single host)

The `Dockerfile` in the repo builds the API binary at `/protopen` with the web frontend embedded. `docker-compose.yml` starts Postgres for you:

```bash
docker compose up -d postgres
docker build -t protopen .
docker run --env-file .env -p 8080:8080 protopen
```

### Fly / Railway / Render

These platforms detect the Dockerfile and build automatically. Point at a managed Postgres, set env vars from `.env.example` (`flyctl secrets set ...`, Railway/Render dashboard, etc.), and deploy.

### Bare VM

Build locally and copy the binary. The API embeds the web dashboard, so the frontend must be built **before** compiling the API:

```bash
npm install
npm run build:web                                         # produces web/dist
(cd api && CGO_ENABLED=0 go build -o protopen-server .)   # embeds web/dist
scp protopen-server vm:/usr/local/bin/
```

Run it under systemd or `tmux` with `.env` exported. Same env vars apply.

## 7. First user

There's no CLI bootstrap for creating the first account. Once the API is up, hit your dashboard URL (`APP_ORIGIN`) and use the sign-up form. The first user has no special privileges — they become an admin only when their email is listed in `ADMIN_EMAILS` (next section).

## 8. Admin access

Set `ADMIN_EMAILS` to a comma-separated list of email addresses that should see the **Admin** panel in the dashboard (user list, site list, force delete). The admin gate is checked on every authenticated request, so changes take effect on next sign-in.

```
ADMIN_EMAILS=you@example.com,cofounder@example.com
```

## Upgrading

Pull the latest code (or rebuild from a new release tag) and restart. Migrations run automatically on boot.

## Troubleshooting

- **"could not connect to database"**: verify `DATABASE_URL` and that Postgres allows connections from your API host's IP.
- **Sites show 404**: check `PUBLIC_CONTENT_URL` points at the right host and (for R2) the bucket is public or custom-domain-bound.
- **Email never arrives**: with `RESEND_API_KEY` empty, this is expected. Check API logs — `email skipped` lines mean the mailer is nil.
- **Wildcard DNS not resolving**: confirm your DNS provider supports wildcard A/CNAME records and that the TTL has passed.

For other issues, open a GitHub issue with reproduction steps and your protopen version.
