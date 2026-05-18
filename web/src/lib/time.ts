// timeAgo formats an ISO timestamp as a short relative-time string.
// Use longForm=true to include the >7-day fallback as a localized date.
export function timeAgo(iso: string, opts: { longForm?: boolean } = {}): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (opts.longForm && days >= 7) return new Date(iso).toLocaleDateString()
  return `${days}d ago`
}
