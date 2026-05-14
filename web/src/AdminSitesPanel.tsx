import { useEffect, useState } from 'react'
import type { AdminProject } from './api'
import { fetchAdminProjects, adminDeleteProject, SessionExpiredError } from './api'
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
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'

type AdminSitesPanelProps = {
  onSessionExpired: () => void
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

export function AdminSitesPanel({ onSessionExpired }: AdminSitesPanelProps) {
  const [projects, setProjects] = useState<AdminProject[]>([])
  const [loading, setLoading] = useState(true)
  const [deleteTarget, setDeleteTarget] = useState<AdminProject | null>(null)

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const load = async () => {
    try {
      setLoading(true)
      const data = await fetchAdminProjects()
      setProjects(data)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not load sites')
    } finally {
      setLoading(false)
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    try {
      await adminDeleteProject(deleteTarget.id)
      setProjects((prev) => prev.filter((p) => p.id !== deleteTarget.id))
      setDeleteTarget(null)
    } catch (err) {
      if (err instanceof SessionExpiredError) { onSessionExpired(); return }
      toast.error(err instanceof Error ? err.message : 'Could not delete site')
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">All sites</h2>
        <p className="text-sm text-muted-foreground">{projects.length} total</p>
      </div>

      {loading ? (
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
                    onClick={() => setDeleteTarget(project)}
                  >
                    Delete
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete site</DialogTitle>
            <DialogDescription>
              This will delete <strong>{deleteTarget?.name}</strong> ({deleteTarget?.orgName}). This cannot be undone.
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
