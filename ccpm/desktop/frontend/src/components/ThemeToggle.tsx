import { useEffect, useRef, useState } from 'react'
import { Sun, Moon, Contrast, Check, type LucideIcon } from 'lucide-react'
import { useTheme, THEMES, type Theme } from '@/lib/theme'
import { cn } from '@/lib/utils'

const ICON: Record<Theme, LucideIcon> = {
  light: Sun,
  midnight: Moon,
  graphite: Contrast,
}

// ThemeToggle lives in the title bar. A single icon button (showing the active
// theme) opens a small menu of the three palettes. Choice persists via the
// ThemeProvider (localStorage), so it survives relaunches and updates.
export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const Active = ICON[theme]

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        title="Theme"
        aria-label="Change theme"
        className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <Active className="size-4" />
      </button>
      {open && (
        <div className="absolute right-0 top-8 z-50 w-44 overflow-hidden rounded-lg border border-border bg-popover p-1 shadow-lg">
          {THEMES.map((t) => {
            const Icon = ICON[t.id]
            const active = theme === t.id
            return (
              <button
                key={t.id}
                onClick={() => {
                  setTheme(t.id)
                  setOpen(false)
                }}
                className={cn(
                  'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent',
                  active ? 'text-foreground' : 'text-muted-foreground',
                )}
              >
                <Icon className="size-3.5 shrink-0" />
                <span className="flex-1 text-xs">
                  {t.label}
                  <span className="ml-1.5 text-[10px] text-muted-foreground">
                    {t.hint}
                  </span>
                </span>
                {active && <Check className="size-3.5 shrink-0 text-primary" />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
