import { Fragment, useEffect, useRef, useState } from 'react'
import type { Project } from './types'
import { DeployHistory } from './DeployHistory'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Check, ChevronDown, ChevronRight, Copy, ExternalLink, Globe, Lock, Trash2 } from 'lucide-react'

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout>>(null)
  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current) }, [])
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => {
            void navigator.clipboard.writeText(text)
            setCopied(true)
            if (timerRef.current) clearTimeout(timerRef.current)
            timerRef.current = setTimeout(() => setCopied(false), 2000)
          }}
          aria-label="Copy URL"
        >
          {copied ? <Check className="text-muted-foreground" /> : <Copy />}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{copied ? 'Copied!' : 'Copy URL'}</TooltipContent>
    </Tooltip>
  )
}

type ProjectsTableProps = {
  projects: Project[]
  deletingProjectID: string | null
  onDelete: (projectID: string) => void
  onVisibilityToggle: (projectID: string, isPublic: boolean) => void
  onProjectsChanged: () => void
  onSessionExpired: () => void
}

export function ProjectsTable({
  projects,
  deletingProjectID,
  onDelete,
  onVisibilityToggle,
  onProjectsChanged,
  onSessionExpired,
}: ProjectsTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())
  const [deleteTarget, setDeleteTarget] = useState<Project | null>(null)

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
                    <div className="flex items-center gap-1">
                      <a
                        href={project.liveUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 max-w-[260px] truncate text-primary underline-offset-4 hover:underline"
                      >
                        <span className="truncate">{project.liveUrl}</span>
                        <ExternalLink className="size-3 shrink-0 text-muted-foreground" />
                      </a>
                      <CopyButton text={project.liveUrl} />
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary">{project.deployCount}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {project.updatedAt}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            onClick={() => onVisibilityToggle(project.id, !project.isPublic)}
                            aria-label={project.isPublic ? 'Make private' : 'Make public'}
                          >
                            {project.isPublic ? <Globe /> : <Lock />}
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{project.isPublic ? 'Make private' : 'Make public'}</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            className="hover:bg-destructive/10 hover:text-destructive"
                            disabled={isDeleting}
                            onClick={() => setDeleteTarget(project)}
                            aria-label="Delete project"
                          >
                            <Trash2 />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Delete</TooltipContent>
                      </Tooltip>
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
      <Dialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete project</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete <span className="font-medium text-foreground">{deleteTarget?.name}</span>? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (deleteTarget) void onDelete(deleteTarget.id)
                setDeleteTarget(null)
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
