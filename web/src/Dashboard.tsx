import { useEffect, useRef, useState } from 'react'
import type { Site, SessionUser } from './types'
import { resendVerification } from './api'
import { UploadPanel } from './UploadPanel'
import { SiteCardGrid } from './SiteCardGrid'
import { SitesTable } from './SitesTable'
import { TokensPanel } from './TokensPanel'
import { ProfilePanel } from './ProfilePanel'
import { PasswordPanel } from './PasswordPanel'
import { AppearancePanel } from './AppearancePanel'
import { DeleteAccountPanel } from './DeleteAccountPanel'
import { OrgMembersPanel } from './OrgMembersPanel'
import { AdminPanel } from './AdminPanel'
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
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { Plus, Upload, Mail, Terminal, Copy, Check, LayoutGrid, List } from 'lucide-react'

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current) }, [])
  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-7 shrink-0"
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true)
          if (timerRef.current) clearTimeout(timerRef.current)
          timerRef.current = setTimeout(() => setCopied(false), 2000)
        })
      }}
    >
      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
    </Button>
  )
}

const installCommand = 'curl -fsSL https://app.velori.dev/install.sh | bash'

function EmptyState({ onUploadOpen }: { onUploadOpen: () => void }) {
  return (
    <Card className="bg-muted border-0 shadow-none">
      <CardContent className="py-10">
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Terminal />
            </EmptyMedia>
            <EmptyTitle>No sites yet</EmptyTitle>
            <EmptyDescription>
              Install the Velori skill, then tell your agent to deploy.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <div className="flex items-center gap-2 rounded-lg bg-background px-3 py-2">
              <code className="font-mono text-xs text-muted-foreground">{installCommand}</code>
              <CopyButton text={installCommand} />
            </div>
          </EmptyContent>
          <button
            type="button"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors"
            onClick={onUploadOpen}
          >
            or drag and drop
          </button>
        </Empty>
      </CardContent>
    </Card>
  )
}

