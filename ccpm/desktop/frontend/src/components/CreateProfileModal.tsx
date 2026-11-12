import { useEffect, useState } from 'react'
import { ExternalLink } from 'lucide-react'
import { api } from '@/lib/api'
import { useGuarded } from '@/lib/useGuarded'
import { useToast } from '@/components/ui/Toast'
import { GhostButton, Modal, PrimaryButton } from '@/components/ui/Modal'
import { validateProfileName } from '@/lib/validate'
import type { Preset } from '@/types'

const FIELD =
  'mt-1.5 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring'

/**
 * The new-profile flow: an Anthropic account signs in through a Terminal, a
 * provider preset is created in place with that provider's own key.
 */
export function CreateProfileModal({
  open,
  title,
  placeholder,
  taken,
  onClose,
  onCreated,
}: {
  open: boolean
  title: string
  placeholder: string
  taken: string[]
  onClose: () => void
  onCreated?: () => void
}) {
  const [name, setName] = useState('')
  const [presetId, setPresetId] = useState('')
  const [key, setKey] = useState('')
  const [presets, setPresets] = useState<Preset[]>([])
  const [busy, setBusy] = useState(false)
  const toast = useToast()
  const guard = useGuarded('Create profile')

  // Runs on close too: the key should not sit in state behind a dismissed dialog.
  useEffect(() => {
    setName('')
    setPresetId('')
    setKey('')
    if (open) api.profiles.presets().then(setPresets).catch(() => setPresets([]))
  }, [open])

  const preset = presets.find((p) => p.id === presetId)
  const trimmed = name.trim()
  const nameErr = trimmed ? validateProfileName(trimmed, taken) : null
  const canSubmit = !busy && !!trimmed && !nameErr && (!preset?.keyRequired || key.trim().length > 0)

  const submit = guard(async () => {
    if (!canSubmit) return
    if (!preset) {
      onClose()
      const r = await api.mutate.createInTerminal(trimmed)
      if (r.ok) toast({ kind: 'info', title: 'Opening Terminal to create profile', desc: r.output })
      else toast({ kind: 'error', title: 'Could not open Terminal', desc: r.error || r.output })
      onCreated?.()
      return
    }
    setBusy(true)
    try {
      // Stays open on failure, key included, so a bad paste can be corrected.
      const r = await api.mutate.createFromPreset(trimmed, preset.id, key.trim())
      if (!r.ok) {
        toast({ kind: 'error', title: 'Could not create profile', desc: r.error || r.output })
        return
      }
      onClose()
      toast({ kind: 'success', title: `Created ${trimmed}`, desc: `Launches through ${preset.label}` })
      onCreated?.()
    } finally {
      setBusy(false)
    }
  })

  return (
    <Modal open={open} onClose={onClose} title={title}>
      <label className="text-xs text-muted-foreground">Profile name</label>
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && submit()}
        placeholder={placeholder}
        className={FIELD}
      />
      {nameErr && <p className="mt-1.5 text-xs text-destructive">{nameErr}</p>}

      <label className="mt-3 block text-xs text-muted-foreground">Runs on</label>
      <select value={presetId} onChange={(e) => setPresetId(e.target.value)} className={FIELD}>
        <option value="">Anthropic (sign in through a Terminal)</option>
        {presets.length > 0 && (
          <optgroup label="Third-party providers">
            {presets.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </optgroup>
        )}
      </select>

      {preset && (
        <div className="mt-3 rounded-md border border-border bg-card p-3 text-xs text-muted-foreground">
          <p className="font-medium text-foreground">{preset.label}</p>
          <p className="mt-1 leading-relaxed">{preset.description}</p>
          <p className="mt-1 leading-relaxed">
            {preset.keyRequired && `Uses ${preset.label}'s own API key and billing. `}
            Compatibility is the provider's own claim, and some Claude Code features may behave differently.
          </p>
          <button
            type="button"
            onClick={() => api.openURL(preset.sourceUrl)}
            className="mt-2 inline-flex cursor-pointer items-center gap-1 text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ExternalLink className="size-3" />
            {preset.label} setup guide
          </button>
        </div>
      )}

      {preset?.keyRequired && (
        <>
          <label className="mt-3 block text-xs text-muted-foreground">{preset.label} API key</label>
          <input
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()}
            className={FIELD}
          />
          <p className="mt-1.5 text-xs text-muted-foreground">Stored in your OS keychain.</p>
        </>
      )}

      <div className="mt-4 flex justify-end gap-2">
        <GhostButton onClick={onClose}>Cancel</GhostButton>
        <PrimaryButton disabled={!canSubmit} onClick={submit}>
          {preset ? 'Create profile' : 'Create in Terminal'}
        </PrimaryButton>
      </div>
    </Modal>
  )
}
