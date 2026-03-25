import type { UploadFile, UploadSummary } from './types'
import { MAX_DEPLOY_BYTES, MAX_FILE_BYTES } from './constants'

export function getRelativePath(file: File) {
  const candidate = (file as File & { webkitRelativePath?: string }).webkitRelativePath
  return candidate && candidate.length > 0 ? candidate : file.name
}

export function normalizeFiles(fileList: FileList): UploadFile[] {
  return Array.from(fileList).map((file) => ({
    file,
    name: file.name,
    size: file.size,
    path: getRelativePath(file),
  }))
}

export function buildUploadSummary(files: UploadFile[]): UploadSummary {
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
  }
}

export function validateSelection(summary: UploadSummary, files: UploadFile[]): string | null {
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

export function formatBytes(bytes: number) {
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`
  }

  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
