import { useState } from 'react'
import { Boxes, Download, Plus } from 'lucide-react'
import { api } from '@/lib/api'
import { useToast } from '@/components/ui/Toast'
import { PromptModal } from '@/components/ui/Modal'
import { validateProfileName } from '@/lib/validate'

export function EmptyState() {
  const [creating, setCreating] = useState(false)
  const toast = useToast()

  async function importHost() {
    const r = await api.mutate.importInTerminal()
    if (r.ok) toast({ kind: 'info', title: 'Opening Terminal', desc: r.output })
    else toast({ kind: 'error', title: 'Could not open Terminal', desc: r.error || r.output })
  }

  return (
    <div className="flex h-full w-full items-center justify-center bg-background p-8">
      <div className="max-w-md text-center">
        <div className="mx-auto mb-5 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
          <Boxes className="size-6" />
        </div>
        <h1 className="text-xl font-semibold tracking-tight">No profiles yet</h1>
        <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">
          Profiles are isolated Claude Code setups — work, personal, a client — each with its
          own auth, skills, MCPs, and hooks. No more logout/login juggling.
        </p>
        <div className="mt-6 flex flex-col items-center gap-2">
          <button
            onClick={() => setCreating(true)}
            className="inline-flex cursor-pointer items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Plus className="size-4" />
            Create your first profile
          </button>
          <button
            onClick={importHost}
            className="inline-flex cursor-pointer items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Download className="size-4" />
            Import existing ~/.claude config
          </button>
          <span className="mt-1 text-[11px] text-muted-foreground">
            sign-in completes in a Terminal window
          </span>
        </div>
      </div>

      <PromptModal
        open={creating}
        title="Create your first profile"
        label="Profile name (opens a Terminal to complete sign-in)"
        placeholder="e.g. work"
        confirmLabel="Create in Terminal"
        validate={(v) => validateProfileName(v, [])}
        onCancel={() => setCreating(false)}
        onConfirm={async (name) => {
          setCreating(false)
          const r = await api.mutate.createInTerminal(name)
          if (r.ok) toast({ kind: 'info', title: 'Opening Terminal to create profile', desc: r.output })
          else toast({ kind: 'error', title: 'Could not open Terminal', desc: r.error || r.output })
        }}
      />
    </div>
  )
}
