export type SiteAuthor = {
  id: string
  name: string
  username: string
}

export type Site = {
  id: string
  name: string
  slug: string
  updatedAt: string
  deployCount: number
  liveUrl: string
  isPublic: boolean
  gitBranch?: string | null
  gitCommitHash?: string | null
  gitRemoteURL?: string | null
  createdBy?: SiteAuthor | null
}

export type SessionUser = {
  id: string
  email: string
  name: string
  username: string
  emailVerifiedAt?: string | null
  orgs: OrgInfo[]
  isAdmin?: boolean
}

export type OrgInfo = {
  id: string
  slug: string
  name: string
  isPersonal: boolean
  role: 'admin' | 'member'
}

export type OrgMember = {
  id: string
  userId: string
  name: string
  email: string
  username: string
  role: string
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

export type Deploy = {
  id: string
  status: string
  label: string | null
  sizeBytes: number
  fileCount: number
  createdAt: string
  isCurrent: boolean
  gitCommitHash?: string | null
  gitBranch?: string | null
  gitCommitMessage?: string | null
  gitDirty?: boolean | null
  gitAuthor?: string | null
  gitRemoteURL?: string | null
}
