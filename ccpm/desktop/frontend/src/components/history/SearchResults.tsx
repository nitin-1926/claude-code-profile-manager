import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import type { SearchHit, SearchResult } from '@/types'
import { cn } from '@/lib/utils'
import { timeAgo } from '@/lib/format'
import { Wrench } from 'lucide-react'

const DEBOUNCE_MS = 250

// Module-level, NOT per-component. HistoryService keeps its cancel tombstones
// for the app's lifetime, but this component unmounts whenever the reader opens.
// A per-instance counter restarts at 0 on remount and reissues a token an
// earlier cleanup already tombstoned, so the very next search returns cancelled
// and the UI renders a false "No matches".
let tokenSeq = 0
const nextToken = () => `s${++tokenSeq}`

/** Groups a flat hit list by session, preserving the backend's newest-first order. */
function groupBySession(hits: SearchHit[]): { id: string; title: string; mtime: number; hits: SearchHit[] }[] {
  const order: string[] = []
  const by = new Map<string, SearchHit[]>()
  for (const h of hits) {
    if (!by.has(h.sessionId)) {
      by.set(h.sessionId, [])
      order.push(h.sessionId)
    }
    by.get(h.sessionId)!.push(h)
  }
  return order.map((id) => {
    const group = by.get(id)!
    return { id, title: group[0].title || id, mtime: group[0].mtime, hits: group }
  })
}

