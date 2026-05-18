import type { AuthFormState, AuthMode, Deploy, OrgInfo, OrgMember, Site, SessionUser, UploadFile, UploadSummary } from './types'
import { API_BASE_URL } from './constants'

export async function fetchSession(): Promise<SessionUser | null> {
  const response = await fetch(`${API_BASE_URL}/api/session`, {
    credentials: 'include',
  })

  if (response.status === 401) {
    return null
  }

  if (!response.ok) {
    throw new Error('Could not load session')
  }

  const data = (await response.json()) as { user: SessionUser }
  return data.user
}

export async function fetchSites(): Promise<Site[]> {
  const response = await fetch(`${API_BASE_URL}/api/sites`, {
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    throw new Error('Could not load sites')
  }

  const data = (await response.json()) as { sites: Site[] }
  return data.sites ?? []
}

export async function postAuth(mode: AuthMode, form: AuthFormState): Promise<SessionUser> {
  const endpoint = mode === 'sign-in' ? '/api/sign-in' : '/api/sign-up'
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(form),
  })

  const data = (await response.json()) as { error?: string; user?: SessionUser }
  if (!response.ok || !data.user) {
    throw new Error(data.error ?? 'Could not authenticate')
  }

  return data.user
}

export async function postSignOut(): Promise<void> {
  await fetch(`${API_BASE_URL}/api/sign-out`, {
    method: 'POST',
    credentials: 'include',
  })
}

export async function postUpload(summary: UploadSummary, files: UploadFile[]): Promise<void> {
  const formData = new FormData()
  formData.set('name', summary.name)
  formData.set('mode', summary.mode)
  formData.set('paths', JSON.stringify(files.map((file) => file.path)))

  files.forEach((file) => {
    formData.append('files', file.file, file.name)
  })

  const response = await fetch(`${API_BASE_URL}/api/uploads`, {
    method: 'POST',
    body: formData,
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Upload failed')
  }
}

export async function deleteSiteById(siteID: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/sites/${siteID}`, {
    method: 'DELETE',
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Could not delete site')
  }
}

export async function updateProjectVisibility(siteID: string, isPublic: boolean): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/sites/${siteID}`, {
    method: 'PATCH',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ isPublic }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Could not update site')
  }
}

