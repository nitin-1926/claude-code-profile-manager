import { useCallback, useEffect, useRef, useState } from 'react'
import { Gauge, Check, type LucideIcon, PanelRight, PanelLeft, PanelTop, PanelBottom } from 'lucide-react'
import { Get, Set, SetRailProfile } from '@/../wailsjs/go/services/PrefsService'
import { All } from '@/../wailsjs/go/services/LimitsService'
import { services } from '@/../wailsjs/go/models'
import { Switch } from '@/components/ui/Switch'
import { cn } from '@/lib/utils'

const MODES = [
  { id: 'always', label: 'Always visible', hint: 'pinned open' },
  { id: 'hover', label: 'Show on hover', hint: 'peeks at the edge' },
  { id: 'hidden', label: 'Hidden', hint: 'off entirely' },
] as const

const EDGES: { id: string; label: string; Icon: LucideIcon }[] = [
  { id: 'left', label: 'Left', Icon: PanelLeft },
  { id: 'right', label: 'Right', Icon: PanelRight },
  { id: 'top', label: 'Top', Icon: PanelTop },
  { id: 'bottom', label: 'Bottom', Icon: PanelBottom },
]

// RailMenu is the title-bar control for the floating usage rail: where it sits,
// when it shows, and which profiles get a ring.
//
// Writes go straight to PrefsService, which reshapes the rail itself — there is
// no second "apply" call for this component to forget.
export function RailMenu() {
  const [open, setOpen] = useState(false)
  const [prefs, setPrefs] = useState<services.DesktopPrefs | null>(null)
  const [profiles, setProfiles] = useState<services.ProfileLimits[]>([])
  const ref = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  // Read on open rather than on mount: the rail's preferences can be changed by
  // another window or a hand-edited file, and a stale menu would write back
  // whatever it happened to load at startup.
  const load = useCallback(() => {
    Get().then(setPrefs).catch(() => setPrefs(null))
    All()
      .then((p) => setProfiles(p ?? []))
      .catch(() => setProfiles([]))
  }, [])

  useEffect(() => {
    if (open) load()
  }, [open, load])

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

  // Set returns what was actually stored, so the menu renders the normalized
  // values rather than its own optimistic guess.
  function write(next: Partial<services.DesktopPrefs>) {
    if (!prefs) return
    Set(services.DesktopPrefs.createFrom({ ...prefs, ...next }))
      .then(setPrefs)
      .catch(() => undefined)
  }

  function toggleProfile(name: string, on: boolean) {
    SetRailProfile(name, on)
      .then(setPrefs)
      .catch(() => undefined)
  }

  // An absent key means enabled, so a newly created profile appears on the rail
  // without the user having to come and find a switch for it.
  const enabled = (name: string) => prefs?.railProfiles?.[name] !== false

  return (
    <div ref={ref} className="relative">
      <button
        ref={triggerRef}
        onClick={() => setOpen((o) => !o)}
        title="Usage rail"
        aria-label="Usage rail settings"
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <Gauge aria-hidden className="size-4" />
      </button>

      {open && (
        <div
          role="menu"
          aria-label="Usage rail"
          className="absolute right-0 top-8 z-50 w-72 overflow-hidden rounded-lg border border-border bg-popover shadow-lg"
        >
          <div className="border-b border-border px-3 py-2">
            <p className="text-xs font-medium text-foreground">Usage rail</p>
            <p className="mt-0.5 text-[10px] leading-snug text-muted-foreground">
              A floating ring per profile, showing your Claude limits.
            </p>
          </div>

          <div className="p-1">
            {MODES.map((m) => {
              const active = prefs?.railMode === m.id
              return (
                <button
                  key={m.id}
                  role="menuitemradio"
                  aria-checked={active}
                  onClick={() => write({ railMode: m.id })}
                  className={cn(
                    'flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent',
                    active ? 'text-foreground' : 'text-muted-foreground',
                  )}
                >
                  <span className="flex-1 text-xs">
                    {m.label}
                    <span className="ml-1.5 text-[10px] text-muted-foreground">{m.hint}</span>
                  </span>
                  {active && <Check aria-hidden className="size-3.5 shrink-0 text-primary" />}
                </button>
              )
            })}
          </div>

          <div className="border-t border-border px-3 py-2">
            <p className="mb-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              Screen edge
            </p>
            <div className="flex gap-1">
              {EDGES.map(({ id, label, Icon }) => {
                const active = prefs?.railEdge === id
                return (
                  <button
                    key={id}
                    title={label}
                    aria-label={label}
                    aria-pressed={active}
                    disabled={prefs?.railMode === 'hidden'}
                    onClick={() => write({ railEdge: id })}
                    className={cn(
                      'flex flex-1 items-center justify-center rounded-md border py-1.5 transition-colors disabled:opacity-40',
                      active
                        ? 'border-primary bg-primary/10 text-primary'
                        : 'border-border text-muted-foreground hover:bg-accent hover:text-foreground',
                    )}
                  >
                    <Icon aria-hidden className="size-3.5" />
                  </button>
                )
              })}
            </div>
          </div>

          <div className="border-t border-border px-3 py-2">
            <p className="mb-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              Profiles
            </p>
            {profiles.length === 0 && (
              <p className="py-1 text-[11px] text-muted-foreground">No profiles yet.</p>
            )}
            {profiles.map((p) => (
              <div key={p.profile} className="flex items-center gap-2 py-1">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs text-foreground">{p.profile}</p>
                  {/* Say why a ring will be empty rather than letting the user
                      wonder. An unavailable profile still gets a switch — the
                      reading may start arriving later. */}
                  <p className="truncate text-[10px] text-muted-foreground">
                    {p.available
                      ? p.plan || 'Subscription'
                      : p.reason === 'not-subscription-account'
                        ? 'Plan does not report limits'
                        : 'No reading yet'}
                  </p>
                </div>
                <Switch
                  on={enabled(p.profile)}
                  disabled={prefs?.railMode === 'hidden'}
                  label={`Show ${p.profile} on the rail`}
                  onClick={() => toggleProfile(p.profile, !enabled(p.profile))}
                />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
