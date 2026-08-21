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
  const triggerRef = useRef<HTMLButtonElement>(null)

  // Closing must hand focus back to the trigger; letting it fall to <body>
  // restarts Tab at the top of the app.
  function close(refocus = true) {
    setOpen(false)
    if (refocus) triggerRef.current?.focus()
  }

  useEffect(() => {
    if (!open) return
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation()
        setOpen(false)
        triggerRef.current?.focus()
      }
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
        ref={triggerRef}
        onClick={() => setOpen((o) => !o)}
        title="Theme"
        aria-label="Change theme"
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <Active aria-hidden className="size-4" />
      </button>
      {open && (
        <div
          role="menu"
          aria-label="Theme"
          className="absolute right-0 top-8 z-50 w-44 overflow-hidden rounded-lg border border-border bg-popover p-1 shadow-lg"
        >
          {THEMES.map((t) => {
            const Icon = ICON[t.id]
            const active = theme === t.id
            return (
              <button
                key={t.id}
                role="menuitemradio"
                aria-checked={active}
                onClick={() => {
                  setTheme(t.id)
                  close()
                }}
                className={cn(
                  'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent',
                  active ? 'text-foreground' : 'text-muted-foreground',
                )}
              >
                <Icon aria-hidden className="size-3.5 shrink-0" />
                <span className="flex-1 text-xs">
                  {t.label}
                  <span className="ml-1.5 text-[10px] text-muted-foreground">
                    {t.hint}
                  </span>
                </span>
                {active && <Check aria-hidden className="size-3.5 shrink-0 text-primary" />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
