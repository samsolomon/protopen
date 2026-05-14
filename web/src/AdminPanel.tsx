import { useEffect, useState } from 'react'
import type { AdminUser, AdminProject, AdminUserOrg } from './api'
import {
  fetchAdminUsers,
  adminDeleteUser,
  fetchAdminProjects,
  adminDeleteProject,
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
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { MoreHorizontal, X } from 'lucide-react'
import { toast } from 'sonner'
import { AdminSettings } from './AdminSettings'

type AdminPanelProps = {
  user: SessionUser
  onSessionExpired: () => void
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function primaryOrg<T extends { isPersonal: boolean }>(orgs: T[]): T | null {
  return orgs.find((o) => !o.isPersonal) ?? orgs[0] ?? null
}

export function AdminPanel({ user, onSessionExpired }: AdminPanelProps) {
  const [tab, setTab] = useState('people')
  const [users, setUsers] = useState<AdminUser[]>([])
  const [projects, setProjects] = useState<AdminProject[]>([])
  const [loadingUsers, setLoadingUsers] = useState(true)
  const [loadingProjects, setLoadingProjects] = useState(false)
  const [deleteUserTarget, setDeleteUserTarget] = useState<AdminUser | null>(null)
  const [deleteProjectTarget, setDeleteProjectTarget] = useState<AdminProject | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'member' | 'admin'>('member')
  const [inviting, setInviting] = useState(false)

  const targetOrgId = primaryOrg(user.orgs)?.id ?? null

  useEffect(() => {
    void loadUsers()
  }, [])

  const loadUsers = async () => {
    try {
      setLoadingUsers(true)
      const data = await fetchAdminUsers()
      setUsers(data)
      setError(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      setError(err instanceof Error ? err.message : 'Could not load users')
    } finally {
      setLoadingUsers(false)
    }
  }

  const loadProjects = async () => {
    try {
      setLoadingProjects(true)
      const data = await fetchAdminProjects()
      setProjects(data)
      setError(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      setError(err instanceof Error ? err.message : 'Could not load projects')
    } finally {
      setLoadingProjects(false)
    }
  }

  const handleTabChange = (value: string) => {
    setTab(value)
    if (value === 'sites' && projects.length === 0) {
      void loadProjects()
    }
  }

  const confirmDeleteUser = async () => {
    if (!deleteUserTarget) return
    try {
      await adminDeleteUser(deleteUserTarget.id)
      setUsers((prev) => prev.filter((u) => u.id !== deleteUserTarget.id))
      setDeleteUserTarget(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not delete user')
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

  const confirmDeleteProject = async () => {
    if (!deleteProjectTarget) return
    try {
      await adminDeleteProject(deleteProjectTarget.id)
      setProjects((prev) => prev.filter((p) => p.id !== deleteProjectTarget.id))
      setDeleteProjectTarget(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      setError(err instanceof Error ? err.message : 'Could not delete project')
    }
  }

  return (
    <div className="flex flex-col gap-6">
      {error && <p className="text-sm text-destructive">{error}</p>}

      <Tabs value={tab} onValueChange={handleTabChange}>
        <TabsList>
          <TabsTrigger value="people">People</TabsTrigger>
          <TabsTrigger value="sites">Sites</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="people">
          <div className="mb-4 flex items-end justify-between gap-4">
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

          {loadingUsers ? (
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
                              onClick={() => setDeleteUserTarget(u)}
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
        </TabsContent>

        <TabsContent value="sites">
          <div className="mb-4">
            <h2 className="text-lg font-semibold tracking-tight">Sites</h2>
            <p className="text-sm text-muted-foreground">{projects.length} total</p>
          </div>

          {loadingProjects ? (
            <p className="text-sm text-muted-foreground">Loading sites...</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Org</TableHead>
                  <TableHead className="text-center">Deploys</TableHead>
                  <TableHead className="text-right">Storage</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead></TableHead>
                  <TableHead className="w-24"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {projects.map((project) => (
                  <TableRow key={project.id}>
                    <TableCell>
                      <a href={project.liveUrl} target="_blank" rel="noopener noreferrer" className="font-medium hover:underline">
                        {project.name}
                      </a>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{project.orgName}</TableCell>
                    <TableCell className="text-center">
                      <Badge variant="secondary">{project.deployCount}</Badge>
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">{formatBytes(project.storageBytes)}</TableCell>
                    <TableCell className="text-muted-foreground">{project.updatedAt}</TableCell>
                    <TableCell>
                      <Badge variant={project.isPublic ? 'secondary' : 'outline'}>
                        {project.isPublic ? 'Public' : 'Private'}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteProjectTarget(project)}
                      >
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </TabsContent>

        <TabsContent value="settings">
          <AdminSettings onSessionExpired={onSessionExpired} />
        </TabsContent>
      </Tabs>

      <Dialog open={!!deleteUserTarget} onOpenChange={() => setDeleteUserTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Force delete account</DialogTitle>
            <DialogDescription>
              This will permanently delete <strong>{deleteUserTarget?.email}</strong> and every site they own. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleteUserTarget(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => void confirmDeleteUser()}>Delete</Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteProjectTarget} onOpenChange={() => setDeleteProjectTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete site</DialogTitle>
            <DialogDescription>
              This will delete <strong>{deleteProjectTarget?.name}</strong> ({deleteProjectTarget?.orgName}). This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleteProjectTarget(null)}>Cancel</Button>
            <Button variant="destructive" onClick={() => void confirmDeleteProject()}>Delete</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
