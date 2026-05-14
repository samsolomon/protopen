# Protopen API Reference

Base URL: `https://app.example.com` &mdash; substitute your own protopen instance hostname (the value of `APP_ORIGIN` in your `.env`).

All endpoints return JSON. Errors use `{"error": "message"}`.

## URL Format

- Authenticated projects: `https://sites.example.com/~{orgSlug}/{projectSlug}`
- Versioned deploys: `https://sites.example.com/~{orgSlug}/{projectSlug}/_v/{deployId}`

## Authentication

Two methods, checked in order:

1. **Bearer token**: `Authorization: Bearer ptk_...` (API tokens, `ptk_` prefix)
2. **Session cookie**: `protopen_session` (set by sign-in, 30-day expiry, HttpOnly)

---

## Auth & Session

### `POST /api/sign-in`

No auth required.

**Request:**
```json
{"email": "demo@protopen.dev", "password": "..."}
```

**Response (200):**
```json
{
  "user": {
    "id": "usr_...",
    "email": "demo@protopen.dev",
    "name": "Demo User",
    "username": "demo",
    "emailVerifiedAt": "2026-04-14T12:00:00Z",
    "orgs": [
      {"id": "org_...", "slug": "demo", "name": "Demo User", "isPersonal": true, "role": "admin"}
    ]
  }
}
```

Sets `protopen_session` cookie. `emailVerifiedAt` is null if the user hasn't verified their email.

### `POST /api/sign-up`

No auth required.

**Request:**
```json
{"email": "...", "password": "...", "name": "..."}
```

Password must be at least 8 characters. If the email has a pending org invite, the account is created as verified. Otherwise, a verification email is sent.

**Response (201):** Same shape as sign-in.

### `GET /api/session`

Auth required. Returns current user or 401.

**Response (200):** Same `{"user": {...}}` shape as sign-in.

### `POST /api/sign-out`

Auth required. Clears session cookie.

**Response (200):**
```json
{"ok": true}
```

### `POST /api/verify-email`

No auth required.

**Request:**
```json
{"token": "..."}
```

Verifies the user's email address using a token from the verification email.

**Response (200):**
```json
{"ok": true}
```

**Errors:** 400 (invalid or expired token)

### `POST /api/resend-verification`

Auth required. Rate limited. Sends a new verification email to the authenticated user. No-op if already verified.

**Response (200):**
```json
{"ok": true}
```

### `POST /api/forgot-password`

No auth required. Rate limited. Always returns 200 to prevent email enumeration.

**Request:**
```json
{"email": "demo@protopen.dev"}
```

If the email exists, a password reset link is sent (1-hour expiry).

**Response (200):**
```json
{"ok": true}
```

### `POST /api/reset-password`

No auth required.

**Request:**
```json
{"token": "...", "password": "..."}
```

Password must be at least 8 characters. Invalidates all existing sessions. Also verifies the user's email if not already verified.

**Response (200):**
```json
{"ok": true}
```

**Errors:** 400 (invalid/expired token, missing fields, short password)

---

## Projects

### `GET /api/projects`

Auth required.

**Query params:** `org` (optional, org slug — defaults to personal org)

**Response (200):**
```json
{
  "projects": [
    {
      "id": "proj_...",
      "name": "My Site",
      "slug": "my-site",
      "updatedAt": "2 hours ago",
      "deployCount": 5,
      "liveUrl": "https://sites.example.com/~demo/my-site",
      "isPublic": true,
      "gitBranch": "main",
      "gitCommitHash": "abc123...",
      "gitRemoteURL": "https://github.com/user/repo"
    }
  ]
}
```

Git fields are null when not available.

### `DELETE /api/projects/{projectID}`

Auth required. Soft-deletes the project.

**Response (200):**
```json
{"ok": true}
```

### `PATCH /api/projects/{projectID}`

Auth required. Update project visibility.

**Request:**
```json
{"isPublic": true}
```

**Response (200):**
```json
{"ok": true, "isPublic": true}
```

### `GET /api/projects/{projectID}/deploys`

Auth required. List deploys for a project.

