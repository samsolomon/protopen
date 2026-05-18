import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CommentStatusFilter } from './api'
import type { SiteComment } from './types'
import {
  deleteComment,
  fetchSiteComments,
  postSiteComment,
  SessionExpiredError,
  toggleResolveComment,
} from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { ArrowLeft, Check, RotateCcw, Trash2 } from 'lucide-react'
import { toast } from 'sonner'

type Mode = 'comment' | 'browse'

type CommentsOverlayProps = {
  siteID: string
  orgSlug: string
  siteSlug: string
  siteName: string
  currentUserID: string
  isOrgAdmin: boolean
  focusCommentID?: string | null
  onBack: () => void
  onSessionExpired: () => void
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function initials(name: string | undefined): string {
  if (!name) return '?'
  return name.split(/\s+/).map((p) => p[0] ?? '').slice(0, 2).join('').toUpperCase()
}

const CONTENT_BASE_URL =
  import.meta.env.VITE_CONTENT_BASE_URL ?? 'http://127.0.0.1:8081'

// Translate an iframe-reported pathname (which carries the /~orgSlug/siteSlug
// or /_v/<deployId> prefix) into a site-relative path so comments persist
// regardless of how the site is served.
function siteRelativePath(serverPath: string, orgSlug: string, siteSlug: string): string {
  const sitePrefix = `/~${orgSlug}/${siteSlug}`
  if (!serverPath.startsWith(sitePrefix)) return serverPath || '/'
  let rest = serverPath.slice(sitePrefix.length)
  const versionMatch = rest.match(/^\/_v\/[^/]+(\/.*)?$/)
  if (versionMatch) rest = versionMatch[1] ?? '/'
  return rest === '' ? '/' : rest
}

export function CommentsOverlay({
  siteID,
  orgSlug,
  siteSlug,
  siteName,
  currentUserID,
  isOrgAdmin,
  focusCommentID,
  onBack,
  onSessionExpired,
}: CommentsOverlayProps) {
  const [comments, setComments] = useState<SiteComment[]>([])
  const [filter, setFilter] = useState<CommentStatusFilter>('open')
  const [mode, setMode] = useState<Mode>('comment')
  const [currentPath, setCurrentPath] = useState('/')
  const [pendingPin, setPendingPin] = useState<{ x: number; y: number; pagePath: string } | null>(null)
  const [draftBody, setDraftBody] = useState('')
  const [replyDraft, setReplyDraft] = useState<{ rootId: string; body: string } | null>(null)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const siteRoot = `${CONTENT_BASE_URL}/~${orgSlug}/${siteSlug}`
  const iframeSrc = `${siteRoot}/?protopen-comments=1`

  const refresh = useCallback(async () => {
    try {
      const data = await fetchSiteComments(siteID, { status: filter })
      setComments(data)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not load comments')
    }
  }, [siteID, filter, onSessionExpired])

  useEffect(() => {
    void refresh()
  }, [refresh])

  // Pins on the current page only.
  const pinsForCurrentPath = useMemo(
    () =>
      comments
        .filter((c) => c.parentId === null && c.pagePath === currentPath && c.pinX != null && c.pinY != null)
        .map((c, i) => ({ id: c.id, x: c.pinX as number, y: c.pinY as number, seq: i + 1 })),
    [comments, currentPath],
  )

  // Send pin updates to the iframe whenever the set changes.
  useEffect(() => {
    iframeRef.current?.contentWindow?.postMessage({ type: 'protopen-set-pins', pins: pinsForCurrentPath }, '*')
  }, [pinsForCurrentPath])

  // Send mode updates to the iframe.
  useEffect(() => {
    iframeRef.current?.contentWindow?.postMessage({ type: 'protopen-set-mode', mode }, '*')
  }, [mode])

  // Receive postMessage events from the iframe.
  useEffect(() => {
    const handler = (e: MessageEvent) => {
      if (e.source !== iframeRef.current?.contentWindow) return
      const msg = e.data
      if (!msg || typeof msg !== 'object') return
      switch (msg.type) {
        case 'protopen-ready': {
          const p = typeof msg.path === 'string' ? siteRelativePath(msg.path, orgSlug, siteSlug) : '/'
          setCurrentPath(p)
          // re-send pins for the freshly loaded page.
          iframeRef.current?.contentWindow?.postMessage({ type: 'protopen-set-pins', pins: pinsForCurrentPath }, '*')
          iframeRef.current?.contentWindow?.postMessage({ type: 'protopen-set-mode', mode }, '*')
          break
        }
        case 'protopen-navigate':
          if (typeof msg.path === 'string') setCurrentPath(siteRelativePath(msg.path, orgSlug, siteSlug))
          break
        case 'protopen-click':
          if (typeof msg.x === 'number' && typeof msg.y === 'number') {
            const path = typeof msg.pagePath === 'string' ? siteRelativePath(msg.pagePath, orgSlug, siteSlug) : currentPath
            setPendingPin({ x: msg.x, y: msg.y, pagePath: path })
            setDraftBody('')
          }
          break
        case 'protopen-pin-click': {
          // Scroll to that thread in the side panel by setting a "focus" attr,
          // which our render uses as a hook.
          if (typeof msg.id === 'string') {
            const el = document.querySelector(`[data-thread-id="${msg.id}"]`)
            if (el && 'scrollIntoView' in el) {
              ;(el as HTMLElement).scrollIntoView({ block: 'start', behavior: 'smooth' })
              ;(el as HTMLElement).animate(
                [{ boxShadow: '0 0 0 2px rgb(59 130 246)' }, { boxShadow: '0 0 0 0 rgb(59 130 246 / 0)' }],
                { duration: 1200 },
              )
            }
          }
          break
        }
      }
    }
    window.addEventListener('message', handler)
    return () => window.removeEventListener('message', handler)
  }, [pinsForCurrentPath, mode, currentPath])

  // Deep-link: when focusCommentID is supplied, highlight it once loaded.
  useEffect(() => {
    if (!focusCommentID) return
    const target = comments.find((c) => c.id === focusCommentID)
    if (!target) return
    if (target.pagePath !== currentPath) {
      // Navigate the iframe to that page (target.pagePath is site-relative).
      if (iframeRef.current) {
        const rel = target.pagePath.startsWith('/') ? target.pagePath : '/' + target.pagePath
        iframeRef.current.src = `${siteRoot}${rel === '/' ? '/' : rel}?protopen-comments=1`
      }
    } else {
      iframeRef.current?.contentWindow?.postMessage(
        { type: 'protopen-highlight-pin', id: target.id },
        '*',
      )
    }
  }, [focusCommentID, comments, currentPath, siteRoot])

  const handleSubmitNew = async () => {
    if (!pendingPin || !draftBody.trim()) return
    try {
      await postSiteComment(siteID, {
        pagePath: pendingPin.pagePath,
        pinX: pendingPin.x,
        pinY: pendingPin.y,
        body: draftBody.trim(),
      })
      setPendingPin(null)
      setDraftBody('')
      await refresh()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not post comment')
    }
  }

  const handleReply = async (rootId: string) => {
    if (!replyDraft || replyDraft.rootId !== rootId || !replyDraft.body.trim()) return
    try {
      await postSiteComment(siteID, {
        pagePath: currentPath,
        parentId: rootId,
        body: replyDraft.body.trim(),
      })
      setReplyDraft(null)
      await refresh()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not reply')
    }
  }

  const handleResolve = async (commentID: string) => {
    try {
      await toggleResolveComment(commentID)
      await refresh()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not update comment')
    }
  }

  const handleDelete = async (commentID: string) => {
    try {
      await deleteComment(commentID)
      await refresh()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not delete comment')
    }
  }

  const canMutateComment = (c: SiteComment) => c.author?.id === currentUserID || isOrgAdmin

  // Group: root comments with their replies.
  const threads = useMemo(() => {
    const byId = new Map<string, { root: SiteComment; replies: SiteComment[] }>()
    for (const c of comments) {
      if (c.parentId === null) byId.set(c.id, { root: c, replies: [] })
    }
    for (const c of comments) {
      if (c.parentId !== null) {
        const t = byId.get(c.parentId)
        if (t) t.replies.push(c)
      }
    }
    return Array.from(byId.values()).sort(
      (a, b) => new Date(a.root.createdAt).getTime() - new Date(b.root.createdAt).getTime(),
    )
  }, [comments])

  // Mobile fallback.
  const isMobile = typeof window !== 'undefined' && window.innerWidth < 768
  if (isMobile) {
    return (
      <div className="flex h-svh flex-col items-center justify-center px-6 text-center">
        <Button variant="ghost" size="sm" onClick={onBack} className="absolute left-3 top-3">
          <ArrowLeft />
          Back
        </Button>
        <p className="text-sm text-muted-foreground">
          Comments overlay requires a viewport of at least 768px. View on desktop to comment.
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-svh flex-col bg-background">
      <header className="flex h-12 items-center justify-between border-b px-4">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft />
            Back
          </Button>
          <span className="text-sm font-medium">{siteName}</span>
          <span className="text-xs text-muted-foreground">{currentPath}</span>
        </div>
        <div className="flex items-center gap-2">
          <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
            <TabsList className="!flex-row">
              <TabsTrigger value="comment">Comment</TabsTrigger>
              <TabsTrigger value="browse">Browse</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <div className="relative flex-1 bg-muted/30">
          <iframe
            ref={iframeRef}
            src={iframeSrc}
            title={`${siteName} preview`}
            className="size-full border-0"
          />
          {pendingPin ? (
            <div className="absolute inset-0 flex items-center justify-center bg-background/60 backdrop-blur-sm">
              <Card className="w-80">
                <CardContent className="flex flex-col gap-3 p-4">
                  <p className="text-sm font-medium">New comment on {pendingPin.pagePath}</p>
                  <Textarea
                    autoFocus
                    placeholder="Leave a comment..."
                    value={draftBody}
                    onChange={(e) => setDraftBody(e.target.value)}
                    rows={3}
                  />
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setPendingPin(null)
                        setDraftBody('')
                      }}
                    >
                      Cancel
                    </Button>
                    <Button size="sm" onClick={() => void handleSubmitNew()} disabled={!draftBody.trim()}>
                      Comment
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          ) : null}
        </div>

        <aside className="flex w-80 flex-col border-l">
          <div className="flex items-center justify-between gap-2 border-b px-4 py-2">
            <Tabs value={filter} onValueChange={(v) => setFilter(v as CommentStatusFilter)}>
              <TabsList className="!flex-row">
                <TabsTrigger value="open">Open</TabsTrigger>
                <TabsTrigger value="resolved">Resolved</TabsTrigger>
                <TabsTrigger value="all">All</TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {threads.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-foreground">
                {filter === 'resolved'
                  ? 'No resolved threads yet.'
                  : mode === 'comment'
                    ? 'Click anywhere on the preview to leave a comment.'
                    : 'No comments yet.'}
              </p>
            ) : (
              <ul className="divide-y">
                {threads.map((t, i) => {
                  const seq = t.root.pagePath === currentPath && t.root.pinX != null ? pinsForCurrentPath.findIndex((p) => p.id === t.root.id) + 1 : 0
                  return (
                    <li key={t.root.id} data-thread-id={t.root.id} className="flex flex-col gap-2 px-4 py-3">
                      <div className="flex items-start gap-2">
                        {seq > 0 ? (
                          <span className="mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-[10px] font-semibold text-primary-foreground">
                            {seq}
                          </span>
                        ) : (
                          <span className="mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-medium text-muted-foreground">
                            {initials(t.root.author?.name)}
                          </span>
                        )}
                        <div className="min-w-0 flex-1">
                          <p className="text-xs text-muted-foreground">
                            <span className="font-medium text-foreground">{t.root.author?.name ?? 'Unknown'}</span>
                            <span> · {timeAgo(t.root.createdAt)}</span>
                            {t.root.pagePath && t.root.pagePath !== '/' ? (
                              <span> · {t.root.pagePath}</span>
                            ) : null}
                          </p>
                          <p className="mt-1 break-words text-sm">{t.root.body}</p>
                          {t.replies.map((reply) => (
                            <div key={reply.id} className="mt-2 rounded-md bg-muted/40 p-2">
                              <p className="text-xs text-muted-foreground">
                                <span className="font-medium text-foreground">{reply.author?.name ?? 'Unknown'}</span>
                                <span> · {timeAgo(reply.createdAt)}</span>
                              </p>
                              <p className="mt-1 break-words text-sm">{reply.body}</p>
                              {canMutateComment(reply) ? (
                                <button
                                  type="button"
                                  className="mt-1 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-destructive"
                                  onClick={() => void handleDelete(reply.id)}
                                >
                                  <Trash2 className="size-3" />
                                  Delete
                                </button>
                              ) : null}
                            </div>
                          ))}
                          {replyDraft?.rootId === t.root.id ? (
                            <div className="mt-2 flex flex-col gap-2">
                              <Textarea
                                autoFocus
                                placeholder="Reply..."
                                value={replyDraft.body}
                                onChange={(e) => setReplyDraft({ rootId: t.root.id, body: e.target.value })}
                                rows={2}
                              />
                              <div className="flex justify-end gap-2">
                                <Button variant="ghost" size="sm" onClick={() => setReplyDraft(null)}>
                                  Cancel
                                </Button>
                                <Button
                                  size="sm"
                                  onClick={() => void handleReply(t.root.id)}
                                  disabled={!replyDraft.body.trim()}
                                >
                                  Reply
                                </Button>
                              </div>
                            </div>
                          ) : (
                            <div className="mt-2 flex items-center gap-3">
                              <button
                                type="button"
                                className="text-xs text-muted-foreground hover:text-foreground"
                                onClick={() => setReplyDraft({ rootId: t.root.id, body: '' })}
                              >
                                Reply
                              </button>
                              <button
                                type="button"
                                className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                                onClick={() => void handleResolve(t.root.id)}
                              >
                                {t.root.resolvedAt ? (
                                  <>
                                    <RotateCcw className="size-3" />
                                    Reopen
                                  </>
                                ) : (
                                  <>
                                    <Check className="size-3" />
                                    Resolve
                                  </>
                                )}
                              </button>
                              {canMutateComment(t.root) ? (
                                <button
                                  type="button"
                                  className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-destructive"
                                  onClick={() => void handleDelete(t.root.id)}
                                >
                                  <Trash2 className="size-3" />
                                  Delete
                                </button>
                              ) : null}
                            </div>
                          )}
                        </div>
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </aside>
      </div>
    </div>
  )
}
