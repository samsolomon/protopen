import { type ChangeEvent, type DragEvent, useMemo, useRef, useState } from 'react'
import type { Project, SessionUser, UploadFile, UploadState, UploadSummary } from './types'
import { postUpload, SessionExpiredError } from './api'
import { normalizeFiles, buildUploadSummary, validateSelection, formatBytes } from './upload-helpers'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from 'sonner'

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
      { label: 'Total deploys', value: projects.reduce((sum, project) => sum + project.deployCount, 0).toString() },
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
      toast.success(`${summary.name} deployed successfully`)
    } catch (uploadError) {
      if (uploadError instanceof SessionExpiredError) {
        onSessionExpired()
      }
      setUploadState('idle')
      setMessage('Drop a folder or zip to deploy')
      setUploadSummary(null)
      const errorMessage = uploadError instanceof Error ? uploadError.message : 'Upload failed'
      setError(errorMessage)
      toast.error(errorMessage)
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
      toast.error(validationError)
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
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Deploy</CardTitle>
          <CardDescription>
            Upload a folder or zip of static files to get a live URL. Signed in as {user.email}.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div
            className={`relative flex flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed p-8 text-center transition-colors ${
              uploadState === 'dragging' ? 'border-primary bg-primary/5' : 'border-muted-foreground/25'
            } ${uploadState === 'uploading' ? 'opacity-60' : ''}`}
            onDragEnter={() => setUploadState('dragging')}
            onDragOver={(event) => event.preventDefault()}
            onDragLeave={() => setUploadState('idle')}
            onDrop={(event) => void handleDrop(event)}
          >
            <p className="text-sm font-medium">{message}</p>
            <p className="text-xs text-muted-foreground">HTML, CSS, JavaScript, images, fonts</p>

            {uploadSummary ? (
              <div className="flex gap-2">
                <Badge variant="secondary">{uploadSummary.fileCount} files</Badge>
                <Badge variant="secondary">{formatBytes(uploadSummary.totalBytes)}</Badge>
                <Badge variant="secondary">{uploadSummary.mode === 'zip' ? 'Zip' : 'Folder'}</Badge>
              </div>
            ) : null}

            {error ? <p className="text-sm text-destructive">{error}</p> : null}

            <div className="flex gap-2">
              {import.meta.env.DEV ? (
                <Button
                  size="sm"
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
                </Button>
              ) : null}
              <Button variant="outline" size="sm" onClick={() => folderInputRef.current?.click()}>
                Choose folder
              </Button>
              <Button variant="outline" size="sm" onClick={() => zipInputRef.current?.click()}>
                Choose zip
              </Button>
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
        </CardContent>
      </Card>

      <div className="grid grid-cols-3 gap-4">
        {stats.map((stat) => (
          <Card key={stat.label} size="sm">
            <CardContent className="pt-0">
              <p className="text-xs text-muted-foreground">{stat.label}</p>
              <p className="text-2xl font-bold tracking-tight">{stat.value}</p>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
