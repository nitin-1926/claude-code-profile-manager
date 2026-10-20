import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import type { Comparison, CompareRow, CompareStatus, Layer } from '@/types'
import { LayerBadge } from '@/components/LayerBadge'
import { SectionLabel } from '@/components/ui/SectionLabel'
import { Switch } from '@/components/ui/Switch'
import { cn } from '@/lib/utils'
import { Eye, EyeOff } from 'lucide-react'

// Left is the profile being viewed, right the one picked: "only in right" reads
// as added, "only in left" as removed (same tones as the Permissions buckets).
const TONE: Record<CompareStatus, string> = {
  same: 'text-muted-foreground',
  changed: 'text-primary',
  'only-left': 'text-destructive',
  'only-right': 'text-secondary',
}
const BAR: Record<CompareStatus, string> = {
  same: 'border-l-transparent',
  changed: 'border-l-primary',
  'only-left': 'border-l-destructive',
  'only-right': 'border-l-secondary',
}
const GRID = 'grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_minmax(0,1.4fr)] gap-4'

function statusLabel(s: CompareStatus, left: string, right: string): string {
  if (s === 'changed') return 'differs'
  if (s === 'only-left') return `only in ${left}`
  if (s === 'only-right') return `only in ${right}`
  return 'same'
}

export function CompareTab({ profile, names }: { profile: string; names: string[] }) {
  const others = names.filter((n) => n !== profile)
  const [picked, setPicked] = useState('')
  // Falls back when the picked profile is renamed or deleted under the tab.
  const other = others.includes(picked) ? picked : (others[0] ?? '')
  const [data, setData] = useState<Comparison | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [hideSame, setHideSame] = useState(true)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  // Env values are often tokens and API keys: masked until revealed per row.
  const [revealed, setRevealed] = useState<Set<string>>(new Set())

  useEffect(() => {
    if (!other) return
    let alive = true
    setData(null)
    setError(null)
    api.compare
      .get(profile, other)
      .then((c) => alive && setData(c))
      .catch((e) => alive && setError(String(e)))
    return () => {
      alive = false
    }
  }, [profile, other])

  const toggle = (set: Set<string>, key: string) => {
    const next = new Set(set)
    if (!next.delete(key)) next.add(key)
    return next
  }

  if (others.length === 0) {
    return (
      <div className="px-6 py-5 text-sm text-muted-foreground">
        No other profiles to compare with. Create or clone one first.
      </div>
    )
  }

  const categories = (data?.categories ?? []).filter((c) => c.rows.length > 0)
  const differing = categories.reduce((n, c) => n + c.rows.filter((r) => r.status !== 'same').length, 0)

  return (
    <div className="px-6 py-5">
      <div className="mb-5 flex flex-wrap items-center gap-x-3 gap-y-2 text-xs text-muted-foreground">
        <span>
          Compare <span className="font-medium text-foreground">{profile}</span> with
        </span>
        <select
          value={other}
          onChange={(e) => setPicked(e.target.value)}
          aria-label="Profile to compare with"
          className="cursor-pointer rounded-md border border-input bg-background px-2 py-1 text-xs text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {others.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
        {data && !data.message && (
          <span className="tabular-nums">
            {differing === 0 ? 'no differences' : `${differing} ${differing === 1 ? 'difference' : 'differences'}`}
          </span>
        )}
        <label className="ml-auto inline-flex items-center gap-2">
          Hide identical
          <Switch on={hideSame} onClick={() => setHideSame((v) => !v)} label="Hide identical rows" />
        </label>
      </div>

      {error && <div className="text-sm text-destructive">Failed to compare profiles: {error}</div>}
      {!error && !data && <div className="text-sm text-muted-foreground">Comparing effective config…</div>}
      {data?.message && <div className="text-sm text-muted-foreground">{data.message}</div>}

      {data && !data.message && (
        <>
          {categories.length > 0 && (
            <div className={cn(GRID, 'mb-2 px-4 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground')}>
              <span>Name</span>
              <span className="truncate normal-case">{data.left}</span>
              <span className="truncate normal-case">{data.right}</span>
            </div>
          )}
          {categories.length === 0 && (
            <div className="text-sm text-muted-foreground">Neither profile has any config to compare.</div>
          )}
          {categories.map((cat) => {
            const diff = cat.rows.filter((r) => r.status !== 'same').length
            const rows = hideSame ? cat.rows.filter((r) => r.status !== 'same') : cat.rows
            return (
              <section key={cat.id} className="mb-5">
                <SectionLabel>
                  {cat.label} · {diff} differ · {cat.rows.length - diff} same
                </SectionLabel>
                <div className="overflow-hidden rounded-xl border border-border bg-card">
                  {rows.length === 0 && (
                    <div className="px-4 py-2.5 text-xs text-muted-foreground">
                      All {cat.rows.length} identical
                    </div>
                  )}
                  {rows.map((row, i) => {
                    const key = `${cat.id}/${row.name}`
                    return (
                      <Row
                        key={key}
                        row={row}
                        leftName={data.left}
                        rightName={data.right}
                        last={i === rows.length - 1}
                        secret={cat.secret}
                        revealed={revealed.has(key)}
                        onReveal={() => setRevealed((s) => toggle(s, key))}
                        expanded={expanded.has(key)}
                        onExpand={() => setExpanded((s) => toggle(s, key))}
                      />
                    )
                  })}
                </div>
              </section>
            )
          })}
        </>
      )}
    </div>
  )
}

