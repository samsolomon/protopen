import { useEffect, useState } from 'react'
import type { AdminUser, AdminUserOrg } from './api'
import {
  fetchAdminUsers,
  adminDeleteUser,
  addOrgMember,
  updateMemberRole,
  removeOrgMember,
  SessionExpiredError,
} from './api'
import type { SessionUser } from './types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Badge } from '@/components/ui/badge'
import { MoreHorizontal, X } from 'lucide-react'
import { toast } from 'sonner'

type AdminPeoplePanelProps = {
  user: SessionUser
  onSessionExpired: () => void
}

function primaryOrg<T extends { isPersonal: boolean }>(orgs: T[]): T | null {
  return orgs.find((o) => !o.isPersonal) ?? orgs[0] ?? null
}

export function AdminPeoplePanel({ user, onSessionExpired }: AdminPeoplePanelProps) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'member' | 'admin'>('member')
  const [inviting, setInviting] = useState(false)

  const targetOrgId = primaryOrg(user.orgs)?.id ?? null

  useEffect(() => {
    void loadUsers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const loadUsers = async () => {
    try {
      setLoading(true)
      const data = await fetchAdminUsers()
      setUsers(data)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not load people')
    } finally {
      setLoading(false)
    }
  }

  const handleInvite = async () => {
    const email = inviteEmail.trim()
    if (!email || !targetOrgId) return
    setInviting(true)
    try {
      const { status } = await addOrgMember(targetOrgId, email, inviteRole)
      setInviteEmail('')
      toast.success(status === 'invited' ? `Invite sent to ${email}` : `${email} added`)
      await loadUsers()
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not add member')
    } finally {
      setInviting(false)
    }
  }

  const handleRemoveFromOrg = async (userId: string, org: AdminUserOrg) => {
    try {
      await removeOrgMember(org.orgId, org.memberId)
      setUsers((prev) =>
        prev.map((u) => (u.id === userId ? { ...u, orgs: u.orgs.filter((o) => o.memberId !== org.memberId) } : u))
      )
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not remove from workspace')
    }
  }

  const handleToggleRole = async (userId: string, primary: AdminUserOrg) => {
    const nextRole = primary.role === 'admin' ? 'member' : 'admin'
    try {
      await updateMemberRole(primary.orgId, primary.memberId, nextRole)
      setUsers((prev) =>
        prev.map((row) =>
          row.id !== userId
            ? row
            : { ...row, orgs: row.orgs.map((o) => (o.memberId === primary.memberId ? { ...o, role: nextRole } : o)) }
        )
      )
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not change role')
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await adminDeleteUser(deleteTarget.id)
      setUsers((prev) => prev.filter((u) => u.id !== deleteTarget.id))
      setDeleteTarget(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not delete user')
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">People</h2>
          <p className="text-sm text-muted-foreground">{users.length} total</p>
        </div>
        <div className="flex items-center gap-2">
          <Input
            type="email"
            placeholder="invite@example.com"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') void handleInvite() }}
            disabled={inviting || !targetOrgId}
            className="w-64"
          />
          <select
            value={inviteRole}
            onChange={(e) => setInviteRole(e.target.value as 'member' | 'admin')}
            disabled={inviting || !targetOrgId}
            className="h-9 rounded-md border bg-background px-2 text-sm"
          >
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
          <Button onClick={() => void handleInvite()} disabled={inviting || !inviteEmail.trim() || !targetOrgId}>
            Invite
          </Button>
        </div>
      </div>

      {loading ? (
        <p className="text-sm text-muted-foreground">Loading people...</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Workspaces</TableHead>
              <TableHead>Joined</TableHead>
              <TableHead className="w-12"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((u) => {
              const primary = primaryOrg(u.orgs)
              return (
                <TableRow key={u.id}>
                  <TableCell className="font-medium">{u.email}</TableCell>
                  <TableCell>{u.name}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {u.orgs.length === 0 ? (
                        <span className="text-xs text-muted-foreground">—</span>
                      ) : (
                        u.orgs.map((org) => (
                          <Badge
                            key={org.memberId}
                            variant={org.role === 'admin' ? 'secondary' : 'outline'}
                            className="gap-1 pr-1"
                          >
                            <span>{org.orgSlug} · {org.role}</span>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => void handleRemoveFromOrg(u.id, org)}
                              aria-label={`Remove from ${org.orgSlug}`}
                            >
                              <X className="h-3 w-3" />
                            </Button>
                          </Badge>
                        ))
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{u.createdAt}</TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={<Button variant="ghost" size="icon-xs" aria-label="Person actions" />}
                      >
                        <MoreHorizontal />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {primary ? (
                          <DropdownMenuItem onClick={() => void handleToggleRole(u.id, primary)}>
                            {primary.role === 'admin'
                              ? `Demote in ${primary.orgSlug}`
                              : `Promote in ${primary.orgSlug}`}
                          </DropdownMenuItem>
                        ) : null}
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleteTarget(u)}
                        >
                          Force delete account
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      <Dialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Force delete account</DialogTitle>
            <DialogDescription>
              This will permanently delete <strong>{deleteTarget?.email}</strong> and every site they own. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => void confirmDelete()}>Delete</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
