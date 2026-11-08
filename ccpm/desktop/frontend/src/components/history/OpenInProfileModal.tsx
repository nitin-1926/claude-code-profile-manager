import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import type { HistorySession, LimitWindow, ProfileLimits } from '@/types'
import { timeAgo } from '@/lib/format'
import { Modal } from '@/components/ui/Modal'
import { useToast } from '@/components/ui/Toast'
import { useGuarded } from '@/lib/useGuarded'

const CREDITS_URL =
  'https://support.claude.com/en/articles/12429409-manage-usage-credits-for-paid-claude-plans'

/** At or above this, the source profile's own reading leads the dialog. */
const NEAR_LIMIT = 90

const WINDOW_NAMES: Record<string, { short: string; long: string }> = {
  five_hour: { short: '5h', long: '5-hour' },
  seven_day: { short: '7d', long: '7-day' },
}

/** A window past its reset has rolled over and is empty until the next reading,
 *  the same rule the notch draws by (rail/model.go usedNow). */
function usedNow(w: LimitWindow): number {
  return w.resetsAt > 0 && w.resetsAt * 1000 <= Date.now() ? 0 : w.usedPercentage
}

/** The window closest to its limit, which is the one that decides headroom. */
function tightest(l: ProfileLimits): LimitWindow | null {
  if (!l.available) return null
  return l.windows.reduce<LimitWindow | null>((a, w) => (!a || usedNow(w) > usedNow(a) ? w : a), null)
}

/** Most headroom first; profiles with no reading last, since unknown is not free. */
function byHeadroom(a: ProfileLimits, b: ProfileLimits): number {
  const ta = tightest(a)
  const tb = tightest(b)
  if (ta && tb) return usedNow(ta) - usedNow(tb) || a.profile.localeCompare(b.profile)
  if (ta) return -1
  if (tb) return 1
  return a.profile.localeCompare(b.profile)
}

/** "15:45" today, "Tue 15:45" on another day. */
function clock(unixSeconds: number): string {
  const d = new Date(unixSeconds * 1000)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (d.toDateString() === new Date().toDateString()) return time
  return `${d.toLocaleDateString([], { weekday: 'short' })} ${time}`
}

/** `work is at its 5-hour limit until 15:45. Continue in labs?`, or null when the
 *  source's reading gives no reason to lead with it. */
function limitLead(source: ProfileLimits | undefined, best: ProfileLimits | undefined): string | null {
  const w = source && tightest(source)
  if (!source || !w) return null
  const used = usedNow(w)
  if (used < NEAR_LIMIT) return null
  const name = WINDOW_NAMES[w.key]?.long ?? w.label
  const state =
    used >= 100
      ? `is at its ${name} limit${w.resetsAt > 0 ? ` until ${clock(w.resetsAt)}` : ''}`
      : `is at ${Math.round(used)}% of its ${name} limit${w.resetsAt > 0 ? `, renewing ${clock(w.resetsAt)}` : ''}`
  const bestWindow = best && tightest(best)
  const ask =
    best && bestWindow && usedNow(bestWindow) < NEAR_LIMIT
      ? `Continue in ${best.profile}?`
      : 'Continue in another profile?'
  return `${source.profile} ${state}. ${ask}`
}

function WindowLine({ w }: { w: LimitWindow }) {
  const renewed = w.resetsAt > 0 && w.resetsAt * 1000 <= Date.now()
  return (
    <span className="tabular-nums">
      {WINDOW_NAMES[w.key]?.short ?? w.label} {Math.round(usedNow(w))}%
      {renewed ? ' · renewed' : w.resetsAt > 0 && ` · renews ${clock(w.resetsAt)}`}
    </span>
  )
}

/**
 * Continue a session under another profile. Each row is one explicit click that
 * acts on that profile: nothing here is preselected, and nothing ever switches
 * on its own.
 */
