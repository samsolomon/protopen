import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function commitURL(remoteURL: string, hash: string): string {
  try {
    const host = new URL(remoteURL).hostname
    if (host === 'bitbucket.org') return `${remoteURL}/commits/${hash}`
    if (host === 'gitlab.com' || host.startsWith('gitlab.')) return `${remoteURL}/-/commit/${hash}`
  } catch {}
  return `${remoteURL}/commit/${hash}`
}
