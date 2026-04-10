# Velori API Reference

Base URL: `https://app.velori.dev`

All endpoints return JSON. Errors use `{"error": "message"}`.

## URL Format

- Authenticated projects: `https://sites.velori.dev/~{orgSlug}/{projectSlug}`
- Anonymous publishes: `https://sites.velori.dev/{slug}`
- Versioned deploys: `https://sites.velori.dev/~{orgSlug}/{projectSlug}/_v/{deployId}`

## Authentication

Two methods, checked in order:

1. **Bearer token**: `Authorization: Bearer vtk_...` (API tokens, `vtk_` prefix)
2. **Session cookie**: `velori_session` (set by sign-in, 30-day expiry, HttpOnly)

---

## Auth & Session

### `POST /api/sign-in`

No auth required.

**Request:**
```json
{"email": "sam@velori.dev", "password": "..."}
```

**Response (200):**
```json
{
  "user": {
    "id": "usr_...",
    "email": "sam@velori.dev",
    "name": "Sam Solomon",
    "username": "sam",
    "orgs": [
      {"id": "org_...", "slug": "sam", "name": "Sam Solomon", "isPersonal": true, "role": "admin"}
    ]
  }
}
```

Sets `velori_session` cookie.

### `POST /api/sign-up`

No auth required.

**Request:**
```json
{"email": "...", "password": "...", "name": "..."}
```

Password must be at least 8 characters.

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
      "liveUrl": "https://sites.velori.dev/~sam/my-site",
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
      "gitAuthor": "sam@velori.dev",
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
    "liveUrl": "https://sites.velori.dev/~sam/my-site",
    "isPublic": false
  }
}
```

**Example:**
```bash
curl -X POST https://app.velori.dev/api/uploads \
  -H "Authorization: Bearer vtk_..." \
  -F "name=my-site" \
  -F "mode=zip" \
  -F "files=@dist.zip"
```

---

## Anonymous Publishing

### `POST /api/v1/publish`

No auth required. Rate limited: 5 per hour per IP. Multipart form data (same fields as `/api/uploads` minus git fields).

**Response (200):**
```json
{
  "siteUrl": "https://sites.velori.dev/bright-canvas-a7k2",
  "slug": "bright-canvas-a7k2",
  "claimToken": "64-char-hex-string",
  "claimUrl": "https://app.velori.dev/claim/bright-canvas-a7k2",
  "expiresAt": "2026-04-10T12:00:00Z"
}
```

Anonymous sites expire in 24 hours. File types are restricted to web assets (HTML, CSS, JS, images, fonts, media, WASM, text, maps, 3D models).

**Example:**
```bash
curl -X POST https://app.velori.dev/api/v1/publish \
  -F "name=my-prototype" \
  -F "files=@index.html"
```

### `POST /api/v1/claim`

Auth required. Claim an anonymous deploy into your account.

**Request:**
```json
{
  "slug": "bright-canvas-a7k2",
  "claimToken": "64-char-hex-string",
  "name": "My Prototype",
  "org": "optional-org-slug"
}
```

**Response (200):**
```json
{
  "projectId": "proj_...",
  "liveUrl": "https://sites.velori.dev/~sam/my-prototype",
  "name": "My Prototype",
  "slug": "my-prototype"
}
```

**Error codes:** 403 (invalid token), 410 (already claimed or expired), 404 (not found)

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
  "token": "vtk_..."
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
    {"id": "org_...", "slug": "sam", "name": "Sam Solomon", "isPersonal": true, "role": "admin"}
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
    {"id": "mem_...", "userId": "usr_...", "name": "Sam Solomon", "email": "sam@velori.dev", "username": "sam", "role": "admin"}
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

Status is `"added"` if user exists, `"invited"` if not (7-day invite).

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

## Comments (Embedded Widget)

### `GET /__velori/api/comments`

Auth required.

**Query params:** `project_id`, `deploy_id`, `page_path` (all required), `include_resolved` (optional, `"true"`)

**Response (200):**
```json
{
  "comments": [
    {
      "id": "cmt_...",
      "projectId": "proj_...",
      "deployId": "dep_...",
      "userId": "usr_...",
      "userName": "Sam Solomon",
      "pagePath": "/",
      "pinX": 45.5,
      "pinY": 67.2,
      "body": "This header needs more contrast",
      "parentId": null,
      "resolvedAt": null,
      "resolvedBy": null,
      "createdAt": "2026-04-09T10:34:56Z",
      "replies": [...]
    }
  ]
}
```

Replies are nested one level deep (no deeper threading).

### `POST /__velori/api/comments`

Auth required.

**Request:**
```json
{
  "projectId": "proj_...",
  "deployId": "dep_...",
  "pagePath": "/",
  "pinX": 45.5,
  "pinY": 67.2,
  "body": "This header needs more contrast",
  "parentId": null
}
```

`pinX`/`pinY` required for root comments (0-100 range). `parentId` for replies (max 1 level).

**Response (201):**
```json
{"comment": {...}}
```

### `PATCH /__velori/api/comments/{commentID}`

Auth required. Author can edit body and pin position. Any org member can resolve/unresolve.

**Request:**
```json
{"body": "Updated text", "resolved": true}
```

**Response (200):**
```json
{"ok": true}
```

### `DELETE /__velori/api/comments/{commentID}`

Auth required. Author or org admin can delete.

**Response (200):**
```json
{"ok": true}
```

### `GET /__velori/api/members`

Auth required. Returns org members for @mention autocomplete.

**Query params:** `project_id` (required)

**Response (200):**
```json
{
  "members": [
    {"id": "usr_...", "name": "Sam Solomon", "username": "sam"}
  ]
}
```

---

## Notifications

### `GET /api/notifications`

Auth required. Returns 50 most recent notifications.

**Response (200):**
```json
{
  "notifications": [
    {
      "id": "ntf_...",
      "type": "mention",
      "actorName": "Jane Smith",
      "projectName": "My Site",
      "commentId": "cmt_...",
      "bodyPreview": "First 100 chars...",
      "linkUrl": "https://sites.velori.dev/~sam/my-site/#vlr-comment=cmt_...",
      "readAt": null,
      "createdAt": "2026-04-09T10:34:56Z"
    }
  ]
}
```

Types: `mention`, `reply`, `resolve`, `new_comment`.

### `POST /api/notifications`

Auth required. Mark notifications as read.

**Request:**
```json
{"ids": ["ntf_...", "ntf_..."]}
```

Or mark all: `{"all": true}`

**Response (200):**
```json
{"ok": true}
```

### `GET /api/notifications/count`

Auth required.

**Response (200):**
```json
{"count": 5}
```

Unread count only.

---

## Constraints

| Constraint | Value |
|-----------|-------|
| Max upload size | 100 MB total |
| Max file size | 20 MB per file |
| Max files (zip) | 5,000 |
| Required entry file | `index.html` |
| Symlinks | Not allowed |
| Anonymous publish rate limit | 5 per hour per IP |
| Anonymous publish expiry | 24 hours |
| Session cookie expiry | 30 days |
| Password minimum | 8 characters |
| Comment threading | 1 level max |
| Notification history | 50 most recent |

## Miscellaneous

### `GET /healthz`

No auth. Returns `{"status": "ok"}`.

### `GET /install.sh`

No auth. Returns the Velori install script (plain text). Installs the CLI binary and agent skill.
