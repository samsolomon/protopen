# Protopen API Reference

Base URL: `https://app.example.com` &mdash; substitute your own protopen instance hostname (the value of `APP_ORIGIN` in your `.env`).

All endpoints return JSON. Errors use `{"error": "message"}`.

## URL Format

- Authenticated sites: `https://sites.example.com/~{orgSlug}/{siteSlug}`
- Versioned deploys: `https://sites.example.com/~{orgSlug}/{siteSlug}/_v/{deployId}`

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

## Sites

### `GET /api/sites`

Auth required.

**Query params:**
- `org` (optional, org slug — defaults to personal org)
- `author` (optional) — set to `me` to limit results to sites created by the
  caller. Any other value is ignored.

The reference dashboard fetches the full org list once and filters client-side
to power its `My sites` / `All sites` scope tabs (persisted in the URL as
`?view=mine|all`). The `?author=me` server-side filter is provided for clients
that prefer a narrower fetch.

**Response (200):**
```json
{
  "sites": [
    {
      "id": "site_...",
      "name": "My Site",
      "slug": "my-site",
      "orgSlug": "demo",
      "updatedAt": "2 hours ago",
      "deployCount": 5,
      "openCommentCount": 2,
      "liveUrl": "https://sites.example.com/~demo/my-site",
      "isPublic": true,
      "gitBranch": "main",
      "gitCommitHash": "abc123...",
      "gitRemoteURL": "https://github.com/user/repo",
      "createdBy": {
        "id": "usr_...",
        "name": "Demo User",
        "username": "demo"
      }
    }
  ]
}
```

Git fields are null when not available. `createdBy` is `null` for legacy
rows that pre-date ownership tracking; the client should render those as
an unknown author.

### `DELETE /api/sites/{siteID}`

