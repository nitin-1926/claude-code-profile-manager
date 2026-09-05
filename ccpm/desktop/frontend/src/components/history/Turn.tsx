import { useEffect, useState } from 'react'
import type { HistoryToolBody, Turn, TurnBlock } from '@/types'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'
import { AlertTriangle, ChevronRight, Image, Loader2, Wrench } from 'lucide-react'

/** Bytes of an expanded tool body we are willing to put in the DOM at once. */
const CHIP_RENDER_CAP = 100_000

export function TurnView({
  turn,
  profile,
  sessionId,
  showThinking,
  isTarget,
  relPath,
}: {
  turn: Turn
  profile: string
  sessionId: string
  showThinking: boolean
  /** The turn a search hit pointed at: flashed, and its tool chips force-open. */
  isTarget: boolean
  /** Which of the session's transcripts this turn came from; "" is its own. */
  relPath: string
}) {
  // Carry each block's ORIGINAL index through the filter. Keying on the
  // filtered position instead means toggling `thinking` shifts every key, and
  // React updates an existing ToolChip's props in place rather than remounting
  // it — so the payload it already fetched stays on screen under a different
  // tool's name, and the re-expand guard means it never refetches.
  const blocks = turn.blocks
    .map((b, i) => ({ b, i }))
    .filter(({ b }) => (b.kind === 'thinking' ? showThinking : true))
  if (blocks.length === 0) return null

  const isUser = turn.role === 'user'
  return (
    <div
      data-turn-index={turn.index}
      className={cn(
        'scroll-mt-4 rounded-lg px-3 py-2 transition-colors',
        isTarget && 'bg-primary/10 ring-1 ring-primary/40',
      )}
    >
      <div className="mb-1 flex items-center gap-2">
        <span
          className={cn(
            'text-[10px] font-semibold uppercase tracking-wider',
            isUser ? 'text-primary' : 'text-muted-foreground',
          )}
        >
          {isUser ? 'You' : 'Claude'}
        </span>
        {turn.isSidechain && (
          <span className="rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
            subagent
          </span>
        )}
        {turn.isMeta && (
          <span className="rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground">
            system
          </span>
        )}
      </div>
      <div className="space-y-1.5">
        {blocks.map(({ b, i }) => (
          <BlockView
            key={i}
            block={b}
            blockIndex={i}
            profile={profile}
            sessionId={sessionId}
            relPath={relPath}
            turnUuid={turn.uuid ?? ''}
            forceOpen={isTarget}
          />
        ))}
      </div>
    </div>
  )
}

function BlockView({
  block,
  blockIndex,
  profile,
  sessionId,
  relPath,
  turnUuid,
  forceOpen,
}: {
  block: TurnBlock
  blockIndex: number
  profile: string
  sessionId: string
  relPath: string
  turnUuid: string
  forceOpen: boolean
}) {
  switch (block.kind) {
    case 'text':
      return (
        <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">
          {block.text}
          {block.truncated && <TruncatedNote full={block.fullBytes} />}
        </p>
      )
    case 'thinking':
      return (
        <p className="whitespace-pre-wrap break-words border-l-2 border-border pl-3 text-xs italic leading-relaxed text-muted-foreground">
          {block.text || '(empty)'}
        </p>
      )
    case 'image':
      return (
        <span className="inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
          <Image className="size-3.5" />
          image
        </span>
      )
    case 'unknown':
      // Claude Code owns this format and does not version it. Showing the
      // unrecognised type is how a format change becomes visible instead of
      // silently eating content.
      return (
        <span className="inline-flex items-center gap-1.5 rounded-md border border-dashed border-border px-2 py-1 text-xs text-muted-foreground">
          <AlertTriangle className="size-3.5" />
          unrecognised block ({block.rawType || 'unknown'})
        </span>
      )
    default:
      return (
        <ToolChip
          block={block}
          blockIndex={blockIndex}
          profile={profile}
          sessionId={sessionId}
          relPath={relPath}
          turnUuid={turnUuid}
          forceOpen={forceOpen}
        />
      )
  }
}

