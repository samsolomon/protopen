import { type FormEvent, useState } from 'react'
import type { AuthFormState, AuthMode, SessionUser } from './types'
import { demoEmail, demoPassword } from './constants'
import { postAuth } from './api'

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
    <div className="app-shell auth-shell">
      <div className="hero-glow hero-glow-left" />
      <div className="hero-glow hero-glow-right" />

      <main className="auth-layout">
        <section className="auth-copy-card">
          <p className="eyebrow">Velori v1</p>
          <h1>Publish static prototypes in seconds.</h1>
          <p>
            Sign in to upload and manage your projects. Live prototype links stay public on the separate content origin.
          </p>
          <div className="auth-tips">
            <span>Demo account</span>
            <strong>{demoEmail}</strong>
            <strong>{demoPassword}</strong>
          </div>
        </section>

        <section className="auth-form-card">
          <div className="auth-toggle">
            <button
              className={authMode === 'sign-in' ? 'primary-button' : 'ghost-button'}
              onClick={() => setAuthMode('sign-in')}
              type="button"
            >
              Sign in
            </button>
            <button
              className={authMode === 'sign-up' ? 'primary-button' : 'ghost-button'}
              onClick={() => setAuthMode('sign-up')}
              type="button"
            >
              Create account
            </button>
          </div>

          <form className="auth-form" onSubmit={(event) => void submitAuth(event)}>
            {authMode === 'sign-up' ? (
              <label>
                <span>Name</span>
                <input
                  value={authForm.name}
                  onChange={(event) => setAuthForm((current) => ({ ...current, name: event.target.value }))}
                  placeholder="Taylor Prototype"
                />
              </label>
            ) : null}
            <label>
              <span>Email</span>
              <input
                type="email"
                value={authForm.email}
                onChange={(event) => setAuthForm((current) => ({ ...current, email: event.target.value }))}
                placeholder="sam@velori.dev"
              />
            </label>
            <label>
              <span>Password</span>
              <input
                type="password"
                value={authForm.password}
                onChange={(event) => setAuthForm((current) => ({ ...current, password: event.target.value }))}
                placeholder="At least 8 characters"
              />
            </label>
            {authError ? <p className="error-banner">{authError}</p> : null}
            <button className="primary-button auth-submit" disabled={authPending} type="submit">
              {authPending ? 'Working...' : authMode === 'sign-in' ? 'Sign in' : 'Create account'}
            </button>
          </form>
        </section>
      </main>
    </div>
  )
}
