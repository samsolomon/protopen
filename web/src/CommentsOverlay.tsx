type CommentsOverlayProps = {
  orgSlug: string
  siteSlug: string
  siteName: string
  focusCommentID?: string | null
  pagePath?: string | null
}

// Default to 'localhost' (not 127.0.0.1) — the dashboard and the session
// cookie are scoped to 'localhost', and browsers treat 127.0.0.1 as a
// different host, so an iframe to 127.0.0.1 ships without the cookie.
const CONTENT_BASE_URL =
  import.meta.env.VITE_CONTENT_BASE_URL ?? 'http://localhost:8081'

// Dashboard wrapper for the deployed-site comment runtime. The runtime is
// injected by the API into every HTML response and renders the entire
// comment chrome — including the topbar with brand link back to the
// dashboard — inside the iframe. Browser back returns to the inbox.
export function CommentsOverlay({ orgSlug, siteSlug, siteName, focusCommentID, pagePath }: CommentsOverlayProps) {
  const siteRoot = `${CONTENT_BASE_URL}/~${orgSlug}/${siteSlug}`
  const cleanPath = pagePath && pagePath.startsWith('/') ? pagePath : '/'
  const iframeSrc = focusCommentID
    ? `${siteRoot}${cleanPath}#protopen-comment=${encodeURIComponent(focusCommentID)}`
    : `${siteRoot}${cleanPath}`

  return (
    <iframe
      src={iframeSrc}
      title={`${siteName} preview`}
      className="h-svh w-full border-0"
    />
  )
}
