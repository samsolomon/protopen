import { type FormEvent, useState } from 'react'
import type { AuthFormState, AuthMode, SessionUser } from './types'
import { postAuth, forgotPassword, resetPassword } from './api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Eye, EyeOff } from 'lucide-react'

function PasswordInput({ id, value, onChange, showPassword, onToggle }: {
  id: string
  value: string
  onChange: (value: string) => void
  showPassword: boolean
  onToggle: () => void
}) {
  return (
    <div className="relative">
      <Input
        id={id}
        type={showPassword ? 'text' : 'password'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="At least 8 characters"
        className="pr-9"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-0 top-0 h-full px-2.5 text-muted-foreground hover:text-foreground"
        onClick={onToggle}
        tabIndex={-1}
      >
        {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </Button>
    </div>
  )
}

type AuthPageProps = {
  onLogin: (user: SessionUser) => void
}

type AuthView = 'auth' | 'forgot' | 'reset'

export function AuthPage({ onLogin }: AuthPageProps) {
  const [authView, setAuthView] = useState<AuthView>(() => {
    const params = new URLSearchParams(window.location.search)
    return params.has('reset-token') ? 'reset' : 'auth'
  })
  const [authMode, setAuthMode] = useState<AuthMode>(
    window.location.pathname === '/sign-up' ? 'sign-up' : 'sign-in'
  )
  const [authForm, setAuthForm] = useState<AuthFormState>(() => {
    const params = new URLSearchParams(window.location.search)
    return { name: '', email: params.get('email') ?? '', password: '' }
  })
  const [authError, setAuthError] = useState<string | null>(null)
  const [authPending, setAuthPending] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  // Forgot password state
  const [forgotEmail, setForgotEmail] = useState('')
  const [forgotPending, setForgotPending] = useState(false)
  const [forgotSent, setForgotSent] = useState(false)

  // Reset password state
  const [resetToken] = useState(() => new URLSearchParams(window.location.search).get('reset-token') ?? '')
  const [resetPasswordValue, setResetPasswordValue] = useState('')
  const [resetPending, setResetPending] = useState(false)
  const [resetError, setResetError] = useState<string | null>(null)
  const [resetDone, setResetDone] = useState(false)

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

  const submitForgot = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setForgotPending(true)
    try {
      await forgotPassword(forgotEmail)
      setForgotSent(true)
    } finally {
      setForgotPending(false)
    }
  }

  const submitReset = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setResetPending(true)
    setResetError(null)
    try {
      await resetPassword(resetToken, resetPasswordValue)
      setResetDone(true)
      window.history.replaceState({}, '', window.location.pathname)
    } catch (err) {
      setResetError(err instanceof Error ? err.message : 'Could not reset password')
    } finally {
      setResetPending(false)
    }
  }

  const backToSignIn = () => {
    setAuthView('auth')
    setAuthMode('sign-in')
    setAuthError(null)
    setForgotSent(false)
    setForgotEmail('')
    window.history.replaceState({}, '', window.location.pathname)
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold tracking-tight">Velori</h1>
          <p className="mt-1 text-sm text-muted-foreground">Publish static prototypes in seconds.</p>
        </div>

        {authView === 'forgot' ? (
          <Card>
            <CardHeader>
              <CardTitle>Forgot password</CardTitle>
              <CardDescription>
                {forgotSent
                  ? 'If an account exists with that email, we sent a reset link.'
                  : "Enter your email and we'll send you a reset link."}
              </CardDescription>
            </CardHeader>
            {forgotSent ? (
              <CardFooter>
                <Button variant="ghost" className="w-full" onClick={backToSignIn}>Back to sign in</Button>
              </CardFooter>
            ) : (
              <form onSubmit={(event) => void submitForgot(event)}>
                <CardContent className="flex flex-col gap-4 pb-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="forgot-email">Email</Label>
                    <Input
                      id="forgot-email"
                      type="email"
                      value={forgotEmail}
                      onChange={(event) => setForgotEmail(event.target.value)}
                      placeholder="you@example.com"
                    />
                  </div>
                </CardContent>
                <CardFooter className="flex-col gap-2">
                  <Button className="w-full" disabled={forgotPending} type="submit">
                    {forgotPending ? 'Sending...' : 'Send reset link'}
                  </Button>
                  <Button variant="ghost" className="w-full" type="button" onClick={backToSignIn}>
                    Back to sign in
                  </Button>
                </CardFooter>
              </form>
            )}
          </Card>
        ) : authView === 'reset' ? (
          <Card>
            <CardHeader>
              <CardTitle>{resetDone ? 'Password reset' : 'Set new password'}</CardTitle>
              <CardDescription>
                {resetDone
                  ? 'Your password has been updated. You can now sign in.'
                  : 'Enter your new password below.'}
              </CardDescription>
            </CardHeader>
            {resetDone ? (
              <CardFooter>
                <Button className="w-full" onClick={backToSignIn}>Sign in</Button>
              </CardFooter>
            ) : (
              <form onSubmit={(event) => void submitReset(event)}>
                <CardContent className="flex flex-col gap-4 pb-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="reset-password">New password</Label>
                    <PasswordInput
                      id="reset-password"
                      value={resetPasswordValue}
                      onChange={setResetPasswordValue}
                      showPassword={showPassword}
                      onToggle={() => setShowPassword((s) => !s)}
                    />
                  </div>
                  {resetError ? <p className="text-sm text-destructive">{resetError}</p> : null}
                </CardContent>
                <CardFooter>
                  <Button className="w-full" disabled={resetPending} type="submit">
                    {resetPending ? 'Resetting...' : 'Reset password'}
                  </Button>
                </CardFooter>
              </form>
            )}
          </Card>
        ) : (
          <Tabs value={authMode} onValueChange={(value) => { setAuthMode(value as AuthMode); setAuthError(null); window.history.replaceState(null, '', value === 'sign-up' ? '/sign-up' : '/sign-in') }}>
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
                  <CardContent className="flex flex-col gap-4 pb-4">
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="signin-email">Email</Label>
                      <Input
                        id="signin-email"
                        type="email"
                        value={authForm.email}
                        onChange={(event) => setAuthForm((current) => ({ ...current, email: event.target.value }))}
                        placeholder="you@example.com"
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <div className="flex items-center justify-between">
                        <Label htmlFor="signin-password">Password</Label>
                        <button
                          type="button"
                          className="text-xs text-muted-foreground hover:text-foreground"
                          onClick={() => { setAuthView('forgot'); setForgotEmail(authForm.email); setAuthError(null) }}
                        >
                          Forgot password?
                        </button>
                      </div>
                      <PasswordInput
                        id="signin-password"
                        value={authForm.password}
                        onChange={(value) => setAuthForm((current) => ({ ...current, password: value }))}
                        showPassword={showPassword}
                        onToggle={() => setShowPassword((s) => !s)}
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
                  <CardContent className="flex flex-col gap-4 pb-4">
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="signup-name">Name</Label>
                      <Input
                        id="signup-name"
                        value={authForm.name}
                        onChange={(event) => setAuthForm((current) => ({ ...current, name: event.target.value }))}
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="signup-email">Email</Label>
                      <Input
                        id="signup-email"
                        type="email"
                        value={authForm.email}
                        onChange={(event) => setAuthForm((current) => ({ ...current, email: event.target.value }))}
                        placeholder="you@example.com"
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <Label htmlFor="signup-password">Password</Label>
                      <PasswordInput
                        id="signup-password"
                        value={authForm.password}
                        onChange={(value) => setAuthForm((current) => ({ ...current, password: value }))}
                        showPassword={showPassword}
                        onToggle={() => setShowPassword((s) => !s)}
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
        )}
      </div>
    </div>
  )
}
