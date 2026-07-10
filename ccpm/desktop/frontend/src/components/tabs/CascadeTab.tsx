import { useEffect, useState, type ReactNode } from 'react'
import { api } from '@/lib/api'
import type { Cascade, CascadeAsset, CascadeSetting, Layer } from '@/types'
import { LayerBadge } from '@/components/LayerBadge'
import { cn } from '@/lib/utils'
import { ArrowRight, Settings2 } from 'lucide-react'

const KIND_ORDER = ['skill', 'agent', 'command', 'rule', 'hook', 'plugin']
const KIND_LABEL: Record<string, string> = {
  skill: 'Skills',
  agent: 'Agents',
  command: 'Commands',
  rule: 'Rules',
  hook: 'Hooks',
  plugin: 'Plugins',
}

export function CascadeTab({ profile }: { profile: string }) {
  const [data, setData] = useState<Cascade | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setData(null)
    setError(null)
    api.cascade
      .get(profile)
      .then((c) => alive && setData(c))
      .catch((e) => alive && setError(String(e)))
    return () => {
      alive = false
    }
  }, [profile])

  if (error) {
    return <div className="px-6 py-5 text-sm text-destructive">Failed to compute cascade: {error}</div>
  }
  if (!data) {
    return <div className="px-6 py-5 text-sm text-muted-foreground">Computing effective config…</div>
  }

  const byLayer = { host: 0, global: 0, profile: 0 } as Record<Layer, number>
  data.assets.forEach((a) => (byLayer[a.layer] += 1))
  const groups = KIND_ORDER.map((k) => ({
    kind: k,
    items: data.assets.filter((a) => a.kind === k),
  })).filter((g) => g.items.length > 0)

  return (
    <div className="px-6 py-5">
      {/* explainer */}
      <div className="mb-5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">Effective config</span>
        <span className="inline-flex items-center gap-1">
          <LayerBadge layer="host" /> <ArrowRight className="size-3" /> <LayerBadge layer="global" />{' '}
          <ArrowRight className="size-3" /> <LayerBadge layer="profile" />
        </span>
        <span className="ml-auto tabular-nums">
          {data.assets.length} assets · {data.settings.length} settings keys
        </span>
      </div>

      {groups.map((g) => (
        <section key={g.kind} className="mb-5">
          <SectionLabel>
            {KIND_LABEL[g.kind] ?? g.kind} · {g.items.length}
          </SectionLabel>
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            {g.items.map((a, i) => (
              <AssetRow key={a.name} asset={a} last={i === g.items.length - 1} />
            ))}
          </div>
        </section>
      ))}

      {data.settings.length > 0 && (
        <section className="mb-2">
          <SectionLabel>Settings · {data.settings.length}</SectionLabel>
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            {data.settings.map((s, i) => (
              <SettingRow key={s.key} setting={s} last={i === data.settings.length - 1} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </h2>
  )
}

function AssetRow({ asset, last }: { asset: CascadeAsset; last: boolean }) {
  return (
    <div className={cn('flex items-center gap-3 px-4 py-2.5', !last && 'border-b border-border')}>
      <LayerBadge layer={asset.layer} />
      <span className="truncate text-sm">{asset.name}</span>
      {asset.shadowedIn && asset.shadowedIn.length > 0 && (
        <span className="inline-flex items-center gap-1 text-[10px] text-muted-foreground/70">
          overrides {asset.shadowedIn.join(', ')}
        </span>
      )}
      <span className="ml-auto truncate font-mono text-[11px] text-muted-foreground/60">
        {asset.source}
      </span>
    </div>
  )
}

function SettingRow({ setting, last }: { setting: CascadeSetting; last: boolean }) {
  return (
    <div className={cn('flex items-center gap-3 px-4 py-2.5', !last && 'border-b border-border')}>
      <LayerBadge layer={setting.layer} />
      <span className="flex items-center gap-1.5 truncate text-sm">
        <Settings2 className="size-3.5 text-muted-foreground/60" />
        {setting.key}
      </span>
      {setting.merged && (
        <span className="text-[10px] text-muted-foreground/70">merges {setting.contributors.join('+')}</span>
      )}
      <span className="ml-auto truncate font-mono text-[11px] text-muted-foreground/60">
        {setting.value}
      </span>
    </div>
  )
}
