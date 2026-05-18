import { useEffect, useState } from 'react'
import { fetchAdminSettings, updateAdminSettings, SessionExpiredError, type AdminSettings as AdminSettingsType } from './api'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'

type AdminSettingsProps = {
  onSessionExpired: () => void
}

export function AdminSettings({ onSessionExpired }: AdminSettingsProps) {
  const [settings, setSettings] = useState<AdminSettingsType | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const load = async () => {
    try {
      setLoading(true)
      const data = await fetchAdminSettings()
      setSettings(data)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not load settings')
    } finally {
      setLoading(false)
    }
  }

  const handleToggle = async (next: boolean) => {
    if (!settings) return
    const previous = settings
    setSettings({ ...settings, thumbnails: { ...settings.thumbnails, enabled: next } })
    setSaving(true)
    try {
      const updated = await updateAdminSettings({ thumbnailsEnabled: next })
      setSettings(updated)
    } catch (err) {
      setSettings(previous)
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not save settings')
    } finally {
      setSaving(false)
    }
  }

  if (loading || !settings) {
    return <p className="text-sm text-muted-foreground">Loading settings...</p>
  }

  const { available, enabled, reason } = settings.thumbnails

  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-start justify-between gap-6 px-4 py-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="thumbnails-toggle" className="text-sm font-normal">Deploy thumbnails</Label>
          <p className="text-sm text-muted-foreground">
            Render a screenshot of each new deploy and show it on dashboard cards.
          </p>
          {!available && reason ? (
            <p className="mt-1 text-xs text-muted-foreground">Unavailable: {reason}</p>
          ) : null}
        </div>
        <Switch
          id="thumbnails-toggle"
          checked={enabled}
          disabled={!available || saving}
          onCheckedChange={(value) => void handleToggle(value)}
        />
      </div>
    </div>
  )
}