Auth required. Soft-deletes the site. Caller must be the site's creator or an
admin of the owning org (see [Site ownership](#site-ownership)).

**Response (200):**
```json
{"ok": true}
```

**Status codes:**
- `200` — site soft-deleted
- `403` — caller is a member but not the creator or an admin
- `404` — site missing

### `PATCH /api/sites/{siteID}`

Auth required. Update site visibility. Caller must be the site's creator or an
admin of the owning org (see [Site ownership](#site-ownership)).

**Request:**
```json
{"isPublic": true}
```

**Response (200):**
```json
{"ok": true, "isPublic": true}
```

Side-effect: setting `isPublic: true` resets the site's `made_public_at`
clock, which determines when the auto-private sweeper (if enabled) reverts
the site. Setting `isPublic: false` clears the clock.

### `POST /api/sites/{siteID}/duplicate`

Auth required. Clones a site within the same org and attributes the new copy
to the caller. Any org member can duplicate — duplication is the
non-destructive escape hatch when a teammate wants to riff on a site they
don't own.

The new site shares its source's deploy storage prefix (deploys are
immutable, so no file copy is required). The duplicate's slug is derived
from the source as `<source-slug>-copy`, `<source-slug>-copy-2`, … to avoid
collisions.

**Response (201):**
```json
{
  "site": {
    "id": "site_...",
    "name": "Copy of My Site",
    "slug": "my-site-copy",
    "updatedAt": "Just now",
    "deployCount": 1,
    "liveUrl": "https://sites.example.com/~demo/my-site-copy",
    "isPublic": true,
    "createdBy": {
      "id": "usr_caller",
      "name": "Caller Name",
      "username": "caller"
    }
  }
}
```

**Status codes:**
- `201` — duplicate created
- `403` — caller is not a member of the source's org
- `404` — source site missing

The reference dashboard, after a successful duplicate, flips its scope tab to
`My sites` so the user lands on the freshly attributed copy rather than the
source they cannot mutate.

### `GET /api/sites/{siteID}/deploys`

Auth required. List deploys for a site.

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

### `POST /api/sites/{siteID}/rollback`

Auth required. Roll back to a previous deploy. Caller must be the site's
creator or an admin of the owning org (see [Site ownership](#site-ownership)).

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

## Comments

Comments are pinned to a DOM element on a page of a site. They form
single-level threads (a root comment plus replies). Posting requires a
signed-in org member. Reading is open to org members on any site and to
anonymous viewers on public sites. Only the comment author or an org
admin can delete; any member can toggle resolve.

Comments are **site-scoped** — they survive across deploys. The
`deployId` field on a comment records the deploy it was first seen on
(metadata only); deleting a deploy nulls the reference but keeps the
comment. Every root comment is anchored to an element via
`elementSelector` + `elementOffsetX`/`elementOffsetY` (offset within the
element's bounding box, both in `[0,1]`). See [`docs/site-anchors.md`](site-anchors.md)
for the `data-comment-anchor` convention that keeps anchors stable
across HTML refactors.

A Figma-style runtime is injected into every deployed HTML response and
renders the comment overlay (Browse/Comment toggle, pin layer, side
panel, composer with `@mention` autocomplete) directly on the live URL.
Anonymous viewers see the overlay and existing comments; the composer
shows a sign-in link instead of a text field. Injection is skipped for
thumbnail-token requests and when `?protopen-comments=0` is set. The
runtime is served at `GET /__protopen/comment-runtime.js` from the
content origin.

### `GET /api/sites/by-slug/{orgSlug}/{siteSlug}/comment-context`

Bootstrap endpoint for the injected runtime. Returns the IDs the
slug-only context needs to call the other comment endpoints.

No auth required for public sites; org membership required for private.

**Response (200):**
```json
{
  "siteId": "site_...",
  "deployId": "dep_...",
  "isPublic": true,
  "siteName": "Mobile Nav"
}
```

**Status codes:** `200` ok, `401` private site without session, `403`
private site, signed in but not a member, `404` site not found.

### `GET /api/sites/{siteID}/comments`

Org members can read on any site. Anonymous visitors can read on public
sites only.

**Query params:**
- `deployId` (optional) — filters to comments first seen on this deploy.
  Omit for site-scoped results (the typical case).
- `status` (optional) — `open` (default), `resolved`, or `all`.
- `pagePath` (optional) — exact-match filter on the page path.

**Response (200):**
```json
{
  "comments": [
    {
      "id": "cm_...",
      "siteId": "site_...",
      "deployId": "dep_...",
      "pagePath": "/",
      "elementSelector": "[data-comment-anchor=\"hero-cta\"]",
      "elementOffsetX": 0.5,
      "elementOffsetY": 0.5,
      "body": "Move this CTA above the fold",
      "parentId": null,
      "resolvedAt": null,
      "resolvedBy": null,
      "createdAt": "2026-05-18T19:30:00Z",
      "author": {"id": "usr_...", "name": "Jane Chen", "username": "jane"},
      "guestName": null
    }
  ]
}
```

Replies have a non-null `parentId` pointing at the root comment and no
anchor fields. Root comments always have `elementSelector` populated;
when the selector can't be resolved at render time the runtime skips
the pin rather than rendering at the document origin.

`deployId` on a comment can be null when the originating deploy has
since been deleted — the comment itself stays site-scoped.

New comments always have `author` populated and `guestName: null`.
Pre-existing rows from before sign-in was required can have a non-null
`guestName` with `author: null`.

### `POST /api/sites/{siteID}/comments`

Auth required. Caller must be a member of the site's org. Rate-limited
per client IP.

**Request:**
```json
{
  "deployId": "dep_...",
  "pagePath": "/",
  "elementSelector": "[data-comment-anchor=\"hero-cta\"]",
  "elementOffsetX": 0.5,
  "elementOffsetY": 0.5,
  "parentId": null,
  "body": "Move this CTA above the fold @jane"
}
```

- `deployId` is optional and defaults to the site's current deploy. It
  marks the deploy the comment was first seen on; the comment itself is
  site-scoped.
- `parentId` is optional and indicates a reply (replies carry no anchor).
- `elementSelector` / `elementOffsetX` / `elementOffsetY` together
  anchor the pin to a DOM element. For root comments these are optional
  but recommended — when omitted the server defaults to body at center
  (`selector: "body"`, both offsets `0.5`). Legacy `pinX` / `pinY` are
  silently ignored.

**Header:** `X-Protopen-Client: runtime` is required on cross-origin
requests (i.e. POSTs from the deployed-site origin). Same-origin
requests from the dashboard and API origin do not need it.

**Response (201):** `{ "comment": {...} }` with the same shape as the
list response.

**Fan-out:**
- Site owner is auto-subscribed to every comment on their site via the
  `site_subscriptions` table (seeded on site creation).
- Authors auto-subscribe to threads they create or reply to.
- `@username` mentions resolved against org members fire a
  `comment_mention` notification and auto-subscribe the mentioned user
  to the thread.
- Every notification recipient gets one row in a single multi-row
  insert. The author never notifies themselves.
- When email is configured, recipients are also emailed off the request
  path — see [Notifications](#notifications).

**Status codes:** `201` created, `400` invalid (missing body, parent
belongs to a different site, deploy not on this site), `401`
unauthenticated, `403` not a member of the org or cross-origin without
the runtime header, `429` rate limited.

### `GET /api/sites/{siteID}/mention-candidates`

Returns up to 10 org members whose username starts with `q`, for the
`@mention` autocomplete in the runtime composer.

Auth required: signed-in org member. Guests get 401.

**Query params:**
- `q` (optional) — case-insensitive prefix.

**Response (200):**
```json
{
  "candidates": [
    {"id": "usr_...", "name": "Jane Chen", "username": "jane"}
  ]
}
```

### `PATCH /api/comments/{commentID}`

Auth required. Caller must be the comment author or an org admin. Used
by the runtime to commit a pin drag.

**Request:** every field is optional; omitted fields are left unchanged.
```json
{
  "elementSelector": "[data-comment-anchor=\"hero-cta\"]",
  "elementOffsetX": 0.5,
  "elementOffsetY": 0.5
}
```

The runtime sends these three fields whenever a pin is dragged onto a
new element — `elementFromPoint` resolves the drop target, the runtime
captures a fresh anchor against it, and the PATCH commits the new
selector + offsets. Legacy `pinX`, `pinY`, and `clearAnchor` fields are
silently ignored: every comment must stay anchored to an element.

**Header:** `X-Protopen-Client: runtime` is required on cross-origin
PATCHes, matching the POST policy.

**Response (200):** `{ "ok": true }`

**Status codes:** `200` ok, `400` invalid body, `401` unauthenticated,
`403` not author and not admin (or cross-origin without the runtime
header), `404` not found.

### `DELETE /api/comments/{commentID}`

Auth required. Caller must be the comment author or an org admin.
Pre-existing guest-authored rows have no `user_id` to match against,
so only admins can delete them. Hard-deletes the row; replies cascade
via FK.

**Status codes:** `200` ok, `403` forbidden, `404` not found.

### `POST /api/comments/{commentID}/resolve`

Auth required. Any org member. Toggles `resolved_at` / `resolved_by`. If
the comment is open, marks resolved by the caller. If already resolved,
clears the resolution.

**Response (200):** `{ "ok": true, "resolved": true }`.

---

## Notifications

The notifications table is comment-driven. A row appears when someone
replies to a thread you're subscribed to, when a comment lands on a site
you own (via `site_subscriptions`), or when you're `@mentioned` in a
comment body.

When an email provider is configured, each notification also sends an email
to recipients who have a verified address and haven't opted out of that
notification type (see `/api/notification-preferences`). Email is dispatched
off the request path, so creating a comment is unaffected by mail latency.

### `GET /api/notification-preferences`

Auth required (session, not bearer token). Returns the signed-in user's email
notification preferences. An absent row yields the opted-in defaults.

**Response (200):**
```json
{"emailOnReply": true, "emailOnMention": true}
```

### `PATCH /api/notification-preferences`

Auth required (session, not bearer token). Updates the signed-in user's
preferences. Both fields optional; only supplied fields change.

**Request:**
```json
{"emailOnReply": false}
```

**Response (200):** same shape as `GET`.

- `emailOnReply` — email when someone replies on a thread the user is subscribed to.
- `emailOnMention` — email when the user is `@mentioned`.
- In-app inbox notifications are unaffected by these toggles; they govern the email channel only.

### `GET /api/notifications`

Auth required.

**Query params:**
- `unread` (optional) — set to `1` to return only unread.

**Response (200):**
```json
{
  "notifications": [
    {
      "id": "notif_...",
      "type": "comment_reply",
      "readAt": null,
      "createdAt": "2026-05-18T19:30:00Z",
      "actor": {"id": "usr_...", "name": "Alex Rivera", "username": "alex"},
      "guestName": null,
      "comment": {
        "id": "cm_reply",
        "body": "Agreed — moving it now",
        "pagePath": "/",
        "siteId": "site_...",
        "siteName": "Pricing Page",
        "siteSlug": "pricing-page",
        "orgSlug": "demo",
        "parentId": "cm_root",
        "resolvedAt": null
      }
    }
  ],
  "unreadCount": 1
}
```

**Notification types:**
- `comment_reply` — fired for thread subscribers and site-level
  subscribers when any comment is posted.
- `comment_mention` — fired for users named in `@mentions`; takes
  precedence over `comment_reply` for the same recipient.

For older guest-authored comments (posted before sign-in-required),
`actor` is null and `guestName` carries the display name. Capped at
100 most recent.

### `POST /api/notifications/{notificationID}/read`

Auth required. Sets `read_at` on a notification belonging to the caller.

**Status codes:** `200` ok, `404` not found or not yours.

### `POST /api/notifications/read-all`

Auth required. Marks every unread notification for the caller as read.

**Response (200):** `{ "ok": true }`.

---

## Uploads (Authenticated Deploy)

### `POST /api/uploads`

Auth required. Multipart form data.

Creating a new site sets `created_by` to the caller. Re-uploading to an
existing site (matched by name within the org) is gated: only the original
creator or an org admin can replace its contents. Other org members get
`403` with a hint to duplicate instead (see
[Site ownership](#site-ownership)).

**Query params:** `org` (optional, org slug)

**Form fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Site name (becomes URL slug) |
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
| `is_public` | string | no | `"true"` or `"false"`. Overrides the instance `defaultSitePrivate` setting on first deploy of a new site (ignored for re-uploads to an existing site, which preserve its current visibility). |

**Response (202):**
```json
{
  "message": "Upload staged and recorded.",
  "status": "queued",
  "site": {
    "id": "site_...",
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

### `DELETE /api/admin/users/{userID}`

Deletes an account. The user's personal organizations are removed along
with it; the action is recorded in the audit log.

**Response (200):** `{ "ok": true }`

**Status codes:** `200` ok, `400` missing user ID, `403` caller not
admin, `404` user not found.

### `GET /api/admin/settings`

Returns instance-wide settings: the deploy-thumbnail capture toggle, the email-provider configuration, and the site-visibility policy.

**Response (200):**
```json
{
  "thumbnails": {
    "available": true,
    "enabled": true,
    "reason": ""
  },
  "email": {
    "provider": "smtp",
    "from": "Protopen <noreply@example.com>",
    "resendKeySet": false,
    "smtpHost": "smtp.example.com",
    "smtpPort": "587",
    "smtpUser": "apikey",
    "smtpPassSet": true,
    "smtpTLS": false,
    "inboundDomain": "reply.example.com",
    "inboundSecretSet": true
  },
  "visibility": {
    "defaultSitePrivate": false,
    "autoPrivateEnabled": false,
    "autoPrivateAfterDays": 30,
    "eligibleForRevertCount": 0
  }
}
```

- `thumbnails.available` — whether this server has the capability (env `THUMBNAILS_ENABLED` set and a Chromium binary resolved).
- `thumbnails.enabled` — current runtime state from `instance_settings.thumbnails_enabled`.
- `thumbnails.reason` — populated only when `available` is `false`, explaining why.
- `email.provider` — `none`, `resend`, or `smtp`.
- `email.resendKeySet` / `email.smtpPassSet` / `email.inboundSecretSet` — whether a secret is stored. **Secret values are never returned.**
- `email.inboundDomain` — the reply-by-email domain; reply-from-email is active when both `inboundDomain` and the inbound secret are set.
- `visibility.defaultSitePrivate` — when `true`, new sites are created private unless the upload request explicitly sets `isPublic`.
- `visibility.autoPrivateEnabled` — when `true`, a background sweep (every 15 min) flips public sites back to private once their `made_public_at` timestamp is older than `autoPrivateAfterDays`. Each reverted site is recorded in `audit_log` with `action = "auto_private_revert"`, `actor_user_id = NULL`, `actor_email = "system:auto-private"`. The sweep is capped at 500 sites per tick.
- `visibility.autoPrivateAfterDays` — integer, 1–3650.
- `visibility.eligibleForRevertCount` — a live count of public sites currently older than `autoPrivateAfterDays`; useful as a preview before enabling auto-revert.

### `PATCH /api/admin/settings`

Update one or more settings.

**Request:**
```json
{
  "thumbnailsEnabled": true,
  "email": {
    "provider": "smtp",
    "from": "Protopen <noreply@example.com>",
    "smtpHost": "smtp.example.com",
    "smtpPort": "587",
    "smtpUser": "apikey",
    "smtpPass": "secret",
    "smtpTLS": false,
    "inboundDomain": "reply.example.com",
    "inboundSecret": "webhook-signing-secret"
  },
  "visibility": {
    "defaultSitePrivate": true,
    "autoPrivateEnabled": true,
    "autoPrivateAfterDays": 30
  }
}
```

**Response (200):** same shape as `GET /api/admin/settings`.

- All keys are optional; only supplied fields change.
- `email.resendKey` / `email.smtpPass` / `email.inboundSecret` are write-only: omit them or send `""` to keep the stored secret; send a non-empty value to replace it.
- Saving email settings rebuilds the mailer immediately — no restart.
- Returns **400** if `visibility.autoPrivateAfterDays` is outside the 1–3650 range.
- Returns **409** if the request asks to enable a feature whose `available` is `false`, with `reason` in the error body.
- Returns **403** if the caller is not admin.
- Thumbnail toggling takes effect immediately: enabling spawns the Chromium allocator and kicks the backstop loop to capture any older deploys that were missed; disabling cancels the allocator and frees memory.
- Visibility changes take effect on the next deploy (default) and the next 15-min cleanup tick (sweeper).

### `POST /api/admin/settings/email-test`

Sends a one-off test message through the currently configured mailer so an admin can confirm the provider works.

**Request:**
```json
{"to": "you@example.com"}
```

**Response (200):** `{"ok": true}`

- Returns **400** if `to` is missing.
- Returns **409** if no email provider is configured.
- Returns **502** with the transport error in `error` if the send fails.
- Returns **403** if the caller is not admin.

### `POST /api/email/inbound`

Webhook for reply-by-email. An email provider with inbound parsing (Postmark,
Mailgun, SendGrid Inbound Parse, Cloudflare Email Workers, etc.) forwards a
parsed reply here; the body is posted onto the thread the reply token points at.

No session auth — gated instead by the webhook signature, the reply token, and
a From-address match. Exempt from the 1 MiB JSON body cap (own 10 MiB limit).

**Headers:**
- `X-Protopen-Signature` — hex HMAC-SHA256 of the raw request body, keyed with
  the configured inbound secret.

**Request:**
```json
{
  "to": "reply+<token>@reply.example.com",
  "from": "Jane Chen <jane@example.com>",
  "text": "the full reply including quoted history",
  "strippedText": "just the new text (optional; used when present)"
}
```

**Response (200):** `{"ok": true}` — also returned (as a no-op) when the reply
body is empty after quote-stripping, so the provider doesn't retry.

- **404** — inbound email is not configured on this instance.
- **403** — bad signature, unknown/expired reply token, or the `from` address
  doesn't match the token's user.
- **410** — the thread the token points at no longer exists.

---

## Site ownership

Every site row carries a `created_by` foreign key onto `users`. The semantics
are intentionally narrow:

- **Mutations gated on creator-or-admin.** `DELETE /api/sites/:id`,
  `PATCH /api/sites/:id`, `POST /api/sites/:id/rollback`, and re-uploads to
  an existing site name via `POST /api/uploads` succeed only when the caller
  is the original creator or holds the `admin` role in the owning org.
  Other org members get `403`.
- **Reads are org-scoped, not creator-scoped.** `GET /api/sites` returns the
  whole org's sites by default; pass `?author=me` to scope down to the
  caller's own work.
- **Duplication is open to all members.** `POST /api/sites/:id/duplicate`
  only requires org membership, so teammates can always clone a site they
  can't modify directly.
- **Deleted users orphan their sites.** The FK uses
  `on delete set null`, so removing a user leaves their sites with
  `created_by = null`. Orphaned sites continue to live under the org and
  fall into the admin-only mutation bucket — no cascade-deletes of work
  that other teammates may still rely on.
- **API tokens authenticate as the issuing user.** Deploys made with a CI
  token are attributed to whoever provisioned the token, not the human
  triggering the build. This matches today's behavior; no special case in
  the ownership rules.
- **Legacy rows can be `null`.** Sites that pre-date migration `018` may
  have `created_by = null` if the owning org has more than one member
  (the backfill is conservative and only fills in unambiguous single-member
  personal orgs). Clients should render `createdBy: null` as an unknown
  author. Such sites are admin-only for mutations by construction.

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
| Comment body max | 8,000 characters |
| Mention `@username` pattern | `[a-zA-Z0-9_-]{2,32}` |
| Element selector max | 200 characters |
| Comment POST rate limit | 20/minute per IP |
| Auto-private sweep cadence | every 15 minutes |
| Auto-private sweep batch cap | 500 sites per tick |
| `autoPrivateAfterDays` range | 1–3650 days |

## Miscellaneous

### `GET /healthz`

No auth. Returns `{"status": "ok"}`.
