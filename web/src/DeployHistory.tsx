import { useEffect, useState } from 'react'
import type { Deploy } from './types'
import { fetchDeploys, rollbackDeploy, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from 'sonner'

type DeployHistoryProps = {
  projectId: string
  onRollback: () => void
  onSessionExpired: () => void
}

export function DeployHistory({ projectId, onRollback, onSessionExpired }: DeployHistoryProps) {
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
            <span className="text-muted-foreground">{deploy.fileCount} files</span>
          </div>
          {!deploy.isCurrent ? (
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