**Response (200):**
```json
{
  "deploys": [
    {
      "id": "dep_...",
      "status": "validated",
      "label": "v2 redesign",
      "sizeBytes": 1024000,
      "fileCount": 42,
      "createdAt": "3 days ago",
      "isCurrent": true,
      "gitCommitHash": "abc123...",
      "gitBranch": "main",
      "gitCommitMessage": "Fix header layout",
      "gitDirty": false,
      "gitAuthor": "demo@protopen.dev",
      "gitRemoteURL": "https://github.com/user/repo"
    }
  ]
}
```

### `POST /api/projects/{projectID}/rollback`

Auth required. Roll back to a previous deploy.

**Request:**
```json
{"deployId": "dep_..."}
```

**Response (200):**
```json
{"ok": true, "currentDeployId": "dep_..."}
```

### `GET /api/sites/{siteID}/thumbnail`

Returns a WebP screenshot of the site's current deploy. Captures are generated
asynchronously after each deploy promotion when the server has thumbnails
enabled (`THUMBNAILS_ENABLED=1` and a Chromium binary available); requests for
deploys that have not yet been captured return 404.

**Auth:**
- Public sites: no session required.
- Private sites: session required, and the user must belong to the site's organization.

**Caching:** the response carries `ETag: "<deployID>"` and
`Cache-Control: private, max-age=300, must-revalidate`. Conditional requests
(`If-None-Match`) return 304.

**Status codes:**
- `200 image/webp` — thumbnail body
- `304` — `If-None-Match` matches the current deploy
- `401` — private site and no session
- `403` — private site and the user is not in the org
- `404` — site/deploy missing, or no thumbnail captured yet

---

## Uploads (Authenticated Deploy)

### `POST /api/uploads`

Auth required. Multipart form data.

**Query params:** `org` (optional, org slug)

**Form fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Project name (becomes URL slug) |
| `mode` | string | no | `"zip"` for zip files, `"files"` for individual files |
| `label` | string | no | Deploy label |
| `files` | file | yes | File(s) to upload |
| `paths` | string | no | JSON array of file paths (overrides filenames) |
| `git_commit_hash` | string | no | Git commit hash |
| `git_branch` | string | no | Git branch |
| `git_commit_message` | string | no | Commit message |
| `git_dirty` | string | no | `"true"` or `"false"` |
| `git_author` | string | no | Git author |
| `git_remote_url` | string | no | Git remote URL |

**Response (202):**
```json
{
  "message": "Upload staged and recorded.",
  "status": "queued",
  "project": {
    "id": "proj_...",
    "name": "My Site",
    "slug": "my-site",
    "updatedAt": "Just now",
    "deployCount": 1,
    "liveUrl": "https://sites.example.com/~demo/my-site",
    "isPublic": false
  }
}
```

**Example:**
```bash
curl -X POST https://app.example.com/api/uploads \
  -H "Authorization: Bearer ptk_..." \
  -F "name=my-site" \
  -F "mode=zip" \
  -F "files=@dist.zip"
```

---

## API Tokens

### `GET /api/tokens`

Auth required.

**Response (200):**
```json
{
  "tokens": [
    {"id": "tok_...", "name": "My Token", "createdAt": "5 days ago"}
  ]
}
```

### `POST /api/tokens`

Auth required.

**Request:**
```json
{"name": "My Token"}
```

Name is optional (defaults to "default").

**Response (201):**
```json
{
  "id": "tok_...",
  "name": "My Token",
  "token": "ptk_..."
}
```

The `token` value is only returned once at creation.

### `DELETE /api/tokens/{tokenID}`

Auth required.

**Response (200):**
```json
{"ok": true}
```

---

## Account

### `PATCH /api/account/profile`

Auth required.

**Request:**
```json
{"name": "New Name", "email": "new@email.com"}
```

**Response (200):**
```json
{"user": {...}}
```

### `POST /api/account/password`

Auth required.

**Request:**
```json
{"currentPassword": "...", "newPassword": "..."}
```

New password must be at least 8 characters.

**Response (200):**
```json
{"ok": true}
```

### `POST /api/account/delete`

Auth required.

**Request:**
```json
{"password": "..."}
```

Cannot delete if you're the sole admin of a shared org. Clears session cookie.

