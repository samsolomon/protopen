import type { SiteAuthor } from './types'

function initialsFor(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

function hueFor(seed: string): number {
  let h = 0
  for (let i = 0; i < seed.length; i += 1) {
    h = (h * 31 + seed.charCodeAt(i)) & 0xffff
  }
  return h % 360
}

type AuthorChipProps = {
  author: SiteAuthor | null | undefined
  showName?: boolean
  className?: string
}

export function AuthorChip({ author, showName = true, className = '' }: AuthorChipProps) {
  if (!author) {
    return (
      <span className={`inline-flex items-center gap-1.5 text-xs text-muted-foreground ${className}`}>
        <span
          aria-hidden="true"
          className="inline-flex size-4 items-center justify-center rounded-full bg-muted text-[9px] font-medium text-muted-foreground"
        >
          ?
        </span>
        {showName ? <span className="truncate">Unknown</span> : null}
      </span>
    )
  }

  const initials = initialsFor(author.name || author.username || '?')
  const hue = hueFor(author.id || author.username || author.name || '0')

  return (
    <span
      className={`inline-flex items-center gap-1.5 text-xs text-muted-foreground ${className}`}
      title={author.name}
    >
      <span
        aria-hidden="true"
        className="inline-flex size-4 items-center justify-center rounded-full text-[9px] font-medium text-white"
        style={{ backgroundColor: `hsl(${hue} 55% 45%)` }}
      >
        {initials}
      </span>
      {showName ? (
        <span className="truncate">{author.name || author.username}</span>
      ) : null}
    </span>
  )
}
