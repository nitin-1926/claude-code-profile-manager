// Small presentation helpers shared across views.

/** Human "time ago" from an RFC3339 string; empty/invalid → "never". */
export function timeAgo(rfc3339: string): string {
  if (!rfc3339) return 'never'
  const then = new Date(rfc3339).getTime()
  if (Number.isNaN(then)) return 'never'
  const secs = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (secs < 60) return 'just now'
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}mo ago`
  return `${Math.floor(months / 12)}y ago`
}

/** RFC3339 → "Jun 30, 2026"; empty/invalid → "—". */
export function shortDate(rfc3339: string): string {
  if (!rfc3339) return '—'
  const d = new Date(rfc3339)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

/** Replace a leading $HOME with ~ for compact display. */
export function tildePath(p: string): string {
  return p.replace(/^\/Users\/[^/]+/, '~').replace(/^\/home\/[^/]+/, '~')
}

/** USD cost: $0.42, $12.30, $1.2k. */
export function money(n: number): string {
  if (n === 0) return '$0'
  if (n < 0.01) return '<$0.01'
  if (n < 1000) return '$' + n.toFixed(2)
  if (n < 1_000_000) return '$' + (n / 1000).toFixed(1) + 'k'
  return '$' + (n / 1_000_000).toFixed(1) + 'M'
}

/** Minutes → "2h 45m" / "45m". */
export function humanMinutes(min: number): string {
  const m = Math.max(0, Math.round(min))
  if (m < 60) return `${m}m`
  return `${Math.floor(m / 60)}h ${m % 60}m`
}

/** Compact token count: 1234 → "1.2K", 1_200_000 → "1.2M". */
export function humanTokens(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return (n / 1000).toFixed(n < 10_000 ? 1 : 0) + 'K'
  if (n < 1_000_000_000) return (n / 1_000_000).toFixed(n < 10_000_000 ? 1 : 0) + 'M'
  return (n / 1_000_000_000).toFixed(1) + 'B'
}
