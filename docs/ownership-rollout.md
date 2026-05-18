# Site ownership rollout — handoff

Tracks the in-flight feature work that adds per-site ownership and a
team-aware dashboard. Delete this file once PR3 lands.

## What shipped

### PR1 — Backend ownership (merged, on `main`)

- Migration `018_sites_created_by.sql`: nullable
  `sites.created_by → users(id) ON DELETE SET NULL`, safe backfill for
  personal-org rows only, partial index on `(org_id, created_by)` where
  `deleted_at IS NULL`. Team-org rows pre-migration stay `NULL`
  (orphan), which the UI handles.
- Write path: `created_by` set on first upload; re-upload to an existing
  slug preserves the original creator.
- `requireSiteMutate(orgID, role, createdBy)` enforces creator-or-admin
  on `DELETE`, `PATCH` visibility, and the existing-site branch of
  upload. Non-creator members get `403` with a duplicate suggestion.
- Read path: `listSites` joins `users` and embeds `createdBy:
  {id,name,username} | null` on every site. New optional
  `?author=me` filter on `GET /api/sites`.
- `POST /api/sites/:id/duplicate`: creates a sibling site with
  `created_by = caller`, `slug = uniqueSlug(orgID, originalSlug)`,
  `name = "Copy of …"`, sharing the original's latest deploy storage
  prefix (no file copy).
- Test harness: in-process Postgres-backed tests cover the
  creator/admin/member matrix for delete, visibility, re-upload, and
  duplicate; list filter and createdBy embedding; orphan handling.
- `docs/api-reference.md`: documents the new field, query param,
  endpoint, and ownership rules (including the API-token caveat —
  tokens act as the owning user so they inherit that user's rights).

### PR2 — Frontend dashboard (merged, on `main`)

- `Site.createdBy` typed nullable; `duplicateSite(siteId)` in
  `web/src/api.ts`. `fetchSites()` unchanged — one fetch, client-side
  filter.
- `My sites` / `All sites` scope tabs above the grid/list toggle.
  Default is `My sites`. URL state via
  `?view=mine|all` with `history.replaceState` (no full reload). Tabs
  are hidden when there are no teammate-authored sites visible
  (single-user / personal-org case).
- Author chip (initials avatar + name) renders in `All sites` only —
  inline in the grid card metadata row and as an `Author` column in
  the list view, between `Branch` and `Updated`. Null creator renders
  as `—`.
- Per-row `canMutate = isCreator || isOrgAdmin`. When false,
  `Make public/private` and `Delete` are hidden from the menu (grid)
  and the icon buttons are hidden (list). `Duplicate` is always shown
  to org members. Duplicate success toasts, refetches, and pops the
  view back to `My sites` so the user lands on the new copy.
- Shared search input next to the tabs filters the active scope
  (case-insensitive, name + slug). No-match state renders a distinct
  message instead of the empty-scope state. Sort was intentionally
  deferred (see "Deferred decisions").

## What's still open

### PR2 — Frontend tests (NOT_STARTED)

Goal: lock in the client behaviour now that we've shipped the UI.

The current `web/src/api.test.ts` already covers `duplicateSite` (201
path, 401 → `SessionExpiredError`, server error, missing-site
response). What's still needed:

1. **Pure scope/filter helpers.** The natural seam is to extract three
   pure functions from `Dashboard.tsx` into a new `web/src/site-scope.ts`:
   - `scopeFromSearch(search: string): 'mine' | 'all'` — already inline.
   - `buildScopeURL(currentHref, scope)` — replaces the side-effectful
     `writeScopeToURL` so it's testable without `window.history`.
   - `filterSitesByScope(sites, scope, currentUserId)` and
     `filterSitesByQuery(sites, query)` — currently inline expressions.
   - `canMutateSite(site, currentUserId, isOrgAdmin)` mirrors the
     server `requireSiteMutate` rule.

   Then `web/src/site-scope.test.ts` exercises each helper. No React
   render harness needed — the project has no component tests yet and
   adding one for this is out of scope.

2. **Wire `Dashboard.tsx` to the new helpers** so the inline logic and
   the tested logic do not drift. Keep the public component API
   identical.

3. Run `cd web && npx tsc --noEmit && npm test -- --run` and verify the
   dashboard still behaves correctly via Chrome DevTools MCP (load the
   page, flip tabs, search, duplicate, confirm the `View all sites`
   button on the empty-mine state).

### PR3 — Seed verification + docs polish (NOT_STARTED)

1. **Verify the demo seed** creates a shared team org containing the
   non-demo seed users so the `All sites` tab actually shows
   teammate-authored rows in a fresh `SEED_DEMO=1` boot. Right now I
   only confirmed it works by manually inserting two teammate-authored
   sites at runtime. The seed in `api/seed.go` (or wherever the demo
   data lives — search for `SEED_DEMO`) may already do this; if not,
   add a shared org membership and a couple of teammate sites.

2. **Docs polish:** the `Site Ownership` section in
   `docs/api-reference.md` is already in place from PR1. Re-read it
   once the frontend is final and add any cross-references that became
   relevant (scope tab URL params, duplicate semantics in the UI).

## Deferred decisions (don't redo without intent)

- **Sort UI** — skipped because the sites API only returns
  `updatedAt` as a pre-formatted relative-time string. A real sort
  needs a sortable timestamp (e.g. `updatedAtISO`) on the response
  plus an optional server-side `?sort=` param. The implicit
  "newest-updated first" order from the server is the default and is
  fine for now.
- **Optimistic creator on upload** — not needed. `UploadPanel`
  triggers a full `fetchSites()` refetch via `onProjectsChanged()` →
  `loadSites()` in `App.tsx`; the refetched site already carries
  `createdBy` from the server, so there's no pre-server placeholder
  to attribute.
- **Server-side filter for `?author=me`** — the param exists and is
  tested, but the frontend uses one unfiltered fetch and filters
  client-side. Keep it that way until site lists get long enough to
  matter.

## Quick reference

- Schema for the new field: `sites.created_by uuid NULL` →
  `users(id) ON DELETE SET NULL`.
- Server rule: `original creator OR admin of the site's org`.
- API: `GET /api/sites?author=me`, `POST /api/sites/:id/duplicate`,
  `createdBy: {id,name,username} | null` on every site response.
- URL state: `?view=all` for `All sites`; absent / anything else =
  `My sites`.
- Commits: PR1 = `11f0570..3100b2b`, PR2 = `454f8db..af6e173`.
