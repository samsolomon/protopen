import { useState, useEffect } from 'react'
import type { SessionUser, OrgMember } from './types'
import { fetchOrgMembers, addOrgMember, removeOrgMember, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import { UserPlus, X } from 'lucide-react'

type OrgMembersPanelProps = {
  user: SessionUser
  onSessionExpired: () => void
}

export function OrgMembersPanel({ user, onSessionExpired }: OrgMembersPanelProps) {
  const personalOrg = user.orgs.find((o) => o.isPersonal)
  const [members, setMembers] = useState<OrgMember[]>([])
  const [loading, setLoading] = useState(true)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviting, setInviting] = useState(false)

  const orgId = personalOrg?.id ?? ''
  const isAdmin = personalOrg?.role === 'admin'

  useEffect(() => {
    if (!orgId) return
    setLoading(true)
    fetchOrgMembers(orgId)
      .then(setMembers)
      .catch((err) => {
        if (err instanceof SessionExpiredError) return onSessionExpired()
        toast.error('Could not load members')
      })
      .finally(() => setLoading(false))
  }, [orgId, onSessionExpired])

  const handleInvite = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!inviteEmail.trim() || !orgId) return

    setInviting(true)
    try {
      const result = await addOrgMember(orgId, inviteEmail.trim(), 'member')
      if (result.status === 'invited') {
        toast.success('Invite sent — they\'ll be added when they sign up')
      } else {
        toast.success('Member added')
      }
      setInviteEmail('')
      const updated = await fetchOrgMembers(orgId)
      setMembers(updated)
    } catch (err) {
      if (err instanceof SessionExpiredError) return onSessionExpired()
      toast.error(err instanceof Error ? err.message : 'Could not add member')
    } finally {
      setInviting(false)
    }
  }

  const handleRemove = async (memberId: string, memberName: string) => {
    if (!orgId) return
    try {
      await removeOrgMember(orgId, memberId)
      setMembers((prev) => prev.filter((m) => m.id !== memberId))
      toast.success(`Removed ${memberName}`)
    } catch (err) {
      if (err instanceof SessionExpiredError) return onSessionExpired()
      toast.error(err instanceof Error ? err.message : 'Could not remove member')
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Team Members</CardTitle>
        <CardDescription>People who can view and manage projects in your workspace.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <div className="space-y-2">
            {members.map((member) => (
              <div
                key={member.id}
                className="flex items-center justify-between rounded-md border px-3 py-2"
              >
                <div className="flex items-center gap-3">
                  <span className="flex size-8 items-center justify-center rounded-full bg-muted text-xs font-medium">
                    {member.name.charAt(0).toUpperCase()}
                  </span>
                  <div>
                    <p className="text-sm font-medium">{member.name}</p>
                    <p className="text-xs text-muted-foreground">{member.email}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant={member.role === 'admin' ? 'default' : 'secondary'}>
                    {member.role}
                  </Badge>
                  {isAdmin && member.userId !== user.id && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 w-7 p-0"
                      onClick={() => handleRemove(member.id, member.name)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}

        {isAdmin && (
          <form onSubmit={handleInvite} className="flex gap-2 pt-2">
            <div className="flex-1">
              <Label htmlFor="invite-email" className="sr-only">
                Email address
              </Label>
              <Input
                id="invite-email"
                type="email"
                placeholder="colleague@company.com"
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
              />
            </div>
            <Button type="submit" size="sm" disabled={inviting || !inviteEmail.trim()}>
              <UserPlus className="mr-1.5 h-3.5 w-3.5" />
              {inviting ? 'Adding...' : 'Add'}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  )
}
