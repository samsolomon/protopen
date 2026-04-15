import { useState, useEffect } from 'react'
import type { SessionUser, OrgMember } from './types'
import { fetchOrgMembers, addOrgMember, removeOrgMember, updateMemberRole, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { UserPlus, Trash2, ChevronDown } from 'lucide-react'

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
  const [removingMember, setRemovingMember] = useState<{ id: string; name: string } | null>(null)
  const [removing, setRemoving] = useState(false)

  const orgId = personalOrg?.id ?? ''
  const isAdmin = personalOrg?.role === 'admin'
  const roleBadgeVariant = (role: string): 'default' | 'secondary' => (role === 'admin' ? 'default' : 'secondary')

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

  const handleRoleChange = async (memberId: string, newRole: string) => {
    if (!orgId) return
    const target = members.find((m) => m.id === memberId)
    if (!target || target.role === newRole) return
    const prevRole = target.role
    setMembers((cur) => cur.map((m) => (m.id === memberId ? { ...m, role: newRole } : m)))
    try {
      await updateMemberRole(orgId, memberId, newRole)
      toast.success(`Role updated to ${newRole}`)
    } catch (err) {
      setMembers((cur) => cur.map((m) => (m.id === memberId ? { ...m, role: prevRole } : m)))
      if (err instanceof SessionExpiredError) return onSessionExpired()
      toast.error(err instanceof Error ? err.message : 'Could not update role')
    }
  }

  const handleRemove = async () => {
    if (!orgId || !removingMember) return
    setRemoving(true)
    try {
      await removeOrgMember(orgId, removingMember.id)
      setMembers((prev) => prev.filter((m) => m.id !== removingMember.id))
      toast.success(`Removed ${removingMember.name}`)
      setRemovingMember(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) return onSessionExpired()
      toast.error(err instanceof Error ? err.message : 'Could not remove member')
    } finally {
      setRemoving(false)
    }
  }

  return (
    <section>
      <h3 className="mb-1 text-sm font-medium">Team members</h3>
      <p className="mb-3 text-sm text-muted-foreground">People who can view and manage projects in your workspace.</p>

      {isAdmin && (
        <form onSubmit={handleInvite} className="mb-4 flex gap-2">
          <div className="flex-1">
            <Label htmlFor="invite-email" className="sr-only">Email address</Label>
            <Input
              id="invite-email"
              type="email"
              placeholder="colleague@company.com"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={inviting || !inviteEmail.trim()}>
            {inviting ? 'Adding...' : 'Add member'}
          </Button>
        </form>
      )}

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : members.length > 0 ? (
        <div className="rounded-lg border bg-card">
          {members.map((member, i) => (
            <div
              key={member.id}
              className={`flex items-center justify-between px-4 py-3${i < members.length - 1 ? ' border-b' : ''}`}
            >
              <div className="flex items-center gap-3">
                <span className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-medium">
                  {member.name.charAt(0).toUpperCase()}
                </span>
                <div>
                  <p className="text-sm">{member.name}</p>
                  <p className="text-xs text-muted-foreground">{member.email}</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {isAdmin && member.userId !== user.id ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger className="cursor-pointer focus:outline-none">
                      <Badge variant={roleBadgeVariant(member.role)} render={<button />}>
                        {member.role}
                        <ChevronDown className="ml-1 h-3 w-3" />
                      </Badge>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuRadioGroup
                        value={member.role}
                        onValueChange={(value) => handleRoleChange(member.id, value)}
                      >
                        <DropdownMenuRadioItem value="admin">Admin</DropdownMenuRadioItem>
                        <DropdownMenuRadioItem value="member">Member</DropdownMenuRadioItem>
                      </DropdownMenuRadioGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : (
                  <Badge variant={roleBadgeVariant(member.role)}>
                    {member.role}
                  </Badge>
                )}
                {isAdmin && member.userId !== user.id && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="hover:bg-destructive/10 hover:text-destructive"
                        onClick={() => setRemovingMember({ id: member.id, name: member.name })}
                        aria-label="Remove member"
                      >
                        <Trash2 />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>Remove member</TooltipContent>
                  </Tooltip>
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">No team members yet.</p>
      )}

      <Dialog open={removingMember !== null} onOpenChange={(open) => { if (!open) setRemovingMember(null) }}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Remove member?</DialogTitle>
            <DialogDescription>
              Are you sure you want to remove {removingMember?.name} from this workspace?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRemovingMember(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => void handleRemove()} disabled={removing}>
              {removing ? 'Removing...' : 'Remove'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
