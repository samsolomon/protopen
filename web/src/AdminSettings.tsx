import { useEffect, useState } from 'react'
import {
  fetchAdminSettings,
  updateAdminSettings,
  sendTestEmail,
  SessionExpiredError,
  type AdminSettings as AdminSettingsType,
  type AdminVisibilityPatch,
} from './api'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'

type AdminSettingsProps = {
  onSessionExpired: () => void
}

type EmailForm = {
  provider: 'none' | 'resend' | 'smtp'
  from: string
  resendKey: string
  smtpHost: string
  smtpPort: string
  smtpUser: string
  smtpPass: string
  smtpTLS: boolean
  inboundDomain: string
  inboundSecret: string
}

function emailFormFrom(s: AdminSettingsType): EmailForm {
  return {
    provider: s.email.provider,
    from: s.email.from,
    resendKey: '',
    smtpHost: s.email.smtpHost,
    smtpPort: s.email.smtpPort,
    smtpUser: s.email.smtpUser,
    smtpPass: '',
    smtpTLS: s.email.smtpTLS,
    inboundDomain: s.email.inboundDomain,
    inboundSecret: '',
  }
}

export function AdminSettings({ onSessionExpired }: AdminSettingsProps) {
  const [settings, setSettings] = useState<AdminSettingsType | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [email, setEmail] = useState<EmailForm | null>(null)
  const [testTo, setTestTo] = useState('')
  const [testing, setTesting] = useState(false)
  // daysInput tracks the raw value of the days field so typing isn't fighting
  // an async PATCH. Pushed to the server on blur/Enter once parsed + valid.
  const [daysInput, setDaysInput] = useState('')

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const load = async () => {
    try {
      setLoading(true)
      const data = await fetchAdminSettings()
      setSettings(data)
      setEmail(emailFormFrom(data))
      setDaysInput(String(data.visibility.autoPrivateAfterDays))
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not load settings')
    } finally {
      setLoading(false)
    }
  }

  // patchVisibility submits one or more fields with optimistic state. On
  // failure we restore the prior settings snapshot so the UI doesn't lie about
  // server state.
  const patchVisibility = async (patch: AdminVisibilityPatch, optimistic?: Partial<AdminSettingsType['visibility']>) => {
    if (!settings) return
    const previous = settings
    if (optimistic) {
      setSettings({ ...settings, visibility: { ...settings.visibility, ...optimistic } })
    }
    setSaving(true)
    try {
      const updated = await updateAdminSettings({ visibility: patch })
      setSettings(updated)
      setDaysInput(String(updated.visibility.autoPrivateAfterDays))
    } catch (err) {
      setSettings(previous)
      setDaysInput(String(previous.visibility.autoPrivateAfterDays))
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not save settings')
    } finally {
      setSaving(false)
    }
  }

  const commitDays = () => {
    if (!settings) return
    const parsed = parseInt(daysInput, 10)
    if (Number.isNaN(parsed) || parsed < 1 || parsed > 3650) {
      // Reset to the last server-confirmed value rather than nag with an error.
      setDaysInput(String(settings.visibility.autoPrivateAfterDays))
      return
    }
    if (parsed === settings.visibility.autoPrivateAfterDays) return
    void patchVisibility({ autoPrivateAfterDays: parsed })
  }

  const handleThumbnailsToggle = async (next: boolean) => {
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

  const handleEmailSave = async () => {
    if (!email) return
    setSaving(true)
    try {
      const updated = await updateAdminSettings({
        email: {
          provider: email.provider,
          from: email.from,
          // Secrets: only send when the admin typed something; a blank field
          // keeps the stored value.
          resendKey: email.resendKey || undefined,
          smtpHost: email.smtpHost,
          smtpPort: email.smtpPort,
          smtpUser: email.smtpUser,
          smtpPass: email.smtpPass || undefined,
          smtpTLS: email.smtpTLS,
          inboundDomain: email.inboundDomain,
          inboundSecret: email.inboundSecret || undefined,
        },
      })
      setSettings(updated)
      setEmail(emailFormFrom(updated))
      toast.success('Email settings saved')
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not save email settings')
    } finally {
      setSaving(false)
    }
  }

  const handleSendTest = async () => {
    if (!testTo.trim()) return
    setTesting(true)
    try {
      await sendTestEmail(testTo.trim())
      toast.success(`Test email sent to ${testTo.trim()}`)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not send test email')
    } finally {
      setTesting(false)
    }
  }

  if (loading || !settings || !email) {
    return <p className="text-sm text-muted-foreground">Loading settings...</p>
  }

  const { available, enabled, reason } = settings.thumbnails
  const selectClass =
    'h-8 w-full rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring'

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-lg border bg-card">
        <div className="flex flex-col gap-4 px-4 py-4">
          <div className="flex flex-col gap-1">
            <Label className="text-sm font-medium">Email provider</Label>
            <p className="text-sm text-muted-foreground">
              Transactional email — verification, password reset, invites, and comment
              notifications. Keys are stored in this instance's database.
            </p>
          </div>

          <div className="grid gap-3 sm:max-w-sm">
            <div className="flex flex-col gap-1">
              <Label htmlFor="email-provider" className="text-xs font-normal text-muted-foreground">Provider</Label>
              <select
                id="email-provider"
                className={selectClass}
                value={email.provider}
                onChange={(e) => setEmail({ ...email, provider: e.target.value as EmailForm['provider'] })}
              >
                <option value="none">None — email disabled</option>
                <option value="resend">Resend</option>
                <option value="smtp">SMTP</option>
              </select>
            </div>

            {email.provider !== 'none' ? (
              <div className="flex flex-col gap-1">
                <Label htmlFor="email-from" className="text-xs font-normal text-muted-foreground">From address</Label>
                <Input
                  id="email-from"
                  placeholder="Protopen <noreply@example.com>"
                  value={email.from}
                  onChange={(e) => setEmail({ ...email, from: e.target.value })}
                />
              </div>
            ) : null}

            {email.provider === 'resend' ? (
              <div className="flex flex-col gap-1">
                <Label htmlFor="email-resend-key" className="text-xs font-normal text-muted-foreground">
                  Resend API key {settings.email.resendKeySet ? '· configured' : ''}
                </Label>
                <Input
                  id="email-resend-key"
                  type="password"
                  placeholder={settings.email.resendKeySet ? '•••••••• (leave blank to keep)' : 're_...'}
                  value={email.resendKey}
                  onChange={(e) => setEmail({ ...email, resendKey: e.target.value })}
                />
              </div>
            ) : null}

            {email.provider === 'smtp' ? (
              <>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="smtp-host" className="text-xs font-normal text-muted-foreground">Host</Label>
                  <Input id="smtp-host" placeholder="smtp.example.com" value={email.smtpHost}
                    onChange={(e) => setEmail({ ...email, smtpHost: e.target.value })} />
                </div>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="smtp-port" className="text-xs font-normal text-muted-foreground">Port</Label>
                  <Input id="smtp-port" placeholder="587" value={email.smtpPort}
                    onChange={(e) => setEmail({ ...email, smtpPort: e.target.value })} />
                </div>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="smtp-user" className="text-xs font-normal text-muted-foreground">Username</Label>
                  <Input id="smtp-user" value={email.smtpUser}
                    onChange={(e) => setEmail({ ...email, smtpUser: e.target.value })} />
                </div>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="smtp-pass" className="text-xs font-normal text-muted-foreground">
                    Password {settings.email.smtpPassSet ? '· configured' : ''}
                  </Label>
                  <Input id="smtp-pass" type="password"
                    placeholder={settings.email.smtpPassSet ? '•••••••• (leave blank to keep)' : ''}
                    value={email.smtpPass}
                    onChange={(e) => setEmail({ ...email, smtpPass: e.target.value })} />
                </div>
                <div className="flex items-center gap-2">
                  <Switch id="smtp-tls" checked={email.smtpTLS}
                    onCheckedChange={(v) => setEmail({ ...email, smtpTLS: v })} />
                  <Label htmlFor="smtp-tls" className="text-sm font-normal">Implicit TLS (port 465)</Label>
                </div>
              </>
            ) : null}

            {email.provider !== 'none' ? (
              <div className="flex flex-col gap-3 border-t pt-3">
                <p className="text-xs text-muted-foreground">
                  Reply-by-email (optional). Set an inbound domain with MX records pointed at
                  a provider that forwards parsed mail to <code>/api/email/inbound</code>. With
                  this configured, notification emails carry a Reply-To so recipients can reply
                  straight from their inbox.
                </p>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="inbound-domain" className="text-xs font-normal text-muted-foreground">Inbound domain</Label>
                  <Input id="inbound-domain" placeholder="reply.example.com" value={email.inboundDomain}
                    onChange={(e) => setEmail({ ...email, inboundDomain: e.target.value })} />
                </div>
                <div className="flex flex-col gap-1">
                  <Label htmlFor="inbound-secret" className="text-xs font-normal text-muted-foreground">
                    Webhook signing secret {settings.email.inboundSecretSet ? '· configured' : ''}
                  </Label>
                  <Input id="inbound-secret" type="password"
                    placeholder={settings.email.inboundSecretSet ? '•••••••• (leave blank to keep)' : ''}
                    value={email.inboundSecret}
                    onChange={(e) => setEmail({ ...email, inboundSecret: e.target.value })} />
                </div>
              </div>
            ) : null}

            <div>
              <Button size="sm" onClick={() => void handleEmailSave()} disabled={saving}>
                Save email settings
              </Button>
            </div>
          </div>

          {email.provider !== 'none' ? (
            <div className="flex flex-col gap-1 border-t pt-4 sm:max-w-sm">
              <Label htmlFor="test-to" className="text-xs font-normal text-muted-foreground">Send a test email</Label>
              <div className="flex gap-2">
                <Input id="test-to" type="email" placeholder="you@example.com" value={testTo}
                  onChange={(e) => setTestTo(e.target.value)} />
                <Button size="sm" variant="outline" onClick={() => void handleSendTest()}
                  disabled={testing || !testTo.trim()}>
                  Send test
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="rounded-lg border bg-card">
        <div className="flex flex-col gap-4 px-4 py-4">
          <div className="flex flex-col gap-1">
            <Label className="text-sm font-medium">Site visibility</Label>
            <p className="text-sm text-muted-foreground">
              Control the default visibility of newly deployed sites and whether public
              sites automatically revert to private. Private sites are only viewable by
              members of the site's workspace.
            </p>
          </div>

          <div className="flex items-center justify-between gap-6">
            <div className="flex flex-col gap-1">
              <Label htmlFor="default-private-toggle" className="text-sm font-normal">Default new sites to private</Label>
              <p className="text-sm text-muted-foreground">
                Site creators can still flip individual sites to public after deploy.
              </p>
            </div>
            <Switch
              id="default-private-toggle"
              checked={settings.visibility.defaultSitePrivate}
              disabled={saving}
              onCheckedChange={(value) => void patchVisibility({ defaultSitePrivate: value }, { defaultSitePrivate: value })}
            />
          </div>

          <div className="flex items-center justify-between gap-6 border-t pt-4">
            <div className="flex flex-col gap-1">
              <Label htmlFor="auto-private-toggle" className="text-sm font-normal">Automatically revert public sites</Label>
              <p className="text-sm text-muted-foreground">
                Public sites become private again once they've been public for the
                configured number of days. Owners are not notified.
              </p>
            </div>
            <Switch
              id="auto-private-toggle"
              checked={settings.visibility.autoPrivateEnabled}
              disabled={saving}
              onCheckedChange={(value) => void patchVisibility({ autoPrivateEnabled: value }, { autoPrivateEnabled: value })}
            />
          </div>

          {settings.visibility.autoPrivateEnabled ? (
            <div className="flex flex-col gap-2 sm:max-w-sm">
              <Label htmlFor="auto-private-days" className="text-xs font-normal text-muted-foreground">
                Revert after (days)
              </Label>
              <Input
                id="auto-private-days"
                type="number"
                min={1}
                max={3650}
                value={daysInput}
                disabled={saving}
                onChange={(e) => setDaysInput(e.target.value)}
                onBlur={commitDays}
                onKeyDown={(e) => { if (e.key === 'Enter') { e.currentTarget.blur() } }}
                className="w-32"
              />
              {settings.visibility.eligibleForRevertCount > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {settings.visibility.eligibleForRevertCount} public {settings.visibility.eligibleForRevertCount === 1 ? 'site is' : 'sites are'} older
                  than {settings.visibility.autoPrivateAfterDays} {settings.visibility.autoPrivateAfterDays === 1 ? 'day' : 'days'} and will be reverted
                  on the next sweep.
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>

      <div className="rounded-lg border bg-card">
        <div className="flex items-center justify-between gap-6 px-4 py-3">
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
            onCheckedChange={(value) => void handleThumbnailsToggle(value)}
          />
        </div>
      </div>
    </div>
  )
}
