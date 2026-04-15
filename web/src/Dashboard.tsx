import { useEffect, useRef, useState } from 'react'
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
import { Plus, Upload, FolderOpen, Mail, Copy, Check } from 'lucide-react'

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
    <div className="rounded-xl bg-muted/60 px-7 pb-7 pt-6">
      <p className="text-[13px] text-muted-foreground">Get started</p>

      <div className="mt-5 flex flex-col gap-5">
        <div>
          <p className="text-[13px] font-medium">Install the skill</p>
          <div className="mt-1.5 flex items-center gap-1.5">
            <code className="font-mono text-[13px] text-muted-foreground">{installCommand}</code>
            <CopyButton text={installCommand} />
          </div>
        </div>
        <div>
          <p className="text-[13px] font-medium">Deploy from your terminal</p>
          <code className="mt-1.5 block font-mono text-[13px] text-muted-foreground">velori deploy my-site</code>
        </div>
      </div>

      <div className="mt-6 flex items-center gap-4 text-[13px] text-muted-foreground">
        <button type="button" className="hover:text-foreground transition-colors" onClick={onUploadOpen}>
          or drag and drop
        </button>
      </div>
    </div>
  )
}

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
            <a href="/" className="text-xl font-semibold tracking-tight hover:opacity-70 transition-opacity" onClick={(e) => { e.preventDefault(); switchView('dashboard') }}>Velori</a>
          </div>
          <div />
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
                <EmptyState onUploadOpen={() => setUploadOpen(true)} />
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
