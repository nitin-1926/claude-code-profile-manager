import { useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import type { CmdResult, Details } from '@/types'
import { useToast } from '@/components/ui/Toast'
import { cn } from '@/lib/utils'
import { Plus, Trash2 } from 'lucide-react'

const MODES = ['default', 'acceptEdits', 'plan', 'auto', 'dontAsk', 'bypassPermissions']
const BUCKETS = [
  { id: 'allow', label: 'Allow', tone: 'text-secondary' },
  { id: 'ask', label: 'Ask', tone: 'text-primary' },
  { id: 'deny', label: 'Deny', tone: 'text-destructive' },
] as const

export function PermissionsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload] = useLive<Details>(() => api.details.get(profile), [profile])
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState<Record<string, string>>({ allow: '', ask: '', deny: '' })
  const [envKey, setEnvKey] = useState('')
  const [envVal, setEnvVal] = useState('')
  const toast = useToast()

  function report(action: string, r: CmdResult) {
    if (r.ok) toast({ kind: 'success', title: `${action} succeeded`, desc: r.output.split('\n')[0] })
    else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
    reload()
    onMutated()
  }

  async function withBusy(fn: () => Promise<void>) {
    setBusy(true)
    try {
      await fn()
    } finally {
      setBusy(false)
    }
  }

  if (!data) return <div className="px-6 py-5 text-sm text-muted-foreground">Loading…</div>
  const p = data.permissions ?? { allow: [], ask: [], deny: [], mode: '' }
  const env = data.env ?? []

  return (
    <div className="px-6 py-5">
      {/* mode */}
      <section className="mb-6">
        <h2 className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Default mode
        </h2>
        <div className="flex flex-wrap gap-1">
          {MODES.map((m) => (
            <button
              key={m}
              disabled={busy}
              onClick={() => withBusy(async () => report(`Set mode ${m}`, await api.mutate.setPermissionMode(m, profile)))}
              className={cn(
                'cursor-pointer rounded-md border px-2.5 py-1 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50',
                p.mode === m
                  ? 'border-primary bg-primary/15 text-primary'
                  : 'border-border text-muted-foreground hover:bg-accent hover:text-foreground',
              )}
            >
              {m}
            </button>
          ))}
        </div>
      </section>

      {/* buckets */}
      {BUCKETS.map((b) => {
        const rules = (p[b.id] ?? []) as string[]
        return (
          <section key={b.id} className="mb-5">
            <h2 className={cn('mb-2 text-[10px] font-semibold uppercase tracking-wider', b.tone)}>
              {b.label} · {rules.length}
            </h2>
            <div className="mb-2 flex gap-2">
              <input
                value={draft[b.id]}
                onChange={(e) => setDraft((d) => ({ ...d, [b.id]: e.target.value }))}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && draft[b.id].trim())
                    withBusy(async () => {
                      report(`Add ${b.label} rule`, await api.mutate.addPermission(b.id, draft[b.id].trim(), profile))
                      setDraft((d) => ({ ...d, [b.id]: '' }))
                    })
                }}
                placeholder={`e.g. Bash(git status:*)`}
                className="flex-1 rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <button
                disabled={busy || !draft[b.id].trim()}
                onClick={() =>
                  withBusy(async () => {
                    report(`Add ${b.label} rule`, await api.mutate.addPermission(b.id, draft[b.id].trim(), profile))
                    setDraft((d) => ({ ...d, [b.id]: '' }))
                  })
                }
                className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
              >
                <Plus className="size-3" /> Add
              </button>
            </div>
            <div className="overflow-hidden rounded-xl border border-border bg-card">
              {rules.length === 0 && <div className="px-4 py-2.5 text-xs text-muted-foreground">No rules</div>}
              {rules.map((rule, i) => (
                <div
                  key={rule}
                  className={cn('group flex items-center gap-3 px-4 py-2', i < rules.length - 1 && 'border-b border-border')}
                >
                  <span className="truncate font-mono text-xs">{rule}</span>
                  <button
                    disabled={busy}
                    title="Remove rule"
                    onClick={() => withBusy(async () => report('Remove rule', await api.mutate.removePermission(rule, profile)))}
                    className="ml-auto flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive group-hover:opacity-100 disabled:opacity-50"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </div>
              ))}
            </div>
          </section>
        )
      })}

      {/* env */}
      <section className="mt-7">
        <h2 className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Environment variables · {env.length}
        </h2>
        <div className="mb-2 flex gap-2">
          <input
            value={envKey}
            onChange={(e) => setEnvKey(e.target.value)}
            placeholder="KEY"
            className="w-48 rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <input
            value={envVal}
            onChange={(e) => setEnvVal(e.target.value)}
            placeholder="value"
            className="flex-1 rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <button
            disabled={busy || !envKey.trim() || !envVal.trim()}
            onClick={() =>
              withBusy(async () => {
                report('Set env var', await api.mutate.setEnv(`${envKey.trim()}=${envVal.trim()}`, profile))
                setEnvKey('')
                setEnvVal('')
              })
            }
            className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            <Plus className="size-3" /> Set
          </button>
        </div>
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          {env.length === 0 && <div className="px-4 py-2.5 text-xs text-muted-foreground">No env vars</div>}
          {env.map((e, i) => (
            <div
              key={e.key}
              className={cn('group flex items-center gap-3 px-4 py-2', i < env.length - 1 && 'border-b border-border')}
            >
              <span className="font-mono text-xs text-foreground">{e.key}</span>
              <span className="truncate font-mono text-xs text-muted-foreground">{e.value}</span>
              <button
                disabled={busy}
                title="Unset"
                onClick={() => withBusy(async () => report(`Unset ${e.key}`, await api.mutate.unsetEnv(e.key, profile)))}
                className="ml-auto flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive group-hover:opacity-100 disabled:opacity-50"
              >
                <Trash2 className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
