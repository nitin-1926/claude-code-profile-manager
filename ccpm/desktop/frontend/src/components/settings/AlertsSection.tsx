import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { DesktopPrefs } from '@/types'
import { Switch } from '@/components/ui/Switch'
import { useToast } from '@/components/ui/Toast'
import { Row } from '@/components/settings/NotchSection'

/**
 * AlertsSection is the opt-in notifications: both off until asked for, and like
 * the notch, app-wide even though it sits in a profile's Settings tab.
 */
export function AlertsSection() {
  const [prefs, setPrefs] = useState<DesktopPrefs | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [denied, setDenied] = useState(false)
  const toast = useToast()
  // Queued like the notch's writes, so two quick clicks cannot race.
  const queue = useRef<Promise<unknown>>(Promise.resolve())

  // macOS shows its sheet only while the answer is undecided, so this is what
  // prompts on opt-in and what notices a later change in System Settings. Kept
  // out of the queue: it can wait on a person for minutes.
  function checkPermission(p: DesktopPrefs) {
    if (!p.notifySessionEnd && !p.limitAlerts) return setDenied(false)
    api
      .requestNotificationPermission()
      .then((granted) => setDenied(!granted))
      .catch(() => setDenied(true))
  }

  useEffect(() => {
    api.prefs
      .get()
      .then((p) => {
        setPrefs(p)
        checkPermission(p)
      })
      .catch((e) => setLoadError(String(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function write(patch: Partial<DesktopPrefs>) {
    setPrefs((p) => (p ? { ...p, ...patch } : p))
    queue.current = queue.current
      .then(() => api.prefs.get())
      .then((cur) => api.prefs.setAlerts({ ...cur, ...patch }))
      .then((stored) => {
        setPrefs(stored)
        checkPermission(stored)
      })
      .catch((e) => {
        toast({ kind: 'error', title: 'Could not save notification settings', desc: String(e) })
        api.prefs.get().then(setPrefs).catch(() => undefined)
      })
  }

  const heading = (
    <h2 className="mb-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Notifications</h2>
  )

  if (!prefs)
    return (
      <div className="mb-6">
        {heading}
        <div className="rounded-xl border border-border bg-card px-4 py-3 text-xs text-muted-foreground">
          {loadError ? `Could not load the notification settings: ${loadError}` : 'Loading notification settings…'}
        </div>
      </div>
    )

  return (
    <div className="mb-6">
      {heading}
      <p className="mb-3 text-xs text-muted-foreground">
        macOS notifications while CCPM is running. Applies to every profile.
      </p>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <Row
          label="Session ended"
          description="The profile, its tokens and estimated cost. Turns on usage tracking, and applies to sessions started after this with ccpm run."
        >
          <Switch
            on={prefs.notifySessionEnd}
            label="Notify when a session ends"
            onClick={() => write({ notifySessionEnd: !prefs.notifySessionEnd })}
          />
        </Row>
        <Row
          label="Limit alerts"
          description="For each 5-hour and 7-day window: at the warning level, at 100%, and when it renews. Reads the limits ccpm’s status line records."
        >
          <Switch
            on={prefs.limitAlerts}
            label="Notify about usage limits"
            onClick={() => write({ limitAlerts: !prefs.limitAlerts })}
          />
        </Row>
        {prefs.limitAlerts && (
          <Row label="Warning level" description="Percent of a window used before the first alert.">
            <input
              // Remounts on save, so a refused value shows what was actually stored.
              key={prefs.limitAlertThreshold}
              type="number"
              min={1}
              max={99}
              defaultValue={prefs.limitAlertThreshold}
              aria-label="Limit alert warning level, percent"
              onBlur={(e) => {
                const v = Number(e.target.value)
                if (v !== prefs.limitAlertThreshold) write({ limitAlertThreshold: v })
              }}
              onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
              className="w-16 rounded-md border border-border bg-background px-2 py-1 text-right text-xs tabular-nums focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </Row>
        )}
      </div>

      {denied && (
        <p className="mt-2 text-[11px] text-muted-foreground">
          macOS is not allowing notifications from CCPM. Turn them on in System Settings, under Notifications.
        </p>
      )}
    </div>
  )
}
