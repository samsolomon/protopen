import { describe, it, expect, vi, beforeEach, afterAll } from 'vitest'
import { fetchSession, fetchSites, postAuth, deleteSiteById, duplicateSite, SessionExpiredError } from './api'

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

beforeEach(() => {
  mockFetch.mockReset()
})

afterAll(() => {
  vi.restoreAllMocks()
})

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  }
}


describe('fetchSession', () => {
  it('returns user on success', async () => {
    const user = { id: '1', email: 'sam@test.com', name: 'Sam', username: 'sam', orgs: [] }
    mockFetch.mockResolvedValue(jsonResponse(200, { user }))

    const result = await fetchSession()
    expect(result).toEqual(user)
    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('/api/session'), expect.objectContaining({ credentials: 'include' }))
  })

  it('returns null on 401 instead of throwing', async () => {
    mockFetch.mockResolvedValue(jsonResponse(401, {}))

    const result = await fetchSession()
    expect(result).toBeNull()
  })

  it('throws on non-401 errors', async () => {
    mockFetch.mockResolvedValue(jsonResponse(500, {}))

    await expect(fetchSession()).rejects.toThrow('Could not load session')
  })
})


describe('postAuth', () => {
  it('returns user on success', async () => {
    const user = { id: '1', email: 'sam@test.com', name: 'Sam', username: 'sam', orgs: [] }
    mockFetch.mockResolvedValue(jsonResponse(200, { user }))

    const result = await postAuth('sign-in', { name: '', email: 'sam@test.com', password: 'password' })
    expect(result).toEqual(user)
  })

  it('throws with server error message on failure', async () => {
    mockFetch.mockResolvedValue(jsonResponse(401, { error: 'Invalid email or password' }))

    await expect(postAuth('sign-in', { name: '', email: 'bad@test.com', password: 'wrong' }))
      .rejects.toThrow('Invalid email or password')
  })

  it('uses sign-up endpoint for sign-up mode', async () => {
    const user = { id: '1', email: 'new@test.com', name: 'New', username: 'new', orgs: [] }
    mockFetch.mockResolvedValue(jsonResponse(200, { user }))

    await postAuth('sign-up', { name: 'New', email: 'new@test.com', password: 'password123' })
    expect(mockFetch).toHaveBeenCalledWith(expect.stringContaining('/api/sign-up'), expect.anything())
  })
})


describe('fetchSites', () => {
  it('returns sites on success', async () => {
    const sites = [{ id: '1', name: 'Test', slug: 'test' }]
    mockFetch.mockResolvedValue(jsonResponse(200, { sites }))

    const result = await fetchSites()
    expect(result).toEqual(sites)
  })

  it('throws SessionExpiredError on 401', async () => {
    mockFetch.mockResolvedValue(jsonResponse(401, {}))

    await expect(fetchSites()).rejects.toBeInstanceOf(SessionExpiredError)
  })

  it('returns empty array when sites is null', async () => {
    mockFetch.mockResolvedValue(jsonResponse(200, { sites: null }))

    const result = await fetchSites()
    expect(result).toEqual([])
  })
})


describe('deleteSiteById', () => {
  it('succeeds on 200', async () => {
    mockFetch.mockResolvedValue(jsonResponse(200, {}))

    await expect(deleteSiteById('site_1')).resolves.toBeUndefined()
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sites/site_1'),
      expect.objectContaining({ method: 'DELETE', credentials: 'include' }),
    )
  })

  it('throws SessionExpiredError on 401', async () => {
    mockFetch.mockResolvedValue(jsonResponse(401, {}))

    await expect(deleteSiteById('site_1')).rejects.toBeInstanceOf(SessionExpiredError)
  })

  it('throws with server error message on failure', async () => {
    mockFetch.mockResolvedValue(jsonResponse(403, { error: 'Not authorized' }))

    await expect(deleteSiteById('site_1')).rejects.toThrow('Not authorized')
  })
})


describe('duplicateSite', () => {
  it('returns the duplicated site on 201', async () => {
    const site = { id: 'site_2', name: 'Copy of Test', slug: 'test-copy' }
    mockFetch.mockResolvedValue(jsonResponse(201, { site }))

    const result = await duplicateSite('site_1')
    expect(result).toEqual(site)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/sites/site_1/duplicate'),
      expect.objectContaining({ method: 'POST', credentials: 'include' }),
    )
  })

  it('throws SessionExpiredError on 401', async () => {
    mockFetch.mockResolvedValue(jsonResponse(401, {}))

    await expect(duplicateSite('site_1')).rejects.toBeInstanceOf(SessionExpiredError)
  })

  it('throws with server error message on failure', async () => {
    mockFetch.mockResolvedValue(jsonResponse(404, { error: 'site not found' }))

    await expect(duplicateSite('site_1')).rejects.toThrow('site not found')
  })

  it('throws when response is missing site', async () => {
    mockFetch.mockResolvedValue(jsonResponse(201, {}))

    await expect(duplicateSite('site_1')).rejects.toThrow('Could not duplicate site')
  })
})
