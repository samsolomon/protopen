import { type FormEvent, useState } from 'react'
import type { AuthFormState, AuthMode, SessionUser } from './types'
import { demoEmail, demoPassword } from './constants'
import { postAuth } from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type AuthPageProps = {
  onLogin: (user: SessionUser) => void
}

export function AuthPage({ onLogin }: AuthPageProps) {
  const [authMode, setAuthMode] = useState<AuthMode>('sign-in')
  const [authForm, setAuthForm] = useState<AuthFormState>({ name: '', email: demoEmail, password: demoPassword })
  const [authError, setAuthError] = useState<string | null>(null)
  const [authPending, setAuthPending] = useState(false)

  const submitAuth = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setAuthPending(true)
    setAuthError(null)

    try {
      const user = await postAuth(authMode, authForm)
      setAuthForm((current) => ({ ...current, password: authMode === 'sign-up' ? '' : current.password }))
      onLogin(user)
    } catch (authFailure) {
      setAuthError(authFailure instanceof Error ? authFailure.message : 'Could not authenticate')
    } finally {
      setAuthPending(false)
    }
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold tracking-tight">Velori</h1>
          <p className="mt-1 text-sm text-muted-foreground">Publish static prototypes in seconds.</p>
        </div>

        <Tabs value={authMode} onValueChange={(value) => { setAuthMode(value as AuthMode); setAuthError(null) }}>
          <TabsList className="w-full">
            <TabsTrigger value="sign-in">Sign in</TabsTrigger>
            <TabsTrigger value="sign-up">Create account</TabsTrigger>
          </TabsList>

          <TabsContent value="sign-in">
            <Card>
              <CardHeader>
                <CardTitle>Sign in</CardTitle>
                <CardDescription>Enter your credentials to access your projects.</CardDescription>
              </CardHeader>
              <form onSubmit={(event) => void submitAuth(event)}>
                <CardContent className="flex flex-col gap-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="signin-email">Email</Label>
                    <Input
                      id="signin-email"
                      type="email"
                      value={authForm.email}
                      onChange={(event) => setAuthForm((current) => ({ ...current, email: event.target.value }))}
                      placeholder="sam@velori.dev"
                    />
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="signin-password">Password</Label>
                    <Input
                      id="signin-password"
                      type="password"
                      value={authForm.password}
                      onChange={(event) => setAuthForm((current) => ({ ...current, password: event.target.value }))}
                      placeholder="At least 8 characters"
                    />
                  </div>
                  {authError ? <p className="text-sm text-destructive">{authError}</p> : null}
                </CardContent>
                <CardFooter>
                  <Button className="w-full" disabled={authPending} type="submit">
                    {authPending ? 'Signing in...' : 'Sign in'}
                  </Button>
                </CardFooter>
              </form>
            </Card>
          </TabsContent>

          <TabsContent value="sign-up">
            <Card>
              <CardHeader>
                <CardTitle>Create account</CardTitle>
                <CardDescription>Get started with your first prototype.</CardDescription>
              </CardHeader>
              <form onSubmit={(event) => void submitAuth(event)}>
                <CardContent className="flex flex-col gap-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="signup-name">Name</Label>
                    <Input
                      id="signup-name"
                      value={authForm.name}
                      onChange={(event) => setAuthForm((current) => ({ ...current, name: event.target.value }))}
                      placeholder="Taylor Prototype"
                    />
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="signup-email">Email</Label>
                    <Input
                      id="signup-email"
                      type="email"
                      value={authForm.email}
                      onChange={(event) => setAuthForm((current) => ({ ...current, email: event.target.value }))}
                      placeholder="sam@velori.dev"
                    />
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="signup-password">Password</Label>
                    <Input
                      id="signup-password"
                      type="password"
                      value={authForm.password}
                      onChange={(event) => setAuthForm((current) => ({ ...current, password: event.target.value }))}
                      placeholder="At least 8 characters"
                    />
                  </div>
                  {authError ? <p className="text-sm text-destructive">{authError}</p> : null}
                </CardContent>
                <CardFooter>
                  <Button className="w-full" disabled={authPending} type="submit">
                    {authPending ? 'Creating account...' : 'Create account'}
                  </Button>
                </CardFooter>
              </form>
            </Card>
          </TabsContent>
        </Tabs>

        <p className="mt-4 text-center text-xs text-muted-foreground">
          Demo: <span className="font-medium text-foreground">{demoEmail}</span> / <span className="font-medium text-foreground">{demoPassword}</span>
        </p>
      </div>
    </div>
  )
}
