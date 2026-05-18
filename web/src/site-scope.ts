import type { Site } from './types'

export type SiteScope = 'mine' | 'all'

export function scopeFromSearch(search: string): SiteScope {
  const params = new URLSearchParams(search)
  return params.get('view') === 'all' ? 'all' : 'mine'
}

export function buildScopeURL(currentHref: string, scope: SiteScope): string {
  const url = new URL(currentHref)
  if (scope === 'all') {
    url.searchParams.set('view', 'all')
  } else {
    url.searchParams.delete('view')
  }
  return url.toString()
}

export function filterSitesByScope(
  sites: Site[],
  scope: SiteScope,
  currentUserId: string,
): Site[] {
  if (scope === 'all') return sites
  return sites.filter((site) => site.createdBy?.id === currentUserId)
}

export function filterSitesByQuery(sites: Site[], query: string): Site[] {
  const trimmed = query.trim().toLowerCase()
  if (!trimmed) return sites
  return sites.filter(
    (site) =>
      site.name.toLowerCase().includes(trimmed) ||
      site.slug.toLowerCase().includes(trimmed),
  )
}

export function canMutateSite(
  site: Site,
  currentUserId: string | undefined,
  isOrgAdmin: boolean,
): boolean {
  if (isOrgAdmin) return true
  if (!currentUserId) return false
  return site.createdBy?.id === currentUserId
}
