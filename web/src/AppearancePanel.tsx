import { useEffect, useState } from 'react'
import { Sun, Moon, Monitor } from 'lucide-react'
import { type Theme, applyTheme, getStoredTheme, setStoredTheme } from './theme'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

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

  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-center justify-between px-4 py-3">
        <div>
          <p className="text-sm">Theme</p>
          <p className="text-sm text-muted-foreground">Choose how Protopen looks for you.</p>
        </div>
        <Tabs value={theme} onValueChange={(value) => setTheme(value as Theme)}>
          <TabsList className="!flex-row">
            <TabsTrigger value="light">
              <Sun />
              Light
            </TabsTrigger>
            <TabsTrigger value="dark">
              <Moon />
              Dark
            </TabsTrigger>
            <TabsTrigger value="system">
              <Monitor />
              System
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
    </div>
  )
}
