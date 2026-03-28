import { describe, it, expect } from 'vitest'
import {
  getRelativePath,
  buildUploadSummary,
  validateSelection,
  formatBytes,
} from './upload-helpers'
import { MAX_DEPLOY_BYTES, MAX_FILE_BYTES } from './constants'
import type { UploadFile, UploadSummary } from './types'

function fakeFile(name: string, size: number, webkitRelativePath = ''): UploadFile {
  return {
    file: { name, size, webkitRelativePath } as unknown as File,
    name,
    size,
    path: webkitRelativePath || name,
  }
}

describe('getRelativePath', () => {
  it('returns webkitRelativePath when present', () => {
    const file = { name: 'index.html', webkitRelativePath: 'my-site/index.html' } as File
    expect(getRelativePath(file)).toBe('my-site/index.html')
  })

  it('falls back to file.name when webkitRelativePath is empty', () => {
    const file = { name: 'index.html', webkitRelativePath: '' } as File
    expect(getRelativePath(file)).toBe('index.html')
  })

  it('falls back to file.name when webkitRelativePath is absent', () => {
    const file = { name: 'style.css' } as File
    expect(getRelativePath(file)).toBe('style.css')
  })
})

describe('buildUploadSummary', () => {
  it('detects single zip upload', () => {
    const files = [fakeFile('my-site.zip', 5000)]
    const summary = buildUploadSummary(files)

    expect(summary.mode).toBe('zip')
    expect(summary.name).toBe('my-site')
    expect(summary.fileCount).toBe(1)
    expect(summary.totalBytes).toBe(5000)
  })

  it('derives name from folder path segment', () => {
    const files = [
      fakeFile('index.html', 100, 'my-app/index.html'),
      fakeFile('style.css', 200, 'my-app/style.css'),
    ]
    const summary = buildUploadSummary(files)

    expect(summary.mode).toBe('files')
    expect(summary.name).toBe('my-app')
    expect(summary.fileCount).toBe(2)
    expect(summary.totalBytes).toBe(300)
    expect(summary.includesIndexHtml).toBe(true)
  })

  it('falls back to "Untitled Prototype" for empty path', () => {
    const files = [{ file: {} as File, name: '', size: 100, path: '' }]
    const summary = buildUploadSummary(files)

    expect(summary.name).toBe('Untitled Prototype')
  })

  it('detects index.html in nested paths', () => {
    const files = [
      fakeFile('app.js', 500, 'site/app.js'),
      fakeFile('index.html', 100, 'site/build/index.html'),
    ]
    const summary = buildUploadSummary(files)

    expect(summary.includesIndexHtml).toBe(true)
  })

  it('reports no index.html when absent', () => {
    const files = [fakeFile('app.js', 500, 'site/app.js')]
    const summary = buildUploadSummary(files)

    expect(summary.includesIndexHtml).toBe(false)
  })
})

describe('validateSelection', () => {
  it('rejects empty files', () => {
    const summary: UploadSummary = {
      name: 'Test',
      mode: 'files',
      fileCount: 0,
      totalBytes: 0,
      includesIndexHtml: false,
    }
    expect(validateSelection(summary, [])).toContain('Select')
  })

  it('rejects total over 100MB', () => {
    const files = [fakeFile('big.bin', MAX_DEPLOY_BYTES + 1)]
    const summary: UploadSummary = {
      name: 'Test',
      mode: 'files',
      fileCount: 1,
      totalBytes: MAX_DEPLOY_BYTES + 1,
      includesIndexHtml: true,
    }
    expect(validateSelection(summary, files)).toContain('100MB')
  })

  it('rejects single file over 20MB with filename', () => {
    const files = [
      fakeFile('index.html', 100),
      fakeFile('huge-video.mp4', MAX_FILE_BYTES + 1),
    ]
    const summary: UploadSummary = {
      name: 'Test',
      mode: 'files',
      fileCount: 2,
      totalBytes: MAX_FILE_BYTES + 101,
      includesIndexHtml: true,
    }
    const error = validateSelection(summary, files)
    expect(error).toContain('huge-video.mp4')
    expect(error).toContain('20MB')
  })

  it('rejects files mode without index.html', () => {
    const files = [fakeFile('app.js', 500)]
    const summary: UploadSummary = {
      name: 'Test',
      mode: 'files',
      fileCount: 1,
      totalBytes: 500,
      includesIndexHtml: false,
    }
    expect(validateSelection(summary, files)).toContain('index.html')
  })

  it('allows zip mode without index.html', () => {
    const files = [fakeFile('site.zip', 5000)]
    const summary: UploadSummary = {
      name: 'site',
      mode: 'zip',
      fileCount: 1,
      totalBytes: 5000,
      includesIndexHtml: false,
    }
    expect(validateSelection(summary, files)).toBeNull()
  })

  it('returns null for valid selection', () => {
    const files = [
      fakeFile('index.html', 200, 'my-site/index.html'),
      fakeFile('style.css', 300, 'my-site/style.css'),
    ]
    const summary: UploadSummary = {
      name: 'my-site',
      mode: 'files',
      fileCount: 2,
      totalBytes: 500,
      includesIndexHtml: true,
    }
    expect(validateSelection(summary, files)).toBeNull()
  })
})

describe('formatBytes', () => {
  it('formats small sizes as KB with 1 KB minimum', () => {
    expect(formatBytes(500)).toBe('1 KB')
  })

  it('rounds KB values', () => {
    expect(formatBytes(1536)).toBe('2 KB')
  })

  it('formats MB with one decimal', () => {
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB')
  })

  it('handles boundary at 1MB', () => {
    expect(formatBytes(1024 * 1024 - 1)).toBe('1024 KB')
    expect(formatBytes(1024 * 1024)).toBe('1.0 MB')
  })
})
