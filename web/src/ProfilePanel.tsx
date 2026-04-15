import { useState } from 'react'
import type { SessionUser } from './types'
import { updateProfile, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'

type ProfilePanelProps = {
  user: SessionUser
  onUserUpdated: (user: SessionUser) => void
  onSessionExpired: () => void
}

export function ProfilePanel({ user, onUserUpdated, onSessionExpired }: ProfilePanelProps) {
  const [name, setName] = useState(user.name)
  const [email, setEmail] = useState(user.email)
  const [saving, setSaving] = useState(false)

  const hasChanges = name.trim() !== user.name || email.trim().toLowerCase() !== user.email

  const handleSave = async () => {
    setSaving(true)
    try {
      const updated = await updateProfile(name, email)
      onUserUpdated(updated)
      toast.success('Profile updated')
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not update profile')
    } finally {
      setSaving(false)
    }
  }

  return (
    <section>
      <h3 className="mb-3 text-sm font-medium">Profile</h3>
      <div className="rounded-lg border bg-card">
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <label htmlFor="profile-name" className="text-sm">Name</label>
          <Input
            id="profile-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="max-w-[240px] text-right"
          />
        </div>
        <div className="flex items-center justify-between px-4 py-3 border-b">
          <label htmlFor="profile-email" className="text-sm">Email</label>
          <Input
            id="profile-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="max-w-[240px] text-right"
          />
        </div>
        <div className="flex items-center justify-between px-4 py-3">
          <p className="text-sm text-muted-foreground">Username: @{user.username}</p>
          <Button size="sm" onClick={() => void handleSave()} disabled={saving || !hasChanges}>
            {saving ? 'Saving...' : 'Save changes'}
          </Button>
        </div>
      </div>
    </section>
  )
}
