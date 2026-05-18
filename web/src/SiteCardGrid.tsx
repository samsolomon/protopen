import { useState } from 'react'
import type { Site } from './types'
import { DeployHistory } from './DeployHistory'
import { SiteThumbnail } from './SiteThumbnail'
import { CopyButton } from './CopyButton'
import { VisibilityBadge } from './VisibilityBadge'
import { AuthorChip } from './AuthorChip'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardAction, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { ExternalLink, Copy, CopyPlus, History, Globe, Lock, MessageSquare, Trash2, MoreHorizontal, GitBranch } from 'lucide-react'
import { toast } from 'sonner'
import { commitURL } from '@/lib/utils'
import { canMutateSite } from './site-scope'

type SiteCardGridProps = {
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

export function SiteCardGrid({
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
}: SiteCardGridProps) {
  const [historySite, setHistorySite] = useState<Site | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Site | null>(null)

  return (
    <>
      <div
        className="gap-4"
        style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))' }}
      >
        {sites.map((site) => {
          const isDeleting = deletingSiteID === site.id
          const canMutate = canMutateSite(site, currentUserId, isOrgAdmin)

          return (
            <Card key={site.id} className="relative pt-0">
              <a
                href={site.liveUrl}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={`Open ${site.name}`}
                className="absolute inset-0 z-0 rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
              />
              <SiteThumbnail site={site} />
              <CardHeader>
                <CardTitle className="truncate" title={site.name}>{site.name}</CardTitle>
                <CardAction className="relative z-10">
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={(e) => e.stopPropagation()}
                          aria-label="Site actions"
                        />
                      }
                    >
                      <MoreHorizontal />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation()
                          window.open(site.liveUrl, '_blank')
                        }}
                      >
                        <ExternalLink />
                        Open site
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation()
                          void navigator.clipboard.writeText(site.liveUrl)
                          toast.success('URL copied')
                        }}
                      >
                        <Copy />
                        Copy URL
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation()
                          setHistorySite(site)
                        }}
                      >
                        <History />
                        Deploy history
                      </DropdownMenuItem>
                      {onDuplicate ? (
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            onDuplicate(site.id)
                          }}
                        >
                          <CopyPlus />
                          Duplicate
                        </DropdownMenuItem>
                      ) : null}
                      {onOpenComments ? (
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            onOpenComments(site.slug)
                          }}
                        >
                          <MessageSquare />
                          Comments
                          {site.openCommentCount > 0 ? (
                            <span className="ml-auto text-xs text-muted-foreground">{site.openCommentCount}</span>
                          ) : null}
                        </DropdownMenuItem>
                      ) : null}
                      {canMutate ? (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={(e) => {
                              e.stopPropagation()
                              onVisibilityToggle(site.id, !site.isPublic)
                            }}
                          >
                            {site.isPublic ? <Lock /> : <Globe />}
                            {site.isPublic ? 'Make private' : 'Make public'}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            disabled={isDeleting}
                            onClick={(e) => {
                              e.stopPropagation()
                              setDeleteTarget(site)
                            }}
                          >
                            <Trash2 />
                            Delete
                          </DropdownMenuItem>
                        </>
                      ) : null}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </CardAction>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  <VisibilityBadge isPublic={site.isPublic} />
                  <Badge variant="secondary">
                    {site.deployCount} {site.deployCount === 1 ? 'deploy' : 'deploys'}
                  </Badge>
                </div>
                <div className="relative z-10 flex items-center gap-1 text-sm">
                  <a
                    href={site.liveUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    title={site.liveUrl}
                    onClick={(e) => e.stopPropagation()}
                    className="inline-flex min-w-0 flex-1 items-center gap-1 truncate text-primary underline-offset-4 hover:underline"
                  >
                    <span className="truncate">{site.liveUrl}</span>
                    <ExternalLink className="size-3 shrink-0 text-muted-foreground" />
                  </a>
                  <CopyButton text={site.liveUrl} />
                </div>
                {site.gitBranch ? (
                  <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <GitBranch className="size-3 shrink-0" />
                    <span className="truncate" title={site.gitBranch}>{site.gitBranch}</span>
                    {site.gitCommitHash ? (
                      site.gitRemoteURL ? (
                        <a
                          href={commitURL(site.gitRemoteURL, site.gitCommitHash)}
                          target="_blank"
                          rel="noopener noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="relative z-10 font-mono hover:underline"
                          title={site.gitCommitHash}
                        >
                          {site.gitCommitHash.slice(0, 8)}
                        </a>
                      ) : (
                        <span className="font-mono" title={site.gitCommitHash}>
                          {site.gitCommitHash.slice(0, 8)}
                        </span>
                      )
                    ) : null}
                  </div>
                ) : null}
                <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span className="truncate">{site.updatedAt}</span>
                  {showAuthor ? (
                    <AuthorChip author={site.createdBy} className="relative z-10 min-w-0" />
                  ) : null}
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>

      <Dialog open={historySite !== null} onOpenChange={(open) => { if (!open) setHistorySite(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{historySite?.name}</DialogTitle>
            <DialogDescription>Deploy history and rollback</DialogDescription>
          </DialogHeader>
          {historySite ? (
            <DeployHistory
              projectId={historySite.id}
              onRollback={onSitesChanged}
              onSessionExpired={onSessionExpired}
            />
          ) : null}
        </DialogContent>
      </Dialog>

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
    </>
  )
}
