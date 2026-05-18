import { describe, it, expect } from 'vitest'
import {
  scopeFromSearch,
  buildScopeURL,
  filterSitesByScope,
  filterSitesByQuery,
  canMutateSite,
} from './site-scope'
import type { Site } from './types'

function makeSite(overrides: Partial<Site> & { id: string }): Site {
  return {
    id: overrides.id,
    name: overrides.name ?? 'Site',
    slug: overrides.slug ?? 'site',
    updatedAt: overrides.updatedAt ?? 'just now',
    deployCount: overrides.deployCount ?? 1,
    liveUrl: overrides.liveUrl ?? 'https://example.test',
    isPublic: overrides.isPublic ?? false,
    gitBranch: overrides.gitBranch,
    gitCommitHash: overrides.gitCommitHash,
    gitRemoteURL: overrides.gitRemoteURL,
    createdBy: overrides.createdBy,
  }
}

describe('scopeFromSearch', () => {
  it('returns "all" when view=all', () => {
    expect(scopeFromSearch('?view=all')).toBe('all')
    expect(scopeFromSearch('?foo=bar&view=all')).toBe('all')
  })

  it('returns "mine" for any other value or when absent', () => {
    expect(scopeFromSearch('')).toBe('mine')
    expect(scopeFromSearch('?view=mine')).toBe('mine')
    expect(scopeFromSearch('?view=anything')).toBe('mine')
    expect(scopeFromSearch('?other=1')).toBe('mine')
  })
})

describe('buildScopeURL', () => {
  it('adds view=all when scope is all', () => {
    const result = buildScopeURL('https://app.test/dashboard', 'all')
    expect(new URL(result).searchParams.get('view')).toBe('all')
  })

  it('removes view param when scope is mine', () => {
    const result = buildScopeURL('https://app.test/dashboard?view=all', 'mine')
    expect(new URL(result).searchParams.has('view')).toBe(false)
  })

  it('preserves other query params', () => {
    const result = buildScopeURL('https://app.test/dashboard?tab=x&view=all', 'mine')
    const url = new URL(result)
    expect(url.searchParams.get('tab')).toBe('x')
    expect(url.searchParams.has('view')).toBe(false)
  })
})

describe('filterSitesByScope', () => {
  const me = 'user_1'
  const other = 'user_2'
  const sites: Site[] = [
    makeSite({ id: 's1', createdBy: { id: me, name: 'Me', username: 'me' } }),
    makeSite({ id: 's2', createdBy: { id: other, name: 'Other', username: 'other' } }),
    makeSite({ id: 's3', createdBy: null }),
  ]

  it('returns all sites for scope=all', () => {
    expect(filterSitesByScope(sites, 'all', me).map((s) => s.id)).toEqual(['s1', 's2', 's3'])
  })

  it('returns only sites created by the current user for scope=mine', () => {
    expect(filterSitesByScope(sites, 'mine', me).map((s) => s.id)).toEqual(['s1'])
  })

  it('excludes orphan sites (createdBy null) from "mine"', () => {
    expect(filterSitesByScope(sites, 'mine', me).some((s) => s.id === 's3')).toBe(false)
  })
})

describe('filterSitesByQuery', () => {
  const sites: Site[] = [
    makeSite({ id: 's1', name: 'Marketing Page', slug: 'marketing' }),
    makeSite({ id: 's2', name: 'Internal Tool', slug: 'tool-v2' }),
    makeSite({ id: 's3', name: 'Docs', slug: 'docs' }),
  ]

  it('returns everything for empty / whitespace query', () => {
    expect(filterSitesByQuery(sites, '').length).toBe(3)
    expect(filterSitesByQuery(sites, '   ').length).toBe(3)
  })

  it('matches on name (case-insensitive)', () => {
    expect(filterSitesByQuery(sites, 'MARKET').map((s) => s.id)).toEqual(['s1'])
  })

  it('matches on slug (case-insensitive)', () => {
    expect(filterSitesByQuery(sites, 'V2').map((s) => s.id)).toEqual(['s2'])
  })

  it('returns empty when nothing matches', () => {
    expect(filterSitesByQuery(sites, 'zzz')).toEqual([])
  })

  it('trims surrounding whitespace before matching', () => {
    expect(filterSitesByQuery(sites, '  MARKET  ').map((s) => s.id)).toEqual(['s1'])
  })
})

describe('canMutateSite', () => {
  const me = 'user_1'
  const other = 'user_2'
  const mine = makeSite({ id: 's1', createdBy: { id: me, name: 'Me', username: 'me' } })
  const theirs = makeSite({ id: 's2', createdBy: { id: other, name: 'Other', username: 'other' } })
  const orphan = makeSite({ id: 's3', createdBy: null })

  it('allows the creator', () => {
    expect(canMutateSite(mine, me, false)).toBe(true)
  })

  it('denies a non-creator member', () => {
    expect(canMutateSite(theirs, me, false)).toBe(false)
  })

  it('allows an org admin regardless of creator', () => {
    expect(canMutateSite(theirs, me, true)).toBe(true)
    expect(canMutateSite(orphan, me, true)).toBe(true)
  })

  it('denies a non-admin on an orphan site', () => {
    expect(canMutateSite(orphan, me, false)).toBe(false)
  })

  it('denies when currentUserId is undefined (and not admin)', () => {
    expect(canMutateSite(mine, undefined, false)).toBe(false)
  })

  it('allows admin even when currentUserId is undefined', () => {
    expect(canMutateSite(theirs, undefined, true)).toBe(true)
  })
})
