import { useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import { useCommand } from '@/lib/useCommand'
import type { Cascade, CascadeAsset } from '@/types'
import { LayerBadge } from '@/components/LayerBadge'
import { ConfirmModal } from '@/components/ui/Modal'
import { cn } from '@/lib/utils'
import { Plus, Trash2 } from 'lucide-react'

const KINDS = ['skill', 'agent', 'command', 'rule', 'hook'] as const
const LABEL: Record<string, string> = {
  skill: 'Skills',
  agent: 'Agents',
  command: 'Commands',
  rule: 'Rules',
  hook: 'Hooks',
}

export function AssetsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload, error] = useLive<Cascade>(() => api.cascade.get(profile), [profile])
  const { busy, run } = useCommand(() => {
    reload()
    onMutated()
  })
  const [pendingRemove, setPendingRemove] = useState<{ kind: string; name: string } | null>(null)

  async function add(kind: string) {
    const dir = await api.pickDirectory()
    if (!dir) return
    await run(`Add ${kind}`, () => api.mutate.addAsset(kind, dir, profile))
  }

  async function remove(kind: string, name: string) {
    await run(`Remove ${name}`, () => api.mutate.removeAsset(kind, name, profile))
  }

  // Surface the failure instead of an indefinite "Loading…" — useLive
  // reports fetch errors and every consumer must render them.
  if (error)
    return (
      <div className="px-6 py-5 text-sm text-destructive">Could not load assets: {error}</div>
    )
  if (!data) return <div className="px-6 py-5 text-sm text-muted-foreground">Loading assets…</div>

  return (
    <div className="px-6 py-5">
      {KINDS.map((kind) => {
        const items = data.assets.filter((a) => a.kind === kind)
        return (
          <section key={kind} className="mb-5">
            <div className="mb-2 flex items-center justify-between">
              <h2 className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                {LABEL[kind]} · {items.length}
              </h2>
              <button
                disabled={busy}
                onClick={() => add(kind)}
                className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
              >
                <Plus className="size-3" />
                Add
              </button>
            </div>
            <div className="overflow-hidden rounded-xl border border-border bg-card">
              {items.length === 0 && (
                <div className="px-4 py-3 text-xs text-muted-foreground">None installed</div>
              )}
              {items.map((a, i) => (
                <AssetRow
                  key={a.name}
                  asset={a}
                  last={i === items.length - 1}
                  busy={busy}
                  onRemove={() => setPendingRemove({ kind, name: a.name })}
                />
              ))}
            </div>
          </section>
        )
      })}
      <ConfirmModal
        open={pendingRemove !== null}
        title={pendingRemove ? `Remove "${pendingRemove.name}"?` : ''}
        message="This removes the asset from this profile. You can add it back later."
        confirmLabel="Remove"
        onCancel={() => setPendingRemove(null)}
        onConfirm={async () => {
          const p = pendingRemove
          setPendingRemove(null)
          if (p) await remove(p.kind, p.name)
        }}
      />
    </div>
  )
}

function AssetRow({
  asset,
  last,
  busy,
  onRemove,
}: {
  asset: CascadeAsset
  last: boolean
  busy: boolean
  onRemove: () => void
}) {
  return (
    <div className={cn('group flex items-center gap-3 px-4 py-2.5', !last && 'border-b border-border')}>
      <LayerBadge layer={asset.layer} />
      <span className="truncate text-sm">{asset.name}</span>
      <span className="ml-auto truncate font-mono text-[11px] text-muted-foreground/60">{asset.source}</span>
      <button
        disabled={busy}
        title={`Remove ${asset.name} from this profile`}
        onClick={onRemove}
        className="flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100 disabled:opacity-50"
      >
        <Trash2 className="size-3.5" />
      </button>
    </div>
  )
}