export function SearchResults({
  profile,
  query,
  includeToolResults,
  onToggleToolResults,
  onOpen,
}: {
  profile: string
  query: string
  includeToolResults: boolean
  onToggleToolResults: (v: boolean) => void
  onOpen: (hit: SearchHit) => void
}) {
  const [result, setResult] = useState<SearchResult | null>(null)
  const [searching, setSearching] = useState(false)
  const [error, setError] = useState<string | null>(null)
  /** Set when a default-scope search found nothing but tool output has matches. */
  const [toolOnlyCount, setToolOnlyCount] = useState(0)

  // The token is the only real cancellation: dropping the JS promise does not
  // stop the Go scan, so a superseded search would still resolve and clobber a
  // newer result set. `live` holds the token this component still cares about.
  const live = useRef('')

  useEffect(() => {
    const q = query.trim()
    setToolOnlyCount(0)
    if (!q) {
      setResult(null)
      setSearching(false)
      return
    }
    const tok = nextToken()
    const wideTok = `${tok}w`
    live.current = tok
    setSearching(true)
    const timer = setTimeout(() => {
      api.history
        .search(profile, q, tok, includeToolResults)
        .then((r) => {
          if (live.current !== tok) return
          setResult(r)
          setError(null)
          setSearching(false)
          // A miss in conversation text is usually a hit in tool output. Offer
          // it rather than showing a bare "no matches" to guess at. Fired after
          // the spinner clears rather than awaited inside it — otherwise the
          // header sits on "Searching..." through a second full profile scan
          // while results are already on screen.
          if (!includeToolResults && r.hits.length === 0 && !r.cancelled) {
            api.history
              .search(profile, q, wideTok, true)
              .then((wide) => {
                if (live.current === tok) setToolOnlyCount(wide.hits.length)
              })
              .catch(() => {})
          }
        })
        .catch((e) => {
          if (live.current !== tok) return
          setError(String(e))
          setSearching(false)
        })
    }, DEBOUNCE_MS)

    return () => {
      clearTimeout(timer)
      live.current = ''
      // Cancel both scans this effect could have started. Landing before either
      // registers is fine — the service keeps a tombstone for that race.
      void api.history.cancelSearch(tok)
      void api.history.cancelSearch(wideTok)
    }
  }, [profile, query, includeToolResults])

  if (!query.trim()) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 text-center">
        <div className="text-sm font-medium">Search this profile's transcripts</div>
        <div className="mt-1 text-xs text-muted-foreground">
          Matches conversation text and the commands and file paths Claude ran, in this
          session and in the work its subagents did. Tool output is excluded by default — it
          is where pasted secrets tend to live.
        </div>
      </div>
    )
  }

  return (
    <>
      <div className="mb-2.5 flex flex-wrap items-center gap-2">
        <h2
          aria-live="polite"
          className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground"
        >
          {searching
            ? 'Searching…'
            : result
              ? `${result.matches}${result.truncated ? '+' : ''} matches in ${result.sessions} sessions`
              : ' '}
        </h2>
        <label className="ml-auto inline-flex cursor-pointer items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={includeToolResults}
            onChange={(e) => onToggleToolResults(e.target.checked)}
            className="size-3 cursor-pointer accent-primary"
          />
          include tool output
        </label>
      </div>

      {error && <div className="text-sm text-destructive">Search failed: {error}</div>}

      {result && result.unreadable > 0 && (
        <div className="mb-2.5 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
          {result.unreadable} transcript{result.unreadable === 1 ? '' : 's'} could not be read, so
          these results are partial.
        </div>
      )}

      {result && !searching && result.hits.length === 0 && (
        <div className="rounded-xl border border-border bg-card p-8 text-center">
          <div className="text-sm font-medium">No matches in conversation text</div>
          {toolOnlyCount > 0 ? (
            <button
              onClick={() => onToggleToolResults(true)}
              className="mt-2 cursor-pointer rounded-md border border-border px-2.5 py-1 text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {toolOnlyCount} match{toolOnlyCount === 1 ? '' : 'es'} in tool output — include it
            </button>
          ) : (
            <div className="mt-1 text-xs text-muted-foreground">
              Nothing in this profile's transcripts matches that.
            </div>
          )}
        </div>
      )}

      {result && result.hits.length > 0 && (
        <div className="space-y-3">
          {groupBySession(result.hits).map((g) => (
            <div key={g.id} className="overflow-hidden rounded-xl border border-border bg-card">
              <div className="flex items-center gap-2 border-b border-border px-4 py-2">
                <span className="truncate text-sm font-medium">{g.title}</span>
                <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">
                  {timeAgo(new Date(g.mtime * 1000).toISOString())}
                </span>
              </div>
              {g.hits.map((h, i) => (
                <button
                  key={i}
                  onClick={() => onOpen(h)}
                  className={cn(
                    'flex w-full cursor-pointer flex-col gap-1 px-4 py-2 text-left transition-colors hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                    i < g.hits.length - 1 && 'border-b border-border',
                  )}
                >
                  <div className="flex items-center gap-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                    <span>{h.role === 'user' ? 'You' : 'Claude'}</span>
                    {h.subagent && (
                      <span
                        className="rounded bg-primary/15 px-1 py-px normal-case text-primary"
                        title="Found in work a subagent did for this session"
                      >
                        subagent
                      </span>
                    )}
                    {h.source !== 'text' && (
                      <span className="inline-flex items-center gap-1">
                        <Wrench className="size-3" />
                        {h.toolName || (h.source === 'tool_result' ? 'tool output' : 'tool call')}
                      </span>
                    )}
                    {h.more > 0 && <span className="ml-auto normal-case">+{h.more} more here</span>}
                  </div>
                  <p className="line-clamp-3 whitespace-pre-wrap break-words text-xs leading-relaxed">
                    <span className="text-muted-foreground">{h.before}</span>
                    <mark className="rounded bg-primary/25 px-0.5 text-foreground">{h.match}</mark>
                    <span className="text-muted-foreground">{h.after}</span>
                  </p>
                </button>
              ))}
            </div>
          ))}
        </div>
      )}

      {result?.truncated && result.hits.length > 0 && (
        <p className="mt-3 text-center text-[11px] text-muted-foreground">
          Showing the newest matches
          {result.droppedSessions > 0 && ` — ${result.droppedSessions} older sessions not scanned`}.
          Narrow the query to see more.
        </p>
      )}
    </>
  )
}
