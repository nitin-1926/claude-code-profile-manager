import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { HistoryPage, HistorySession, Turn } from '@/types'
import { cn } from '@/lib/utils'
import { tildePath, timeAgo } from '@/lib/format'
import { ArrowLeft, ChevronDown, ChevronUp, List } from 'lucide-react'
import { TurnView } from './Turn'

/** Turns fetched per page. The largest transcript on disk decodes to ~10k. */
const PAGE = 200

export function TranscriptReader({
  profile,
  session,
  turnUuid,
  relPath = '',
  onBack,
}: {
  profile: string
  session: HistorySession
  /** When arriving from a search hit, the turn to land on and flash. */
  turnUuid?: string
  /**
   * Which of the session's transcripts to read. Empty is the session's own; a
   * subagent path arrives from a search hit whose match was in work a subagent
   * did. Claude Code does not copy that work back into the parent, so this is
   * the only way to reach it.
   */
  relPath?: string
  onBack: () => void
}) {
  const [page, setPage] = useState<HistoryPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [showThinking, setShowThinking] = useState(false)
  const [showSidechain, setShowSidechain] = useState(false)
  const [outlineOpen, setOutlineOpen] = useState(false)
  const [target, setTarget] = useState(-1)
  // Where to land once a page fetched by the prompt stepper arrives. Transcript
  // returns TargetIndex -1, so without this a step across a page boundary
  // loaded 200 turns, highlighted nothing, and left the scroll container where
  // it was — the button read as broken and needed a second click.
  const [land, setLand] = useState<null | 'first' | 'last'>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const gen = useRef(0)
  const fetchPage = useCallback(
    async (fn: () => Promise<HistoryPage>) => {
      const mine = ++gen.current
      setBusy(true)
      try {
        const p = await fn()
        if (gen.current !== mine) return
        setPage(p)
        setTarget(p.targetIndex)
        setError(null)
      } catch (e) {
        if (gen.current === mine) setError(String(e))
      } finally {
        if (gen.current === mine) setBusy(false)
      }
    },
    [],
  )

  // Hoisted so the error state's Retry re-runs exactly the call that failed
  // rather than an approximation of it.
  const openInitial = useCallback(
    () =>
      fetchPage(() =>
        turnUuid
          ? api.history.transcriptAround(profile, session.id, relPath, turnUuid, PAGE)
          : api.history.transcript(profile, session.id, relPath, 0, PAGE),
      ),
    [profile, session.id, relPath, turnUuid, fetchPage],
  )

  useEffect(() => {
    setPage(null)
    setError(null)
    void openInitial()
  }, [openInitial])

  // A search hit may point into a hidden subagent turn. Landing on nothing is
  // worse than showing more than asked, so the jump force-enables the toggle.
  useEffect(() => {
    if (target < 0 || !page) return
    const t = page.turns.find((x) => x.index === target)
    if (t?.isSidechain) setShowSidechain(true)
  }, [target, page])

  useEffect(() => {
    if (target < 0) return
    const el = scrollRef.current?.querySelector(`[data-turn-index="${target}"]`)
    el?.scrollIntoView({ block: 'center' })
  }, [target, page, showSidechain])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Escape') return
      // Modal listens for Escape on window too, and ProfileView renders its
      // dialogs as siblings of the tab content — so without this, dismissing a
      // Rename/Delete dialog ALSO closed the reader, losing the page, the
      // scroll position and every expanded tool chip.
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
      onBack()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onBack])

  const visible = useMemo(
    () => (page?.turns ?? []).filter((t) => (t.isSidechain ? showSidechain : true)),
    [page, showSidechain],
  )
  const sidechainCount = useMemo(
    () => (page?.turns ?? []).filter((t) => t.isSidechain).length,
    [page],
  )
  const thinkingCount = useMemo(
    () => (page?.turns ?? []).reduce((n, t) => n + t.blocks.filter((b) => b.kind === 'thinking').length, 0),
    [page],
  )
  // Prompts are the only anchor a human actually remembers. Turn 6,214 is not.
  //
  // Scoped to the loaded page, not the session — the reader only ever holds one
  // page. The count and the stepper both say so rather than implying they cover
  // the whole transcript, because a stepper that silently stops at an invisible
  // boundary reads as broken.
  const prompts = useMemo(
    () => visible.filter((t) => t.role === 'user' && !t.isMeta && firstText(t).length > 0),
    [visible],
  )

  function goTo(index: number) {
    setTarget(index)
    const el = scrollRef.current?.querySelector(`[data-turn-index="${index}"]`)
    el?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }

  function step(dir: 1 | -1) {
    if (prompts.length === 0) return
    const cur = prompts.findIndex((p) => p.index === target)
    const next = cur < 0 ? (dir > 0 ? 0 : prompts.length - 1) : cur + dir
    // At a page edge, page rather than clamp. Math.min used to make the button
    // a silent no-op while more prompts sat one "Load later turns" away.
    if (next < 0 && hasPrev) {
      setLand('last')
      void fetchPage(() => api.history.transcript(profile, session.id, relPath, Math.max(0, offset - PAGE), PAGE))
      return
    }
    if (next >= prompts.length && hasNext) {
      setLand('first')
      void fetchPage(() => api.history.transcript(profile, session.id, relPath, offset + PAGE, PAGE))
      return
    }
    const clamped = Math.max(0, Math.min(prompts.length - 1, next))
    goTo(prompts[clamped].index)
  }

  const offset = page?.offset ?? 0
  const total = page?.total ?? 0
  const hasPrev = offset > 0
  const hasNext = offset + (page?.turns.length ?? 0) < total

  // Resolve a pending landing once the stepper's page has arrived.
  //
  // A page can hold no prompt at all: a session that is mostly tool calls has
  // runs of 200 turns with nothing a human typed — measured on a real 2,806-turn
  // session, the whole of turns 200-399. Landing on such a page and giving up
  // left the reader mid-page with nothing highlighted, which is the same broken
  // "the button did nothing" the landing intent exists to fix. So a prompt-less
  // page keeps paging in the same direction until a prompt turns up or the
  // transcript ends. Bounded by the page count, and the stepper is disabled
  // throughout because `busy` stays true across each hop.
  useEffect(() => {
    if (!land || busy || !page) return
    if (prompts.length > 0) {
      goTo(land === 'first' ? prompts[0].index : prompts[prompts.length - 1].index)
      setLand(null)
      return
    }
    if (land === 'first' && hasNext) {
      void fetchPage(() => api.history.transcript(profile, session.id, relPath, offset + PAGE, PAGE))
      return
    }
    if (land === 'last' && hasPrev) {
      void fetchPage(() => api.history.transcript(profile, session.id, relPath, Math.max(0, offset - PAGE), PAGE))
      return
    }
    // No prompt anywhere further in that direction. Stop here rather than
    // leaving the intent armed to fire against some later page.
    setLand(null)
    // goTo only reads scrollRef, which never changes identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [land, busy, page, prompts, hasNext, hasPrev, offset, fetchPage, profile, session.id, relPath])

  return (
    <div className="flex h-full flex-col">
      <header className="shrink-0 border-b border-border px-6 py-3">
        <div className="flex items-center gap-2">
          <button
            onClick={onBack}
            className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2 py-1 text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ArrowLeft className="size-3.5" />
            Back
          </button>
          <h2 className="truncate text-sm font-medium">{session.title || session.id}</h2>
          <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
            {timeAgo(session.lastTs)}
          </span>
        </div>
        <p className="mt-1 truncate text-[11px] text-muted-foreground">
          {session.cwd ? tildePath(session.cwd) : '—'}
          {session.branch && ` · ${session.branch}`}
          {relPath !== '' && ' · ' + relPath.split('/').pop()}
          {total > 0 && ` · ${total} turns`}
          {' · point-in-time read'}
        </p>

        <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
          <Toggle
            active={showThinking}
            disabled={thinkingCount === 0}
            onClick={() => setShowThinking((v) => !v)}
            label={`thinking${thinkingCount ? ` (${thinkingCount})` : ''}`}
          />
          {relPath !== '' && (
            <span className="rounded-md border border-primary/40 bg-primary/10 px-2 py-1 text-xs">
              subagent transcript
            </span>
          )}
          {prompts.length > 0 && (
            <>
              <span className="mx-1 h-4 w-px bg-border" />
              <button
                onClick={() => setOutlineOpen((v) => !v)}
                aria-expanded={outlineOpen}
                className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2 py-1 text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <List className="size-3.5" />
                {prompts.length} prompts{total > visible.length ? ' on this page' : ''}
              </button>
              <IconStep onClick={() => step(-1)} label="Previous prompt" disabled={busy} up />
              <IconStep onClick={() => step(1)} label="Next prompt" disabled={busy} />
            </>
          )}
          {page && page.unknownBlocks > 0 && (
            <span
              className="ml-auto rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground"
              title="Claude Code wrote block types this version of ccpm does not render"
            >
              {page.unknownBlocks} unrecognised blocks
            </span>
          )}
        </div>

        {outlineOpen && prompts.length > 0 && (
          <ol className="mt-2 max-h-40 overflow-y-auto rounded-lg border border-border bg-card">
            {prompts.map((p, i) => (
              <li key={p.index}>
                <button
                  onClick={() => {
                    goTo(p.index)
                    setOutlineOpen(false)
                  }}
                  className={cn(
                    'flex w-full cursor-pointer gap-2 px-3 py-1.5 text-left text-xs transition-colors hover:bg-accent',
                    i < prompts.length - 1 && 'border-b border-border',
                  )}
                >
                  <span className="shrink-0 tabular-nums text-muted-foreground">{i + 1}</span>
                  <span className="truncate">{firstText(p).slice(0, 140)}</span>
                </button>
              </li>
            ))}
          </ol>
        )}
      </header>

      <div ref={scrollRef} className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
        {error && (
          <div className="flex items-center gap-2 text-sm text-destructive">
            <span>Could not open this transcript: {error}</span>
            <button
              onClick={() => void openInitial()}
              className="cursor-pointer rounded-md border border-border px-2 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              Retry
            </button>
          </div>
        )}
        {!page && !error && <div className="text-sm text-muted-foreground">Opening transcript…</div>}
        {page && !error && page.total === 0 && (
          <div className="rounded-xl border border-border bg-card p-8 text-center">
            <div className="text-sm font-medium">This transcript is no longer readable</div>
            <div className="mt-1 text-xs text-muted-foreground">
              The file may have been removed since the session list was built.
            </div>
          </div>
        )}

        {hasPrev && (
          <LoadMore
            busy={busy}
            label="Load earlier turns"
            onClick={() =>
              void fetchPage(() =>
                api.history.transcript(profile, session.id, relPath, Math.max(0, offset - PAGE), PAGE),
              )
            }
          />
        )}

        <div className="space-y-2">
          {visible.map((t) => (
            <TurnView
              key={t.index}
              turn={t}
              profile={profile}
              sessionId={session.id}
              showThinking={showThinking}
              isTarget={t.index === target}
              relPath={relPath}
            />
          ))}
        </div>

        {hasNext && (
          <LoadMore
            busy={busy}
            label="Load later turns"
            onClick={() =>
              void fetchPage(() => api.history.transcript(profile, session.id, relPath, offset + PAGE, PAGE))
            }
          />
        )}

        {page && page.total > 0 && (
          <p className="mt-4 text-center text-[11px] text-muted-foreground">
            turns {offset + 1}–{offset + page.turns.length} of {total}
          </p>
        )}
      </div>
    </div>
  )
}

function firstText(t: Turn): string {
  return t.blocks.find((b) => b.kind === 'text')?.text?.trim() ?? ''
}

function Toggle({
  active,
  disabled,
  onClick,
  label,
}: {
  active: boolean
  disabled: boolean
  onClick: () => void
  label: string
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      aria-pressed={active}
      className={cn(
        'rounded-md border px-2 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        disabled
          ? 'cursor-default border-border text-muted-foreground/40'
          : active
            ? 'cursor-pointer border-primary/40 bg-primary/10 text-foreground'
            : 'cursor-pointer border-border text-muted-foreground hover:text-foreground',
      )}
    >
      {label}
    </button>
  )
}

// Disabled while a page is in flight. A step that crosses a page boundary starts
// a fetch; a step the other way before it lands moves `target` without bumping
// the fetch generation, so the older response still wins and silently undoes the
// newer navigation. LoadMore is gated the same way for the same reason.
function IconStep({
  onClick,
  label,
  disabled,
  up,
}: {
  onClick: () => void
  label: string
  disabled?: boolean
  up?: boolean
}) {
  const Icon = up ? ChevronUp : ChevronDown
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className={cn(
        'inline-flex size-6 items-center justify-center rounded-md border border-border text-muted-foreground transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        disabled ? 'cursor-default opacity-50' : 'cursor-pointer hover:bg-accent hover:text-foreground',
      )}
    >
      <Icon className="size-3.5" />
    </button>
  )
}

function LoadMore({ busy, label, onClick }: { busy: boolean; label: string; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      disabled={busy}
      className={cn(
        'my-2 w-full rounded-lg border border-border py-1.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        busy ? 'cursor-default text-muted-foreground/50' : 'cursor-pointer text-muted-foreground hover:bg-accent hover:text-foreground',
      )}
    >
      {busy ? 'Loading…' : label}
    </button>
  )
}
