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
  onBack,
}: {
  profile: string
  session: HistorySession
  /** When arriving from a search hit, the turn to land on and flash. */
  turnUuid?: string
  onBack: () => void
}) {
  const [page, setPage] = useState<HistoryPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [showThinking, setShowThinking] = useState(false)
  const [showSidechain, setShowSidechain] = useState(false)
  const [outlineOpen, setOutlineOpen] = useState(false)
  const [target, setTarget] = useState(-1)
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

  useEffect(() => {
    setPage(null)
    setError(null)
    void fetchPage(() =>
      turnUuid
        ? api.history.transcriptAround(profile, session.id, turnUuid, PAGE)
        : api.history.transcript(profile, session.id, 0, PAGE),
    )
  }, [profile, session.id, turnUuid, fetchPage])

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
      if (e.key === 'Escape') onBack()
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
    const clamped = Math.max(0, Math.min(prompts.length - 1, next))
    goTo(prompts[clamped].index)
  }

  const offset = page?.offset ?? 0
  const total = page?.total ?? 0
  const hasPrev = offset > 0
  const hasNext = offset + (page?.turns.length ?? 0) < total

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
          <Toggle
            active={showSidechain}
            disabled={sidechainCount === 0}
            onClick={() => setShowSidechain((v) => !v)}
            label={`subagents${sidechainCount ? ` (${sidechainCount})` : ''}`}
          />
          {prompts.length > 0 && (
            <>
              <span className="mx-1 h-4 w-px bg-border" />
              <button
                onClick={() => setOutlineOpen((v) => !v)}
                aria-expanded={outlineOpen}
                className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2 py-1 text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <List className="size-3.5" />
                {prompts.length} prompts
              </button>
              <IconStep onClick={() => step(-1)} label="Previous prompt" up />
              <IconStep onClick={() => step(1)} label="Next prompt" />
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
        {error && <div className="text-sm text-destructive">Could not open this transcript: {error}</div>}
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
                api.history.transcript(profile, session.id, Math.max(0, offset - PAGE), PAGE),
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
              expandAll={false}
            />
          ))}
        </div>

        {hasNext && (
          <LoadMore
            busy={busy}
            label="Load later turns"
            onClick={() =>
              void fetchPage(() => api.history.transcript(profile, session.id, offset + PAGE, PAGE))
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

function IconStep({ onClick, label, up }: { onClick: () => void; label: string; up?: boolean }) {
  const Icon = up ? ChevronUp : ChevronDown
  return (
    <button
      onClick={onClick}
      title={label}
      aria-label={label}
      className="inline-flex size-6 cursor-pointer items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
