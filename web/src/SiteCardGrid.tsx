import { useState } from 'react'
import type { Site } from './types'
import { DeployHistory } from './DeployHistory'
import { Button } from '@/components/ui/button'
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
import { ExternalLink, Copy, History, Globe, Lock, Trash2, MoreHorizontal } from 'lucide-react'
import { toast } from 'sonner'

const HUES = [15, 45, 145, 200, 265, 330, 175, 55]

function hashCode(str: string): number {
  let h = 0
  for (let i = 0; i < str.length; i++) {
    h = ((h << 5) - h + str.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

function getColor(id: string) {
  const hue = HUES[hashCode(id) % HUES.length]
  return {
    bg: `oklch(0.75 0.12 ${hue})`,
    text: `oklch(0.98 0.01 ${hue})`,
  }
}

function getInitials(name: string): string {
  const words = name.split(/[\s\-_.\d]+/).filter(Boolean)
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase()
  const w = words[0] || name
  return w.slice(0, 2).toUpperCase()
}

type SiteCardGridProps = {
  sites: Site[]
  deletingSiteID: string | null
  onDelete: (siteID: string) => void
  onVisibilityToggle: (siteID: string, isPublic: boolean) => void
  onSitesChanged: () => void
  onSessionExpired: () => void
}

export function SiteCardGrid({
  sites,
  deletingSiteID,
  onDelete,
  onVisibilityToggle,
  onSitesChanged,
  onSessionExpired,
}: SiteCardGridProps) {
  const [historySite, setHistorySite] = useState<Site | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Site | null>(null)

  return (
    <>
      <div
        className="gap-4"
        style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))' }}
      >
        {sites.map((site) => {
          const color = getColor(site.id)
          const initials = getInitials(site.name)
          const isDeleting = deletingSiteID === site.id

          return (
            <Card
              key={site.id}
              className="cursor-pointer pt-0"
              onClick={() => window.open(site.liveUrl, '_blank')}
            >
              <div
                className="flex aspect-[16/10] items-center justify-center rounded-t-xl"
                style={{ background: color.bg }}
              >
                <span
                  className="text-2xl font-semibold select-none"
                  style={{ color: color.text }}
                >
                  {initials}
                </span>
              </div>
              <CardHeader>
                <CardTitle className="truncate">{site.name}</CardTitle>
                <CardAction>
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
                    </DropdownMenuContent>
                  </DropdownMenu>
                </CardAction>
              </CardHeader>
              <CardContent>
                <p className="text-xs text-muted-foreground">{site.updatedAt}</p>
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
