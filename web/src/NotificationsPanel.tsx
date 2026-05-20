import { useEffect, useState } from 'react'
import {
  fetchNotificationPreferences,
  updateNotificationPreferences,
  SessionExpiredError,
  type NotificationPreferences,
} from './api'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'

type NotificationsPanelProps = {
  onSessionExpired: () => void
}

type Row = {
  key: keyof NotificationPreferences
  id: string
  label: string
  description: string
}

const ROWS: Row[] = [
  {
    key: 'emailOnReply',
    id: 'notif-reply',
    label: 'Replies to my threads',
    description: 'Email me when someone replies on a comment thread I started or joined.',
  },
  {
    key: 'emailOnMention',
    id: 'notif-mention',
    label: 'Mentions',
    description: 'Email me when someone @mentions me in a comment.',
  },
]

export function NotificationsPanel({ onSessionExpired }: NotificationsPanelProps) {
  const [prefs, setPrefs] = useState<NotificationPreferences | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const load = async () => {
    try {
      setLoading(true)
      setPrefs(await fetchNotificationPreferences())
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not load preferences')
    } finally {
      setLoading(false)
    }
  }

  const handleToggle = async (key: keyof NotificationPreferences, next: boolean) => {
    if (!prefs) return
    const previous = prefs
    setPrefs({ ...prefs, [key]: next })
    setSaving(true)
    try {
      setPrefs(await updateNotificationPreferences({ [key]: next }))
    } catch (err) {
      setPrefs(previous)
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not save preferences')
    } finally {
      setSaving(false)
    }
  }

  if (loading || !prefs) {
    return <p className="text-sm text-muted-foreground">Loading preferences...</p>
  }

  return (
    <div className="rounded-lg border bg-card">
      {ROWS.map((row, i) => (
        <div
          key={row.key}
          className={`flex items-start justify-between gap-6 px-4 py-3 ${i > 0 ? 'border-t' : ''}`}
        >
          <div className="flex flex-col gap-1">
            <Label htmlFor={row.id} className="text-sm font-normal">{row.label}</Label>
            <p className="text-sm text-muted-foreground">{row.description}</p>
          </div>
          <Switch
            id={row.id}
            checked={prefs[row.key]}
            disabled={saving}
            onCheckedChange={(value) => void handleToggle(row.key, value)}
          />
        </div>
      ))}
    </div>
  )
}
