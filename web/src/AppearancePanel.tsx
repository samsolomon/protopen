import { useEffect, useState } from 'react'
import { Sun, Moon, Monitor } from 'lucide-react'
import { type Theme, applyTheme, getStoredTheme, setStoredTheme } from './theme'

export function AppearancePanel() {
  const [theme, setTheme] = useState<Theme>(getStoredTheme)

  useEffect(() => {
    setStoredTheme(theme)
    applyTheme(theme)

    if (theme !== 'system') return

    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const listener = () => applyTheme('system')
    media.addEventListener('change', listener)
    return () => media.removeEventListener('change', listener)
  }, [theme])

  const options: { value: Theme; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: 'Light', icon: Sun },
    { value: 'dark', label: 'Dark', icon: Moon },
    { value: 'system', label: 'System', icon: Monitor },
  ]

  return (
    <section>
      <h3 className="mb-3 text-sm font-medium">Appearance</h3>
      <div className="rounded-lg border bg-card">
        <div className="flex items-center justify-between px-4 py-3">
          <div>
            <p className="text-sm">Theme</p>
            <p className="text-sm text-muted-foreground">Choose how Velori looks for you.</p>
          </div>
          <div className="flex gap-1">
            {options.map(({ value, label, icon: Icon }) => (
              <button
                key={value}
                type="button"
                onClick={() => setTheme(value)}
                className={`flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors ${
                  theme === value
                    ? 'border-primary bg-primary/5 text-primary'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                }`}
              >
                <Icon className="size-3.5" />
                {label}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
