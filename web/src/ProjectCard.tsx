import type { Project } from './types'

type ProjectCardProps = {
  project: Project
  isDeleting: boolean
  onDelete: (projectID: string) => void
}

export function ProjectCard({ project, isDeleting, onDelete }: ProjectCardProps) {
  return (
    <article className="project-card">
      <div className="project-preview">
        <span>/{project.slug}</span>
      </div>
      <div className="project-meta">
        <div>
          <h3>{project.name}</h3>
          <p>{project.updatedAt}</p>
        </div>
        <span className="pill">{project.deployCount} deploys</span>
      </div>
      <div className="project-actions">
        <a href={project.liveUrl}>{project.liveUrl}</a>
        <button className="ghost-button small" onClick={() => void navigator.clipboard.writeText(project.liveUrl)}>
          Copy URL
        </button>
        <button
          className="ghost-button small destructive-button"
          disabled={isDeleting}
          onClick={() => void onDelete(project.id)}
        >
          {isDeleting ? 'Deleting...' : 'Delete'}
        </button>
      </div>
    </article>
  )
}
