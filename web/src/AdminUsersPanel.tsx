import { useEffect, useState } from 'react'
import type { AdminUser, AdminUserOrg } from './api'
import {
  fetchAdminUsers,
  adminDeleteUser,
  addOrgMember,
  updateMemberRole,
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
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ChevronDown, MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'

type AdminUsersPanelProps = {
  user: SessionUser
  onSessionExpired: () => void
}

function primaryOrg<T extends { isPersonal: boolean }>(orgs: T[]): T | null {
  return orgs.find((o) => !o.isPersonal) ?? orgs[0] ?? null
}

export function AdminUsersPanel({ user, onSessionExpired }: AdminUsersPanelProps) {
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
      toast.error(err instanceof Error ? err.message : 'Could not load users')
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

  const handleChangeRole = async (userId: string, org: AdminUserOrg, nextRole: 'admin' | 'member') => {
    if (org.role === nextRole) return
    try {
      await updateMemberRole(org.orgId, org.memberId, nextRole)
      setUsers((prev) =>
        prev.map((row) =>
          row.id !== userId
            ? row
            : { ...row, orgs: row.orgs.map((o) => (o.memberId === org.memberId ? { ...o, role: nextRole } : o)) }
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
          <h2 className="text-lg font-semibold tracking-tight">Users</h2>
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
        <p className="text-sm text-muted-foreground">Loading users...</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Joined</TableHead>
              <TableHead className="w-12"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((u) => {
              const sharedOrg = u.orgs.find((o) => !o.isPersonal) ?? null
              return (
                <TableRow key={u.id}>
                  <TableCell className="font-medium">{u.email}</TableCell>
                  <TableCell>{u.name}</TableCell>
                  <TableCell>
                    {sharedOrg ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <button
                            type="button"
                            className="inline-flex cursor-pointer items-center gap-1 rounded text-sm hover:underline focus:outline-none"
                            aria-label="Change role"
                          >
                            {sharedOrg.role === 'admin' ? 'Admin' : 'Member'}
                            <ChevronDown className="h-3 w-3 opacity-60" />
                          </button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="start">
                          <DropdownMenuItem
                            onClick={() => void handleChangeRole(u.id, sharedOrg, 'admin')}
                            disabled={sharedOrg.role === 'admin'}
                          >
                            Admin
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => void handleChangeRole(u.id, sharedOrg, 'member')}
                            disabled={sharedOrg.role === 'member'}
                          >
                            Member
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : (
                      <span className="text-sm text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">{u.createdAt}</TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-xs" aria-label="Person actions">
                          <MoreHorizontal />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
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
