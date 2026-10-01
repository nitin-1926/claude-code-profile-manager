import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import { useCommand } from '@/lib/useCommand'
import type { Details } from '@/types'
import { ConfirmModal, Modal } from '@/components/ui/Modal'
import { cn } from '@/lib/utils'
import { Switch } from '@/components/ui/Switch'
import { Plug, Plus, Puzzle, Trash2 } from 'lucide-react'

export function McpPluginsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload, error] = useLive<Details>(() => api.details.get(profile), [profile])
  const { busy, run } = useCommand(() => {
    reload()
    onMutated()
  })
  const [addingMcp, setAddingMcp] = useState(false)
  const [addingPlugin, setAddingPlugin] = useState(false)
  const [pending, setPending] = useState<{ kind: 'mcp' | 'plugin'; name: string } | null>(null)

  // Surface the failure instead of an indefinite "Loading…" — useLive
  // reports fetch errors and every consumer must render them.
  if (error)
    return (
      <div className="px-6 py-5 text-sm text-destructive">Could not load MCP servers and plugins: {error}</div>
    )
  if (!data) return <div className="px-6 py-5 text-sm text-muted-foreground">Loading…</div>
  const mcp = data.mcp ?? []
  const plugins = data.plugins ?? []

  return (
    <div className="px-6 py-5">
      <section className="mb-6">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            <Plug className="size-3.5" /> MCP servers · {mcp.length}
          </h2>
          <button
            disabled={busy}
            onClick={() => setAddingMcp(true)}
            className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          >
            <Plus className="size-3" /> Add
          </button>
        </div>
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          {mcp.length === 0 && <div className="px-4 py-3 text-xs text-muted-foreground">No MCP servers</div>}
          {mcp.map((m, i) => (
            <div
              key={m.name}
              className={cn('group flex items-center gap-3 px-4 py-2.5', i < mcp.length - 1 && 'border-b border-border')}
            >
              <span className="text-sm">{m.name}</span>
              <span className="rounded bg-muted px-1.5 py-px text-[10px] text-muted-foreground">{m.type}</span>
              <span className="ml-auto font-mono text-[11px] text-muted-foreground/60">{(m.sources ?? []).join(' · ')}</span>
              <RemoveBtn
                busy={busy}
                title={`Remove ${m.name} from this profile`}
                onClick={() => setPending({ kind: 'mcp', name: m.name })}
              />
            </div>
          ))}
        </div>
      </section>

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h2 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            <Puzzle className="size-3.5" /> Plugins · {plugins.length}
          </h2>
          <button
            disabled={busy}
            onClick={() => setAddingPlugin(true)}
            className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
          >
            <Plus className="size-3" /> Install
          </button>
        </div>
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          {plugins.length === 0 && <div className="px-4 py-3 text-xs text-muted-foreground">No plugins</div>}
          {plugins.map((p, i) => (
            <div
              key={p.name}
              className={cn('group flex items-center gap-3 px-4 py-2.5', i < plugins.length - 1 && 'border-b border-border')}
            >
              <span className="truncate text-sm">{p.name}</span>
              <div className="ml-auto flex items-center gap-2">
                <Switch
                  on={p.enabled}
                  disabled={busy}
                  onClick={() =>
                    run(`${p.enabled ? 'Disable' : 'Enable'} ${p.name}`, () => api.mutate.togglePlugin(p.name, !p.enabled, profile))
                  }
                />
                <RemoveBtn
                  busy={busy}
                  title={`Uninstall ${p.name}`}
                  onClick={() => setPending({ kind: 'plugin', name: p.name })}
                />
              </div>
            </div>
          ))}
        </div>
      </section>

      <TwoFieldModal
        open={addingMcp}
        title="Add stdio MCP server"
        f1={{ label: 'Server name', placeholder: 'e.g. github' }}
        f2={{ label: 'Command', placeholder: 'e.g. npx' }}
        confirmLabel="Add server"
        onCancel={() => setAddingMcp(false)}
        onConfirm={async (name, command) => {
          setAddingMcp(false)
          await run(`Add MCP ${name}`, () => api.mutate.addStdioMCP(name, command, profile))
        }}
      />
      <OneFieldModal
        open={addingPlugin}
        title="Install plugin"
        label="Plugin (name@marketplace)"
        placeholder="e.g. github@claude-plugins-official"
        confirmLabel="Install"
        onCancel={() => setAddingPlugin(false)}
        onConfirm={async (plugin) => {
          setAddingPlugin(false)
          await run(`Install ${plugin}`, () => api.mutate.installPlugin(plugin, profile))
        }}
      />
      <ConfirmModal
        open={pending !== null}
        title={pending ? (pending.kind === 'mcp' ? `Remove "${pending.name}"?` : `Uninstall "${pending.name}"?`) : ''}
        message={
          pending?.kind === 'plugin'
            ? 'This uninstalls the plugin from this profile. You can install it again later.'
            : 'This removes the MCP server from this profile. You can add it back later.'
        }
        confirmLabel={pending?.kind === 'plugin' ? 'Uninstall' : 'Remove'}
        onCancel={() => setPending(null)}
        onConfirm={async () => {
          const p = pending
          setPending(null)
          if (p?.kind === 'mcp') await run(`Remove ${p.name}`, () => api.mutate.removeMCP(p.name, profile))
          else if (p) await run(`Uninstall ${p.name}`, () => api.mutate.removePlugin(p.name, profile))
        }}
      />
    </div>
  )
}

