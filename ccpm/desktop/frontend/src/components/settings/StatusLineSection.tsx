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
 * so the profile follows the global again. Each scope keeps its OWN draft, so
 * switching between them never discards work in progress.
 *
 * State is ordered lists rather than a segment→row map. Order within a row is
 * what the status line prints, and a map cannot express it — with one, saving
 * would silently re-sort a row someone had arranged by hand.
 *
 * Deliberately a plain effect, not useLive. Saving writes ~/.ccpm/config.json,
 * which the Go watcher watches, so a useLive subscription would refetch its own
 * write on a 300 ms delay and fight the local state. HistoryTab and UsageTab
 * avoid the identical loop the same way. The cost is that an external change
 * needs the Refresh button below.
 */
export function StatusLineSection({ profile }: { profile: string }) {
  const [cfg, setCfg] = useState<StatusLineConfig | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [scope, setScope] = useState<Scope>('global')
  // One draft per scope. Switching scope used to overwrite a single draft,
  // silently throwing away every unsaved edit with no undo and no warning.
  const [drafts, setDrafts] = useState<Record<Scope, StatusLineBuckets>>({
    global: emptyBuckets(),
    profile: emptyBuckets(),
  })
  const [busy, setBusy] = useState(false)
  // Set to "<segmentKey>:<control>" when an interaction should move focus after
  // the re-render — see the effect below.
  const [refocus, setRefocus] = useState<string | null>(null)
  const toast = useToast()

  // Fetches carry a generation so a response for a profile the user has since
  // switched away from is discarded. ProfileView also remounts this on a
  // profile switch; the ref covers overlapping loads within one profile.
  const gen = useRef(0)

  // `written` names the scope a save or reset just wrote. That scope always
  // takes the fresh value from disk. Every OTHER scope takes it too if its draft
  // was clean — equal to what was on disk before — and keeps its draft only when
  // it holds unsaved edits.
  //
  // Both halves are needed, and the first attempt had only one. Replacing every
  // draft discarded unsaved work in the scope not being saved. Replacing only
  // the written scope broke the default case instead: a profile with no
  // override resolves to the global, so saving the global changes the profile's
  // layout too, and its untouched draft was left on the old value — showing
  // "unsaved changes" nobody made, locking Refresh, and letting a Save there pin
  // the outdated layout as an override.
  const load = useCallback(
    async (preferred?: Scope, written?: Scope) => {
      const mine = ++gen.current
      // Editing is locked for the duration. Without this, edits made while
      // Refresh is in flight are silently replaced by setDrafts when it lands,
      // and a scope switch is undone by setScope restoring the scope captured
      // when Refresh started.
      setBusy(true)
      try {
        const c = await api.statusline.get(profile)
        if (mine !== gen.current) return
        setCfg(c)
        setLoadError(null)
        const fresh: Record<Scope, StatusLineBuckets> = { global: clone(c.global), profile: clone(c.layout) }
        setDrafts((d) => {
          if (!written || !cfg) return fresh
          const before: Record<Scope, StatusLineBuckets> = { global: cfg.global, profile: cfg.layout }
          const next = { ...d }
          for (const s of ['global', 'profile'] as const) {
            if (s === written || sameBuckets(d[s], before[s])) next[s] = fresh[s]
          }
          return next
        })
        // Open on whichever scope is actually in force, so the controls match
        // what this profile's sessions are showing rather than a default guess.
        setScope(preferred ?? (c.hasOverride ? 'profile' : 'global'))
      } catch (e) {
        if (mine !== gen.current) return
        // Only a FAILED FIRST load may replace the section. A refresh that
        // fails must not wipe a screen full of unsaved edits, so it reports and
        // leaves the content alone.
        if (cfg === null) setLoadError(String(e))
        else toast({ kind: 'error', title: 'Could not refresh the status line settings', desc: String(e) })
      } finally {
        if (mine === gen.current) setBusy(false)
      }
    },
    [profile, cfg, toast],
  )

  // Mount-only: `load` changes identity as cfg/toast change, and re-running it
  // on every one of those would refetch mid-edit.
  const bootstrap = useRef(false)
  useEffect(() => {
    if (bootstrap.current) return
    bootstrap.current = true
    void load()
  }, [load])

  // Restore focus after a move. Reordering changes the DOM around the button
  // that was clicked, and a button that becomes disabled at a row boundary is
  // blurred by the browser — either way keyboard users lose their place and
  // land back at the top of the document.
  useEffect(() => {
    if (!refocus) return
    const el = document.querySelector<HTMLElement>(`[data-slk="${CSS.escape(refocus)}"]`)
    el?.focus()
    setRefocus(null)
  }, [refocus])

  const draft = drafts[scope]
  const setDraft = (next: StatusLineBuckets) => setDrafts((d) => ({ ...d, [scope]: next }))

  /** Move one segment to a different row, appending it at that row's end. */
  function place(key: string, to: StatusLineRow) {
    // Re-selecting the row a segment is already on must do nothing. Without
    // this the filter-then-append below moves it to the end of its own row, so
    // clicking a checked radio — or pressing Home while already on Off —
    // silently reorders the printed status line and marks the form dirty.
    if (draft[to].includes(key)) return
    const next: StatusLineBuckets = {
      row1: draft.row1.filter((k) => k !== key),
      row2: draft.row2.filter((k) => k !== key),
      off: draft.off.filter((k) => k !== key),
    }
    next[to] = [...next[to], key]
    setDraft(next)
    setRefocus(`${key}:${to}`)
  }

  /** Move one segment up or down within the row it is already on. */
  function nudge(key: string, row: StatusLineRow, delta: -1 | 1) {
    const list = [...draft[row]]
    const i = list.indexOf(key)
    const j = i + delta
    if (i < 0 || j < 0 || j >= list.length) return
    ;[list[i], list[j]] = [list[j], list[i]]
    setDraft({ ...draft, [row]: list })
    // If this button is about to become disabled (the segment reached an end),
    // put focus on the opposite arrow rather than letting it fall to <body>.
    const atEnd = delta === -1 ? j === 0 : j === list.length - 1
    setRefocus(`${key}:${atEnd ? (delta === -1 ? 'down' : 'up') : delta === -1 ? 'up' : 'down'}`)
  }

  async function save() {
    if (!cfg) return
    const target = scope === 'profile' ? `the layout for ${profile}` : 'the layout for every profile'
    setBusy(true)
    try {
      // Every segment is sent exactly once — the CLI refuses an incomplete
      // layout rather than guessing, so a bug here surfaces as a visible error.
      const r = await api.statusline.set(scope === 'profile' ? profile : '', draft.row1, draft.row2, draft.off)
      report(toast, `Saved ${target}`, `Could not save ${target}`, r)
      if (r.ok) await load(scope, scope)
    } catch (e) {
      // The bridge call itself can reject (the app closing mid-call, a binding
      // fault). Without this the click fails silently and leaves an unhandled
      // rejection, so the user sees the spinner stop and nothing else.
      toast({ kind: 'error', title: `Could not save ${target}`, desc: String(e) })
    } finally {
      setBusy(false)
    }
  }

  async function reset() {
    const target =
      scope === 'profile' ? `${profile} to follow the global status line` : 'the status line to its default layout'
    setBusy(true)
    try {
      const r = await api.statusline.reset(scope === 'profile' ? profile : '')
      report(toast, `Set ${target}`, `Could not set ${target}`, r)
      if (r.ok) await load('global', scope)
    } catch (e) {
      toast({ kind: 'error', title: `Could not set ${target}`, desc: String(e) })
    } finally {
      setBusy(false)
    }
  }

  if (loadError)
    return (
      <div className="mb-6 rounded-xl border border-border bg-card px-4 py-3">
        <div className="text-xs text-destructive">Could not load the status line config: {loadError}</div>
        <button
          onClick={() => {
            setLoadError(null)
            void load()
          }}
          className="mt-2 cursor-pointer rounded-md border border-border px-2.5 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Retry
        </button>
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
  // Refresh re-reads disk into both drafts, so it is gated on unsaved work in
  // EITHER scope, not just the one on screen.
  const anyDirty = !sameBuckets(cfg.global, drafts.global) || !sameBuckets(cfg.layout, drafts.profile)
  // A profile with no override resolves to the global, so its draft matches and
  // `dirty` is false — yet the copy invites you to pin it. Saving an unchanged
  // layout is exactly how you turn "follows the global" into an override.
  const pinning = scope === 'profile' && !cfg.hasOverride
  const canSave = dirty || pinning
  const canReset = scope === 'profile' ? cfg.hasOverride : true
  const byKey = new Map(cfg.segments.map((s) => [s.key, s]))

  // One flat list, headers included, so React reorders rather than unmounting
  // when a segment changes row — which is what preserves the focused DOM node.
  const items: { id: string; header?: (typeof GROUPS)[number]; key?: string; row: StatusLineRow; i: number }[] = []
  for (const g of GROUPS) {
    items.push({ id: `h:${g.row}`, header: g, row: g.row, i: -1 })
    draft[g.row].forEach((key, i) => items.push({ id: `s:${key}`, key, row: g.row, i }))
  }

  return (
    <div className="mb-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Status line</h2>
        <div className="flex items-center gap-2">
          <div role="group" aria-label="Which layout to edit" className="flex items-center gap-1 rounded-md border border-border p-0.5">
            {(['global', 'profile'] as Scope[]).map((s) => (
              <button
                key={s}
                // aria-pressed, not colour alone: without it a screen reader
                // announces both options identically, and Save writes to
                // whichever one is selected.
                aria-pressed={scope === s}
                disabled={busy}
                onClick={() => setScope(s)}
                className={cn(
                  'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors disabled:cursor-default disabled:opacity-50',
                  scope === s ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {s === 'global' ? 'Global default' : 'This profile'}
                {s === 'profile' && cfg.hasOverride && (
                  <>
                    <span className="sr-only"> (has an override)</span>
                    <span aria-hidden="true"> •</span>
                  </>
                )}
              </button>
            ))}
          </div>
          <button
            onClick={() => void load(scope)}
            disabled={busy || anyDirty}
            title={
              anyDirty
                ? 'Save or reset your changes first — refreshing would discard them'
                : 'Re-read the status line settings from disk'
            }
            className="cursor-pointer rounded-md border border-border p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
          >
            <RotateCcw className="size-3" />
            <span className="sr-only">Refresh status line settings</span>
          </button>
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
              <span className="font-mono">ccpm config set statusline true</span>, then Refresh.
            </span>
          </>
        )}
      </p>

      <Preview draft={draft} byKey={byKey} />

      <div className="mt-3 overflow-hidden rounded-xl border border-border bg-card">
        {items.map((it) =>
          it.header ? (
            <div
              key={it.id}
              className="border-b border-border bg-muted/40 px-4 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground"
            >
              {it.header.title}
              {draft[it.row].length === 0 && (
                <span className="ml-2 font-normal normal-case tracking-normal">{it.header.empty}</span>
              )}
            </div>
          ) : (
            <SegmentRow
              key={it.id}
              segment={byKey.get(it.key!)!}
              row={it.row}
              disabled={busy}
              canMoveUp={it.row !== 'off' && it.i > 0}
              canMoveDown={it.row !== 'off' && it.i < draft[it.row].length - 1}
              onPlace={(to) => place(it.key!, to)}
              onNudge={(d) => nudge(it.key!, it.row, d)}
            />
          ),
        )}
      </div>

      <div className="mt-3 flex items-center gap-2">
        <button
          disabled={busy || !canSave}
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
        {dirty ? (
          <span className="text-[11px] text-muted-foreground">Unsaved changes</span>
        ) : pinning ? (
          <span className="text-[11px] text-muted-foreground">Saves today’s layout as {profile}’s own</span>
        ) : null}
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
  // The WAI-ARIA radio-group pattern, which declaring role="radiogroup"
  // promises: arrow keys move the selection, Home/End jump to the ends, and a
  // roving tabindex keeps the whole group to a single tab stop. Nine segments
  // would otherwise be 27 tab stops in one section.
  function onKeyDown(e: React.KeyboardEvent) {
    const i = ROWS.findIndex((r) => r.value === row)
    let next = -1
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (i + 1) % ROWS.length
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (i - 1 + ROWS.length) % ROWS.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = ROWS.length - 1
    if (next < 0) return
    e.preventDefault()
    onPlace(ROWS[next].value)
  }

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
          <MoveButton
            slk={`${segment.key}:up`}
            label={`Move ${segment.label} earlier`}
            disabled={disabled || !canMoveUp}
            onClick={() => onNudge(-1)}
          >
            <ChevronUp className="size-3" />
          </MoveButton>
          <MoveButton
            slk={`${segment.key}:down`}
            label={`Move ${segment.label} later`}
            disabled={disabled || !canMoveDown}
            onClick={() => onNudge(1)}
          >
            <ChevronDown className="size-3" />
          </MoveButton>
        </div>
      )}

      <div
        role="radiogroup"
        aria-label={segment.label}
        onKeyDown={onKeyDown}
        className="flex shrink-0 items-center gap-0.5 rounded-md border border-border p-0.5"
      >
        {ROWS.map((r) => {
          const checked = row === r.value
          return (
            <button
              key={r.value}
              data-slk={`${segment.key}:${r.value}`}
              role="radio"
              aria-checked={checked}
              // Roving tabindex: only the checked option is a tab stop.
              tabIndex={checked ? 0 : -1}
              disabled={disabled}
              onClick={() => onPlace(r.value)}
              className={cn(
                'cursor-pointer rounded px-2 py-0.5 text-[11px] transition-colors disabled:cursor-default disabled:opacity-50',
                checked ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {r.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function MoveButton({
  slk,
  label,
  disabled,
  onClick,
  children,
}: {
  slk: string
  label: string
  disabled: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      data-slk={slk}
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
 * click is visible before saving.
 *
 * A segment with no sample falls back to its catalog LABEL rather than its raw
 * key, so a segment added to the Go catalog before this map degrades to
 * "reasoning effort" rather than "effort" — readable either way, and
 * TestPreviewSampleCoversEveryCatalogSegment fails the build if one is missing.
 */
function Preview({ draft, byKey }: { draft: StatusLineBuckets; byKey: Map<string, StatusLineSegment> }) {
  const line = (keys: string[]) => keys.map((k) => SAMPLE[k] ?? byKey.get(k)?.label ?? k).join(' · ')
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

/**
 * Sample values, mirroring statusLinePreviewInput in
 * cmd/statusline_configure.go.
 *
 * Both reset clocks are same-day, which is the one form that cannot go stale:
 * a dated example has to name a weekday, and the previous literal said
 * "Mon 8 Sep" for a date that is a Tuesday and in the past — a rendering the
 * real status line would never produce, since it only prints a clock for a
 * reset still in the future.
 */
const SAMPLE: Record<string, string> = {
  profile: '⬢ work',
  workspace: 'ccpm/cmd',
  branch: '⎇ main',
  model: 'Opus 5',
  context: 'ctx 34%',
  effort: 'effort high',
  five_hour: '5h 42% ↺16:15',
  seven_day: '7d 78% ↺09:00',
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

/**
 * report renders a CmdResult as a toast. Success and failure take separate
 * phrasings because a single past-tense action string produced titles like
 * "Saved for work failed".
 */
function report(
  toast: ReturnType<typeof useToast>,
  success: string,
  failure: string,
  r: CmdResult,
) {
  if (r.ok) toast({ kind: 'success', title: success })
  else toast({ kind: 'error', title: failure, desc: (r.error || r.output).split('\n')[0] })
}
