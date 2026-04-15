import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { API_BASE_URL } from './constants'
import { SessionExpiredError } from './api'

type DeviceAuthPageProps = {
  onSessionExpired: () => void
}

export function DeviceAuthPage({ onSessionExpired }: DeviceAuthPageProps) {
  const code = new URLSearchParams(window.location.search).get('code')
  const [status, setStatus] = useState<'ready' | 'approving' | 'done' | 'error'>('ready')
  const [error, setError] = useState<string | null>(null)

  const approve = async () => {
    if (!code) return
    setStatus('approving')
    setError(null)

    try {
      const response = await fetch(`${API_BASE_URL}/api/auth/device/${code}`, {
        method: 'POST',
        credentials: 'include',
      })

      if (response.status === 401) {
        onSessionExpired()
        throw new SessionExpiredError()
      }

      if (!response.ok) {
        const data = (await response.json()) as { error?: string }
        throw new Error(data.error ?? 'Could not approve device')
      }

      setStatus('done')
    } catch (err) {
      if (err instanceof SessionExpiredError) return
      setError(err instanceof Error ? err.message : 'Something went wrong')
      setStatus('error')
    }
  }

  if (!code) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background p-4">
        <Card className="w-full max-w-sm">
          <CardHeader>
            <CardTitle>Invalid link</CardTitle>
            <CardDescription>This device authorization link is missing a code.</CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold tracking-tight">Velori</h1>
        </div>

        <Card>
          {status === 'done' ? (
            <>
              <CardHeader>
                <CardTitle>Device authorized</CardTitle>
                <CardDescription>You can close this tab and return to your terminal.</CardDescription>
              </CardHeader>
            </>
          ) : (
            <>
              <CardHeader>
                <CardTitle>Authorize device</CardTitle>
                <CardDescription>
                  A device is requesting access to your Velori account.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="rounded-lg bg-muted px-4 py-3 text-center font-mono text-lg tracking-widest">
                  {code}
                </div>
                {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
              </CardContent>
              <CardFooter>
                <Button
                  className="w-full"
                  onClick={() => void approve()}
                  disabled={status === 'approving'}
                >
                  {status === 'approving' ? 'Approving...' : 'Approve'}
                </Button>
              </CardFooter>
            </>
          )}
        </Card>
      </div>
    </div>
  )
}
