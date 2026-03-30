import { useEffect, useState } from 'react'
import { fetchNotifications, markNotificationsRead, SessionExpiredError } from './api'
import type { Notification } from './types'
import { Button } from '@/components/ui/button'

type NotificationsPanelProps = {
  onSessionExpired: () => void
  onCountChange?: (count: number) => void
}

function timeAgo(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  if (ms < 60000) return 'just now'
  if (ms < 3600000) return `${Math.floor(ms / 60000)}m ago`
  if (ms < 86400000) return `${Math.floor(ms / 3600000)}h ago`
  return `${Math.floor(ms / 86400000)}d ago`
}

function notificationText(n: Notification): string {
  switch (n.type) {
    case 'mention':
      return `mentioned you in ${n.projectName}`
    case 'reply':
      return `replied in ${n.projectName}`
    case 'resolve':
      return `resolved a thread in ${n.projectName}`
    case 'new_comment':
      return `commented on ${n.projectName}`
    default:
      return `activity in ${n.projectName}`
  }
}

function groupByDay(notifications: Notification[]): { label: string; items: Notification[] }[] {
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const yesterday = today - 86400000

  const groups: { label: string; items: Notification[] }[] = []
  let currentLabel = ''
  let currentItems: Notification[] = []

  for (const n of notifications) {
    const t = new Date(n.createdAt).getTime()
    const label = t >= today ? 'Today' : t >= yesterday ? 'Yesterday' : 'Older'
    if (label !== currentLabel) {
      if (currentItems.length > 0) groups.push({ label: currentLabel, items: currentItems })
      currentLabel = label
      currentItems = []
    }
    currentItems.push(n)
  }
  if (currentItems.length > 0) groups.push({ label: currentLabel, items: currentItems })

  return groups
}

export function NotificationsPanel({ onSessionExpired, onCountChange }: NotificationsPanelProps) {
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    void load()
  }, [])

  const load = async () => {
    try {
      const loaded = await fetchNotifications()
      setNotifications(loaded)
      onCountChange?.(loaded.filter((n) => !n.readAt).length)
    } catch (err) {
      if (err instanceof SessionExpiredError) onSessionExpired()
    } finally {
      setLoading(false)
    }
  }

  const handleMarkAllRead = async () => {
    await markNotificationsRead()
    setNotifications((prev) => prev.map((n) => ({ ...n, readAt: n.readAt ?? new Date().toISOString() })))
    onCountChange?.(0)
  }

  const handleClick = async (n: Notification) => {
    if (!n.readAt) {
      await markNotificationsRead([n.id])
      setNotifications((prev) => {
        const updated = prev.map((x) => (x.id === n.id ? { ...x, readAt: new Date().toISOString() } : x))
        onCountChange?.(updated.filter((x) => !x.readAt).length)
        return updated
      })
    }
    window.open(n.linkUrl, '_blank')
  }

  const unreadCount = notifications.filter((n) => !n.readAt).length
  const groups = groupByDay(notifications)

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20 text-muted-foreground text-sm">Loading...</div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-semibold">Notifications</h2>
        {unreadCount > 0 && (
          <Button variant="ghost" size="sm" onClick={handleMarkAllRead}>
            Mark all as read
          </Button>
        )}
      </div>

      {notifications.length === 0 ? (
        <div className="text-center py-16 text-muted-foreground text-sm">No notifications yet</div>
      ) : (
        <div className="space-y-6">
          {groups.map((group) => (
            <div key={group.label}>
              <div className="text-xs font-medium text-muted-foreground mb-2 px-1">{group.label}</div>
              <div className="space-y-1">
                {group.items.map((n) => (
                  <button
                    key={n.id}
                    onClick={() => handleClick(n)}
                    className={`w-full text-left px-3 py-2.5 rounded-lg transition-colors hover:bg-accent/50 flex items-start gap-3 ${
                      n.readAt ? 'opacity-60' : ''
                    }`}
                  >
                    <div
                      className={`mt-1.5 h-2 w-2 rounded-full flex-shrink-0 ${
                        n.readAt ? 'bg-transparent' : 'bg-primary'
                      }`}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="text-sm">
                        <span className="font-medium">{n.actorName}</span>{' '}
                        <span className="text-muted-foreground">{notificationText(n)}</span>
                      </div>
                      {n.bodyPreview && (
                        <div className="text-xs text-muted-foreground mt-0.5 truncate">{n.bodyPreview}</div>
                      )}
                    </div>
                    <span className="text-xs text-muted-foreground flex-shrink-0 mt-0.5">{timeAgo(n.createdAt)}</span>
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
