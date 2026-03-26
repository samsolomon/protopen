import { useState } from 'react'
import type { Project, SessionUser } from './types'
import { UploadPanel } from './UploadPanel'
import { ProjectCard } from './ProjectCard'
import { TokensPanel } from './TokensPanel'
import { CLIDocs } from './CLIDocs'
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
import { Plus, User, Settings, LogOut } from 'lucide-react'

type DashboardView = 'dashboard' | 'docs'

type DashboardProps = {
  user: SessionUser
  projects: Project[]
  isLoading: boolean
  deletingProjectID: string | null
  error: string | null
  setError: (error: string | null) => void
  onSignOut: () => void
  onDeleteProject: (projectID: string) => void
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
  onDeleteProject,
  onProjectsChanged,
  onSessionExpired,
}: DashboardProps) {
  const [view, setView] = useState<DashboardView>('dashboard')
  const [uploadOpen, setUploadOpen] = useState(false)

  const switchView = (next: DashboardView) => {
    setView(next)
    window.scrollTo(0, 0)
  }

  return (
    <div className="min-h-svh bg-background">
      <header className="border-b">
        <div className="mx-auto flex h-14 max-w-4xl items-center justify-between px-4">
          <div className="flex items-center gap-4">
            <h1 className="text-lg font-bold tracking-tight">Velori</h1>
            <nav className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                className={view === 'dashboard' ? 'bg-muted' : ''}
                onClick={() => switchView('dashboard')}
              >
                Projects
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className={view === 'docs' ? 'bg-muted' : ''}
                onClick={() => switchView('docs')}
              >
                CLI
              </Button>
            </nav>
          </div>
          <div className="flex items-center gap-2">
          <Button size="sm" onClick={() => setUploadOpen(true)}>
            <Plus />
            Create project
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              className="flex cursor-pointer items-center gap-2 rounded-full border py-1 pr-3 pl-1 text-sm font-medium outline-none hover:bg-muted"
            >
              <span className="flex size-7 items-center justify-center rounded-full bg-muted text-xs font-medium">
                {user.name.charAt(0).toUpperCase()}
              </span>
              {user.name}
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem>
                <User />
                Profile
              </DropdownMenuItem>
              <DropdownMenuItem>
                <Settings />
                Settings
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => void onSignOut()}>
                <LogOut />
                Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          </div>
        </div>
      </header>

      <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
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
            onClose={() => setUploadOpen(false)}
          />
        </DialogContent>
      </Dialog>

      <main className="mx-auto max-w-4xl px-4 py-8">
        {view === 'docs' ? (
          <CLIDocs />
        ) : (
          <div className="flex flex-col gap-8">
            <section>
              <h2 className="mb-4 text-lg font-semibold tracking-tight">Your projects</h2>
              {isLoading ? (
                <Card>
                  <CardContent className="py-8 text-center text-sm text-muted-foreground">
                    Loading projects...
                  </CardContent>
                </Card>
              ) : projects.length === 0 ? (
                <Card>
                  <CardContent className="py-8 text-center text-sm text-muted-foreground">
                    No projects yet. Upload your first static prototype.
                  </CardContent>
                </Card>
              ) : (
                <div className="grid gap-4 sm:grid-cols-2">
                  {projects.map((project) => (
                    <ProjectCard
                      key={project.id}
                      project={project}
                      isDeleting={deletingProjectID === project.id}
                      onDelete={onDeleteProject}
                      onProjectsChanged={onProjectsChanged}
                      onSessionExpired={onSessionExpired}
                    />
                  ))}
                </div>
              )}
            </section>

            <TokensPanel
              onSessionExpired={onSessionExpired}
              onViewDocs={() => switchView('docs')}
            />
          </div>
        )}
      </main>
    </div>
  )
}
