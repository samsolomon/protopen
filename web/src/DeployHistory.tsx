import { useEffect, useState } from 'react'
import type { Deploy } from './types'
import { fetchDeploys, rollbackDeploy, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ExternalLink } from 'lucide-react'
import { toast } from 'sonner'
import { commitURL } from '@/lib/utils'

type DeployHistoryProps = {
  projectId: string
  onRollback: () => void
  onSessionExpired: () => void
  canRollback?: boolean
}

export function DeployHistory({ projectId, onRollback, onSessionExpired, canRollback }: DeployHistoryProps) {
  const [deploys, setDeploys] = useState<Deploy[]>([])
  const [loading, setLoading] = useState(true)
  const [rollingBack, setRollingBack] = useState<string | null>(null)

  useEffect(() => {
    void loadDeploys()
  }, [projectId])

  const loadDeploys = async () => {
    try {
      setLoading(true)
      const loaded = await fetchDeploys(projectId)
      setDeploys(loaded)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
      }
    } finally {
      setLoading(false)
    }
  }

  const handleRollback = async (deployId: string) => {
    setRollingBack(deployId)
    try {
      await rollbackDeploy(projectId, deployId)
      toast.success('Rolled back successfully')
      await loadDeploys()
      onRollback()
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not rollback')
    } finally {
      setRollingBack(null)
    }
  }

  if (loading) {
    return <p className="py-2 text-xs text-muted-foreground">Loading history...</p>
  }

  if (deploys.length === 0) {
    return <p className="py-2 text-xs text-muted-foreground">No deploys yet.</p>
  }

  return (
    <div className="flex flex-col gap-1">
      {deploys.map((deploy) => (
        <div
          key={deploy.id}
          className={`flex items-center justify-between rounded-md border px-3 py-2 text-xs ${
            deploy.isCurrent ? 'border-primary/30 bg-primary/5' : ''
          }`}
        >
          <div className="flex items-center gap-2">
            {deploy.isCurrent ? <Badge variant="default">live</Badge> : null}
            <span className="text-muted-foreground">{deploy.createdAt}</span>
            {deploy.label ? <span className="font-medium">{deploy.label}</span> : null}
            {deploy.gitCommitHash ? (
              deploy.gitRemoteURL ? (
                <a
                  href={commitURL(deploy.gitRemoteURL, deploy.gitCommitHash)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-0.5 font-mono text-muted-foreground hover:underline"
                  title={deploy.gitCommitHash}
                >
                  {deploy.gitCommitHash.slice(0, 8)}{deploy.gitDirty ? '*' : ''}
                  <ExternalLink className="size-2.5" />
                </a>
              ) : (
                <span className="font-mono text-muted-foreground" title={deploy.gitCommitHash}>
                  {deploy.gitCommitHash.slice(0, 8)}{deploy.gitDirty ? '*' : ''}
                </span>
              )
            ) : null}
            {deploy.gitBranch ? <Badge variant="outline">{deploy.gitBranch}</Badge> : null}
            {deploy.gitCommitMessage ? (
              <span className="max-w-[200px] truncate text-muted-foreground" title={deploy.gitCommitMessage}>
                {deploy.gitCommitMessage}
              </span>
            ) : null}
            {!deploy.gitCommitHash ? <span className="text-muted-foreground">{deploy.fileCount} files</span> : null}
          </div>
          {!deploy.isCurrent && canRollback ? (
            <Button
              variant="ghost"
              size="xs"
              disabled={rollingBack === deploy.id}
              onClick={() => void handleRollback(deploy.id)}
            >
              {rollingBack === deploy.id ? 'Rolling back...' : 'Rollback'}
            </Button>
          ) : null}
        </div>
      ))}
    </div>
  )
}
