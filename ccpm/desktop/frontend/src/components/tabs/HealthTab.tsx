import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import type { HealthResult } from '@/types'
import { cn } from '@/lib/utils'
import { RefreshCw, Stethoscope, Terminal } from 'lucide-react'

export function HealthTab() {
  const [res, setRes] = useState<HealthResult | null>(null)
  const [loading, setLoading] = useState(true)

  async function run() {
    setLoading(true)
    try {
      setRes(await api.health.doctor())
    } catch (e) {
      setRes({ available: false, ccpmPath: '', output: '', error: String(e) })
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void run()
  }, [])

  return (
    <div className="px-6 py-5">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-medium">
          <Stethoscope className="size-4 text-muted-foreground" />
          ccpm doctor
        </h2>
        <button
          onClick={() => void run()}
          className="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <RefreshCw className={cn('size-3.5', loading && 'animate-spin')} />
          Re-run
        </button>
      </div>

      {res && !res.available && (
        <div className="rounded-xl border border-border bg-card p-6 text-center">
          <Terminal className="mx-auto mb-3 size-6 text-muted-foreground" />
          <div className="text-sm font-medium">ccpm CLI not found</div>
          <div className="mt-1 text-xs text-muted-foreground">
            Install it (`npm i -g @ngcodes/ccpm`) or ensure it's on PATH. {res.error}
          </div>
        </div>
      )}

      {res && res.available && (
        <>
          <pre className="overflow-x-auto rounded-xl border border-border bg-card p-4 font-mono text-xs leading-relaxed">
            {res.output.split('\n').map((line, i) => (
              <div key={i} className={lineClass(line)}>
                {line || ' '}
              </div>
            ))}
          </pre>
          <div className="mt-2 text-[11px] text-muted-foreground">via {res.ccpmPath}</div>
        </>
      )}
    </div>
  )
}

function lineClass(line: string): string {
  if (/[✗✘]|\bfail|\berror|\bmissing|\bexpired/i.test(line)) return 'text-destructive'
  if (/[⚠]|\bwarn|\bdrift|\bstale/i.test(line)) return 'text-primary'
  if (/[✓✔]|\bok\b|\bhealthy|\bvalid/i.test(line)) return 'text-secondary'
  return 'text-foreground/80'
}