export async function fetchDeploys(siteID: string): Promise<Deploy[]> {
  const response = await fetch(`${API_BASE_URL}/api/sites/${siteID}/deploys`, {
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    throw new Error('Could not load deploys')
  }

  const data = (await response.json()) as { deploys: Deploy[] }
  return data.deploys ?? []
}

export async function duplicateSite(siteID: string): Promise<Site> {
  const response = await fetch(`${API_BASE_URL}/api/sites/${siteID}/duplicate`, {
    method: 'POST',
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  const data = (await response.json()) as { error?: string; site?: Site }
  if (!response.ok || !data.site) {
    throw new Error(data.error ?? 'Could not duplicate site')
  }

  return data.site
}

export async function rollbackDeploy(siteID: string, deployId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/sites/${siteID}/rollback`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ deployId }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Could not rollback')
  }
}

export class SessionExpiredError extends Error {
  constructor() {
    super('Your session expired. Sign in again.')
  }
}

export type ApiToken = {
  id: string
  name: string
  createdAt: string
}

export async function fetchTokens(): Promise<ApiToken[]> {
  const response = await fetch(`${API_BASE_URL}/api/tokens`, {
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    throw new Error('Could not load tokens')
  }

  const data = (await response.json()) as { tokens: ApiToken[] }
  return data.tokens ?? []
}

export async function createToken(name: string): Promise<{ id: string; name: string; token: string }> {
  const response = await fetch(`${API_BASE_URL}/api/tokens`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Could not create token')
  }

  return (await response.json()) as { id: string; name: string; token: string }
}

export async function deleteToken(tokenId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/tokens/${tokenId}`, {
    method: 'DELETE',
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const body = (await response.json()) as { error?: string }
    throw new Error(body.error ?? 'Could not delete token')
  }
}

export async function updateProfile(name: string, email: string): Promise<SessionUser> {
  const response = await fetch(`${API_BASE_URL}/api/account/profile`, {
    method: 'PATCH',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, email }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  const data = (await response.json()) as { error?: string; user?: SessionUser }
  if (!response.ok || !data.user) {
    throw new Error(data.error ?? 'Could not update profile')
  }

  return data.user
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/account/password`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ currentPassword, newPassword }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not change password')
  }
}

export async function deleteAccount(password: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/account/delete`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not delete account')
  }
}

export async function fetchOrgMembers(orgId: string): Promise<OrgMember[]> {
  const response = await fetch(`${API_BASE_URL}/api/orgs/${orgId}/members`, {
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    throw new Error('Could not load members')
  }

  const data = (await response.json()) as { members: OrgMember[] }
  return data.members ?? []
}

export async function addOrgMember(orgId: string, email: string, role: string): Promise<{ status: string }> {
  const response = await fetch(`${API_BASE_URL}/api/orgs/${orgId}/members`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, role }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  const data = (await response.json()) as { error?: string; status?: string }
  if (!response.ok) {
    throw new Error(data.error ?? 'Could not add member')
  }

  return { status: data.status ?? 'added' }
}

export async function updateMemberRole(orgId: string, memberId: string, role: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/orgs/${orgId}/members/${memberId}`, {
    method: 'PATCH',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not update role')
  }
}

export async function removeOrgMember(orgId: string, memberId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/orgs/${orgId}/members/${memberId}`, {
    method: 'DELETE',
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not remove member')
  }
}

export async function forgotPassword(email: string): Promise<void> {
  await fetch(`${API_BASE_URL}/api/forgot-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
}

export async function resetPassword(token: string, password: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/reset-password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, password }),
  })

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not reset password')
  }
}

export async function verifyEmail(token: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/verify-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  })

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not verify email')
  }
}

export async function resendVerification(): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/resend-verification`, {
    method: 'POST',
    credentials: 'include',
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not resend verification')
  }
}

export async function createOrg(name: string, slug: string): Promise<OrgInfo> {
  const response = await fetch(`${API_BASE_URL}/api/orgs`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, slug }),
  })

  if (response.status === 401) {
    throw new SessionExpiredError()
  }

  const data = (await response.json()) as { error?: string; org?: OrgInfo }
  if (!response.ok || !data.org) {
    throw new Error(data.error ?? 'Could not create organization')
  }

  return data.org
}

// ---- Admin ----

export type AdminUserOrg = {
  orgId: string
  orgSlug: string
  orgName: string
  memberId: string
  role: string
  isPersonal: boolean
}

export type AdminUser = {
  id: string
  email: string
  name: string
  username: string
  emailVerifiedAt?: string | null
  createdAt: string
  orgs: AdminUserOrg[]
}

export async function fetchAdminUsers(): Promise<AdminUser[]> {
  const response = await fetch(`${API_BASE_URL}/api/admin/users`, {
    credentials: 'include',
  })

  if (response.status === 401) throw new SessionExpiredError()
  if (response.status === 403) throw new Error('Forbidden')

  const data = (await response.json()) as { users?: AdminUser[] }
  return data.users ?? []
}

export async function adminDeleteUser(userId: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}/api/admin/users/${userId}`, {
    method: 'DELETE',
    credentials: 'include',
  })

  if (response.status === 401) throw new SessionExpiredError()
  if (!response.ok) {
    const data = (await response.json()) as { error?: string }
    throw new Error(data.error ?? 'Could not delete user')
  }
}

export type AdminSettings = {
  thumbnails: {
    available: boolean
    enabled: boolean
    reason?: string
  }
}

export async function fetchAdminSettings(): Promise<AdminSettings> {
  const response = await fetch(`${API_BASE_URL}/api/admin/settings`, {
    credentials: 'include',
  })

  if (response.status === 401) throw new SessionExpiredError()
  if (response.status === 403) throw new Error('Forbidden')
  if (!response.ok) throw new Error('Could not load settings')

  return (await response.json()) as AdminSettings
}

export async function updateAdminSettings(patch: { thumbnailsEnabled?: boolean }): Promise<AdminSettings> {
  const response = await fetch(`${API_BASE_URL}/api/admin/settings`, {
    method: 'PATCH',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })

  if (response.status === 401) throw new SessionExpiredError()
  if (!response.ok) {
    const data = (await response.json().catch(() => ({}))) as { error?: string }
    throw new Error(data.error ?? 'Could not update settings')
  }

  return (await response.json()) as AdminSettings
}


