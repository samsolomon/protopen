# Protopen Build Plan

This document turns the v1 product plan into an implementation checklist that can drive the build toward something testable.

## v1 Goal

Build a testable Protopen MVP that lets a signed-in user upload a folder or zip of prebuilt static assets, receive a stable public URL, and redeploy the same project by name.

## Definition of Testable

The MVP is testable when all of the following are true:

- a user can create an account and sign in locally
- a user can upload either a folder or a zip from the dashboard
- the system validates upload shape and size limits
- a successful upload creates or updates a project
- each project has a stable URL
- the latest successful deploy is served at that URL
- hosted prototype content is served from a separate untrusted content origin
- a second upload with the same exact project name updates the existing project
- a second upload with a different name creates a new project

## First End-to-End Demo Target

The first truly useful internal demo should support this script:

1. create an account
2. sign in to the dashboard
3. upload a valid static folder
4. receive a stable public URL
5. open that URL in a separate browser session without signing in
6. redeploy a changed version of the same project name
7. refresh the public URL and see the new version

This demo script works locally today.

## Completed Milestones

These milestones are done or functionally complete. They are kept here as reference, not as active work.

### Milestone 0 - Local Vertical Slice (done)

- [x] Create `web/` app scaffold
- [x] Create `api/` app scaffold
- [x] Add local quick-start instructions
- [x] Add basic dashboard UI
- [x] Add mock projects API
- [x] Add mock upload endpoint

### Milestone 7 - Simple Account System (done)

Built early because most other milestones depend on auth.

- [x] Choose auth implementation (email/password + HTTP-only session cookie)
- [x] Implement account creation
- [x] Implement sign-in
- [x] Implement sign-out
- [x] Protect dashboard and management APIs behind auth
- [x] Keep project viewing public by link
- [x] Associate projects with the signed-in user

Password reset is deferred — not needed for local testing. Revisit before broader access.

### Milestone 2 - Persistence and Project Model (done)

- [x] Stand up PostgreSQL locally via docker-compose
- [x] Add migration tooling (auto-run on API startup)
- [x] Create `users`, `projects`, and `deploys` tables
- [x] Add `current_deploy_id` on projects
- [x] Add `status`, `size_bytes`, `file_count` on deploys
- [x] Replace in-memory store with database-backed queries
- [x] Implement exact-name match lookup per user
- [x] Implement create-vs-update logic in the API layer

Soft delete: the `deleted_at` column exists on projects and the API supports DELETE, but there is no UI to restore deleted projects. Good enough for v1.

### Milestone 1 - Real Upload Contract (done)

- [x] Accept folder selection in the web app
- [x] Accept zip selection in the web app
- [x] Support drag-and-drop upload interaction
- [x] Send real multipart file uploads to the API
- [x] Validate `index.html` requirement for folder uploads
- [x] Validate `100MB` max deploy size
- [x] Validate `20MB` max individual file size
- [x] Reject invalid zip upload shapes
- [x] Basic upload states in UI (idle, dragging, uploading, success) and error banners

Remaining polish (structured server error display, richer status messaging) is tracked under dashboard polish below.

### Milestone 3 - Real File Ingest (done)

- [x] Support multipart upload endpoint for zip and folder uploads
- [x] Normalize uploaded file paths
- [x] Store uploaded files in a temporary ingest area
- [x] Extract zip uploads server-side
- [x] Guard against zip bombs and excessive file counts
- [x] Reject symlinks and unsupported archive entries
- [x] Re-run validation against extracted content
- [x] Detect a usable site root containing `index.html`
- [x] Path traversal protection via `normalizeUploadPath()` (rejects `../` and absolute paths)

### Milestone 5 - Serving and URL Resolution (done)

- [x] Implement route resolution for `~username/project-slug`
- [x] Resolve project to current deploy via `current_deploy_id`
- [x] Serve `index.html` for the project root
- [x] Serve static asset files from the current deploy
- [x] Support SPA fallback rules for unmatched routes
- [x] Return 404 for missing assets
- [x] Add cache headers (no-store for HTML, 5-minute public cache for assets)

### Milestone 6 - Origin Isolation (local done)

- [x] Choose local-dev origin model (app on :8080, content on :8081)
- [x] Serve uploaded prototype content from separate untrusted content origin
- [x] Add baseline security headers for hosted content (CORP, X-Content-Type-Options, Referrer-Policy)
- [x] Uploaded HTML runs on a different origin than the dashboard

