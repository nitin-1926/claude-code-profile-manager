import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { CmdResult, StatusLineBuckets, StatusLineConfig, StatusLineRow, StatusLineSegment } from '@/types'
import { useToast } from '@/components/ui/Toast'
import { cn } from '@/lib/utils'
import { ChevronDown, ChevronUp, RotateCcw } from 'lucide-react'

/** Which layout the section is editing. */
type Scope = 'global' | 'profile'

const ROWS: { value: StatusLineRow; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'row1', label: 'Row 1' },
  { value: 'row2', label: 'Row 2' },
]

const GROUPS: { row: StatusLineRow; title: string; empty: string }[] = [
  { row: 'row1', title: 'Row 1', empty: 'Nothing on row 1 — it will not be printed.' },
  { row: 'row2', title: 'Row 2', empty: 'Nothing on row 2 — it will not be printed.' },
  { row: 'off', title: 'Hidden', empty: 'Every segment is showing.' },
]

/**
 * StatusLineSection edits which segments `ccpm statusline` renders, on which of
 * its two rows, and in what order.
 *
 * Two scopes share one control: the global default, and this profile's
 * override. Editing under "This profile" creates the override; Reset removes it
 * so the profile follows the global again.
 *
 * State is three ordered lists rather than a segment→row map. Order within a
 * row is what the status line actually prints, and a map cannot express it —
 * with one, saving would silently re-sort a row someone had arranged by hand.
 *
 * Deliberately a plain effect, not useLive. Saving writes ~/.ccpm/config.json,
 * which the Go watcher watches, so a useLive subscription would refetch its own
 * write on a 300 ms delay and fight the local state. HistoryTab and UsageTab
 * avoid the identical loop the same way.
 */
