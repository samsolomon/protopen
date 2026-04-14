import { useState } from 'react'
import type { Project, SessionUser } from './types'
import { resendVerification } from './api'
import { UploadPanel } from './UploadPanel'
import { ProjectCard } from './ProjectCard'
import { ProjectsTable } from './ProjectsTable'
import { TokensPanel } from './TokensPanel'
import { ProfilePanel } from './ProfilePanel'
import { PasswordPanel } from './PasswordPanel'
import { AppearancePanel } from './AppearancePanel'
import { DeleteAccountPanel } from './DeleteAccountPanel'
import { OrgMembersPanel } from './OrgMembersPanel'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
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
import { Plus, Terminal, Upload, FolderOpen, Mail } from 'lucide-react'

type DashboardView = 'dashboard' | 'settings'

type DashboardProps = {
  user: SessionUser
  projects: Project[]
  isLoading: boolean
  deletingProjectID: string | null
  error: string | null
  setError: (error: string | null) => void
  onSignOut: () => void
  onUserUpdated: (user: SessionUser) => void
  onDeleteProject: (projectID: string) => void
  onVisibilityToggle: (projectID: string, isPublic: boolean) => void
  onProjectsChanged: () => void
  onSessionExpired: () => void
}

export function Dashboard({
  user,
  projects,
  isLoading,
  deletingProjectID,
  error,
  setError,
  onSignOut,
  onUserUpdated,
  onDeleteProject,
  onVisibilityToggle,
  onProjectsChanged,
  onSessionExpired,
}: DashboardProps) {
  const [view, setView] = useState<DashboardView>('dashboard')
  const [settingsTab, setSettingsTab] = useState('account')
  const [uploadOpen, setUploadOpen] = useState(false)
  const [pendingFiles, setPendingFiles] = useState<FileList | null>(null)
  const [verificationSent, setVerificationSent] = useState(false)

  const switchView = (next: DashboardView) => {
    if (next === 'settings') setSettingsTab('account')
    setView(next)
    window.scrollTo(0, 0)
  }

  const handleUploadOpenChange = (open: boolean) => {
    setUploadOpen(open)
    if (!open) setPendingFiles(null)
  }

  return (
    <div className="min-h-svh bg-background">
      <header className="sticky top-0 z-50 bg-background/70 backdrop-blur-[40px] backdrop-saturate-150">
        <div className="mx-auto grid h-14 max-w-[1440px] grid-cols-3 items-center px-4">
          <div className="flex items-center">
            <h1 className="text-xl font-semibold tracking-tight">Velori</h1>
          </div>
          <nav className="flex items-center justify-center gap-1">
            <Button
              variant="ghost"
              size="default"
              className={view === 'dashboard' ? 'bg-muted' : ''}
              onClick={() => switchView('dashboard')}
            >
              Projects
            </Button>
            <Button
              variant="ghost"
              size="default"
              className={view === 'settings' ? 'bg-muted' : ''}
              onClick={() => switchView('settings')}
            >
              Settings
            </Button>
          </nav>
          <div className="flex items-center justify-end gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost">{user.name}</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onClick={() => { setSettingsTab('account'); setView('settings') }}>
                Profile
              </DropdownMenuItem>
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
            onProjectsChanged={onProjectsChanged}
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

        {view === 'settings' ? (
          <Tabs value={settingsTab} onValueChange={setSettingsTab} orientation="vertical" className="gap-8">
            <TabsList variant="line" className="w-full sm:w-48 flex-shrink-0">
              <TabsTrigger value="account">Account</TabsTrigger>
              <TabsTrigger value="appearance">Appearance</TabsTrigger>
              <TabsTrigger value="team">Team</TabsTrigger>
              <TabsTrigger value="tokens">API Tokens</TabsTrigger>
              <TabsTrigger value="danger">Delete Account</TabsTrigger>
            </TabsList>
            <TabsContent value="account" className="max-w-2xl">
              <div className="flex flex-col gap-8">
                <ProfilePanel
                  user={user}
                  onUserUpdated={onUserUpdated}
                  onSessionExpired={onSessionExpired}
                />
                <PasswordPanel onSessionExpired={onSessionExpired} />
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
            <TabsContent value="danger" className="max-w-2xl">
              <DeleteAccountPanel
                onAccountDeleted={onSignOut}
                onSessionExpired={onSessionExpired}
              />
            </TabsContent>
          </Tabs>
        ) : (
          <div className="flex flex-col gap-8">
            <section>
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-lg font-semibold tracking-tight">Your projects</h2>
                <Button onClick={() => setUploadOpen(true)}>
                  <Plus />
                  Create project
                </Button>
              </div>
              {isLoading ? (
                <Card>
                  <CardContent className="py-8 text-center text-sm text-muted-foreground">
                    Loading projects...
                  </CardContent>
                </Card>
              ) : projects.length === 0 ? (
                <Card>
                  <CardContent className="py-8">
                    <div className="flex flex-col gap-4">
                      <div className="flex items-start gap-3">
                        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted">
                          <Terminal className="size-4" />
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <p className="text-sm font-medium">Tell the agent</p>
                            <Badge variant="secondary" className="text-[10px]">Easiest</Badge>
                          </div>
                          <p className="mt-0.5 text-sm text-muted-foreground">
                            Run <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">velori deploy my-site</code> from your terminal.
                          </p>
                        </div>
                      </div>
                      <div className="flex items-start gap-3">
                        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted">
                          <Upload className="size-4" />
                        </div>
                        <div>
                          <p className="text-sm font-medium">Drag and drop</p>
                          <p className="mt-0.5 text-sm text-muted-foreground">
                            Drag a folder or zip file onto this page.
                          </p>
                        </div>
                      </div>
                      <div className="flex items-start gap-3">
                        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted">
                          <FolderOpen className="size-4" />
                        </div>
                        <div>
                          <p className="text-sm font-medium">Select a folder</p>
                          <p className="mt-0.5 text-sm text-muted-foreground">
                            Click <span className="font-medium text-foreground">Create project</span> above to browse your files.
                          </p>
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ) : (
                <>
                  <div className="hidden sm:block">
                    <ProjectsTable
                      projects={projects}
                      deletingProjectID={deletingProjectID}
                      onDelete={onDeleteProject}
                      onVisibilityToggle={onVisibilityToggle}
                      onProjectsChanged={onProjectsChanged}
                      onSessionExpired={onSessionExpired}
                    />
                  </div>
                  <div className="flex flex-col gap-4 sm:hidden">
                    {projects.map((project) => (
                      <ProjectCard
                        key={project.id}
                        project={project}
                        isDeleting={deletingProjectID === project.id}
                        onDelete={onDeleteProject}
                        onVisibilityToggle={onVisibilityToggle}
                        onProjectsChanged={onProjectsChanged}
                        onSessionExpired={onSessionExpired}
                      />
                    ))}
                  </div>
                </>
              )}
            </section>

          </div>
        )}
      </main>
    </div>
  )
}
