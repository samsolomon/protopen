import { ChangeEvent, DragEvent, useEffect, useMemo, useRef, useState } from 'react'

type Project = {
  id: string
  name: string
  slug: string
  updatedAt: string
  deployCount: number
  liveUrl: string
}

type UploadMode = 'files' | 'zip'

type UploadFile = {
  file: File
  name: string
  size: number
  path: string
}

type UploadSummary = {
  name: string
  mode: UploadMode
  fileCount: number
  totalBytes: number
  includesIndexHtml: boolean
}

const API_BASE_URL = 'http://localhost:8080'
const MAX_DEPLOY_BYTES = 100 * 1024 * 1024
const MAX_FILE_BYTES = 20 * 1024 * 1024

type UploadState = 'idle' | 'dragging' | 'uploading' | 'success'

function App() {
  const [projects, setProjects] = useState<Project[]>([])
  const [uploadState, setUploadState] = useState<UploadState>('idle')
  const [message, setMessage] = useState('Drop a folder or zip to deploy')
  const [error, setError] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [uploadSummary, setUploadSummary] = useState<UploadSummary | null>(null)
  const zipInputRef = useRef<HTMLInputElement | null>(null)
  const folderInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    void loadProjects()
  }, [])

  const stats = useMemo(
    () => [
      { label: 'Projects', value: projects.length.toString().padStart(2, '0') },
      {
        label: 'Fastest publish',
        value: '<10s',
      },
      { label: 'Deploys this week', value: projects.reduce((sum, p) => sum + p.deployCount, 0).toString() },
    ],
    [projects],
  )

  const loadProjects = async () => {
    try {
      setIsLoading(true)
      const response = await fetch(`${API_BASE_URL}/api/projects`)

      if (!response.ok) {
        throw new Error('Could not load projects')
      }

      const data = (await response.json()) as { projects: Project[] }
      setProjects(data.projects)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Could not load projects')
    } finally {
      setIsLoading(false)
    }
  }

  const startUpload = async (summary: UploadSummary, files: UploadFile[]) => {
    setUploadState('uploading')
    setMessage(`Uploading ${summary.name}...`)
    setError(null)
    setUploadSummary(summary)

    try {
      const formData = new FormData()
      formData.set('name', summary.name)
      formData.set('mode', summary.mode)
      formData.set('paths', JSON.stringify(files.map((file) => file.path)))

      files.forEach((file) => {
        formData.append('files', file.file, file.name)
      })

      const response = await fetch(`${API_BASE_URL}/api/uploads`, {
        method: 'POST',
        body: formData,
      })

      if (!response.ok) {
        const body = (await response.json()) as { error?: string }
        throw new Error(body.error ?? 'Upload placeholder failed')
      }

      await loadProjects()
      setUploadState('success')
      setMessage(`Live soon - ${summary.name} added to the queue`)
    } catch (uploadError) {
      setUploadState('idle')
      setMessage('Drop a folder or zip to deploy')
      setUploadSummary(null)
      setError(uploadError instanceof Error ? uploadError.message : 'Upload placeholder failed')
    }
  }

  const getRelativePath = (file: File) => {
    const candidate = (file as File & { webkitRelativePath?: string }).webkitRelativePath
    return candidate && candidate.length > 0 ? candidate : file.name
  }

  const normalizeFiles = (fileList: FileList) => {
    return Array.from(fileList).map((file) => ({
      file,
      name: file.name,
      size: file.size,
      path: getRelativePath(file),
    }))
  }

  const buildUploadSummary = (files: UploadFile[]) => {
    const firstPath = files[0]?.path ?? 'prototype'
    const firstName = files[0]?.name ?? 'prototype'
    const isZipUpload = files.length === 1 && firstName.toLowerCase().endsWith('.zip')
    const candidateName = isZipUpload ? firstName.replace(/\.zip$/i, '') : firstPath.split('/')[0]

    return {
      name: candidateName || 'Untitled Prototype',
      mode: isZipUpload ? 'zip' : 'files',
      fileCount: files.length,
      totalBytes: files.reduce((sum, file) => sum + file.size, 0),
      includesIndexHtml: files.some((file) => file.path.toLowerCase().endsWith('index.html')),
    } satisfies UploadSummary
  }

  const validateSelection = (summary: UploadSummary, files: UploadFile[]) => {
    if (files.length === 0) {
      return 'Select a folder or zip to deploy'
    }

    if (summary.totalBytes > MAX_DEPLOY_BYTES) {
      return 'Deploy exceeds the 100MB upload limit'
    }

    const oversizedFile = files.find((file) => file.size > MAX_FILE_BYTES)
    if (oversizedFile) {
      return `${oversizedFile.name} exceeds the 20MB per-file limit`
    }

    if (summary.mode === 'files' && !summary.includesIndexHtml) {
      return 'No index.html found. Velori hosts built static sites only.'
    }

    return null
  }

  const handleSelection = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) {
      return
    }

    const files = normalizeFiles(fileList)
    const summary = buildUploadSummary(files)
    const validationError = validateSelection(summary, files)

    setUploadSummary(summary)

    if (validationError) {
      setUploadState('idle')
      setMessage('Drop a folder or zip to deploy')
      setError(validationError)
      return
    }

    await startUpload(summary, files)
  }

  const handlePickedFiles = async (event: ChangeEvent<HTMLInputElement>) => {
    await handleSelection(event.target.files)
    event.target.value = ''
  }

  const handleDrop = async (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setUploadState('idle')
    await handleSelection(event.dataTransfer.files)
  }

  return (
    <div className="app-shell">
      <div className="hero-glow hero-glow-left" />
      <div className="hero-glow hero-glow-right" />

      <header className="topbar">
        <div>
          <p className="eyebrow">Velori v1</p>
          <h1>Publish static prototypes in seconds.</h1>
        </div>
        <button className="ghost-button">Simple account</button>
      </header>

      <main className="layout">
        <section className="upload-panel">
          <div className="upload-copy">
            <p className="eyebrow">Dashboard</p>
            <h2>Netlify for non-engineers</h2>
            <p>
              Upload a folder or zip of built static files and get a live, shareable URL without touching hosting
              infrastructure.
            </p>
          </div>

          <div
            className={`dropzone ${uploadState === 'dragging' ? 'dragging' : ''} ${uploadState === 'uploading' ? 'uploading' : ''}`}
            onDragEnter={() => setUploadState('dragging')}
            onDragOver={(event) => event.preventDefault()}
            onDragLeave={() => setUploadState('idle')}
            onDrop={(event) => void handleDrop(event)}
          >
            <div className="dropzone-ring" />
            <p className="dropzone-title">{message}</p>
            <p className="dropzone-subtitle">Static assets only: HTML, CSS, JavaScript, images, fonts</p>
            {uploadSummary ? (
              <div className="upload-summary">
                <span>{uploadSummary.fileCount} files</span>
                <span>{formatBytes(uploadSummary.totalBytes)}</span>
                <span>{uploadSummary.mode === 'zip' ? 'Zip upload' : 'Folder upload'}</span>
              </div>
            ) : null}
            {error ? <p className="error-banner">{error}</p> : null}
            <div className="dropzone-actions">
              <button
                className="primary-button"
                onClick={() =>
                  void startUpload(
                    {
                      name: 'prototype',
                      mode: 'files',
                      fileCount: 2,
                      totalBytes: 2600,
                      includesIndexHtml: true,
                    },
                    [
                      {
                        file: new File(['<!doctype html><html><body><h1>Velori demo</h1></body></html>'], 'index.html', {
                          type: 'text/html',
                        }),
                        name: 'index.html',
                        size: 1800,
                        path: 'prototype/index.html',
                      },
                      {
                        file: new File(['body { font-family: sans-serif; }'], 'styles.css', { type: 'text/css' }),
                        name: 'styles.css',
                        size: 800,
                        path: 'prototype/styles.css',
                      },
                    ],
                  )
                }
              >
                Simulate upload
              </button>
              <button className="secondary-button" onClick={() => folderInputRef.current?.click()}>
                Choose folder
              </button>
              <button className="secondary-button" onClick={() => zipInputRef.current?.click()}>
                Choose zip
              </button>
              <input
                ref={folderInputRef}
                className="sr-only"
                type="file"
                multiple
                {...({ webkitdirectory: '', directory: '' } as Record<string, string>)}
                onChange={(event) => void handlePickedFiles(event)}
              />
              <input
                ref={zipInputRef}
                className="sr-only"
                type="file"
                accept=".zip,application/zip"
                onChange={(event) => void handlePickedFiles(event)}
              />
            </div>
          </div>

          <div className="stats-grid">
            {stats.map((stat) => (
              <article key={stat.label} className="stat-card">
                <p>{stat.label}</p>
                <strong>{stat.value}</strong>
              </article>
            ))}
          </div>
        </section>

        <section className="projects-panel">
          <div className="section-header">
            <div>
              <p className="eyebrow">Your projects</p>
              <h2>Stable URLs, instant redeploys</h2>
            </div>
            <button className="secondary-button">View deploy guide</button>
          </div>

          <div className="project-grid">
            {isLoading ? <article className="project-card project-card-empty">Loading projects...</article> : null}
            {projects.map((project) => (
              <article key={project.id} className="project-card">
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
                  <button className="ghost-button small">Copy URL</button>
                </div>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  )
}

function formatBytes(bytes: number) {
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`
  }

  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default App
