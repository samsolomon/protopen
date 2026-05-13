import { useEffect, useState } from 'react'
import type { Site, SessionUser } from './types'
import { fetchSession, fetchSites, postSignOut, deleteSiteById, updateProjectVisibility, verifyEmail, SessionExpiredError } from './api'
import { AuthPage } from './AuthPage'
import { CLIAuthPage } from './CLIAuthPage'
import { DeviceAuthPage } from './DeviceAuthPage'
import { Dashboard } from './Dashboard'
import { Toaster } from '@/components/ui/sonner'

const isCLIAuth = new URLSearchParams(window.location.search).has('cli-auth')
const isDeviceAuth = window.location.pathname === '/auth/device'

function App() {
  const [user, setUser] = useState<SessionUser | null>(null)
  const [sessionLoading, setSessionLoading] = useState(true)
  const [sites, setSites] = useState<Site[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletingSiteID, setDeletingSiteID] = useState<string | null>(null)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const verifyToken = params.get('verify-token')
    if (verifyToken) {
      void verifyEmail(verifyToken)
        .catch(() => {})
        .finally(() => {
          window.history.replaceState({}, '', window.location.pathname)
          void loadSession()
        })
    } else {
      void loadSession()
    }
  }, [])

  useEffect(() => {
    if (!user) {
      setSites([])
      setIsLoading(false)
      return
    }

    void loadSites()
  }, [user?.id])

  const loadSession = async () => {
    try {
      setSessionLoading(true)
      const sessionUser = await fetchSession()
      setUser(sessionUser)
    } catch {
      // ignored — user lands on auth page
    } finally {
      setSessionLoading(false)
    }
  }

  const loadSites = async () => {
    try {
      setIsLoading(true)
      const loaded = await fetchSites()
      setSites(loaded)
      setError(null)
    } catch (loadError) {
      if (loadError instanceof SessionExpiredError) {
        setUser(null)
        return
      }
      setError(loadError instanceof Error ? loadError.message : 'Could not load sites')
    } finally {
      setIsLoading(false)
    }
  }

  const signOut = async () => {
    await postSignOut()
    setUser(null)
    setSites([])
    setError(null)
  }

  const deleteSite = async (siteID: string) => {
    try {
      setDeletingSiteID(siteID)
      setError(null)
      await deleteSiteById(siteID)
      setSites((current) => current.filter((site) => site.id !== siteID))
    } catch (deleteError) {
      if (deleteError instanceof SessionExpiredError) {
        setUser(null)
      }
      setError(deleteError instanceof Error ? deleteError.message : 'Could not delete site')
    } finally {
      setDeletingSiteID(null)
    }
  }

  const toggleVisibility = async (siteID: string, isPublic: boolean) => {
    setSites((current) => current.map((s) => (s.id === siteID ? { ...s, isPublic } : s)))
    try {
      await updateProjectVisibility(siteID, isPublic)
    } catch (toggleError) {
      if (toggleError instanceof SessionExpiredError) {
        setUser(null)
        return
      }
      setSites((current) => current.map((s) => (s.id === siteID ? { ...s, isPublic: !isPublic } : s)))
      setError(toggleError instanceof Error ? toggleError.message : 'Could not update site')
    }
  }

  const handleSessionExpired = () => {
    setUser(null)
  }

  let content
  if (sessionLoading) {
    content = (
      <div className="flex min-h-svh items-center justify-center bg-background">
        <p className="text-sm text-muted-foreground">Checking session...</p>
      </div>
    )
  } else if (!user) {
    content = <AuthPage onLogin={setUser} />
  } else if (isDeviceAuth) {
    content = <DeviceAuthPage onSessionExpired={handleSessionExpired} />
  } else if (isCLIAuth) {
    content = <CLIAuthPage onSessionExpired={handleSessionExpired} />
  } else {
    content = (
      <Dashboard
        user={user}
        sites={sites}
        isLoading={isLoading}
        deletingSiteID={deletingSiteID}
        error={error}
        setError={setError}
        onSignOut={() => void signOut()}
        onUserUpdated={setUser}
        onDeleteSite={(id) => void deleteSite(id)}
        onVisibilityToggle={(id, isPublic) => void toggleVisibility(id, isPublic)}
        onSitesChanged={() => void loadSites()}
        onSessionExpired={handleSessionExpired}
      />
    )
  }

  return (
    <>
      {content}
      <Toaster />
    </>
  )
}

export default App
