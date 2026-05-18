import { Fragment, useState } from 'react'
import type { Site } from './types'
import { DeployHistory } from './DeployHistory'
import { CopyButton } from './CopyButton'
import { SiteThumbnail } from './SiteThumbnail'
import { VisibilityBadge } from './VisibilityBadge'
import { AuthorChip } from './AuthorChip'
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
import { ChevronDown, ChevronRight, CopyPlus, ExternalLink, GitBranch, Globe, Lock, MessageSquare, Trash2 } from 'lucide-react'
import { commitURL } from '@/lib/utils'
import { canMutateSite } from './site-scope'

type SitesTableProps = {
  sites: Site[]
  deletingSiteID: string | null
  showAuthor?: boolean
  currentUserId?: string
  isOrgAdmin?: boolean
  onDelete: (siteID: string) => void
  onVisibilityToggle: (siteID: string, isPublic: boolean) => void
  onDuplicate?: (siteID: string) => void
  onOpenComments?: (siteSlug: string) => void
  onSitesChanged: () => void
  onSessionExpired: () => void
}

export function SitesTable({
  sites,
  deletingSiteID,
  showAuthor = false,
  currentUserId,
  isOrgAdmin = false,
  onDelete,
  onVisibilityToggle,
  onDuplicate,
  onOpenComments,
  onSitesChanged,
  onSessionExpired,
}: SitesTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())
  const [deleteTarget, setDeleteTarget] = useState<Site | null>(null)

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
            <TableHead className="w-[88px]"><span className="sr-only">Thumbnail</span></TableHead>
            <TableHead>Name</TableHead>
            <TableHead>URL</TableHead>
            <TableHead className="text-center">Deploys</TableHead>
            <TableHead>Latest</TableHead>
            {showAuthor ? <TableHead>Author</TableHead> : null}
            <TableHead>Updated</TableHead>
            <TableHead>Visibility</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sites.map((site) => {
            const isExpanded = expandedRows.has(site.id)
            const isDeleting = deletingSiteID === site.id
            const canMutate = canMutateSite(site, currentUserId, isOrgAdmin)

            return (
              <Fragment key={site.id}>
                <TableRow>
                  <TableCell className="py-2">
                    <SiteThumbnail site={site} size="row" />
                  </TableCell>
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => toggleExpand(site.id)}
                        aria-label={isExpanded ? 'Hide history' : 'Show history'}
                      >
                        {isExpanded ? <ChevronDown /> : <ChevronRight />}
                      </Button>
                      <span className="block max-w-[200px] truncate" title={site.name}>{site.name}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <a
                        href={site.liveUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        title={site.liveUrl}
                        className="inline-flex items-center gap-1 max-w-[260px] truncate text-primary underline-offset-4 hover:underline"
                      >
                        <span className="truncate">{site.liveUrl}</span>
                        <ExternalLink className="size-3 shrink-0 text-muted-foreground" />
                      </a>
                      <CopyButton text={site.liveUrl} />
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary">{site.deployCount}</Badge>
                  </TableCell>
                  <TableCell>
                    {site.gitBranch ? (
                      <div className="flex items-center gap-1.5 text-muted-foreground">
                        <GitBranch className="size-3 shrink-0" />
                        <span className="truncate max-w-[120px]">{site.gitBranch}</span>
                        {site.gitCommitHash ? (
                          site.gitRemoteURL ? (
                            <a
                              href={commitURL(site.gitRemoteURL, site.gitCommitHash)}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="font-mono text-xs hover:underline"
                              title={site.gitCommitHash}
                            >
                              {site.gitCommitHash.slice(0, 8)}
                            </a>
                          ) : (
                            <span className="font-mono text-xs" title={site.gitCommitHash}>
                              {site.gitCommitHash.slice(0, 8)}
                            </span>
                          )
                        ) : null}
                      </div>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  {showAuthor ? (
                    <TableCell>
                      <AuthorChip author={site.createdBy} />
                    </TableCell>
                  ) : null}
                  <TableCell className="text-muted-foreground">
                    {site.updatedAt}
                  </TableCell>
                  <TableCell>
                    <VisibilityBadge isPublic={site.isPublic} />
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      {onOpenComments ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => onOpenComments(site.slug)}
                              aria-label="Comments"
                            >
                              <MessageSquare />
                              {site.openCommentCount > 0 ? (
                                <span className="ml-1 text-xs font-medium">{site.openCommentCount}</span>
                              ) : null}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>
                            {site.openCommentCount > 0 ? `${site.openCommentCount} open comment${site.openCommentCount === 1 ? '' : 's'}` : 'Comments'}
                          </TooltipContent>
                        </Tooltip>
                      ) : null}
                      {onDuplicate ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => onDuplicate(site.id)}
                              aria-label="Duplicate site"
                            >
                              <CopyPlus />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Duplicate</TooltipContent>
                        </Tooltip>
                      ) : null}
                      {canMutate ? (
                        <>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                onClick={() => onVisibilityToggle(site.id, !site.isPublic)}
                                aria-label={site.isPublic ? 'Make private' : 'Make public'}
                              >
                                {site.isPublic ? <Globe /> : <Lock />}
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>{site.isPublic ? 'Make private' : 'Make public'}</TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon-xs"
                                className="hover:bg-destructive/10 hover:text-destructive"
                                disabled={isDeleting}
                                onClick={() => setDeleteTarget(site)}
                                aria-label="Delete site"
                              >
                                <Trash2 />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Delete</TooltipContent>
                          </Tooltip>
                        </>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
                {isExpanded && (
                  <TableRow className="hover:bg-transparent">
                    <TableCell colSpan={showAuthor ? 9 : 8} className="bg-muted/30 px-4 py-3">
                      <DeployHistory
                        projectId={site.id}
                        onRollback={onSitesChanged}
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
            <DialogTitle>Delete site</DialogTitle>
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
