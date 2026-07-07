import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { useLive } from '@/lib/useLive'
import type { CmdResult, SettingKV } from '@/types'
import { useToast } from '@/components/ui/Toast'
import { Modal } from '@/components/ui/Modal'
import { cn } from '@/lib/utils'
import { Plus, Save } from 'lucide-react'

export function SettingsTab({ profile, onMutated }: { profile: string; onMutated: () => void }) {
  const [data, reload] = useLive<SettingKV[]>(() => api.settings.get(profile), [profile])
  const [busy, setBusy] = useState(false)
  const [adding, setAdding] = useState(false)
  const toast = useToast()

  function report(action: string, r: CmdResult) {
    if (r.ok) toast({ kind: 'success', title: `${action} succeeded`, desc: r.output.split('\n')[0] })
    else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
    reload()
    onMutated()
  }

  async function save(key: string, value: string) {
    setBusy(true)
    try {
      report(`Set ${key}`, await api.mutate.setSetting(key, value, profile))
    } finally {
      setBusy(false)
    }
  }

  if (!data) return <div className="px-6 py-5 text-sm text-muted-foreground">Loading settings…</div>
  const rows = data ?? []

  return (
    <div className="px-6 py-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Effective settings · {rows.length}
        </h2>
        <button
          disabled={busy}
          onClick={() => setAdding(true)}
          className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        >
          <Plus className="size-3" /> New key
        </button>
      </div>

      <p className="mb-3 text-xs text-muted-foreground">
        Values are JSON. Editing writes to this profile's ccpm settings fragment via{' '}
        <span className="font-mono">ccpm settings set</span>. Permissions, plugins, and MCP live in their own tabs.
      </p>

      <div className="space-y-2">
        {rows.length === 0 && (
          <div className="rounded-xl border border-border bg-card px-4 py-3 text-xs text-muted-foreground">
            No editable settings
          </div>
        )}
        {rows.map((kv) => (
          <SettingRow key={kv.key} kv={kv} busy={busy} onSave={(v) => save(kv.key, v)} />
        ))}
      </div>

      <NewKeyModal
        open={adding}
        onCancel={() => setAdding(false)}
        onConfirm={async (key, value) => {
          setAdding(false)
          await save(key, value)
        }}
      />
    </div>
  )
}

function SettingRow({ kv, busy, onSave }: { kv: SettingKV; busy: boolean; onSave: (v: string) => void }) {
  const [value, setValue] = useState(kv.value)
  useEffect(() => setValue(kv.value), [kv.value])
  const dirty = value !== kv.value
  const valid = isJSON(value)

  return (
    <div className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-2.5">
      <span className="w-56 shrink-0 truncate font-mono text-xs text-foreground">{kv.key}</span>
      <input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && dirty && valid && onSave(value)}
        spellCheck={false}
        className={cn(
          'flex-1 rounded-md border bg-background px-2.5 py-1.5 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring',
          valid ? 'border-input' : 'border-destructive',
        )}
      />
      <button
        disabled={busy || !dirty || !valid}
        onClick={() => onSave(value)}
        title={valid ? 'Save' : 'Value must be valid JSON'}
        className="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-40"
      >
        <Save className="size-3.5" />
      </button>
    </div>
  )
}

function NewKeyModal({
  open,
  onCancel,
  onConfirm,
}: {
  open: boolean
  onCancel: () => void
  onConfirm: (key: string, value: string) => void
}) {
  const [key, setKey] = useState('')
  const [value, setValue] = useState('')
  const valid = key.trim() && isJSON(value)
  return (
    <Modal open={open} onClose={onCancel} title="New setting">
      <label className="text-xs text-muted-foreground">Key (dot notation, e.g. statusLine.type)</label>
      <input
        autoFocus
        value={key}
        onChange={(e) => setKey(e.target.value)}
        placeholder="theme"
        className="mt-1.5 mb-3 w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <label className="text-xs text-muted-foreground">Value (JSON — e.g. "dark", true, 32768)</label>
      <input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder='"dark"'
        spellCheck={false}
        className={cn(
          'mt-1.5 w-full rounded-md border bg-background px-3 py-2 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring',
          value && !isJSON(value) ? 'border-destructive' : 'border-input',
        )}
      />
      <div className="mt-4 flex justify-end gap-2">
        <button onClick={onCancel} className="cursor-pointer rounded-md px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent">
          Cancel
        </button>
        <button
          disabled={!valid}
          onClick={() => onConfirm(key.trim(), value)}
          className="cursor-pointer rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          Set
        </button>
      </div>
    </Modal>
  )
}

function isJSON(s: string): boolean {
  if (!s.trim()) return false
  try {
    JSON.parse(s)
    return true
  } catch {
    return false
  }
}
