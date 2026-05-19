import { siteCommentURL } from './constants'

type CommentsOverlayProps = {
  orgSlug: string
  siteSlug: string
  siteName: string
  focusCommentID?: string | null
  pagePath?: string | null
}

// Dashboard wrapper for the deployed-site comment runtime. The runtime is
// injected by the API into every HTML response and renders the entire
// comment chrome — including the topbar with brand link back to the
// dashboard — inside the iframe. Browser back returns to the inbox.
export function CommentsOverlay({ orgSlug, siteSlug, siteName, focusCommentID, pagePath }: CommentsOverlayProps) {
  const iframeSrc = siteCommentURL(orgSlug, siteSlug, pagePath, focusCommentID)

  return (
    <iframe
      src={iframeSrc}
      title={`${siteName} preview`}
      className="h-svh w-full border-0"
    />
  )
}
