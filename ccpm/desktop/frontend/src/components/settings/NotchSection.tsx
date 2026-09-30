import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import type { DesktopPrefs, ProfileLimits, RailEdge, RailMain, RailMode } from '@/types'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/Toast'
import { cn } from '@/lib/utils'

const REVEALS: { value: RailMode; label: string }[] = [
  { value: 'hover', label: 'On hover' },
  { value: 'always', label: 'Always open' },
]

const EDGES: { value: RailEdge; label: string }[] = [
  { value: 'top', label: 'Top' },
  { value: 'left', label: 'Left' },
  { value: 'right', label: 'Right' },
  { value: 'bottom', label: 'Bottom' },
]

const MAINS: { value: RailMain; label: string }[] = [
  { value: 'five_hour', label: '5-hour' },
  { value: 'seven_day', label: 'Weekly' },
]

/**
 * NotchSection is the usage notch's settings: a master switch, and only when it
 * is on, how the notch reveals, where it sits, what it draws, and which
 * profiles get a ring. The notch is app-wide, so none of this is per-profile
 * even though it sits in a profile's Settings tab.
 *
 * Writes go straight to PrefsService, which reshapes the notch itself — there
 * is no second "apply" call for this component to forget.
 */
export function NotchSection() {
  const [prefs, setPrefs] = useState<DesktopPrefs | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [limits, , limitsError] = useLive<ProfileLimits[]>(() => api.limits.all(), [])
  const toast = useToast()
  // Writes are queued so each one merges onto what the previous one stored,
  // and two quick clicks cannot race each other back to a stale file.
  const queue = useRef<Promise<unknown>>(Promise.resolve())

  useEffect(() => {
    api.prefs
      .get()
      .then(setPrefs)
      .catch((e) => setLoadError(String(e)))
  }, [])

  function failed(e: unknown) {
    toast({ kind: 'error', title: 'Could not save notch settings', desc: String(e) })
    api.prefs.get().then(setPrefs).catch(() => undefined)
  }

  // setNotch stores only the notch's own fields, merged in Go under a lock, so
  // the theme toggle (which writes the same file) is never put back by a copy
  // read before it changed. It returns what was stored, so the section renders
  // normalized values rather than its own guess.
  function write(patch: Partial<DesktopPrefs>) {
    setPrefs((p) => (p ? { ...p, ...patch } : p))
    queue.current = queue.current
      .then(() => api.prefs.get())
      .then((cur) => api.prefs.setNotch({ ...cur, ...patch }))
      .then(setPrefs)
      .catch(failed)
  }

  function toggleProfile(name: string, on: boolean) {
    setPrefs((p) => (p ? { ...p, railProfiles: { ...p.railProfiles, [name]: on } } : p))
    queue.current = queue.current
      .then(() => api.prefs.setRailProfile(name, on))
      .then(setPrefs)
      .catch(failed)
  }

  const heading = (
    <h2 className="mb-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Usage notch</h2>
  )

  if (loadError)
    return (
      <div className="mb-6">
        {heading}
        <div className="rounded-xl border border-border bg-card px-4 py-3 text-xs text-destructive">
          Could not load the notch settings: {loadError}
        </div>
      </div>
    )
  if (!prefs)
    return (
      <div className="mb-6">
        {heading}
        <div className="rounded-xl border border-border bg-card px-4 py-3 text-xs text-muted-foreground">
          Loading notch settings…
        </div>
      </div>
    )

  const profiles = limits ?? []
  // An absent key means enabled, so a newly created profile appears on the
  // notch without the user having to come and find a switch for it.
  const enabled = (name: string) => prefs.railProfiles?.[name] !== false
  // The notch reads limits that only ccpm's own status line records. When no
  // profile has a reading and at least one could have one (a plan that does
  // not report limits never will), say why every ring is a dash.
  const noReadings =
    profiles.length > 0 && !profiles.some((p) => p.available) && profiles.some((p) => p.reason === 'no-data')

  return (
    <div className="mb-6">
      {heading}
      <p className="mb-3 text-xs text-muted-foreground">
        A ring per profile at the edge of your screen, showing how much of its Claude plan limits is used.
        Applies to every profile.
      </p>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <Row label="Show usage notch" description="Off removes it from the screen entirely.">
          <Switch on={prefs.railOn} label="Show usage notch" onClick={() => write({ railOn: !prefs.railOn })} />
        </Row>

        {prefs.railOn && (
          <>
            <Row label="Reveal" description="Open when the pointer reaches it, or stay open.">
              <Choice label="Reveal" options={REVEALS} value={prefs.railMode} onChange={(v) => write({ railMode: v })} />
            </Row>
            <Row label="Position" description="Top joins the camera notch on Macs that have one.">
              <Choice label="Position" options={EDGES} value={prefs.railEdge} onChange={(v) => write({ railEdge: v })} />
            </Row>
            <Row label="Main ring" description="The big ring. The other window is the thin inner ring.">
              <Choice label="Main ring" options={MAINS} value={prefs.railMain} onChange={(v) => write({ railMain: v })} />
            </Row>
            <Row label="Show percentage" description="The main ring's figure under each ring. Off gives the rings more room.">
              <Switch
                on={prefs.railPercent}
                label="Show percentage"
                onClick={() => write({ railPercent: !prefs.railPercent })}
              />
            </Row>

            <div className="border-b border-border bg-muted/40 px-4 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              Profiles
            </div>
            {limitsError ? (
              <div className="px-4 py-2.5 text-[11px] text-destructive">Could not load profiles: {limitsError}</div>
            ) : limits === null ? (
              <div className="px-4 py-2.5 text-[11px] text-muted-foreground">Loading profiles…</div>
            ) : (
              profiles.length === 0 && (
                <div className="px-4 py-2.5 text-[11px] text-muted-foreground">No profiles yet.</div>
              )
            )}
            {profiles.map((p) => (
              <Row
                key={p.profile}
                label={p.profile}
                // Say why a ring will be empty rather than letting the user
                // wonder. An unavailable profile still gets a switch: the
                // reading may start arriving later.
                description={
                  p.available
                    ? p.plan || 'Subscription'
                    : p.reason === 'not-subscription-account'
                      ? 'Plan does not report limits'
                      : 'No reading yet'
                }
              >
                <Switch
                  on={enabled(p.profile)}
                  label={`Show ${p.profile} on the notch`}
                  onClick={() => toggleProfile(p.profile, !enabled(p.profile))}
                />
              </Row>
            ))}
          </>
        )}
      </div>

      {prefs.railOn && noReadings && (
        <p className="mt-2 text-[11px] text-muted-foreground">
          No profile has a reading yet. The notch shows the limits ccpm’s own status line records while you use
          Claude Code, so a profile with a custom statusLine shows a dash.
        </p>
      )}
    </div>
  )
}

function Row({ label, description, children }: { label: string; description: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-foreground">{label}</div>
        <div className="text-[11px] text-muted-foreground">{description}</div>
      </div>
      {children}
    </div>
  )
}

// A segmented control with the WAI-ARIA radio-group behaviour its role
// promises: arrow keys move the selection (and focus with it), Home/End jump to
// the ends, and a roving tabindex keeps the group to a single tab stop.
function Choice<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: { value: T; label: string }[]
  value: T
  onChange: (v: T) => void
}) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  function onKeyDown(e: React.KeyboardEvent) {
    const i = options.findIndex((o) => o.value === value)
    let next = -1
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (i + 1) % options.length
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (i - 1 + options.length) % options.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = options.length - 1
    if (next < 0) return
    e.preventDefault()
    refs.current[next]?.focus()
    onChange(options[next].value)
  }

  return (
    <div
      role="radiogroup"
      aria-label={label}
      onKeyDown={onKeyDown}
      className="flex shrink-0 items-center gap-0.5 rounded-md border border-border p-0.5"
    >
      {options.map((o, i) => {
        const checked = o.value === value
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => onChange(o.value)}
            className={cn(
              'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              checked ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}
