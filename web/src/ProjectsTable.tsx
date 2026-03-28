import { Fragment, useState } from 'react'
import type { Project } from './types'
import { DeployHistory } from './DeployHistory'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ChevronDown, ChevronRight, Copy, Globe, Layers, Lock, Trash2 } from 'lucide-react'
import { toast } from 'sonner'

type ProjectsTableProps = {
  projects: Project[]
  deletingProjectID: string | null
  onDelete: (projectID: string) => void
  onVisibilityToggle: (projectID: string, isPublic: boolean) => void
  onProjectsChanged: () => void
  onSessionExpired: () => void
  onViewVersions: (project: Project) => void
}

export function ProjectsTable({
  projects,
  deletingProjectID,
  onDelete,
  onVisibilityToggle,
  onProjectsChanged,
  onSessionExpired,
  onViewVersions,
}: ProjectsTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())

  const toggleExpand = (id: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  return (
    <div className="overflow-hidden rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/50 hover:bg-muted/50">
            <TableHead>Name</TableHead>
            <TableHead>URL</TableHead>
            <TableHead className="text-center">Deploys</TableHead>
            <TableHead>Updated</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {projects.map((project) => {
            const isExpanded = expandedRows.has(project.id)
            const isDeleting = deletingProjectID === project.id

            return (
              <Fragment key={project.id}>
                <TableRow>
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => toggleExpand(project.id)}
                        aria-label={isExpanded ? 'Hide history' : 'Show history'}
                      >
                        {isExpanded ? <ChevronDown /> : <ChevronRight />}
                      </Button>
                      <span className="block max-w-[200px] truncate">{project.name}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <a
                      href={project.liveUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="block max-w-[260px] truncate text-primary underline-offset-4 hover:underline"
                    >
                      {project.liveUrl}
                    </a>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary">{project.deployCount}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {project.updatedAt}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => onVisibilityToggle(project.id, !project.isPublic)}
                        aria-label={project.isPublic ? 'Make private' : 'Make public'}
                      >
                        {project.isPublic ? <Globe /> : <Lock />}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => onViewVersions(project)}
                        aria-label="View versions"
                      >
                        <Layers />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => {
                          void navigator.clipboard.writeText(project.liveUrl)
                          toast('URL copied to clipboard')
                        }}
                        aria-label="Copy URL"
                      >
                        <Copy />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                        disabled={isDeleting}
                        onClick={() => void onDelete(project.id)}
                        aria-label="Delete project"
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
                {isExpanded && (
                  <TableRow className="hover:bg-transparent">
                    <TableCell colSpan={5} className="bg-muted/30 px-4 py-3">
                      <DeployHistory
                        projectId={project.id}
                        onRollback={onProjectsChanged}
                        onSessionExpired={onSessionExpired}
                      />
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