function Row({
  row,
  leftName,
  rightName,
  last,
  secret,
  revealed,
  onReveal,
  expanded,
  onExpand,
}: {
  row: CompareRow
  leftName: string
  rightName: string
  last: boolean
  secret: boolean
  revealed: boolean
  onReveal: () => void
  expanded: boolean
  onExpand: () => void
}) {
  const masked = secret && !revealed
  // MCP definitions are shown as a summary; a difference can sit in a field
  // the summary leaves out (args, env values, URL query).
  const hiddenDiff = row.status === 'changed' && row.left === row.right && !secret
  return (
    <div className={cn(GRID, 'border-l-2 px-4 py-2', BAR[row.status], !last && 'border-b border-b-border')}>
      <div className="flex min-w-0 items-start gap-1.5">
        <div className="min-w-0">
          <div className="truncate font-mono text-xs text-foreground" title={row.name}>
            {row.name}
          </div>
          <div className={cn('text-[10px]', TONE[row.status])}>
            {statusLabel(row.status, leftName, rightName)}
            {hiddenDiff && <span className="text-muted-foreground"> in hidden fields</span>}
          </div>
        </div>
        {secret && (
          <button
            type="button"
            title={revealed ? 'Hide values' : 'Show values'}
            aria-label={`${revealed ? 'Hide' : 'Show'} values of ${row.name}`}
            aria-pressed={revealed}
            onClick={onReveal}
            className="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {revealed ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
          </button>
        )}
      </div>
      <Value value={row.left} layer={row.leftLayer} masked={masked} expanded={expanded} onExpand={onExpand} />
      <Value value={row.right} layer={row.rightLayer} masked={masked} expanded={expanded} onExpand={onExpand} />
    </div>
  )
}

function Value({
  value,
  layer,
  masked,
  expanded,
  onExpand,
}: {
  value: string
  layer: Layer | ''
  masked: boolean
  expanded: boolean
  onExpand: () => void
}) {
  // A present side always has a layer; an absent one has none.
  if (!layer) return <span className="self-center text-[11px] italic text-muted-foreground/50">not set</span>
  return (
    <div className="flex min-w-0 items-start gap-2">
      <LayerBadge layer={layer} className="mt-0.5 shrink-0" />
      {masked ? (
        <span className="font-mono text-[11px] text-muted-foreground">••••••••</span>
      ) : (
        <button
          type="button"
          onClick={onExpand}
          aria-expanded={expanded}
          title={expanded ? 'Collapse' : 'Show full value'}
          className={cn(
            'min-w-0 cursor-pointer rounded text-left font-mono text-[11px] text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            expanded ? 'whitespace-pre-wrap break-all' : 'truncate',
          )}
        >
          {value === '' ? '""' : value}
        </button>
      )}
    </div>
  )
}
