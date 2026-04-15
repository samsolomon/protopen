import { useState } from 'react'
import { changePassword, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'

type PasswordPanelProps = {
  onSessionExpired: () => void
}

export function PasswordPanel({ onSessionExpired }: PasswordPanelProps) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const handleSubmit = async () => {
    setError(null)

    if (newPassword.length < 8) {
      setError('New password must be at least 8 characters')
      return
    }

    if (newPassword !== confirmPassword) {
      setError('New passwords do not match')
      return
    }

    setSaving(true)
    try {
      await changePassword(currentPassword, newPassword)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      toast.success('Password changed')
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not change password')
    } finally {
      setSaving(false)
    }
  }

  return (
    <section>
      <h3 className="mb-3 text-sm font-medium">Password</h3>
      <div className="rounded-lg border bg-card">
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <label htmlFor="current-password" className="text-sm">Current password</label>
          <Input
            id="current-password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            className="max-w-[240px]"
          />
        </div>
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <label htmlFor="new-password" className="text-sm">New password</label>
          <Input
            id="new-password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder="At least 8 characters"
            className="max-w-[240px]"
          />
        </div>
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <label htmlFor="confirm-password" className="text-sm">Confirm new password</label>
          <Input
            id="confirm-password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            className="max-w-[240px]"
          />
        </div>
        <div className="flex items-center justify-between px-4 py-3">
          {error ? <p className="text-sm text-destructive">{error}</p> : <span />}
          <Button
            size="sm"
            onClick={() => void handleSubmit()}
            disabled={saving || !currentPassword || !newPassword || !confirmPassword}
          >
            {saving ? 'Changing...' : 'Change password'}
          </Button>
        </div>
      </div>
    </section>
  )
}
