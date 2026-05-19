import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import type { Site, SessionUser } from './types'
import { duplicateSite, resendVerification } from './api'
import {
  scopeFromSearch,
  buildScopeURL,
  filterSitesByScope,
  filterSitesByQuery,
  type SiteScope,
} from './site-scope'
import { UploadPanel } from './UploadPanel'
import { SiteCardGrid } from './SiteCardGrid'
import { SitesTable } from './SitesTable'
import { ProfilePanel } from './ProfilePanel'
import { PasswordPanel } from './PasswordPanel'
import { AppearancePanel } from './AppearancePanel'
import { DeleteAccountPanel } from './DeleteAccountPanel'
import { AdminUsersPanel } from './AdminUsersPanel'
import { AdminSettings } from './AdminSettings'
import { InboxPage } from './InboxPage'
import { CommentsOverlay } from './CommentsOverlay'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { Input } from '@/components/ui/input'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Plus, Upload, Mail, LayoutGrid, List, Search } from 'lucide-react'

function EmptyState({ onUploadOpen }: { onUploadOpen: () => void }) {
  return (
    <Empty className="bg-muted py-10">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Upload />
        </EmptyMedia>
        <EmptyTitle>No sites yet</EmptyTitle>
        <EmptyDescription>
          Upload a folder or zip to deploy your first site.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button onClick={onUploadOpen}>
          <Plus />
          Create site
        </Button>
      </EmptyContent>
    </Empty>
  )
}

function EmptyMineState({ onViewAll, onUploadOpen }: { onViewAll: () => void; onUploadOpen: () => void }) {
  return (
    <Empty className="bg-muted py-10">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Upload />
        </EmptyMedia>
        <EmptyTitle>You haven't created any sites</EmptyTitle>
        <EmptyDescription>
          Switch to All sites to see your teammates' work, or upload your first deploy.
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button variant="outline" onClick={onViewAll}>View all sites</Button>
        <Button onClick={onUploadOpen}>
          <Plus />
          Create site
        </Button>
      </EmptyContent>
    </Empty>
  )
}

function writeScopeToURL(scope: SiteScope) {
  window.history.replaceState(null, '', buildScopeURL(window.location.href, scope))
}

type DashboardView = 'dashboard' | 'settings' | 'inbox' | 'comments'

function settingsTabFromPath(path: string, isAdmin: boolean): string {
  if (path === '/settings/appearance') return 'appearance'
  if (path === '/settings/users') return isAdmin ? 'users' : 'profile'
  if (path === '/settings/instance') return isAdmin ? 'instance' : 'profile'
  return 'profile'
}

const COMMENTS_PATH_RE = /^\/sites\/([^/]+)\/comments\/?$/

function viewFromPath(path: string): { view: DashboardView; commentsSlug?: string } {
  if (path.startsWith('/settings')) return { view: 'settings' }
  if (path === '/inbox') return { view: 'inbox' }
  const match = COMMENTS_PATH_RE.exec(path)
  if (match) return { view: 'comments', commentsSlug: match[1] }
  return { view: 'dashboard' }
}

type DashboardProps = {
  user: SessionUser
  sites: Site[]
  isLoading: boolean
  deletingSiteID: string | null
  error: string | null
  setError: (error: string | null) => void
  onSignOut: () => void
  onUserUpdated: (user: SessionUser) => void
  onDeleteSite: (siteID: string) => void
  onVisibilityToggle: (siteID: string, isPublic: boolean) => void
  onSitesChanged: () => void
  onSessionExpired: () => void
}

