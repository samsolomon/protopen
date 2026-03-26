import { useEffect, useState } from 'react'
import type { Project, SessionUser } from './types'
import { fetchSession, fetchProjects, postSignOut, deleteProjectById, SessionExpiredError } from './api'
import { AuthPage } from './AuthPage'
import { Dashboard } from './Dashboard'
import { Toaster } from '@/components/ui/sonner'

function App() {
  const [user, setUser] = useState<SessionUser | null>(null)
  const [sessionLoading, setSessionLoading] = useState(true)
  const [projects, setProjects] = useState<Project[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletingProjectID, setDeletingProjectID] = useState<string | null>(null)

  useEffect(() => {
    void loadSession()
  }, [])

  useEffect(() => {
    if (!user) {
      setProjects([])
      setIsLoading(false)
      return
    }

    void loadProjects()
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

  const loadProjects = async () => {
    try {
      setIsLoading(true)
      const loaded = await fetchProjects()
      setProjects(loaded)
      setError(null)
    } catch (loadError) {
      if (loadError instanceof SessionExpiredError) {
        setUser(null)
        return
      }
      setError(loadError instanceof Error ? loadError.message : 'Could not load projects')
    } finally {
      setIsLoading(false)
    }
  }

  const signOut = async () => {
    await postSignOut()
    setUser(null)
    setProjects([])
    setError(null)
  }

  const deleteProject = async (projectID: string) => {
    try {
      setDeletingProjectID(projectID)
      setError(null)
      await deleteProjectById(projectID)
      setProjects((current) => current.filter((project) => project.id !== projectID))
    } catch (deleteError) {
      if (deleteError instanceof SessionExpiredError) {
        setUser(null)
      }
      setError(deleteError instanceof Error ? deleteError.message : 'Could not delete project')
    } finally {
      setDeletingProjectID(null)
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
  } else {
    content = (
      <Dashboard
        user={user}
        projects={projects}
        isLoading={isLoading}
        deletingProjectID={deletingProjectID}
        error={error}
        setError={setError}
        onSignOut={() => void signOut()}
        onUserUpdated={setUser}
        onDeleteProject={(id) => void deleteProject(id)}
        onProjectsChanged={() => void loadProjects()}
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