export function StatusLineSection({ profile }: { profile: string }) {
  const [cfg, setCfg] = useState<StatusLineConfig | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [scope, setScope] = useState<Scope>('global')
  const [draft, setDraft] = useState<StatusLineBuckets>(emptyBuckets)
  const [busy, setBusy] = useState(false)
  const toast = useToast()

  // Fetches carry a generation so a response for a profile the user has since
  // switched away from is discarded rather than painting one profile's layout
  // under another's name. SettingsTab also keys this component on profile, so a
  // switch remounts; this guards overlapping loads within one profile.
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
        setDraft(clone(next === 'profile' ? c.layout : c.global))
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
    setDraft(clone(next === 'profile' ? cfg.layout : cfg.global))
  }

  /** Move one segment to a different row, appending it at that row's end. */
  function place(key: string, to: StatusLineRow) {
    setDraft((d) => {
      const next: StatusLineBuckets = {
        row1: d.row1.filter((k) => k !== key),
        row2: d.row2.filter((k) => k !== key),
        off: d.off.filter((k) => k !== key),
      }
      next[to] = [...next[to], key]
      return next
    })
  }

  /** Move one segment up or down within the row it is already on. */
  function nudge(key: string, row: StatusLineRow, delta: -1 | 1) {
    setDraft((d) => {
      const list = [...d[row]]
      const i = list.indexOf(key)
      const j = i + delta
      if (i < 0 || j < 0 || j >= list.length) return d
      ;[list[i], list[j]] = [list[j], list[i]]
      return { ...d, [row]: list }
    })
  }

  function report(action: string, r: CmdResult) {
    if (r.ok) toast({ kind: 'success', title: action })
    else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
  }

  async function save() {
    const action = scope === 'profile' ? `Saved for ${profile}` : 'Saved for every profile'
    setBusy(true)
    try {
      // Every segment is sent exactly once — the CLI refuses an incomplete
      // layout rather than guessing, so a bug here surfaces as a visible error.
      const r = await api.statusline.set(scope === 'profile' ? profile : '', draft.row1, draft.row2, draft.off)
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

  const saved = scope === 'profile' ? cfg.layout : cfg.global
  const dirty = !sameBuckets(saved, draft)
  // A profile with no override has nothing of its own to reset.
  const canReset = scope === 'profile' ? cfg.hasOverride : true
  const byKey = new Map(cfg.segments.map((s) => [s.key, s]))

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
          ? 'Which segments every profile’s status line shows, on which row, and in what order.'
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

      <Preview draft={draft} />

      <div className="mt-3 overflow-hidden rounded-xl border border-border bg-card">
        {GROUPS.map((g) => (
          <div key={g.row}>
            <div className="border-b border-border bg-muted/40 px-4 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              {g.title}
            </div>
            {draft[g.row].length === 0 ? (
              <div className="border-b border-border px-4 py-2.5 text-[11px] text-muted-foreground">{g.empty}</div>
            ) : (
              draft[g.row].map((key, i) => {
                const seg = byKey.get(key)
                if (!seg) return null
                return (
                  <SegmentRow
                    key={key}
                    segment={seg}
                    row={g.row}
                    disabled={busy}
                    canMoveUp={g.row !== 'off' && i > 0}
                    canMoveDown={g.row !== 'off' && i < draft[g.row].length - 1}
                    onPlace={(to) => place(key, to)}
                    onNudge={(delta) => nudge(key, g.row, delta)}
                  />
                )
              })
            )}
          </div>
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
  row,
  disabled,
  canMoveUp,
  canMoveDown,
  onPlace,
  onNudge,
}: {
  segment: StatusLineSegment
  row: StatusLineRow
  disabled: boolean
  canMoveUp: boolean
  canMoveDown: boolean
  onPlace: (to: StatusLineRow) => void
  onNudge: (delta: -1 | 1) => void
}) {
  return (
    <div className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-foreground">{segment.label}</div>
        <div className="truncate text-[11px] text-muted-foreground">{segment.description}</div>
      </div>

      {/* Reordering only means something on a row that is printed, so the
          hidden group gets no arrows rather than disabled ones. */}
      {row !== 'off' && (
        <div className="flex shrink-0 items-center gap-0.5">
          <MoveButton label={`Move ${segment.label} earlier`} disabled={disabled || !canMoveUp} onClick={() => onNudge(-1)}>
            <ChevronUp className="size-3" />
          </MoveButton>
          <MoveButton label={`Move ${segment.label} later`} disabled={disabled || !canMoveDown} onClick={() => onNudge(1)}>
            <ChevronDown className="size-3" />
          </MoveButton>
        </div>
      )}

      <div
        role="radiogroup"
        aria-label={segment.label}
        className="flex shrink-0 items-center gap-0.5 rounded-md border border-border p-0.5"
      >
        {ROWS.map((r) => (
          <button
            key={r.value}
            role="radio"
            aria-checked={row === r.value}
            disabled={disabled}
            onClick={() => onPlace(r.value)}
            className={cn(
              'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors disabled:cursor-default disabled:opacity-50',
              row === r.value ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {r.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function MoveButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className="cursor-pointer rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-25 disabled:hover:bg-transparent"
    >
      {children}
    </button>
  )
}

/**
 * Preview renders the pending layout against sample values, so the effect of a
 * click is visible before saving. Deliberately mirrors `ccpm statusline
 * configure`'s printed preview; the sample covers every segment so nothing
 * reads as broken merely for want of data.
 */
function Preview({ draft }: { draft: StatusLineBuckets }) {
  const line = (keys: string[]) => keys.map((k) => SAMPLE[k] ?? k).join(' · ')
  const one = line(draft.row1)
  const two = line(draft.row2)

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

const emptyBuckets = (): StatusLineBuckets => ({ row1: [], row2: [], off: [] })

const clone = (b: StatusLineBuckets): StatusLineBuckets => ({
  row1: [...b.row1],
  row2: [...b.row2],
  off: [...b.off],
})

/** Order-sensitive comparison — a reordered row is a real change to save. */
function sameBuckets(a: StatusLineBuckets, b: StatusLineBuckets): boolean {
  const same = (x: string[], y: string[]) => x.length === y.length && x.every((v, i) => v === y[i])
  return same(a.row1, b.row1) && same(a.row2, b.row2) && same(a.off, b.off)
}
