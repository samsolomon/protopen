import { useState } from 'react'
import type { SessionUser } from './types'
import { updateProfile, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
      <h2 className="mb-4 text-lg font-semibold tracking-tight">Profile</h2>
      <Card>
        <CardHeader>
          <CardTitle>Your information</CardTitle>
          <CardDescription>Update your name and email address.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="profile-name">Name</Label>
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Your name"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="profile-email">Email</Label>
            <Input
              id="profile-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Your username is <span className="font-medium">@{user.username}</span>
          </p>
          <Button onClick={() => void handleSave()} disabled={saving || !hasChanges}>
            {saving ? 'Saving...' : 'Save changes'}
          </Button>
        </CardContent>
      </Card>
    </section>
  )
}
