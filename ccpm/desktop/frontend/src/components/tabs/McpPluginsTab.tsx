import { useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import type { CmdResult, Details } from '@/types'
import { useToast } from '@/components/ui/Toast'
import { Modal } from '@/components/ui/Modal'
import { cn } from '@/lib/utils'
import { Plug, Plus, Puzzle, Trash2 } from 'lucide-react'

export function McpPluginsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload, error] = useLive<Details>(() => api.details.get(profile), [profile])
  const [busy, setBusy] = useState(false)
  const [addingMcp, setAddingMcp] = useState(false)
  const [addingPlugin, setAddingPlugin] = useState(false)
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
                onClick={() => withBusy(async () => report(`Remove ${m.name}`, await api.mutate.removeMCP(m.name, profile)))}
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
                    withBusy(async () =>
                      report(`${p.enabled ? 'Disable' : 'Enable'} ${p.name}`, await api.mutate.togglePlugin(p.name, !p.enabled, profile)),
                    )
                  }
                />
                <RemoveBtn
                  busy={busy}
                  title={`Uninstall ${p.name}`}
                  onClick={() => withBusy(async () => report(`Uninstall ${p.name}`, await api.mutate.removePlugin(p.name, profile)))}
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
          await withBusy(async () => report(`Add MCP ${name}`, await api.mutate.addStdioMCP(name, command, profile)))
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
          await withBusy(async () => report(`Install ${plugin}`, await api.mutate.installPlugin(plugin, profile)))
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
      className="flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground opacity-0 transition-all hover:bg-destructive/15 hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100 disabled:opacity-50"
    >
      <Trash2 className="size-3.5" />
    </button>
  )
}

// A proper iOS-style switch: track + a clearly-contrasting white knob.
function Switch({ on, disabled, onClick }: { on: boolean; disabled?: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card disabled:opacity-50',
        on ? 'bg-primary' : 'bg-input',
      )}
    >
      <span
        className={cn(
          // bg-background + a border, not bg-white: a white knob on the light
          // theme's --input sits at 1.33:1 and reads as no knob at all.
          'inline-block size-4 rounded-full border border-border bg-background shadow-sm transition-transform',
          on ? 'translate-x-[18px]' : 'translate-x-0.5',
        )}
      />
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
  const ok = v.trim().length > 0
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <label className="text-xs text-muted-foreground">{label}</label>
      <input
        autoFocus
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
  const ok = a.trim() && b.trim()
  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <label className="text-xs text-muted-foreground">{f1.label}</label>
      <input
        autoFocus
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
