import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { HistorySession, SearchHit } from '@/types'
import { humanTokens, money, tildePath, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { GitBranch, Play, Search } from 'lucide-react'
import { useToast } from '@/components/ui/Toast'
import { useGuarded } from '@/lib/useGuarded'
import { TranscriptReader } from '@/components/history/TranscriptReader'
import { SearchResults } from '@/components/history/SearchResults'

// One input, two scopes. Two separate boxes — a cwd filter beside a full-text
// search — would look near-identical and behave completely differently, so the
// scope is an explicit switch on a single query instead.
const SCOPES = [
  { id: 'list', label: 'Filter list' },
  { id: 'search', label: 'Search transcripts' },
] as const
type ScopeId = (typeof SCOPES)[number]['id']

/** Where the reader was opened from, so Escape can return there intact. */
type Origin = { kind: 'list' } | { kind: 'search'; hit: SearchHit }

type Reading = { session: HistorySession; origin: Origin }

export function HistoryTab({ profile }: { profile: string }) {
  const [sessions, setSessions] = useState<HistorySession[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [scope, setScope] = useState<ScopeId>('list')
  const [includeToolResults, setIncludeToolResults] = useState(false)
  const [reading, setReading] = useState<Reading | null>(null)

  // Plain effect, deliberately NOT useLive: useLive re-fetches on the watcher's
  // ccpm:changed event, and building the history index writes into the watched
  // <profileDir>/usage directory. UsageTab avoids the same loop the same way.
  //
  // The generation counter discards a response from a superseded fetch. Without
  // it, switching profile mid-fetch can paint the previous profile's sessions
  // under the new profile's header — the worst possible bug in an app whose
  // entire premise is that profiles are isolated.
  const gen = useRef(0)
  const load = useCallback(() => {
    const mine = ++gen.current
    setSessions(null)
    setError(null)
    api.history
      .sessions(profile)
      .then((s) => {
        if (gen.current === mine) setSessions(s)
      })
      .catch((e) => {
        if (gen.current === mine) setError(String(e))
      })
  }, [profile])

  useEffect(() => {
    setQuery('')
    setScope('list')
    setReading(null)
    load()
  }, [load])

  if (reading) {
    return (
      <TranscriptReader
        profile={profile}
        session={reading.session}
        turnUuid={reading.origin.kind === 'search' ? reading.origin.hit.turnUuid : undefined}
        onBack={() => setReading(null)}
      />
    )
  }

  if (error) {
    return <div className="px-6 py-5 text-sm text-destructive">Could not load history: {error}</div>
  }
  if (!sessions) {
    return <div className="px-6 py-5 text-sm text-muted-foreground">Loading history…</div>
  }

  const filtered =
    scope === 'list' && query.trim()
      ? sessions.filter((s) => matchesFilter(s, query.trim().toLowerCase()))
      : sessions

  return (
    <div className="px-6 py-5">
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={scope === 'list' ? 'Filter by project, branch or title…' : 'Search every transcript…'}
            aria-label={scope === 'list' ? 'Filter sessions' : 'Search transcripts'}
            className="w-full rounded-lg border border-border bg-card py-1.5 pl-8 pr-2.5 text-xs placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <div className="flex gap-0.5 rounded-lg border border-border p-0.5">
          {SCOPES.map((s) => (
            <button
              key={s.id}
              onClick={() => setScope(s.id)}
              className={cn(
                'cursor-pointer rounded-md px-2.5 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                scope === s.id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>

      {scope === 'search' ? (
        <SearchResults
          profile={profile}
          query={query}
          includeToolResults={includeToolResults}
          onToggleToolResults={setIncludeToolResults}
          onOpen={(hit) => {
            const session =
              sessions.find((s) => s.id === hit.sessionId) ?? sessionFromHit(hit)
            setReading({ session, origin: { kind: 'search', hit } })
          }}
        />
      ) : (
        <SessionList
          sessions={filtered}
          total={sessions.length}
          filtering={query.trim().length > 0}
          profile={profile}
          onOpen={(s) => setReading({ session: s, origin: { kind: 'list' } })}
        />
      )}
    </div>
  )
}

function matchesFilter(s: HistorySession, q: string): boolean {
  return (
    s.title.toLowerCase().includes(q) ||
    s.cwd.toLowerCase().includes(q) ||
    s.branch.toLowerCase().includes(q)
  )
}

/** A search hit in a session the list does not carry (its transcript exists but
 *  it never produced a usage record) still has enough to open a reader. */
function sessionFromHit(hit: SearchHit): HistorySession {
  return {
    id: hit.sessionId,
    title: hit.title ?? hit.sessionId,
    cwd: hit.cwd ?? '',
    branch: '',
    model: '',
    responses: 0,
    turns: 0,
    tokens: 0,
    cost: 0,
    firstTs: '',
    lastTs: '',
    openable: true,
  }
}

function SessionList({
  sessions,
  total,
  filtering,
  profile,
  onOpen,
}: {
  sessions: HistorySession[]
  total: number
  filtering: boolean
  profile: string
  onOpen: (s: HistorySession) => void
}) {
  if (total === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 text-center">
        <div className="text-sm font-medium">No sessions yet</div>
        <div className="mt-1 text-xs text-muted-foreground">
          History is read from this profile's Claude Code transcripts under{' '}
          <code className="rounded bg-muted px-1 py-px">projects/</code>. Run Claude Code with this
          profile and they will appear here.
        </div>
      </div>
    )
  }
  if (sessions.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 text-center">
        <div className="text-sm font-medium">No sessions match that filter</div>
        <div className="mt-1 text-xs text-muted-foreground">
          Filtering {total} sessions by project, branch and title. Switch to{' '}
          <span className="text-foreground">Search transcripts</span> to look inside the
          conversations instead.
        </div>
      </div>
    )
  }

  return (
    <>
      <h2 className="mb-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        Sessions · {sessions.length}
        {filtering && total !== sessions.length && ` of ${total}`}
      </h2>
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        {sessions.map((s, i) => (
          <SessionRow
            key={s.id}
            s={s}
            last={i === sessions.length - 1}
            profile={profile}
            onOpen={() => onOpen(s)}
          />
        ))}
      </div>
    </>
  )
}

// Three lines rather than eight columns: the row carries title, project, branch,
// model, responses, tokens, cost and last-active, which crushes into
// unreadability on one line at this window's width.
function SessionRow({
  s,
  last,
  profile,
  onOpen,
}: {
  s: HistorySession
  last: boolean
  profile: string
  onOpen: () => void
}) {
  const openable = s.openable
  const toast = useToast()
  const guard = useGuarded('Resume')

  const resume = guard(async () => {
    const r = await api.history.resume(profile, s.id)
    if (r.ok) {
      // Name the directory: `claude --resume` scopes by cwd, so seeing where it
      // landed is how a wrong target becomes visible rather than silent.
      toast({ kind: 'info', title: 'Resuming in Terminal', desc: r.output })
    } else {
      toast({ kind: 'error', title: 'Could not resume', desc: r.error || r.output })
    }
  })
  return (
    <div
      className={cn(
        'group flex items-start gap-3 px-4 py-2.5 transition-colors',
        !last && 'border-b border-border',
        openable ? 'cursor-pointer hover:bg-accent/40' : 'opacity-60',
      )}
      role={openable ? 'button' : undefined}
      tabIndex={openable ? 0 : undefined}
      onClick={openable ? onOpen : undefined}
      onKeyDown={(e) => {
        if (openable && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          onOpen()
        }
      }}
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="truncate text-sm font-medium">{s.title || s.id}</span>
          {!openable && (
            <span
              className="shrink-0 rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground"
              title="The transcript for this session has been removed, so it cannot be opened"
            >
              transcript pruned
            </span>
          )}
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[11px] text-muted-foreground">
          <span className="truncate">{s.cwd ? tildePath(s.cwd) : '—'}</span>
          {s.branch && (
            <span className="inline-flex shrink-0 items-center gap-1">
              <GitBranch className="size-3" />
              {s.branch}
            </span>
          )}
          {s.model && <span className="shrink-0 truncate">{shortModel(s.model)}</span>}
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[11px] tabular-nums text-muted-foreground">
          {s.responses > 0 && <span>{s.responses} responses</span>}
          {s.turns > 0 && <span>{s.turns} turns</span>}
          {s.tokens > 0 && <span>{humanTokens(s.tokens)} tokens</span>}
          {s.cost > 0 && <span className="text-primary">{money(s.cost)}</span>}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2 pt-0.5">
        <span className="text-[11px] text-muted-foreground">{timeAgo(s.lastTs)}</span>
        {openable && (
          <button
            title="Resume this session in Terminal"
            aria-label="Resume this session in Terminal"
            onClick={(e) => {
              e.stopPropagation()
              resume()
            }}
            className="inline-flex size-6 cursor-pointer items-center justify-center rounded-md border border-border text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-foreground focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100"
          >
            <Play className="size-3" />
          </button>
        )}
      </div>
    </div>
  )
}

/** "claude-opus-5" reads better than the full dated id in a dense row. */
function shortModel(model: string): string {
  return model.replace(/^claude-/, '').replace(/-\d{8}$/, '')
}