**Response (200):**
```json
{"ok": true}
```

---

## Organizations

### `GET /api/orgs`

Auth required.

**Response (200):**
```json
{
  "orgs": [
    {"id": "org_...", "slug": "demo", "name": "Demo User", "isPersonal": true, "role": "admin"}
  ]
}
```

### `POST /api/orgs`

Auth required.

**Request:**
```json
{"name": "Acme Corp", "slug": "acme-corp"}
```

**Response (201):**
```json
{
  "org": {"id": "org_...", "slug": "acme-corp", "name": "Acme Corp", "isPersonal": false, "role": "admin"}
}
```

### `GET /api/orgs/{orgID}/members`

Auth required. Must be a member.

**Response (200):**
```json
{
  "members": [
    {"id": "mem_...", "userId": "usr_...", "name": "Demo User", "email": "demo@protopen.dev", "username": "demo", "role": "admin"}
  ]
}
```

### `POST /api/orgs/{orgID}/members`

Auth required. Must be admin.

**Request:**
```json
{"email": "jane@example.com", "role": "member"}
```

**Response (201):**
```json
{"ok": true, "status": "added"}
```

Status is `"added"` if user exists, `"invited"` if not (7-day invite). When invited, an email is sent to the invitee with a signup link.

### `PATCH /api/orgs/{orgID}/members/{memberID}`

Auth required. Must be admin. Cannot change own role or demote last admin.

**Request:**
```json
{"role": "admin"}
```

**Response (200):**
```json
{"ok": true}
```

### `DELETE /api/orgs/{orgID}/members/{memberID}`

Auth required. Must be admin. Cannot remove last admin.

**Response (200):**
```json
{"ok": true}
```

---

## Admin

Admin endpoints require the caller's email to appear in `ADMIN_EMAILS`. Non-admins receive 403.

Instance admins also bypass the per-org admin check on the write endpoints under `/api/orgs/{orgID}/members` — they can add, change, or remove members of any organization without being an org admin of it. The last-admin protection still applies.

### `GET /api/admin/users`

Lists every account on the instance with its organization memberships.

**Response (200):**
```json
{
  "users": [
    {
      "id": "usr_...",
      "email": "jane@protopen.dev",
      "name": "Jane Chen",
      "username": "jane",
      "emailVerifiedAt": "2026-05-14T18:08:20Z",
      "createdAt": "2 hours ago",
      "orgs": [
        {
          "orgId": "org_...",
          "orgSlug": "demo",
          "orgName": "Demo User",
          "memberId": "mem_...",
          "role": "admin",
          "isPersonal": false
        }
      ]
    }
  ]
}
```

### `GET /api/admin/settings`

Returns instance-wide settings. Currently exposes the deploy-thumbnail capture toggle.

**Response (200):**
```json
{
  "thumbnails": {
    "available": true,
    "enabled": true,
    "reason": ""
  }
}
```

- `available` — whether this server has the capability (env `THUMBNAILS_ENABLED` set and a Chromium binary resolved).
- `enabled` — current runtime state from `instance_settings.thumbnails_enabled`.
- `reason` — populated only when `available` is `false`, explaining why.

### `PATCH /api/admin/settings`

Update one or more settings.

**Request:**
```json
{"thumbnailsEnabled": true}
```

**Response (200):** same shape as `GET /api/admin/settings`.

- Returns **409** if the request asks to enable a feature whose `available` is `false`, with `reason` in the error body.
- Returns **403** if the caller is not admin.
- Toggling takes effect immediately: enabling spawns the Chromium allocator and kicks the backstop loop to capture any older deploys that were missed; disabling cancels the allocator and frees memory.

---

## Constraints

| Constraint | Value |
|-----------|-------|
| Max upload size | 250 MB total |
| Max file size | 250 MB per file |
| Max files (zip) | 5,000 |
| Required entry file | `index.html` |
| Symlinks | Not allowed |
| Session cookie expiry | 30 days |
| Password minimum | 8 characters |
| Email verification token expiry | 24 hours |
| Password reset token expiry | 1 hour |

## Miscellaneous

### `GET /healthz`

No auth. Returns `{"status": "ok"}`.