export function OpenInProfileModal({
  source,
  session,
  onClose,
}: {
  source: string
  session: HistorySession | null
  onClose: () => void
}) {
  const [limits, setLimits] = useState<ProfileLimits[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const toast = useToast()
  const guard = useGuarded('Open in profile')

  const open = session !== null
  useEffect(() => {
    if (!open) return
    let current = true
    setLimits(null)
    setError(null)
    api.limits
      .all()
      .then((l) => {
        if (current) setLimits(l)
      })
      .catch((e) => {
        if (current) setError(`Could not load profiles: ${String(e)}`)
      })
    return () => {
      current = false
    }
  }, [open])

  const choose = (target: string) =>
    guard(async () => {
      if (!session || busy) return
      setBusy(target)
      setError(null)
      try {
        const r = await api.history.openInProfile(source, target, session.id)
        if (r.ok) {
          toast({ kind: 'info', title: `Continuing in ${target}`, desc: r.output })
          onClose()
        } else {
          setError(r.error || r.output)
        }
      } finally {
        setBusy(null)
      }
    })

  const others = (limits ?? []).filter((l) => l.profile !== source).sort(byHeadroom)
  const lead = limitLead(limits?.find((l) => l.profile === source), others[0])

  return (
    <Modal open={open} onClose={onClose} title="Open in profile">
      {lead && <p className="mb-2.5 text-sm font-medium">{lead}</p>}
      <p className="truncate text-xs text-muted-foreground">{session?.title || session?.id}</p>

      <ul className="mt-2.5 list-disc space-y-1 pl-4 text-xs text-muted-foreground">
        <li>
          The conversation continues under the target profile's account, skills, MCP servers and
          billing. The original stays in {source}.
        </li>
        <li>
          Switching accounts starts a cold cache, so the next message re-sends the whole conversation
          and costs more than a normal turn on that account.
        </li>
        <li>
          Pro and Max accounts can continue past a limit on the same account with{' '}
          <button
            type="button"
            onClick={() => api.openURL(CREDITS_URL)}
            className="cursor-pointer text-foreground underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            usage credits
          </button>
          , often the better option.
        </li>
        <li>
          <code className="rounded bg-muted px-1 py-px">/rewind</code> checkpoints stay behind with{' '}
          {source}.
        </li>
      </ul>

      <div className="mt-3 max-h-52 space-y-1.5 overflow-y-auto">
        {limits === null && !error && <p className="text-xs text-muted-foreground">Loading profiles…</p>}
        {limits !== null && others.length === 0 && (
          <p className="text-xs text-muted-foreground">
            There are no other profiles yet. Add one with <code>ccpm add</code>.
          </p>
        )}
        {others.map((l) => (
          <button
            key={l.profile}
            type="button"
            disabled={busy !== null}
            onClick={choose(l.profile)}
            className="block w-full cursor-pointer rounded-lg border border-border px-3 py-2 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
          >
            <div className="flex items-baseline gap-2">
              <span className="truncate text-sm font-medium">{l.profile}</span>
              <span className="truncate text-[11px] text-muted-foreground">
                {[l.account, l.plan].filter(Boolean).join(' · ')}
              </span>
              <span className="ml-auto shrink-0 text-[11px] text-primary">
                {busy === l.profile ? 'Continuing…' : `Continue in ${l.profile}`}
              </span>
            </div>
            <div className="mt-0.5 flex flex-wrap gap-x-3 text-[11px] text-muted-foreground">
              {l.available ? (
                <>
                  {l.windows.map((w) => (
                    <WindowLine key={w.key} w={w} />
                  ))}
                  <span>read {timeAgo(new Date(l.capturedAt * 1000).toISOString())}</span>
                </>
              ) : (
                <span>unknown</span>
              )}
            </div>
          </button>
        ))}
      </div>

      {error && <p className="mt-2.5 text-xs text-destructive">{error}</p>}

      <div className="mt-3 flex justify-end">
        <button
          type="button"
          onClick={onClose}
          className="cursor-pointer rounded-md px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Cancel
        </button>
      </div>
    </Modal>
  )
}