function RemoveBtn({ busy, title, onClick }: { busy: boolean; title: string; onClick: () => void }) {
  return (
    <button
      disabled={busy}
      title={title}
      onClick={onClick}
      className="flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring group-hover:opacity-100 disabled:opacity-50"
    >
      <Trash2 className="size-3.5" />
    </button>
  )
}


function OneFieldModal({
  open,
  title,
  label,
  placeholder,
  confirmLabel,
  onCancel,
  onConfirm,
}: {
  open: boolean
  title: string
  label: string
  placeholder: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: (v: string) => void
}) {
  const [v, setV] = useState('')
  // Start empty on every open: the component stays mounted while closed, and
  // the last value pre-filled would invite a duplicate add.
  useEffect(() => {
    if (open) setV('')
  }, [open])
  const ok = v.trim().length > 0
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <label className="text-xs text-muted-foreground">{label}</label>
      <input
        value={v}
        onChange={(e) => setV(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && ok && onConfirm(v.trim())}
        placeholder={placeholder}
        className="mt-1.5 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <ModalFooter onCancel={onCancel} disabled={!ok} confirmLabel={confirmLabel} onConfirm={() => onConfirm(v.trim())} />
    </Modal>
  )
}

function TwoFieldModal({
  open,
  title,
  f1,
  f2,
  confirmLabel,
  onCancel,
  onConfirm,
}: {
  open: boolean
  title: string
  f1: { label: string; placeholder: string }
  f2: { label: string; placeholder: string }
  confirmLabel: string
  onCancel: () => void
  onConfirm: (a: string, b: string) => void
}) {
  const [a, setA] = useState('')
  const [b, setB] = useState('')
  useEffect(() => {
    if (open) {
      setA('')
      setB('')
    }
  }, [open])
  const ok = a.trim() && b.trim()
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <label className="text-xs text-muted-foreground">{f1.label}</label>
      <input
        value={a}
        onChange={(e) => setA(e.target.value)}
        placeholder={f1.placeholder}
        className="mt-1.5 mb-3 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <label className="text-xs text-muted-foreground">{f2.label}</label>
      <input
        value={b}
        onChange={(e) => setB(e.target.value)}
        placeholder={f2.placeholder}
        className="mt-1.5 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <ModalFooter onCancel={onCancel} disabled={!ok} confirmLabel={confirmLabel} onConfirm={() => onConfirm(a.trim(), b.trim())} />
    </Modal>
  )
}

function ModalFooter({
  onCancel,
  onConfirm,
  disabled,
  confirmLabel,
}: {
  onCancel: () => void
  onConfirm: () => void
  disabled: boolean
  confirmLabel: string
}) {
  return (
    <div className="mt-4 flex justify-end gap-2">
      <button onClick={onCancel} className="cursor-pointer rounded-md px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent">
        Cancel
      </button>
      <button
        disabled={disabled}
        onClick={onConfirm}
        className="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {confirmLabel}
      </button>
    </div>
  )
}
