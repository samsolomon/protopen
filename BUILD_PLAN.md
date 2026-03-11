# Velori Build Plan

This document turns the v1 product plan into an implementation checklist that can drive the build toward something testable.

## v1 Goal

Build a testable Velori MVP that lets a signed-in user upload a folder or zip of prebuilt static assets, receive a stable public URL, and redeploy the same project by name.

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

## Milestone Order

### Milestone 0 - Local Vertical Slice

Purpose: get the repo into a working local-dev state with a visible UI and a reachable API.

- [x] Create `web/` app scaffold
- [x] Create `api/` app scaffold
- [x] Add local quick-start instructions
- [x] Add basic dashboard UI
- [x] Add mock projects API
- [x] Add mock upload endpoint

Exit criteria:

- [x] `npm run build` passes in `web/`
- [x] `go build .` passes in `api/`

### Milestone 1 - Real Upload Contract

Purpose: make the upload flow behave like the real product, even before storage is implemented.

- [x] Accept folder selection in the web app
- [x] Accept zip selection in the web app
- [x] Support drag-and-drop upload interaction
- [x] Send file metadata to the API
- [x] Validate `index.html` requirement for folder uploads
- [x] Validate `100MB` max deploy size
- [x] Validate `20MB` max individual file size
- [x] Reject invalid zip upload shapes
- [ ] Display server validation errors as structured UI states instead of inline text only
- [ ] Add loading, success, and failure states for the full upload card experience

Exit criteria:

- [ ] Invalid uploads fail with explicit user-facing reasons
- [ ] Valid uploads create or update a project through the API contract

### Milestone 2 - Persistence and Project Model

Purpose: replace the in-memory API with a durable project and deploy model.

- [ ] Stand up PostgreSQL locally or via Supabase dev instance
- [x] Add migration tooling
- [x] Create `users` table
- [x] Create `projects` table
- [x] Create `deploys` table
- [x] Add `current_deploy_id` on projects
- [x] Add `status` on deploys
- [x] Add `size_bytes` and `file_count` on deploys
- [ ] Add soft delete support for projects
- [x] Replace in-memory store with database-backed queries
- [x] Implement exact-name match lookup per user
- [x] Implement create-vs-update logic in the API layer

Exit criteria:

- [ ] Restarting the API does not lose projects or deploy history
- [x] Uploading the same project name increments deploy count on the same project
- [x] Uploading a different project name creates a new project row

### Milestone 3 - Real File Ingest

Purpose: move from metadata-only uploads to real file transfer and validation.

- [x] Support multipart upload endpoint for zip uploads
- [x] Support multipart upload endpoint for folder uploads
- [x] Normalize uploaded file paths
- [x] Store uploaded files in a temporary ingest area
- [x] Extract zip uploads server-side
- [ ] Guard against path traversal in archive extraction
- [x] Guard against zip bombs and excessive file counts
- [x] Reject symlinks and unsupported archive entries
- [x] Re-run validation against extracted content
- [x] Detect a usable site root containing `index.html`

Exit criteria:

- [x] The API receives actual files, not just metadata
- [ ] Invalid archives are rejected safely
- [ ] Valid folder and zip uploads produce the same internal deploy representation

### Milestone 4 - Immutable Deploy Storage

Purpose: make each deploy durable and safe to serve.

- [ ] Define storage key structure for immutable deploy prefixes
- [ ] Add local object storage strategy for development
- [ ] Upload validated deploy contents into a unique deploy prefix
- [ ] Store deploy metadata after successful write
- [ ] Update `projects.current_deploy_id` only after deploy success
- [ ] Preserve previous deploys internally
- [ ] Add cleanup for failed temp uploads

Exit criteria:

- [ ] Each successful deploy has its own immutable storage location
- [ ] A failed deploy never replaces the current live deploy

### Milestone 5 - Serving and URL Resolution

Purpose: serve the latest successful deploy at a stable public URL.

