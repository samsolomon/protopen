import { useState } from 'react'
import type { Site } from './types'
import { API_BASE_URL } from './constants'
import { cn } from '@/lib/utils'

const HUES = [15, 45, 145, 200, 265, 330, 175, 55]

function hashCode(str: string): number {
  let h = 0
  for (let i = 0; i < str.length; i++) {
    h = ((h << 5) - h + str.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

function getColor(id: string) {
  const hue = HUES[hashCode(id) % HUES.length]
  return {
    bg: `oklch(0.75 0.12 ${hue})`,
    text: `oklch(0.98 0.01 ${hue})`,
  }
}

function getInitials(name: string): string {
  const words = name.split(/[\s\-_.\d]+/).filter(Boolean)
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase()
  const w = words[0] || name
  return w.slice(0, 2).toUpperCase()
}

type SiteThumbnailProps = {
  site: Site
  size?: 'card' | 'row'
  className?: string
}

export function SiteThumbnail({ site, size = 'card', className }: SiteThumbnailProps) {
  const [failed, setFailed] = useState(false)
  const color = getColor(site.id)

  const containerCls = cn(
    'aspect-[16/10] shrink-0 overflow-hidden',
    size === 'card' ? 'w-full rounded-t-xl' : 'w-16 rounded-md',
    className,
  )
  const initialsCls = size === 'card' ? 'text-2xl' : 'text-[10px]'

  if (failed) {
    return (
      <div
        className={cn(containerCls, 'flex items-center justify-center')}
        style={{ background: color.bg }}
      >
        <span className={cn('font-semibold select-none', initialsCls)} style={{ color: color.text }}>
          {getInitials(site.name)}
        </span>
      </div>
    )
  }
  return (
    <img
      src={`${API_BASE_URL}/api/sites/${site.id}/thumbnail`}
      crossOrigin="use-credentials"
      onError={() => setFailed(true)}
      alt=""
      loading="lazy"
      className={cn(containerCls, 'object-cover')}
    />
  )
}
