import { useEffect, useState, type ReactNode } from 'react'
import { api } from '@/lib/api'
import type { Block, Usage, UsageNamed } from '@/types'
import { humanMinutes, humanTokens, money } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Flame, Timer } from 'lucide-react'

const WINDOWS = [
  { id: 'all', label: 'All' },
  { id: '90d', label: '90d' },
  { id: '30d', label: '30d' },
  { id: '7d', label: '7d' },
] as const

export function UsageTab({ profile }: { profile: string }) {
  const [win, setWin] = useState<string>('all')
  const [data, setData] = useState<Usage | null>(null)
  const [active, setActive] = useState<Block | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setData(null)
    setError(null)
    api.usage
      .get(profile, win)
      .then((u) => alive && setData(u))
      .catch((e) => alive && setError(String(e)))
    // active 5-hour block (burn/projection) — independent of the window filter
    api.usage
      .blocks(profile)
      .then((bs) => alive && setActive(bs.find((b) => b.isActive) ?? null))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [profile, win])

  return (
    <div className="px-6 py-5">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Token usage
        </h2>
        <div className="flex gap-0.5 rounded-lg border border-border p-0.5">
          {WINDOWS.map((w) => (
            <button
              key={w.id}
              onClick={() => setWin(w.id)}
              className={cn(
                'cursor-pointer rounded-md px-2.5 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                win === w.id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {w.label}
            </button>
          ))}
        </div>
      </div>

      {error && <div className="text-sm text-destructive">Failed to load usage: {error}</div>}
      {!data && !error && <div className="text-sm text-muted-foreground">Loading usage…</div>}

      {data && data.totals.total === 0 && (
        <div className="rounded-xl border border-border bg-card p-8 text-center">
          <div className="text-sm font-medium">No usage recorded in this window</div>
          <div className="mt-1 text-xs text-muted-foreground">
            Usage is derived from this profile's Claude Code transcripts.
            {!data.trackingEnabled && ' Enable the SessionEnd hook with `ccpm config set usage_tracking true` to keep it warm.'}
          </div>
        </div>
      )}

      {active && <ActiveBlockCard block={active} />}

      {data && data.totals.total > 0 && (
        <>
          <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-6">
            <Stat label="Cost (est.)" value={money(data.cost)} accent />
            <Stat label="Total" value={humanTokens(data.totals.total)} />
            <Stat label="Input" value={humanTokens(data.totals.input)} />
            <Stat label="Output" value={humanTokens(data.totals.output)} />
            <Stat label="Cache read" value={humanTokens(data.totals.cacheRead)} />
            <Stat label="Messages" value={humanTokens(data.messages)} />
          </div>

          <section className="mt-7">
            <SectionLabel>Activity</SectionLabel>
            <div className="rounded-xl border border-border bg-card p-4">
              <Heatmap days={data.byDay} />
            </div>
          </section>

          <div className="mt-7 grid gap-5 lg:grid-cols-2">
            <NamedTable title="By model" rows={data.byModel} />
            <NamedTable title="By project" rows={data.byProject} />
          </div>

          <section className="mt-7">
            <SectionLabel>Sessions</SectionLabel>
            <div className="rounded-xl border border-border bg-card px-4 py-3 text-xs text-muted-foreground">
              {data.sessions.length} sessions recorded. Open the{' '}
              <span className="text-foreground">History</span> tab to browse, read and search them.
            </div>
          </section>
        </>
      )}
    </div>
  )
}

// ActiveBlockCard is the live "current 5-hour block" monitor: cost/tokens so
// far, burn rate, time left in the window, and the projected end-of-block cost.
function ActiveBlockCard({ block }: { block: Block }) {
  const pct = Math.min(100, Math.max(0, ((300 - block.remainingMinutes) / 300) * 100))
  return (
    <div className="mb-5 rounded-xl border border-primary/30 bg-primary/5 p-4">
      <div className="mb-3 flex items-center gap-2">
        <Flame className="size-4 text-primary" />
        <span className="text-sm font-medium">Current 5-hour block</span>
        <span className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground">
          <Timer className="size-3.5" />
          {humanMinutes(block.remainingMinutes)} left
        </span>
      </div>
      <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-primary" style={{ width: `${pct}%` }} />
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <BlockStat label="Cost so far" value={money(block.cost)} accent />
        <BlockStat label="Tokens" value={humanTokens(block.total)} />
        <BlockStat label="Burn" value={`${money(block.costPerHour)}/hr`} />
        <BlockStat label="Projected" value={money(block.projectedCost)} />
      </div>
    </div>
  )
}

function BlockStat({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div>
      <div className={cn('text-lg font-semibold tabular-nums', accent && 'text-primary')}>{value}</div>
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
    </div>
  )
}

function Stat({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className={cn('text-xl font-semibold tabular-nums', accent && 'text-primary')}>{value}</div>
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
    </div>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </h2>
  )
}

function NamedTable({ title, rows }: { title: string; rows: UsageNamed[] }) {
  const max = Math.max(1, ...rows.map((r) => r.total))
  return (
    <div>
      <SectionLabel>{title}</SectionLabel>
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        {rows.length === 0 && <div className="px-4 py-3 text-xs text-muted-foreground">No data</div>}
        {rows.slice(0, 8).map((r, i) => (
          <div
            key={r.name + i}
            className={cn('flex items-center gap-3 px-4 py-2', i < Math.min(8, rows.length) - 1 && 'border-b border-border')}
          >
            <span className="w-40 truncate text-sm">{r.name || '—'}</span>
            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary/70" style={{ width: `${(r.total / max) * 100}%` }} />
            </div>
            {r.cost > 0 && (
              <span className="w-14 text-right text-xs tabular-nums text-primary">{money(r.cost)}</span>
            )}
            <span className="w-14 text-right text-xs tabular-nums text-muted-foreground">
              {humanTokens(r.total)}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

// Heatmap: GitHub-style grid, 7 day-rows × ~26 week-columns, amber ramp. Bucketing
// matches the CLI's pickBucket (internal/usage/heatmap.go) so CLI and GUI agree.
function Heatmap({ days }: { days: { date: string; total: number }[] }) {
  const totals = new Map(days.map((d) => [d.date, d.total]))
  const max = Math.max(1, ...days.map((d) => d.total))
  const weeks = 26

  const end = new Date()
  end.setHours(0, 0, 0, 0)
  // align end to Saturday
  const grid: { date: string; total: number; bucket: number }[][] = []
  const start = new Date(end)
  start.setDate(start.getDate() - (weeks * 7 - 1))
  // walk back to the Sunday on/before start
  start.setDate(start.getDate() - start.getDay())

  for (let w = 0; w < weeks; w++) {
    const col: { date: string; total: number; bucket: number }[] = []
    for (let d = 0; d < 7; d++) {
      const cur = new Date(start)
      cur.setDate(start.getDate() + w * 7 + d)
      // Local date key — byDay is bucketed by local calendar day, so formatting
      // in UTC (toISOString) would shift cells a day for non-UTC users.
      const key = `${cur.getFullYear()}-${String(cur.getMonth() + 1).padStart(2, '0')}-${String(cur.getDate()).padStart(2, '0')}`
      const total = totals.get(key) ?? 0
      col.push({ date: key, total, bucket: bucketOf(total, max) })
    }
    grid.push(col)
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex gap-[3px] overflow-x-auto">
        {grid.map((col, wi) => (
          <div key={wi} className="flex flex-col gap-[3px]">
            {col.map((cell) => (
              <div
                key={cell.date}
                title={`${cell.date}: ${humanTokens(cell.total)} tokens`}
                className={cn('size-3 rounded-[3px]', BUCKET_CLASS[cell.bucket])}
              />
            ))}
          </div>
        ))}
      </div>
      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
        <span>Less</span>
        {[0, 1, 2, 3, 4, 5].map((b) => (
          <div key={b} className={cn('size-3 rounded-[3px]', BUCKET_CLASS[b])} />
        ))}
        <span>More</span>
      </div>
    </div>
  )
}

const BUCKET_CLASS = [
  'bg-muted/40',
  'bg-primary/20',
  'bg-primary/40',
  'bg-primary/60',
  'bg-primary/80',
  'bg-primary',
] as const

function bucketOf(total: number, max: number): number {
  if (total <= 0) return 0
  const f = total / max
  if (f <= 0.1) return 1
  if (f <= 0.25) return 2
  if (f <= 0.5) return 3
  if (f <= 0.75) return 4
  return 5
}
