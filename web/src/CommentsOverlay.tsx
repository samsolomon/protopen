import { Button } from '@/components/ui/button'
import { ArrowLeft } from 'lucide-react'

type CommentsOverlayProps = {
  orgSlug: string
  siteSlug: string
  siteName: string
  focusCommentID?: string | null
  onBack: () => void
}

const CONTENT_BASE_URL =
  import.meta.env.VITE_CONTENT_BASE_URL ?? 'http://127.0.0.1:8081'

// Dashboard wrapper for the deployed-site comment runtime. The runtime is
// injected by the API into every HTML response and renders the entire
// comment chrome (pins, sidebar, composer, mention autocomplete) inside
// the iframe. This component contributes only the back button and the
// optional deep-link to a specific thread.
export function CommentsOverlay({ orgSlug, siteSlug, siteName, focusCommentID, onBack }: CommentsOverlayProps) {
  const siteRoot = `${CONTENT_BASE_URL}/~${orgSlug}/${siteSlug}`
  const iframeSrc = focusCommentID
    ? `${siteRoot}/#protopen-comment=${encodeURIComponent(focusCommentID)}`
    : `${siteRoot}/`

  return (
    <div className="flex h-svh flex-col bg-background">
      <header className="flex h-12 items-center justify-between border-b px-4">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft />
            Back
          </Button>
          <span className="text-sm font-medium">{siteName}</span>
        </div>
      </header>
      <iframe
        src={iframeSrc}
        title={`${siteName} preview`}
        className="size-full border-0"
      />
    </div>
  )
}