function ToolChip({
  block,
  blockIndex,
  profile,
  sessionId,
  relPath,
  turnUuid,
  forceOpen,
}: {
  block: TurnBlock
  blockIndex: number
  profile: string
  sessionId: string
  relPath: string
  turnUuid: string
  forceOpen: boolean
}) {
  const [open, setOpen] = useState(forceOpen)
  const [body, setBody] = useState<HistoryToolBody | null>(null)
  const [loading, setLoading] = useState(false)
  // Distinct from `body`: a failed fetch must not be stored as a success-shaped
  // empty body, or the re-expand guard below makes the blank permanent.
  const [failed, setFailed] = useState(false)

  // Force-open must also FETCH, not just expand. The chip's inline preview is
  // only the call's identifying argument (a Bash command, an Edit's path) while
  // search matches the whole input, so arriving from a hit and merely expanding
  // shows a file path where the match was — the jump resolves to nothing, which
  // is exactly what force-expanding was added to prevent.
  //
  // useState also seeds from forceOpen once, so without the effect only the
  // first target after a page load would expand at all.
  useEffect(() => {
    if (!forceOpen) return
    setOpen(true)
    void load()
    // load is stable for a given block; re-running on forceOpen alone is right.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [forceOpen])

  const isResult = block.kind === 'tool_result'
  const label = isResult ? 'result' : block.toolName || 'tool'
  const summary = (block.preview ?? '').split('\n')[0].slice(0, 120)

  // load fetches the full payload once. Shared by the manual toggle and the
  // force-open path so both show the same thing.
  async function load() {
    if (body || loading || !turnUuid) return
    setLoading(true)
    setFailed(false)
    try {
      setBody(await api.history.toolBody(profile, sessionId, relPath, turnUuid, blockIndex))
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }

  async function toggle() {
    const next = !open
    setOpen(next)
    if (next) await load()
  }

  const shown = body?.body ?? block.preview ?? ''
  const overCap = shown.length > CHIP_RENDER_CAP

  return (
    <div className="rounded-md border border-border bg-muted/30">
      <button
        onClick={toggle}
        aria-expanded={open}
        className="flex w-full cursor-pointer items-center gap-1.5 px-2 py-1 text-left text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {loading ? (
          <Loader2 className="size-3 shrink-0 animate-spin text-muted-foreground" />
        ) : (
          <ChevronRight
            className={cn('size-3 shrink-0 text-muted-foreground transition-transform', open && 'rotate-90')}
          />
        )}
        <Wrench className={cn('size-3 shrink-0', block.isError ? 'text-destructive' : 'text-muted-foreground')} />
        <span className={cn('shrink-0 font-medium', block.isError && 'text-destructive')}>{label}</span>
        <span className="truncate text-muted-foreground">{summary}</span>
        {block.fullBytes > 0 && (
          <span className="ml-auto shrink-0 tabular-nums text-[10px] text-muted-foreground">
            {humanBytes(block.fullBytes)}
          </span>
        )}
      </button>
      {open && failed && (
        <div className="border-t border-border px-2.5 py-2 text-[11px] text-destructive">
          Could not load this tool output — collapse and expand to retry.
        </div>
      )}
      {open && !failed && (
        <pre className="max-h-96 overflow-auto border-t border-border px-2.5 py-2 text-[11px] leading-relaxed">
          {overCap ? shown.slice(0, CHIP_RENDER_CAP) : shown}
          {(overCap || body?.truncated || (!body && block.truncated)) && (
            <TruncatedNote full={body?.fullBytes || block.fullBytes} />
          )}
        </pre>
      )}
    </div>
  )
}

function TruncatedNote({ full }: { full: number }) {
  return (
    <span className="mt-1 block text-[10px] not-italic text-muted-foreground">
      … truncated for display ({humanBytes(full)} total)
    </span>
  )
}

function humanBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
