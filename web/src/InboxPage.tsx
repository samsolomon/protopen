import { useEffect, useState } from 'react'
import type { Notification } from './types'
import {
  fetchNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  SessionExpiredError,
} from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from 'sonner'
import { initialsFor } from './AuthorChip'
import { timeAgo } from './lib/time'
import { siteCommentURL } from './constants'

type InboxFilter = 'all' | 'unread'

type InboxPageProps = {
  onSessionExpired: () => void
}

export function InboxPage({ onSessionExpired }: InboxPageProps) {
  const [filter, setFilter] = useState<InboxFilter>('all')
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const [loading, setLoading] = useState(true)

  const load = async (next: InboxFilter) => {
    setLoading(true)
    try {
      const data = await fetchNotifications({ unread: next === 'unread' })
      setNotifications(data.notifications)
      setUnreadCount(data.unreadCount)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not load notifications')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load(filter)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter])

  // Plain click: mark read then navigate same-tab. Modifier-clicks
  // (Cmd/Ctrl/Shift/middle) skip preventDefault so the browser handles the
  // anchor's href as new tab / new window — exactly what users expect from
  // a link.
  const handleClick = (e: React.MouseEvent<HTMLAnchorElement>, n: Notification) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
    e.preventDefault()
    void (async () => {
      if (!n.readAt) {
        try {
          await markNotificationRead(n.id)
        } catch (err) {
          if (err instanceof SessionExpiredError) {
            onSessionExpired()
            return
          }
        }
      }
      if (n.comment) {
        window.location.href = siteCommentURL(n.comment.orgSlug, n.comment.siteSlug, n.comment.pagePath, n.comment.id)
      }
    })()
  }

  const handleMarkAllRead = async () => {
    try {
      await markAllNotificationsRead()
      setNotifications((prev) => prev.map((n) => ({ ...n, readAt: n.readAt ?? new Date().toISOString() })))
      setUnreadCount(0)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not mark notifications read')
    }
  }

  return (
    <div className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <Tabs value={filter} onValueChange={(value) => setFilter(value as InboxFilter)}>
            <TabsList>
              <TabsTrigger value="all">All</TabsTrigger>
              <TabsTrigger value="unread">
                Unread{unreadCount > 0 ? ` (${unreadCount})` : ''}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
        {unreadCount > 0 ? (
          <Button variant="ghost" size="sm" onClick={() => void handleMarkAllRead()}>
            Mark all read
          </Button>
        ) : null}
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading notifications...</p>
      ) : notifications.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-muted-foreground">
            {filter === 'unread' ? 'No unread notifications.' : 'No notifications yet.'}
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border bg-card">
          {notifications.map((n, i) => {
            const c = n.comment
            const actor = n.actor?.name ?? 'Someone'
            const isUnread = !n.readAt
            return (
              <a
                key={n.id}
                href={c ? siteCommentURL(c.orgSlug, c.siteSlug, c.pagePath, c.id) : '#'}
                onClick={(e) => handleClick(e, n)}
                className={`flex w-full items-start gap-3 px-4 py-3 text-left no-underline text-inherit transition-colors hover:bg-muted/40 ${
                  i < notifications.length - 1 ? 'border-b' : ''
                }`}
              >
                <div
                  className={`flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-medium ${
                    isUnread ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'
                  }`}
                  aria-hidden
                >
                  {initialsFor(actor)}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm">
                    <span className="font-medium">{actor}</span>
                    <span className="text-muted-foreground"> commented on </span>
                    <span className="font-medium">{c?.siteName ?? 'a site'}</span>
                    <span className="text-muted-foreground"> · {timeAgo(n.createdAt, { longForm: true })}</span>
                  </p>
                  {c?.body ? (
                    <p className="mt-1 truncate text-sm text-muted-foreground">{c.body}</p>
                  ) : null}
                </div>
                {isUnread ? (
                  <span className="mt-2 size-2 shrink-0 rounded-full bg-primary" aria-label="unread" />
                ) : null}
              </a>
            )
          })}
        </div>
      )}
    </div>
  )
}
