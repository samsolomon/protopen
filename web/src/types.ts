export type Project = {
  id: string
  name: string
  slug: string
  updatedAt: string
  deployCount: number
  liveUrl: string
}

export type SessionUser = {
  id: string
  email: string
  name: string
  username: string
}

export type UploadMode = 'files' | 'zip'

export type UploadFile = {
  file: File
  name: string
  size: number
  path: string
}

export type UploadSummary = {
  name: string
  mode: UploadMode
  fileCount: number
  totalBytes: number
  includesIndexHtml: boolean
}

export type AuthMode = 'sign-in' | 'sign-up'

export type AuthFormState = {
  name: string
  email: string
  password: string
}

export type UploadState = 'idle' | 'dragging' | 'uploading' | 'success'
