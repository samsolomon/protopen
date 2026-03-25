import { type ChangeEvent, type DragEvent, useMemo, useRef, useState } from 'react'
import type { Project, SessionUser, UploadFile, UploadState, UploadSummary } from './types'
import { postUpload, SessionExpiredError } from './api'
import { normalizeFiles, buildUploadSummary, validateSelection, formatBytes } from './upload-helpers'

type UploadPanelProps = {
  user: SessionUser
  projects: Project[]
  error: string | null
  setError: (error: string | null) => void
  onProjectsChanged: () => void
  onSessionExpired: () => void
}

export function UploadPanel({ user, projects, error, setError, onProjectsChanged, onSessionExpired }: UploadPanelProps) {
  const [uploadState, setUploadState] = useState<UploadState>('idle')
  const [message, setMessage] = useState('Drop a folder or zip to deploy')
  const [uploadSummary, setUploadSummary] = useState<UploadSummary | null>(null)
  const zipInputRef = useRef<HTMLInputElement | null>(null)
  const folderInputRef = useRef<HTMLInputElement | null>(null)

  const stats = useMemo(
    () => [
      { label: 'Projects', value: projects.length.toString().padStart(2, '0') },
      { label: 'Fastest publish', value: '<10s' },
      { label: 'Deploys this week', value: projects.reduce((sum, project) => sum + project.deployCount, 0).toString() },
    ],
    [projects],
  )

  const startUpload = async (summary: UploadSummary, files: UploadFile[]) => {
    setUploadState('uploading')
    setMessage(`Uploading ${summary.name}...`)
    setError(null)
    setUploadSummary(summary)

    try {
      await postUpload(summary, files)
      onProjectsChanged()
      setUploadState('success')
      setMessage(`Live now - ${summary.name} was deployed`)
    } catch (uploadError) {
      if (uploadError instanceof SessionExpiredError) {
        onSessionExpired()
      }
      setUploadState('idle')
      setMessage('Drop a folder or zip to deploy')
      setUploadSummary(null)
      setError(uploadError instanceof Error ? uploadError.message : 'Upload failed')
    }
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
        <p className="dropzone-subtitle">Signed in as {user.email}. Static assets only: HTML, CSS, JavaScript, images, fonts</p>
        {uploadSummary ? (
          <div className="upload-summary">
            <span>{uploadSummary.fileCount} files</span>
            <span>{formatBytes(uploadSummary.totalBytes)}</span>
            <span>{uploadSummary.mode === 'zip' ? 'Zip upload' : 'Folder upload'}</span>
          </div>
        ) : null}
        {error ? <p className="error-banner">{error}</p> : null}
        <div className="dropzone-actions">
          {import.meta.env.DEV ? (
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
          ) : null}
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
  )
}
