import type { Project, SessionUser } from './types'
import { UploadPanel } from './UploadPanel'
import { ProjectCard } from './ProjectCard'
import { TokensPanel } from './TokensPanel'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

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
  return (
    <div className="min-h-svh bg-background">
      <header className="border-b">
        <div className="mx-auto flex h-14 max-w-4xl items-center justify-between px-4">
          <h1 className="text-lg font-bold tracking-tight">Velori</h1>
          <div className="flex items-center gap-3">
            <span className="text-sm text-muted-foreground">{user.name}</span>
            <Button variant="ghost" size="sm" onClick={() => void onSignOut()}>
              Sign out
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-4xl px-4 py-8">
        <div className="flex flex-col gap-8">
          <UploadPanel
            user={user}
            projects={projects}
            error={error}
            setError={setError}
            onProjectsChanged={onProjectsChanged}
            onSessionExpired={onSessionExpired}
          />

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

          <TokensPanel onSessionExpired={onSessionExpired} />
        </div>
      </main>
    </div>
  )
}
