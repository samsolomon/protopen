export const API_BASE_URL = import.meta.env.VITE_API_URL || ''
// Default to 'localhost' (not 127.0.0.1) — the dashboard session cookie is
// scoped to localhost; browsers treat 127.0.0.1 as a different host and
// won't ship the cookie cross-origin.
export const CONTENT_BASE_URL =
  import.meta.env.VITE_CONTENT_BASE_URL ?? 'http://localhost:8081'
export const MAX_DEPLOY_BYTES = 100 * 1024 * 1024
export const MAX_FILE_BYTES = 20 * 1024 * 1024

// pagePath stored by the runtime is the full `/~org/site/...` location.pathname.
// Fall back to the site root if it's missing or malformed.
export function siteCommentURL(orgSlug: string, siteSlug: string, pagePath: string | null | undefined, commentID?: string | null) {
  const prefix = `/~${orgSlug}/${siteSlug}`
  const path = pagePath && pagePath.startsWith(prefix) ? pagePath : `${prefix}/`
  const hash = commentID ? `#protopen-comment=${encodeURIComponent(commentID)}` : ''
  return `${CONTENT_BASE_URL}${path}${hash}`
}