export function Dashboard({
  user,
  sites,
  isLoading,
  deletingSiteID,
  error,
  setError,
  onSignOut,
  onUserUpdated,
  onDeleteSite,
  onVisibilityToggle,
  onSitesChanged,
  onSessionExpired,
}: DashboardProps) {
  const initialRoute = viewFromPath(window.location.pathname)
  const [view, setView] = useState<DashboardView>(initialRoute.view)
  const [commentsSlug, setCommentsSlug] = useState<string | null>(initialRoute.commentsSlug ?? null)
  const [settingsTab, setSettingsTab] = useState(() =>
    settingsTabFromPath(window.location.pathname, !!user.isAdmin)
  )
  const [uploadOpen, setUploadOpen] = useState(false)
  const [pendingFiles, setPendingFiles] = useState<FileList | null>(null)
  const [verificationSent, setVerificationSent] = useState(false)
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() =>
    localStorage.getItem('protopen-site-view') === 'list' ? 'list' : 'grid'
  )
  const [scope, setScopeState] = useState<SiteScope>(() => scopeFromSearch(window.location.search))
  const [searchQuery, setSearchQuery] = useState('')

  const setScope = (next: SiteScope) => {
    setScopeState(next)
    writeScopeToURL(next)
  }

  const hasTeammateSites = sites.some(
    (site) => site.createdBy && site.createdBy.id !== user.id,
  )
  const effectiveScope: SiteScope = hasTeammateSites ? scope : 'all'
  const scopedSites = filterSitesByScope(sites, effectiveScope, user.id)
  const visibleSites = filterSitesByQuery(scopedSites, searchQuery)

  const activeOrg = user.orgs.find((org) => org.isPersonal) ?? user.orgs[0]
  const isOrgAdmin = activeOrg?.role === 'admin'

  const handleDuplicate = async (siteID: string) => {
    try {
      const site = await duplicateSite(siteID)
      toast.success(`Duplicated as "${site.name}"`)
      onSitesChanged()
      if (hasTeammateSites && scope !== 'mine') {
        setScope('mine')
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Could not duplicate site'
      toast.error(message)
    }
  }

  const navigateTo = (path: string) => {
    window.history.pushState(null, '', path)
  }

  const switchView = (next: DashboardView) => {
    if (next === 'settings') {
      setSettingsTab('profile')
      navigateTo('/settings')
    } else if (next === 'inbox') {
      navigateTo('/inbox')
    } else if (next !== 'comments') {
      navigateTo('/')
    }
    setView(next)
    window.scrollTo(0, 0)
  }

  const openComments = (siteSlug: string, focusCommentID?: string) => {
    setCommentsSlug(siteSlug)
    setView('comments')
    const url = focusCommentID
      ? `/sites/${siteSlug}/comments?focus=${encodeURIComponent(focusCommentID)}`
      : `/sites/${siteSlug}/comments`
    navigateTo(url)
    window.scrollTo(0, 0)
  }

  const switchSettingsTab = (tab: string) => {
    setSettingsTab(tab)
    navigateTo(tab === 'profile' ? '/settings' : `/settings/${tab}`)
  }

  useEffect(() => {
    const onPopState = () => {
      const path = window.location.pathname
      const route = viewFromPath(path)
      setView(route.view)
      if (route.view === 'settings') {
        setSettingsTab(settingsTabFromPath(path, !!user.isAdmin))
      }
      if (route.view === 'comments') {
        setCommentsSlug(route.commentsSlug ?? null)
      } else {
        setCommentsSlug(null)
      }
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [user.isAdmin])

  const handleUploadOpenChange = (open: boolean) => {
    setUploadOpen(open)
    if (!open) setPendingFiles(null)
  }

  if (view === 'comments' && commentsSlug) {
    const target = sites.find((s) => s.slug === commentsSlug)
    if (!target) {
      // The site list hasn't loaded yet (or doesn't include this slug). Render a
      // minimal placeholder rather than the dashboard chrome — keeps the URL
      // honest while waiting.
      return (
        <div className="flex h-svh items-center justify-center text-sm text-muted-foreground">
          Loading site...
        </div>
      )
    }
    const params = new URLSearchParams(window.location.search)
    const focus = params.get('focus')
    return (
      <CommentsOverlay
        orgSlug={target.orgSlug}
        siteSlug={target.slug}
        siteName={target.name}
        focusCommentID={focus}
        onBack={() => switchView('dashboard')}
      />
    )
  }

  return (
    <div className="min-h-svh bg-background">
      <header className="sticky top-0 z-50 bg-background/70 backdrop-blur-[40px] backdrop-saturate-150">
        <div className="mx-auto grid h-14 max-w-[1440px] grid-cols-3 items-center px-4">
          <div className="flex items-center">
            <a href="/" className="text-xl font-semibold tracking-tight hover:opacity-70 transition-opacity" onClick={(e) => { e.preventDefault(); switchView('dashboard') }}>Protopen</a>
          </div>
          <nav className="flex items-center justify-center gap-1">
            <button
              type="button"
              onClick={() => switchView('dashboard')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                view === 'dashboard'
                  ? 'bg-muted text-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
              aria-current={view === 'dashboard' ? 'page' : undefined}
            >
              Sites
            </button>
            <button
              type="button"
              onClick={() => switchView('inbox')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                view === 'inbox'
                  ? 'bg-muted text-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
              aria-current={view === 'inbox' ? 'page' : undefined}
            >
              Inbox
            </button>
          </nav>
          <div className="flex items-center justify-end gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost">{user.name}</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onClick={() => switchView('settings')}>
                Settings
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => void onSignOut()}>
                Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          </div>
        </div>
      </header>

      <Dialog open={uploadOpen} onOpenChange={handleUploadOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Deploy</DialogTitle>
            <DialogDescription>Upload a folder or zip of static files to get a live URL.</DialogDescription>
          </DialogHeader>
          <UploadPanel
            error={error}
            setError={setError}
            onProjectsChanged={onSitesChanged}
            onSessionExpired={onSessionExpired}
            onClose={() => handleUploadOpenChange(false)}
            initialFiles={pendingFiles}
          />
        </DialogContent>
      </Dialog>

      <main
        className="mx-auto max-w-[1440px] px-4 py-8"
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault()
          if (e.dataTransfer.files.length > 0) {
            setPendingFiles(e.dataTransfer.files)
            setUploadOpen(true)
          }
        }}
      >
        {!user.emailVerifiedAt ? (
          <div className="mb-6 flex items-center gap-3 rounded-lg border border-border bg-muted/50 px-4 py-3">
            <Mail className="size-4 shrink-0 text-muted-foreground" />
            <p className="flex-1 text-sm text-muted-foreground">
              Check your email to verify your account.
            </p>
            <Button
              variant="ghost"
              size="sm"
              disabled={verificationSent}
              onClick={() => {
                setVerificationSent(true)
                void resendVerification().catch(() => setVerificationSent(false))
              }}
            >
              {verificationSent ? 'Sent' : 'Resend'}
            </Button>
          </div>
        ) : null}

        {view === 'inbox' ? (
          <InboxPage
            onSessionExpired={onSessionExpired}
            onOpenComment={(_orgSlug, siteSlug, commentId) => openComments(siteSlug, commentId)}
          />
        ) : view === 'settings' ? (
          <Tabs value={settingsTab} onValueChange={switchSettingsTab} orientation="vertical" className="gap-8">
            <TabsList variant="line" className="w-full sm:w-48 flex-shrink-0">
              <TabsTrigger value="profile">Profile</TabsTrigger>
              <TabsTrigger value="appearance">Appearance</TabsTrigger>
              {user.isAdmin && <TabsTrigger value="users">Users</TabsTrigger>}
              {user.isAdmin && <TabsTrigger value="instance">Instance</TabsTrigger>}
            </TabsList>
            <TabsContent value="profile" className="max-w-2xl">
              <div className="flex flex-col gap-8">
                <ProfilePanel
                  user={user}
                  onUserUpdated={onUserUpdated}
                  onSessionExpired={onSessionExpired}
                />
                <PasswordPanel onSessionExpired={onSessionExpired} />
                <DeleteAccountPanel
                  onAccountDeleted={onSignOut}
                  onSessionExpired={onSessionExpired}
                />
              </div>
            </TabsContent>
            <TabsContent value="appearance" className="max-w-2xl">
              <AppearancePanel />
            </TabsContent>
            {user.isAdmin && (
              <>
                <TabsContent value="users" className="min-w-0 flex-1">
                  <AdminUsersPanel user={user} onSessionExpired={onSessionExpired} />
                </TabsContent>
                <TabsContent value="instance" className="max-w-2xl">
                  <AdminSettings onSessionExpired={onSessionExpired} />
                </TabsContent>
              </>
            )}
          </Tabs>
        ) : (
          <div className="flex flex-col gap-8">
            <section>
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-lg font-semibold tracking-tight">Sites</h2>
                <Button onClick={() => setUploadOpen(true)}>
                  <Plus />
                  Create site
                </Button>
              </div>
              {sites.length > 0 ? (
                <div className="mb-4 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-3">
                    {hasTeammateSites ? (
                      <Tabs value={scope} onValueChange={(value) => setScope(value as SiteScope)}>
                        <TabsList>
                          <TabsTrigger value="mine">My sites</TabsTrigger>
                          <TabsTrigger value="all">All sites</TabsTrigger>
                        </TabsList>
                      </Tabs>
                    ) : null}
                    <div className="relative">
                      <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
                      <Input
                        type="search"
                        value={searchQuery}
                        onChange={(e) => setSearchQuery(e.target.value)}
                        placeholder="Search sites"
                        aria-label="Search sites"
                        className="h-8 w-48 pl-7"
                      />
                    </div>
                  </div>
                  <Tabs
                    value={viewMode}
                    onValueChange={(value) => {
                      const next = value as 'grid' | 'list'
                      setViewMode(next)
                      localStorage.setItem('protopen-site-view', next)
                    }}
                  >
                    <TabsList>
                      <TabsTrigger value="grid" aria-label="Grid view">
                        <LayoutGrid />
                      </TabsTrigger>
                      <TabsTrigger value="list" aria-label="List view">
                        <List />
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                </div>
              ) : null}
              {isLoading ? (
                <Card>
                  <CardContent className="py-8 text-center text-sm text-muted-foreground">
                    Loading sites...
                  </CardContent>
                </Card>
              ) : sites.length === 0 ? (
                <EmptyState onUploadOpen={() => setUploadOpen(true)} />
              ) : visibleSites.length === 0 ? (
                searchQuery.trim() ? (
                  <Card>
                    <CardContent className="py-8 text-center text-sm text-muted-foreground">
                      No sites match "{searchQuery}".
                    </CardContent>
                  </Card>
                ) : (
                  <EmptyMineState
                    onViewAll={() => setScope('all')}
                    onUploadOpen={() => setUploadOpen(true)}
                  />
                )
              ) : (
                viewMode === 'grid' ? (
                  <SiteCardGrid
                    sites={visibleSites}
                    deletingSiteID={deletingSiteID}
                    showAuthor={effectiveScope === 'all'}
                    currentUserId={user.id}
                    isOrgAdmin={isOrgAdmin}
                    onDelete={onDeleteSite}
                    onVisibilityToggle={onVisibilityToggle}
                    onDuplicate={handleDuplicate}
                    onOpenComments={(slug) => openComments(slug)}
                    onSitesChanged={onSitesChanged}
                    onSessionExpired={onSessionExpired}
                  />
                ) : (
                  <SitesTable
                    sites={visibleSites}
                    deletingSiteID={deletingSiteID}
                    showAuthor={effectiveScope === 'all'}
                    currentUserId={user.id}
                    isOrgAdmin={isOrgAdmin}
                    onDelete={onDeleteSite}
                    onVisibilityToggle={onVisibilityToggle}
                    onDuplicate={handleDuplicate}
                    onOpenComments={(slug) => openComments(slug)}
                    onSitesChanged={onSitesChanged}
                    onSessionExpired={onSessionExpired}
                  />
                )
              )}
            </section>

          </div>
        )}
      </main>
    </div>
  )
}