type DashboardView = 'dashboard' | 'settings' | 'admin'

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
  const [view, setView] = useState<DashboardView>(() => {
    return window.location.pathname.startsWith('/settings') ? 'settings' : 'dashboard'
  })
  const [settingsTab, setSettingsTab] = useState(() => {
    const path = window.location.pathname
    if (path === '/settings/appearance') return 'appearance'
    if (path === '/settings/team') return 'team'
    if (path === '/settings/tokens') return 'tokens'
    return 'account'
  })
  const [uploadOpen, setUploadOpen] = useState(false)
  const [pendingFiles, setPendingFiles] = useState<FileList | null>(null)
  const [verificationSent, setVerificationSent] = useState(false)
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() =>
    localStorage.getItem('velori-project-view') === 'list' ? 'list' : 'grid'
  )

  const currentOrg = user.orgs.find(o => o.isPersonal)
  const plan = currentOrg?.plan ?? 'tinkerer'
  const canMakePrivate = plan === 'pro' || plan === 'team'
  const canRollback = plan === 'pro' || plan === 'team'

  const navigateTo = (path: string) => {
    window.history.pushState(null, '', path)
  }

  const switchView = (next: DashboardView) => {
    if (next === 'settings') {
      setSettingsTab('account')
      navigateTo('/settings')
    } else {
      navigateTo('/')
    }
    setView(next)
    window.scrollTo(0, 0)
  }

  const switchSettingsTab = (tab: string) => {
    setSettingsTab(tab)
    navigateTo(tab === 'account' ? '/settings' : `/settings/${tab}`)
  }

  useEffect(() => {
    const onPopState = () => {
      const path = window.location.pathname
      if (path.startsWith('/settings')) {
        setView('settings')
        if (path === '/settings/appearance') setSettingsTab('appearance')
        else if (path === '/settings/team') setSettingsTab('team')
        else if (path === '/settings/tokens') setSettingsTab('tokens')
        else setSettingsTab('account')
      } else {
        setView('dashboard')
      }
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const handleUploadOpenChange = (open: boolean) => {
    setUploadOpen(open)
    if (!open) setPendingFiles(null)
  }

  return (
    <div className="min-h-svh bg-background">
      <header className="sticky top-0 z-50 bg-background/70 backdrop-blur-[40px] backdrop-saturate-150">
        <div className="mx-auto grid h-14 max-w-[1440px] grid-cols-3 items-center px-4">
          <div className="flex items-center">
            <a href="/" className="text-xl font-semibold tracking-tight hover:opacity-70 transition-opacity" onClick={(e) => { e.preventDefault(); switchView('dashboard') }}>Velori</a>
          </div>
          <div />
          <div className="flex items-center justify-end gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost">{user.name}</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onClick={() => switchView('settings')}>
                Settings
              </DropdownMenuItem>
              {user.isAdmin && (
                <DropdownMenuItem onClick={() => switchView('admin')}>
                  Admin
                </DropdownMenuItem>
              )}
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

        {view === 'admin' && user.isAdmin ? (
          <AdminPanel onSessionExpired={onSessionExpired} />
        ) : view === 'settings' ? (
          <Tabs value={settingsTab} onValueChange={switchSettingsTab} orientation="vertical" className="gap-8">
            <TabsList variant="line" className="w-full sm:w-48 flex-shrink-0">
              <TabsTrigger value="account">Account</TabsTrigger>
              <TabsTrigger value="appearance">Appearance</TabsTrigger>
              <TabsTrigger value="team">Team</TabsTrigger>
              <TabsTrigger value="tokens">API Tokens</TabsTrigger>
            </TabsList>
            <TabsContent value="account" className="max-w-2xl">
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
            <TabsContent value="team" className="max-w-2xl">
              <OrgMembersPanel
                user={user}
                onSessionExpired={onSessionExpired}
              />
            </TabsContent>
            <TabsContent value="tokens" className="max-w-2xl">
              <TokensPanel
                onSessionExpired={onSessionExpired}
              />
            </TabsContent>
          </Tabs>
        ) : (
          <div className="flex flex-col gap-8">
            <section>
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-lg font-semibold tracking-tight">Your sites</h2>
                <div className="flex items-center gap-2">
                  {sites.length > 0 ? (
                    <div className="flex gap-0.5 rounded-lg bg-muted p-0.5">
                      {([['grid', LayoutGrid], ['list', List]] as const).map(([mode, Icon]) => (
                        <button
                          key={mode}
                          type="button"
                          onClick={() => {
                            setViewMode(mode)
                            localStorage.setItem('velori-project-view', mode)
                          }}
                          className={`flex cursor-pointer items-center rounded-md border p-1.5 transition-colors ${
                            viewMode === mode
                              ? 'border-primary bg-primary/5 text-primary'
                              : 'border-transparent text-muted-foreground hover:text-foreground'
                          }`}
                          aria-label={mode === 'grid' ? 'Grid view' : 'List view'}
                        >
                          <Icon className="size-3.5" />
                        </button>
                      ))}
                    </div>
                  ) : null}
                  <Button onClick={() => setUploadOpen(true)}>
                    <Plus />
                    Create site
                  </Button>
                </div>
              </div>
              {isLoading ? (
                <Card>
                  <CardContent className="py-8 text-center text-sm text-muted-foreground">
                    Loading sites...
                  </CardContent>
                </Card>
              ) : sites.length === 0 ? (
                <EmptyState onUploadOpen={() => setUploadOpen(true)} />
              ) : (
                viewMode === 'grid' ? (
                  <SiteCardGrid
                    sites={sites}
                    deletingSiteID={deletingSiteID}
                    onDelete={onDeleteSite}
                    onVisibilityToggle={onVisibilityToggle}
                    onSitesChanged={onSitesChanged}
                    onSessionExpired={onSessionExpired}
                    canMakePrivate={canMakePrivate}
                    canRollback={canRollback}
                  />
                ) : (
                  <SitesTable
                    sites={sites}
                    deletingSiteID={deletingSiteID}
                    onDelete={onDeleteSite}
                    onVisibilityToggle={onVisibilityToggle}
                    onSitesChanged={onSitesChanged}
                    onSessionExpired={onSessionExpired}
                    canMakePrivate={canMakePrivate}
                    canRollback={canRollback}
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
