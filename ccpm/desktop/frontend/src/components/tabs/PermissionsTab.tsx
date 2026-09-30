import { useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import { useCommand } from '@/lib/useCommand'
import type { Details } from '@/types'
import { ConfirmModal } from '@/components/ui/Modal'
import { cn } from '@/lib/utils'
import { Eye, EyeOff, Plus, Trash2 } from 'lucide-react'

const MODES = ['default', 'acceptEdits', 'plan', 'auto', 'dontAsk', 'bypassPermissions']
const BUCKETS = [
  { id: 'allow', label: 'Allow', tone: 'text-secondary' },
  { id: 'ask', label: 'Ask', tone: 'text-primary' },
  { id: 'deny', label: 'Deny', tone: 'text-destructive' },
] as const

export function PermissionsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload, error] = useLive<Details>(() => api.details.get(profile), [profile])
  const { busy, run } = useCommand(() => {
    reload()
    onMutated()
  })
  const [draft, setDraft] = useState<Record<string, string>>({ allow: '', ask: '', deny: '' })
  const [envKey, setEnvKey] = useState('')
  const [envVal, setEnvVal] = useState('')
  // Env values are often tokens and API keys: masked until revealed per row.
  const [revealed, setRevealed] = useState<Set<string>>(new Set())
  const [pending, setPending] = useState<{ kind: 'rule' | 'env'; name: string } | null>(null)

  // Drafts are cleared only on success, so a rejected rule stays editable.
  async function addRule(bucket: (typeof BUCKETS)[number]) {
    const rule = draft[bucket.id].trim()
    if (!rule) return
    if (await run(`Add ${bucket.label} rule`, () => api.mutate.addPermission(bucket.id, rule, profile)))
      setDraft((d) => ({ ...d, [bucket.id]: '' }))
  }

  async function setEnv() {
    const kv = `${envKey.trim()}=${envVal.trim()}`
    if (await run('Set env var', () => api.mutate.setEnv(kv, profile))) {
      setEnvKey('')
      setEnvVal('')
    }
  }

  function toggleReveal(key: string) {
    setRevealed((s) => {
      const next = new Set(s)
      if (!next.delete(key)) next.add(key)
      return next
    })
  }

  // Surface the failure instead of an indefinite "Loading…" — useLive
  // reports fetch errors and every consumer must render them.
  if (error)
    return (
      <div className="px-6 py-5 text-sm text-destructive">Could not load permissions: {error}</div>
    )
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
              onClick={() => run(`Set mode ${m}`, () => api.mutate.setPermissionMode(m, profile))}
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
                onKeyDown={(e) => e.key === 'Enter' && !busy && void addRule(b)}
                placeholder={`e.g. Bash(git status:*)`}
                className="flex-1 rounded-md border border-input bg-background px-3 py-1.5 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <button
                disabled={busy || !draft[b.id].trim()}
                onClick={() => void addRule(b)}
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
                    onClick={() => setPending({ kind: 'rule', name: rule })}
                    className="ml-auto flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100 disabled:opacity-50"
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
            onClick={() => void setEnv()}
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
              <span className="truncate font-mono text-xs text-muted-foreground">
                {revealed.has(e.key) ? e.value : '••••••••'}
              </span>
              <button
                title={revealed.has(e.key) ? 'Hide value' : 'Show value'}
                aria-label={`${revealed.has(e.key) ? 'Hide' : 'Show'} value of ${e.key}`}
                aria-pressed={revealed.has(e.key)}
                onClick={() => toggleReveal(e.key)}
                className="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {revealed.has(e.key) ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
              </button>
              <button
                disabled={busy}
                title="Unset"
                onClick={() => setPending({ kind: 'env', name: e.key })}
                className="ml-auto flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100 disabled:opacity-50"
              >
                <Trash2 className="size-3.5" />
              </button>
            </div>
          ))}
        </div>
      </section>

      <ConfirmModal
        open={pending !== null}
        title={pending ? (pending.kind === 'rule' ? `Remove rule "${pending.name}"?` : `Unset ${pending.name}?`) : ''}
        message={
          pending?.kind === 'env'
            ? "This removes the variable from this profile's settings."
            : "This removes the rule from this profile's permissions."
        }
        confirmLabel={pending?.kind === 'env' ? 'Unset' : 'Remove'}
        onCancel={() => setPending(null)}
        onConfirm={async () => {
          const p = pending
          setPending(null)
          if (p?.kind === 'rule') await run('Remove rule', () => api.mutate.removePermission(p.name, profile))
          else if (p) await run(`Unset ${p.name}`, () => api.mutate.unsetEnv(p.name, profile))
        }}
      />
    </div>
  )
}
