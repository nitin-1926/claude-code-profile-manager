import { useEffect, useRef, useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import type { BundlePlaceholder, BundlePreview, BundleRunnable } from '@/types'
import { api } from '@/lib/api'
import { useCommand } from '@/lib/useCommand'
import { useGuarded } from '@/lib/useGuarded'
import { validateProfileName } from '@/lib/validate'
import { useToast } from '@/components/ui/Toast'
import { GhostButton, Modal, PrimaryButton } from '@/components/ui/Modal'

/** Export a profile to a bundle: pick a location, then `ccpm export`. */
export function ExportModal({ open, profile, onClose }: { open: boolean; profile: string; onClose: () => void }) {
  const [includeSecrets, setIncludeSecrets] = useState(false)
  const { busy, run } = useCommand()
  const guard = useGuarded('Export')
  useEffect(() => {
    if (open) setIncludeSecrets(false)
  }, [open])

  async function exportTo() {
    const path = await api.pickExportPath(profile)
    if (!path) return
    if (await run('Export', () => api.mutate.exportProfile(profile, path, includeSecrets))) onClose()
  }

  return (
    <Modal open={open} onClose={onClose} title={`Export "${profile}"`}>
      <p className="text-sm text-muted-foreground">
        Bundles the profile&apos;s skills, agents, commands, rules, hooks, settings and MCP servers into one file
        you can share. Credentials and session history stay out, and every env and MCP header value is
        replaced with a placeholder whoever imports it fills in.
      </p>
      <label className="mt-3 flex cursor-pointer items-start gap-2 text-xs">
        <input
          type="checkbox"
          checked={includeSecrets}
          onChange={(e) => setIncludeSecrets(e.target.checked)}
          className="mt-0.5 accent-primary"
        />
        <span>
          Include secret values (only for your own machines)
          <span className="block text-muted-foreground">Keeps env and MCP header values; keep the file private.</span>
        </span>
      </label>
      <div className="mt-4 flex justify-end gap-2">
        <GhostButton onClick={onClose}>Cancel</GhostButton>
        <PrimaryButton disabled={busy} onClick={guard(exportTo)}>
          Choose location…
        </PrimaryButton>
      </div>
    </Modal>
  )
}

const RUNNABLE_GROUPS: { kind: BundleRunnable['kind']; label: string }[] = [
  { kind: 'hook', label: 'Hooks' },
  { kind: 'mcp', label: 'MCP servers' },
  { kind: 'command', label: 'Command settings' },
  { kind: 'env', label: 'Env keys' },
]

function placeholderLabel(p: BundlePlaceholder) {
  const value = `${p.field}.${p.key}`
  return p.server ? `MCP ${p.server}: ${value}` : value
}

type ImportState =
  | { step: 'loading' }
  | { step: 'error'; message: string }
  | { step: 'review'; preview: BundlePreview }
  | { step: 'done'; name: string; output: string }

/**
 * Import a bundle: show what it would run (the CLI's dry run), take the new
 * profile's name, then import with --trust-bundle. Open while `path` is set.
 */
export function ImportBundleModal({
  path,
  names,
  onClose,
  onImported,
}: {
  path: string | null
  names: string[]
  onClose: () => void
  /** Called once the import succeeded; the caller refreshes, then selects. */
  onImported: (name: string) => Promise<void>
}) {
  const [state, setState] = useState<ImportState>({ step: 'loading' })
  const [name, setName] = useState('')
  const running = useRef(false)
  const [busy, setBusy] = useState(false)
  const toast = useToast()
  const guard = useGuarded('Import')

  useEffect(() => {
    if (!path) return
    let live = true
    setState({ step: 'loading' })
    api.mutate.previewBundle(path).then(
      (preview) => {
        if (!live) return
        setState({ step: 'review', preview })
        setName(preview.name)
      },
      (e: unknown) => live && setState({ step: 'error', message: String(e) }),
    )
    return () => {
      live = false
    }
  }, [path])

  const nameErr = validateProfileName(name.trim(), names)

  async function doImport() {
    if (!path || state.step !== 'review' || nameErr || running.current) return
    running.current = true
    setBusy(true)
    try {
      const n = name.trim()
      const r = await api.mutate.importBundle(path, n)
      if (!r.ok) {
        toast({ kind: 'error', title: 'Import failed', desc: r.error || r.output })
        return
      }
      // The CLI's output says how to sign in and which values to fill in.
      setState({ step: 'done', name: n, output: r.output })
      await onImported(n)
    } finally {
      running.current = false
      setBusy(false)
    }
  }

  return (
    <Modal open={path !== null} onClose={onClose} title={state.step === 'done' ? `Imported "${state.name}"` : 'Import a profile bundle'}>
      {state.step === 'loading' && <p className="text-sm text-muted-foreground">Reading bundle…</p>}
      {state.step === 'error' && <p className="text-sm break-words text-destructive">{state.message}</p>}
      {state.step === 'review' && (
        <>
          <label className="text-xs text-muted-foreground">New profile name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1.5 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {nameErr && <p className="mt-1.5 text-xs text-destructive">{nameErr}</p>}
          <ReviewLists preview={state.preview} />
        </>
      )}
      {state.step === 'done' && (
        <pre className="max-h-64 overflow-auto rounded-md bg-muted p-2 font-mono text-[11px] break-all whitespace-pre-wrap">
          {state.output}
        </pre>
      )}
      <div className="mt-4 flex justify-end gap-2">
        {state.step === 'done' ? (
          <PrimaryButton onClick={onClose}>Done</PrimaryButton>
        ) : (
          <>
            <GhostButton onClick={onClose}>Cancel</GhostButton>
            {state.step === 'review' && (
              <PrimaryButton disabled={!!nameErr || busy} onClick={guard(doImport)}>
                {state.preview.runnables.length > 0 ? 'Trust and import' : 'Import'}
              </PrimaryButton>
            )}
          </>
        )}
      </div>
    </Modal>
  )
}

function ReviewLists({ preview }: { preview: BundlePreview }) {
  // A settings env key sits in both the profile's settings.json and its
  // fragment; one fill-in covers both.
  const holes = [...new Set(preview.placeholders.map(placeholderLabel))]
  return (
    <div className="mt-3 max-h-72 space-y-3 overflow-y-auto text-xs">
      {preview.runnables.length > 0 ? (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 p-2">
          <p className="flex items-start gap-1.5 text-foreground">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0 text-destructive" />
            This bundle runs or changes the following every time the profile launches. Import it only if you
            trust whoever made it.
          </p>
          {RUNNABLE_GROUPS.map(({ kind, label }) => {
            const items = preview.runnables.filter((r) => r.kind === kind)
            if (items.length === 0) return null
            return (
              <div key={kind} className="mt-2">
                <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</div>
                <ul className="mt-0.5 space-y-0.5">
                  {items.map((r, i) => (
                    <li key={i} className="font-mono text-[11px] break-all" title={r.source}>
                      {r.detail}
                    </li>
                  ))}
                </ul>
              </div>
            )
          })}
        </div>
      ) : (
        <p className="text-muted-foreground">This bundle runs no hooks, MCP servers or commands.</p>
      )}
      {holes.length > 0 && (
        <div>
          <p className="text-muted-foreground">
            These values were stripped when the bundle was exported. After importing, fill them in (ccpm prints how):
          </p>
          <ul className="mt-1 space-y-0.5">
            {holes.map((label) => (
              <li key={label} className="font-mono text-[11px] break-all">
                {label}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
