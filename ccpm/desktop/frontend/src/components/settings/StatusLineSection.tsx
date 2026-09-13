import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { CmdResult, StatusLineConfig, StatusLineRow, StatusLineSegment } from '@/types'
import { useToast } from '@/components/ui/Toast'
import { cn } from '@/lib/utils'
import { RotateCcw } from 'lucide-react'

/** Which layout the section is editing. */
type Scope = 'global' | 'profile'

const ROWS: { value: StatusLineRow; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'row1', label: 'Row 1' },
  { value: 'row2', label: 'Row 2' },
]

/**
 * StatusLineSection edits which segments `ccpm statusline` renders, and on
 * which of its two rows.
 *
 * Two scopes share one control: the global default, and this profile's
 * override. Editing under "This profile" creates the override; Reset removes it
 * so the profile follows the global again.
 *
 * Deliberately a plain effect rather than useLive. Saving writes
 * ~/.ccpm/config.json, which the Go watcher watches, so a useLive subscription
 * would refetch its own write on a 300 ms delay and fight the optimistic state
 * below. HistoryTab and UsageTab avoid the identical loop the same way.
 */
export function StatusLineSection({ profile }: { profile: string }) {
  const [cfg, setCfg] = useState<StatusLineConfig | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [scope, setScope] = useState<Scope>('global')
  const [rows, setRows] = useState<Record<string, StatusLineRow>>({})
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  // Fetches carry a generation so a response for a profile the user has since
  // switched away from is discarded rather than painting one profile's layout
  // under another's name — the same guard HistoryTab uses, and the worst
  // available bug in an app whose whole premise is isolation.
  const gen = useRef(0)

  const load = useCallback(
    async (preferred?: Scope) => {
      const mine = ++gen.current
      try {
        const c = await api.statusline.get(profile)
        if (mine !== gen.current) return
        setCfg(c)
        setError(null)
        // Open on whichever scope is actually in force, so the controls match
        // what this profile's sessions are showing rather than a default guess.
        const next = preferred ?? (c.hasOverride ? 'profile' : 'global')
        setScope(next)
        setRows(toRowMap(next === 'profile' ? c.segments : c.global))
      } catch (e) {
        if (mine === gen.current) setError(String(e))
      }
    },
    [profile],
  )

  useEffect(() => {
    void load()
  }, [load])

  function switchScope(next: Scope) {
    if (!cfg) return
    setScope(next)
    // Seed the profile scope from whatever it currently resolves to, so
    // switching to it and saving reproduces today's status line rather than
    // silently resetting to the built-in.
    setRows(toRowMap(next === 'profile' ? cfg.segments : cfg.global))
  }

  function report(action: string, r: CmdResult) {
    if (r.ok) toast({ kind: 'success', title: action })
    else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
  }

  async function save() {
    if (!cfg) return
    const action = scope === 'profile' ? `Saved for ${profile}` : 'Saved for every profile'
    setBusy(true)
    try {
      const bucket = (want: StatusLineRow) => cfg.segments.filter((s) => rows[s.key] === want).map((s) => s.key)
      // Every segment is sent exactly once — the CLI refuses an incomplete
      // layout rather than guessing, so a bug here surfaces as a visible error.
      const r = await api.statusline.set(
        scope === 'profile' ? profile : '',
        bucket('row1'),
        bucket('row2'),
        bucket('off'),
      )
      report(action, r)
      if (r.ok) await load(scope)
    } catch (e) {
      // The bridge call itself can reject (the app closing mid-call, a binding
      // fault). Without this the click fails silently and leaves an unhandled
      // rejection, so the user sees a spinner stop and nothing else.
      toast({ kind: 'error', title: `${action} failed`, desc: String(e) })
    } finally {
      setBusy(false)
    }
  }

  async function reset() {
    const action = scope === 'profile' ? `${profile} follows the global status line` : 'Restored the default layout'
    setBusy(true)
    try {
      const r = await api.statusline.reset(scope === 'profile' ? profile : '')
      report(action, r)
      if (r.ok) await load('global')
    } catch (e) {
      toast({ kind: 'error', title: 'Reset failed', desc: String(e) })
    } finally {
      setBusy(false)
    }
  }

  if (error)
    return (
      <div className="mb-6 rounded-xl border border-border bg-card px-4 py-3 text-xs text-destructive">
        Could not load the status line config: {error}
      </div>
    )
  if (!cfg)
    return (
      <div className="mb-6 rounded-xl border border-border bg-card px-4 py-3 text-xs text-muted-foreground">
        Loading status line settings…
      </div>
    )

  const saved = scope === 'profile' ? cfg.segments : cfg.global
  const dirty = saved.some((s) => rows[s.key] !== s.row)
  // A profile with no override has nothing of its own to reset.
  const canReset = scope === 'profile' ? cfg.hasOverride : true

  return (
    <div className="mb-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Status line</h2>
        <div className="flex items-center gap-1 rounded-md border border-border p-0.5">
          {(['global', 'profile'] as Scope[]).map((s) => (
            <button
              key={s}
              onClick={() => switchScope(s)}
              className={cn(
                'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors',
                scope === s ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {s === 'global' ? 'Global default' : 'This profile'}
              {s === 'profile' && cfg.hasOverride && ' •'}
            </button>
          ))}
        </div>
      </div>

      <p className="mb-3 text-xs text-muted-foreground">
        {scope === 'global'
          ? 'Which segments every profile’s status line shows, and on which row.'
          : cfg.hasOverride
            ? `${profile} has its own layout, which wins over the global default.`
            : `${profile} follows the global default. Saving here gives it a layout of its own.`}{' '}
        A segment still drops out when Claude Code sends no data for it — the usage windows are absent on API-key
        profiles.
        {!cfg.enabled && (
          <>
            {' '}
            <span className="text-foreground">
              The status line is currently switched off — turn it on with{' '}
              <span className="font-mono">ccpm config set statusline true</span>.
            </span>
          </>
        )}
      </p>

      <Preview segments={cfg.segments} rows={rows} />

      <div className="mt-3 overflow-hidden rounded-xl border border-border bg-card">
        {cfg.segments.map((s, i) => (
          <SegmentRow
            key={s.key}
            segment={s}
            value={rows[s.key] ?? 'off'}
            first={i === 0}
            disabled={busy}
            onChange={(row) => setRows((r) => ({ ...r, [s.key]: row }))}
          />
        ))}
      </div>

      <div className="mt-3 flex items-center gap-2">
        <button
          disabled={busy || !dirty}
          onClick={save}
          className="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-[11px] font-medium text-primary-foreground transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-40"
        >
          {scope === 'profile' ? `Save for ${profile}` : 'Save for every profile'}
        </button>
        <button
          disabled={busy || !canReset}
          onClick={reset}
          className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-40"
        >
          <RotateCcw className="size-3" />
          {scope === 'profile' ? 'Follow the global default' : 'Restore defaults'}
        </button>
        {dirty && <span className="text-[11px] text-muted-foreground">Unsaved changes</span>}
      </div>
    </div>
  )
}

function SegmentRow({
  segment,
  value,
  first,
  disabled,
  onChange,
}: {
  segment: StatusLineSegment
  value: StatusLineRow
  first: boolean
  disabled: boolean
  onChange: (row: StatusLineRow) => void
}) {
  return (
    <div className={cn('flex items-center gap-3 px-4 py-2.5', !first && 'border-t border-border')}>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-foreground">{segment.label}</div>
        <div className="truncate text-[11px] text-muted-foreground">{segment.description}</div>
      </div>
      <div
        role="radiogroup"
        aria-label={segment.label}
        className="flex shrink-0 items-center gap-0.5 rounded-md border border-border p-0.5"
      >
        {ROWS.map((r) => (
          <button
            key={r.value}
            role="radio"
            aria-checked={value === r.value}
            disabled={disabled}
            onClick={() => onChange(r.value)}
            className={cn(
              'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors disabled:cursor-default disabled:opacity-50',
              value === r.value
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {r.label}
          </button>
        ))}
      </div>
    </div>
  )
}

/**
 * Preview renders the pending layout against sample values, so the effect of a
 * click is visible before saving. Deliberately mirrors `ccpm statusline
 * configure`'s printed preview; the sample covers every segment so nothing
 * reads as broken merely for want of data.
 */
function Preview({ segments, rows }: { segments: StatusLineSegment[]; rows: Record<string, StatusLineRow> }) {
  const line = (want: StatusLineRow) =>
    segments
      .filter((s) => rows[s.key] === want)
      .map((s) => SAMPLE[s.key] ?? s.label)
      .join(' · ')

  const one = line('row1')
  const two = line('row2')

  return (
    <div className="rounded-xl border border-border bg-background px-4 py-3 font-mono text-[11px] leading-relaxed">
      {one === '' && two === '' ? (
        <div className="text-muted-foreground">(every segment off — the status line prints nothing)</div>
      ) : (
        <>
          {one !== '' && <div className="truncate text-foreground">{one}</div>}
          {two !== '' && <div className="truncate text-foreground">{two}</div>}
        </>
      )}
    </div>
  )
}

/** Sample values, matching statusLinePreviewInput in cmd/statusline_configure.go. */
const SAMPLE: Record<string, string> = {
  profile: '⬢ work',
  workspace: 'ccpm/cmd',
  branch: '⎇ main',
  model: 'Opus 5',
  context: 'ctx 34%',
  effort: 'effort high',
  five_hour: '5h 42% ↺16:15',
  seven_day: '7d 78% ↺Mon 8 Sep 09:00',
  cost: '$1.23',
}

function toRowMap(segments: StatusLineSegment[]): Record<string, StatusLineRow> {
  const out: Record<string, StatusLineRow> = {}
  for (const s of segments) out[s.key] = s.row
  return out
}
