import { useEffect, useState } from 'react'
import { createToken, SessionExpiredError } from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Terminal, Copy, Check, RotateCcw } from 'lucide-react'
import { toast } from 'sonner'

type CLIAuthPageProps = {
  onSessionExpired: () => void
}

export function CLIAuthPage({ onSessionExpired }: CLIAuthPageProps) {
  const [token, setToken] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const loading = token === null && error === null

  const generate = async () => {
    setToken(null)
    setError(null)
    setCopied(false)
    try {
      const result = await createToken('cli')
      setToken(result.token)
    } catch (err) {
      if (err instanceof SessionExpiredError) {
        onSessionExpired()
        return
      }
      setError(err instanceof Error ? err.message : 'Could not generate token')
    }
  }

  useEffect(() => {
    void generate()
  }, [])

  const handleCopy = () => {
    if (!token) return
    void navigator.clipboard.writeText(token)
    setCopied(true)
    toast('Copied to clipboard')
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold tracking-tight">Velori</h1>
          <p className="mt-1 text-sm text-muted-foreground">CLI Authentication</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Terminal className="size-4" />
              {copied ? 'You\'re all set' : 'Your API Token'}
            </CardTitle>
            <CardDescription>
              {loading
                ? 'Generating a token for your CLI...'
                : error
                  ? 'Something went wrong.'
                  : copied
                    ? 'Go back to your terminal and paste the token.'
                    : 'Copy this token and paste it into your terminal.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {loading ? (
              <div className="flex items-center justify-center rounded-lg border bg-muted/50 p-6">
                <p className="text-sm text-muted-foreground">Generating token...</p>
              </div>
            ) : error ? (
              <div className="flex flex-col gap-3">
                <div className="rounded-lg border border-destructive/20 bg-destructive/5 p-3">
                  <p className="text-sm text-destructive">{error}</p>
                </div>
                <Button variant="outline" onClick={() => void generate()}>
                  <RotateCcw className="size-3.5" />
                  Try again
                </Button>
              </div>
            ) : copied ? (
              <div className="flex flex-col items-center gap-2 rounded-lg border bg-muted/50 p-6">
                <div className="flex size-10 items-center justify-center rounded-full bg-primary">
                  <Check className="size-5 text-primary-foreground" />
                </div>
                <p className="text-sm font-medium">Token copied</p>
                <p className="text-xs text-muted-foreground">You can close this tab.</p>
              </div>
            ) : (
              <>
                <code className="break-all rounded-lg border bg-muted/50 p-3 font-mono text-xs leading-relaxed">
                  {token}
                </code>
                <Button onClick={handleCopy}>
                  <Copy className="size-3.5" />
                  Copy token
                </Button>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