Production origin model and stronger isolation are deferred to pre-launch hardening.

## Current Status

Protopen is at a real local testable checkpoint. The end-to-end demo script works:

- sign in with the demo account
- upload a static folder
- receive a live URL on the content origin
- open the hosted prototype without signing in
- redeploy the same project name and keep the same URL
- delete a project from the dashboard

## What to Build Next

These are ordered by priority. Each item is scoped to be completable independently.

### 1. Code health (done)

- [x] Split `api/main.go` into domain files: auth.go, handlers.go, httputil.go, upload.go, storage.go, serve.go, seed.go, migrate/
- [x] Split `web/src/App.tsx` into components: AuthPage, Dashboard, UploadPanel, ProjectCard + types, constants, api, upload-helpers
- [x] Remove dead "View deploy guide" button
- [x] Gate the "Simulate upload" button behind `import.meta.env.DEV`
- [x] Migrate frontend to shadcn/ui + Tailwind CSS

### 2. Zip smoke test (done)

- [x] Add end-to-end smoke test for zip upload flow (`scripts/zip-smoke-test.sh`)
- [x] Verify zip and folder uploads produce the same served result

### 3. Dashboard polish

This is what testers will notice first.

- [ ] Show project empty state (exists but could be stronger)
- [ ] Sort projects recent-first
- [ ] Add deploy status badges (deploy count is shown, status is not)
- [ ] Add upload progress indicator and final success/failure messaging
- [ ] Show structured server validation errors instead of generic error banner
- [ ] Improve error recovery (retry button, clearer failure reasons)

### 4. Frontend tests

No frontend tests exist today. Start with the highest-value coverage.

- [ ] Add test setup (Vitest + testing-library)
- [ ] Test auth flow: sign-in form, sign-up form, session restore, sign-out
- [ ] Test upload validation: file size limits, missing index.html, empty selection
- [ ] Test project list rendering and delete action

### 5. Ingest cleanup

Failed or abandoned uploads leave directories in `.data/ingest/` forever.

- [ ] Clean up temp ingest directories after successful deploy promotion
- [ ] Add startup or periodic cleanup for orphaned ingest directories
- [ ] Add expired session cleanup (sessions table grows indefinitely)

### 6. Documentation

- [ ] Document how root-relative asset paths are handled
- [ ] Document that service workers are not supported in v1
- [ ] Document allowed and blocked file types for v1

### 7. Visibility defaults

Visibility is per-site today with no instance-wide or per-user default. Self-hosters who want every prototype gated behind a login have to flip each site individually.

- [ ] Add instance-wide default visibility via env var (e.g. `DEFAULT_VISIBILITY=private`)
- [ ] Apply default on project creation when the CLI/upload does not specify visibility
- [ ] Optional: per-user default in dashboard settings, overriding the instance default
- [ ] Document the precedence (per-upload flag > per-user default > instance default > public)

## Deferred to Pre-Launch Hardening

These items are not needed for local testing but must be done before broader access.

### Production Storage

The current filesystem-based storage under `.data/ingest/` works for local dev. Each deploy gets its own `storage_prefix` path and `current_deploy_id` only updates on success, so the deploy model is already effectively immutable. Before production:

- [ ] Define a proper storage key structure for deploy prefixes
- [ ] Add object storage backend (S3 or equivalent)
- [ ] Ensure deleted files from older deploys do not leak into newer deploys

### Production Origin Isolation

- [ ] Choose production origin model for app vs content
- [ ] Serve dashboard from trusted app origin with proper cookie scoping
- [ ] Confirm session cookies do not flow to the content origin
- [ ] Explicitly disallow service workers in v1

### Security and Abuse Prevention

- [ ] Add upload rate limiting
- [ ] Add per-user storage quota
- [ ] Add per-user project cap
- [ ] Add server-side request logging for upload failures
- [ ] Reject or clearly fail unsupported backend assumptions

### Observability

- [ ] Add structured API logs
- [ ] Add request IDs
- [ ] Log deploy lifecycle state changes
- [ ] Log validation failures with reason codes
- [ ] Log serving resolution failures

### Auth Improvements

- [ ] Add password reset or account recovery flow

## Testing Checklist

- [x] API tests for upload validation rules
- [x] API tests for exact-name redeploy behavior
- [x] End-to-end smoke test for folder upload and stable URL redeploy
- [x] End-to-end smoke test for zip upload flow
- [ ] Frontend tests for auth flow
- [ ] Frontend tests for upload validation UI
