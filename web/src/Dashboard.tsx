import type { Project, SessionUser } from './types'
import { UploadPanel } from './UploadPanel'
import { ProjectCard } from './ProjectCard'

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
    <div className="app-shell">
      <div className="hero-glow hero-glow-left" />
      <div className="hero-glow hero-glow-right" />

      <header className="topbar">
        <div>
          <p className="eyebrow">Velori v1</p>
          <h1>Publish static prototypes in seconds.</h1>
        </div>
        <div className="topbar-user">
          <span>{user.name}</span>
          <button className="ghost-button" onClick={() => void onSignOut()}>
            Sign out
          </button>
        </div>
      </header>

      <main className="layout">
        <UploadPanel
          user={user}
          projects={projects}
          error={error}
          setError={setError}
          onProjectsChanged={onProjectsChanged}
          onSessionExpired={onSessionExpired}
        />

        <section className="projects-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Your projects</p>
              <h2>Stable URLs, instant redeploys</h2>
            </div>
          </div>

          <div className="project-grid">
            {isLoading ? <article className="project-card project-card-empty">Loading projects...</article> : null}
            {!isLoading && projects.length === 0 ? (
              <article className="project-card project-card-empty">No projects yet. Upload your first static prototype.</article>
            ) : null}
            {projects.map((project) => (
              <ProjectCard
                key={project.id}
                project={project}
                isDeleting={deletingProjectID === project.id}
                onDelete={onDeleteProject}
              />
            ))}
          </div>
        </section>
      </main>
    </div>
  )
}
