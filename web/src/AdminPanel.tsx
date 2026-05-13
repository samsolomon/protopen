import { useEffect, useState } from 'react'
import type { AdminUser, AdminProject } from './api'
import { fetchAdminUsers, adminDeleteUser, fetchAdminProjects, adminDeleteProject, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
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
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'

type AdminPanelProps = {
  onSessionExpired: () => void
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

export function AdminPanel({ onSessionExpired }: AdminPanelProps) {
  const [tab, setTab] = useState('users')
  const [users, setUsers] = useState<AdminUser[]>([])
  const [projects, setProjects] = useState<AdminProject[]>([])
  const [loadingUsers, setLoadingUsers] = useState(true)
  const [loadingProjects, setLoadingProjects] = useState(false)
  const [deleteUserTarget, setDeleteUserTarget] = useState<AdminUser | null>(null)
  const [deleteProjectTarget, setDeleteProjectTarget] = useState<AdminProject | null>(null)
  const [error, setError] = useState<string | null>(null)

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
      setError(err instanceof Error ? err.message : 'Could not delete user')
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
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="sites">Sites</TabsTrigger>
        </TabsList>

        <TabsContent value="users">
          <div className="mb-4">
            <h2 className="text-lg font-semibold tracking-tight">Users</h2>
            <p className="text-sm text-muted-foreground">{users.length} total</p>
          </div>

          {loadingUsers ? (
            <p className="text-sm text-muted-foreground">Loading users...</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Email</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Joined</TableHead>
                  <TableHead className="w-24"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user) => (
                  <TableRow key={user.id}>
                    <TableCell className="font-medium">{user.email}</TableCell>
                    <TableCell>{user.name}</TableCell>
                    <TableCell className="text-muted-foreground">{user.createdAt}</TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteUserTarget(user)}
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
      </Tabs>

      <Dialog open={!!deleteUserTarget} onOpenChange={() => setDeleteUserTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete user</DialogTitle>
            <DialogDescription>
              This will permanently delete <strong>{deleteUserTarget?.email}</strong> and all their sites. This cannot be undone.
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
