import { useEffect, useState } from 'react'
import type { Deploy, Project } from './types'
import { fetchDeploys, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { ArrowLeft } from 'lucide-react'

type VersionViewerProps = {
  project: Project
  onClose: () => void
  onSessionExpired: () => void
}

function deployUrl(liveUrl: string, deploy: Deploy): string {
  if (deploy.isCurrent) return liveUrl
  return `${liveUrl}/_v/${deploy.id}/`
}

function deployLabel(deploy: Deploy, index: number, total: number): string {
  const num = `v${total - index}`
  const parts = [num]
  if (deploy.isCurrent) parts.push('(Live)')
  if (deploy.label) parts.push(`— ${deploy.label}`)
  parts.push(`— ${deploy.createdAt}`)
  return parts.join(' ')
}

export function VersionViewer({ project, onClose, onSessionExpired }: VersionViewerProps) {
  const [deploys, setDeploys] = useState<Deploy[]>([])
  const [loading, setLoading] = useState(true)
  const [mode, setMode] = useState<'single' | 'split'>('single')
  const [leftId, setLeftId] = useState('')
  const [rightId, setRightId] = useState('')

  useEffect(() => {
    void loadDeploys()
  }, [project.id])

  const loadDeploys = async () => {
    try {
      setLoading(true)
      const loaded = await fetchDeploys(project.id)
      setDeploys(loaded)
      if (loaded.length > 0) {
        const current = loaded.find((d) => d.isCurrent) ?? loaded[0]
        setLeftId(current.id)
        const second = loaded.find((d) => !d.isCurrent) ?? current
        setRightId(second.id)
      }
    } catch (err) {
      if (err instanceof SessionExpiredError) onSessionExpired()
    } finally {
      setLoading(false)
    }
  }

  const leftDeploy = deploys.find((d) => d.id === leftId)
  const rightDeploy = deploys.find((d) => d.id === rightId)

  return (
    <div className="fixed inset-0 z-[100] flex flex-col bg-background">
      {/* Control bar */}
      <div className="flex h-10 shrink-0 items-center gap-3 border-b px-3">
        <Button variant="ghost" size="sm" onClick={onClose}>
          <ArrowLeft className="mr-1 size-4" />
          Back
        </Button>

        <div className="mx-2 h-5 w-px bg-border" />

        <span className="text-sm font-medium">{project.name}</span>

        {!loading && deploys.length > 0 && (
          <>
            <select
              className="ml-4 rounded-md border bg-transparent px-2 py-1 text-xs outline-none"
              value={leftId}
              onChange={(e) => setLeftId(e.target.value)}
            >
              {deploys.map((d, i) => (
                <option key={d.id} value={d.id}>
                  {deployLabel(d, i, deploys.length)}
                </option>
              ))}
            </select>

            {deploys.length > 1 && (
              <div className="ml-auto hidden items-center gap-1 sm:flex">
                <Button
                  variant={mode === 'single' ? 'default' : 'ghost'}
                  size="xs"
                  onClick={() => setMode('single')}
                >
                  Single
                </Button>
                <Button
                  variant={mode === 'split' ? 'default' : 'ghost'}
                  size="xs"
                  onClick={() => setMode('split')}
                >
                  Split
                </Button>
              </div>
            )}

            {mode === 'split' && (
              <select
                className="ml-2 rounded-md border bg-transparent px-2 py-1 text-xs outline-none sm:ml-0"
                value={rightId}
                onChange={(e) => setRightId(e.target.value)}
              >
                {deploys.map((d, i) => (
                  <option key={d.id} value={d.id}>
                    {deployLabel(d, i, deploys.length)}
                  </option>
                ))}
              </select>
            )}
          </>
        )}
      </div>

      {/* Iframe area */}
      {loading ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Loading deploys...
        </div>
      ) : deploys.length === 0 ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          No deploys yet.
        </div>
      ) : (
        <div className="flex flex-1">
          {leftDeploy && (
            <iframe
              key={leftId}
              src={deployUrl(project.liveUrl, leftDeploy)}
              className={`border-0 ${mode === 'split' ? 'w-1/2 border-r' : 'w-full'}`}
              style={{ height: '100%' }}
              title={`${project.name} - ${deployLabel(leftDeploy, deploys.indexOf(leftDeploy), deploys.length)}`}
            />
          )}
          {mode === 'split' && rightDeploy && (
            <iframe
              key={rightId}
              src={deployUrl(project.liveUrl, rightDeploy)}
              className="w-1/2 border-0"
              style={{ height: '100%' }}
              title={`${project.name} - ${deployLabel(rightDeploy, deploys.indexOf(rightDeploy), deploys.length)}`}
            />
          )}
        </div>
      )}
    </div>
  )
}
