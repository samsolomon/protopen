import { useState } from 'react'
import type { Project } from './types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DeployHistory } from './DeployHistory'
import { toast } from 'sonner'

type ProjectCardProps = {
  project: Project
  isDeleting: boolean
  onDelete: (projectID: string) => void
  onVisibilityToggle: (projectID: string, isPublic: boolean) => void
  onProjectsChanged: () => void
  onSessionExpired: () => void
}

export function ProjectCard({ project, isDeleting, onDelete, onVisibilityToggle, onProjectsChanged, onSessionExpired }: ProjectCardProps) {
  const [showHistory, setShowHistory] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center justify-between">
          <span className="truncate">{project.name}</span>
          <Badge variant="secondary">{project.deployCount} deploys</Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-1">
        <p className="text-sm text-muted-foreground">{project.updatedAt}</p>
        <a
          href={project.liveUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="text-sm text-primary underline-offset-4 hover:underline truncate"
        >
          {project.liveUrl}
        </a>
        {showHistory ? (
          <div className="mt-2">
            <DeployHistory
              projectId={project.id}
              onRollback={onProjectsChanged}
              onSessionExpired={onSessionExpired}
            />
          </div>
        ) : null}
      </CardContent>
      <CardFooter className="gap-2">
        <Button variant="ghost" size="sm" onClick={() => onVisibilityToggle(project.id, !project.isPublic)}>
          {project.isPublic ? 'Public' : 'Private'}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setShowHistory(!showHistory)}>
          {showHistory ? 'Hide history' : 'History'}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            void navigator.clipboard.writeText(project.liveUrl)
            toast('URL copied to clipboard')
          }}
        >
          Copy URL
        </Button>
        <Button
          variant="destructive"
          size="sm"
          disabled={isDeleting}
          onClick={() => setConfirmDelete(true)}
        >
          {isDeleting ? 'Deleting...' : 'Delete'}
        </Button>
      </CardFooter>
      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete project</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete <span className="font-medium text-foreground">{project.name}</span>? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>Cancel</Button>
            <Button
              variant="destructive"
              onClick={() => {
                void onDelete(project.id)
                setConfirmDelete(false)
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