- [x] Implement route resolution for `~username/project-slug`
- [ ] Resolve project to current deploy
- [x] Serve `index.html` for the project root
- [x] Serve static asset files from the current deploy
- [x] Support SPA fallback rules for unmatched routes
- [x] Return 404 for missing assets
- [ ] Ensure deleted files from older deploys do not leak into newer deploys
- [x] Add cache headers appropriate for HTML vs static assets

Exit criteria:

- [x] A shared project URL renders the latest successful deploy
- [ ] A redeploy changes the served content without changing the URL

### Milestone 6 - Origin Isolation

Purpose: safely separate trusted app surfaces from untrusted uploaded code.

- [x] Choose local-dev origin model for app vs content
- [ ] Choose production origin model for app vs content
- [ ] Serve dashboard/account UI from trusted app origin
- [x] Serve uploaded prototype content from separate untrusted content origin
- [ ] Confirm cookies or sessions do not flow to the content origin unnecessarily
- [x] Add baseline security headers for hosted content
- [ ] Explicitly disallow service workers in v1

Exit criteria:

- [x] Uploaded HTML runs on a different origin than the dashboard
- [ ] A prototype cannot access trusted app session context by origin

### Milestone 7 - Simple Account System

Purpose: require sign-in for upload/manage while keeping viewing public.

- [ ] Choose auth implementation for the simple account system
- [ ] Implement account creation
- [ ] Implement sign-in
- [ ] Implement sign-out
- [ ] Protect dashboard and management APIs behind auth
- [ ] Keep project viewing public by link
- [ ] Associate projects with the signed-in user
- [ ] Add basic password reset or account recovery path if needed by chosen auth approach

Exit criteria:

- [ ] Anonymous users cannot access the dashboard
- [ ] Anonymous users can still view live project URLs
- [ ] Each user only sees and manages their own projects

### Milestone 8 - Dashboard Polish for Testers

Purpose: make the MVP usable enough for real tester feedback.

- [ ] Show project empty state
- [ ] Show recent-first sorting
- [ ] Add copy URL action
- [ ] Add delete project action
- [ ] Add deploy status badges
- [ ] Add upload progress and final status messaging
- [ ] Add simple deploy guide copy for supported uploads
- [ ] Improve error states for broken uploads

Exit criteria:

- [ ] A non-technical tester can sign in, upload, copy a link, redeploy, and delete a project without help

## Cross-Cutting Checklists

### Supported Site Contract

- [ ] Require a usable `index.html`
- [ ] Support prebuilt static assets only
- [ ] Reject or clearly fail unsupported backend assumptions
- [ ] Document how root-relative asset paths are handled
- [ ] Document SPA fallback behavior
- [ ] Document that service workers are not supported in v1

### Security and Abuse

- [ ] Separate trusted app origin from untrusted content origin
- [ ] Add upload rate limiting
- [ ] Add per-user storage quota
- [ ] Add per-user project cap
- [ ] Add server-side request logging for upload failures
- [ ] Add archive safety checks
- [ ] Define allowed and blocked file behaviors for v1

### Observability

- [ ] Add structured API logs
- [ ] Add request IDs
- [ ] Log deploy lifecycle state changes
- [ ] Log validation failures with reason codes
- [ ] Log serving resolution failures

### Testing

- [ ] Add API tests for upload validation rules
- [ ] Add API tests for exact-name redeploy behavior
- [ ] Add frontend tests for upload validation UI
- [ ] Add end-to-end test for folder upload flow
- [ ] Add end-to-end test for zip upload flow
- [ ] Add end-to-end test for stable URL redeploy behavior

## First End-to-End Demo Target

The first truly useful internal demo should support this script:

1. create an account
2. sign in to the dashboard
3. upload a valid static folder
4. receive a stable public URL
5. open that URL in a separate browser session without signing in
6. redeploy a changed version of the same project name
7. refresh the public URL and see the new version

## Immediate Next Build Steps

These are the next items to build from the current repo state:

- [x] replace the in-memory project store with a real database-backed project model
- [ ] implement real multipart upload handling in the API
- [ ] choose and document the local app-origin vs content-origin setup
- [x] persist deploy metadata and introduce deploy lifecycle states
- [ ] serve stored deploys at stable project URLs
