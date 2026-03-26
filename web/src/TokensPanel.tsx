import { useEffect, useState } from 'react'
import { type ApiToken, fetchTokens, createToken, deleteToken, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'

type TokensPanelProps = {
  onSessionExpired: () => void
  onViewDocs?: () => void
}

export function TokensPanel({ onSessionExpired, onViewDocs }: TokensPanelProps) {
  const [tokens, setTokens] = useState<ApiToken[]>([])
  const [newTokenName, setNewTokenName] = useState('')
  const [revealedToken, setRevealedToken] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    void loadTokens()
  }, [])

  const loadTokens = async () => {
    try {
      const loaded = await fetchTokens()
      setTokens(loaded)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
      }
    }
  }

  const handleCreate = async () => {
    setCreating(true)
    try {
      const result = await createToken(newTokenName || 'default')
      setRevealedToken(result.token)
      setNewTokenName('')
      await loadTokens()
      toast.success('Token created')
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not create token')
    } finally {
      setCreating(false)
    }
  }

  const handleRevoke = async (tokenId: string) => {
    try {
      await deleteToken(tokenId)
      setTokens((current) => current.filter((t) => t.id !== tokenId))
      toast('Token revoked')
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      toast.error(err instanceof Error ? err.message : 'Could not revoke token')
    }
  }

  return (
    <section>
      <h2 className="mb-4 text-lg font-semibold tracking-tight">API Tokens</h2>
      <Card>
        <CardHeader>
          <CardTitle>Deploy from the command line</CardTitle>
          <CardDescription>
            Create a token to deploy with the Velori CLI or curl.
            {onViewDocs ? (
              <>
                {' '}
                <Button variant="link" className="inline h-auto p-0" onClick={onViewDocs}>
                  View CLI documentation &rarr;
                </Button>
              </>
            ) : null}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {revealedToken ? (
            <div className="flex flex-col gap-2 rounded-lg border bg-muted/50 p-3">
              <p className="text-sm font-medium">Your new token (shown once):</p>
              <code className="break-all rounded bg-background p-2 text-xs">{revealedToken}</code>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void navigator.clipboard.writeText(revealedToken)
                    toast('Token copied')
                  }}
                >
                  Copy
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setRevealedToken(null)}>
                  Dismiss
                </Button>
              </div>
            </div>
          ) : null}

          <div className="flex gap-2">
            <Input
              value={newTokenName}
              onChange={(e) => setNewTokenName(e.target.value)}
              placeholder="Token name (e.g. my-agent)"
              onKeyDown={(e) => { if (e.key === 'Enter') void handleCreate() }}
            />
            <Button onClick={() => void handleCreate()} disabled={creating}>
              {creating ? 'Creating...' : 'Create token'}
            </Button>
          </div>

          {tokens.length > 0 ? (
            <div className="flex flex-col gap-2">
              {tokens.map((token) => (
                <div key={token.id} className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <p className="text-sm font-medium">{token.name}</p>
                    <p className="text-xs text-muted-foreground">{token.createdAt}</p>
                  </div>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => void handleRevoke(token.id)}
                  >
                    Revoke
                  </Button>
                </div>
              ))}
            </div>
          ) : null}
        </CardContent>
      </Card>
    </section>
  )
}
