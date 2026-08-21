import { useState } from 'react'
import { cn } from '@/lib/utils'
import { timeAgo } from '@/lib/format'
import { api } from '@/lib/api'
import { useToast } from '@/components/ui/Toast'
import { PromptModal } from '@/components/ui/Modal'
import { validateProfileName } from '@/lib/validate'
import type { Profile } from '@/types'
import { Boxes, KeyRound, Plus, Sparkles } from 'lucide-react'

interface Props {
  profiles: Profile[]
  selected: string
  onSelect: (name: string) => void
  names: string[]
  onMutated: () => void
}

export function Sidebar({ profiles, selected, onSelect, names, onMutated }: Props) {
  const [creating, setCreating] = useState(false)
  const toast = useToast()
  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-border bg-sidebar">
      {/* brand — sits below the titlebar strip, clear of the macOS lights */}
      <div className="flex items-center gap-2 px-3 pb-3 pt-3.5">
        <div className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Boxes className="size-4" />
        </div>
        <div className="leading-tight">
          <div className="text-[13px] font-semibold tracking-tight text-foreground">CCPM</div>
          <div className="text-[10px] uppercase tracking-wider text-muted-foreground">
            profile manager
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between px-3 pb-1.5 pt-1">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
          Profiles · {profiles.length}
        </span>
        <button
          onClick={() => setCreating(true)}
          title="Create a new profile"
          className="flex size-6 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Plus className="size-3.5" />
        </button>
      </div>

      <PromptModal
        open={creating}
        title="Create a new profile"
        label="Profile name (opens a Terminal to complete sign-in)"
        placeholder="e.g. personal"
        confirmLabel="Create in Terminal"
        validate={(v) => validateProfileName(v, names)}
        onCancel={() => setCreating(false)}
        onConfirm={async (name) => {
          setCreating(false)
          const r = await api.mutate.createInTerminal(name)
          if (r.ok) toast({ kind: 'info', title: 'Opening Terminal to create profile', desc: r.output })
          else toast({ kind: 'error', title: 'Could not open Terminal', desc: r.error || r.output })
          onMutated()
        }}
      />

      <nav className="flex-1 overflow-y-auto px-2 pb-3">
        {profiles.map((p) => (
          <ProfileRow
            key={p.name}
            profile={p}
            active={p.name === selected}
            onClick={() => onSelect(p.name)}
          />
        ))}
      </nav>
    </aside>
  )
}

function ProfileRow({
  profile,
  active,
  onClick,
}: {
  profile: Profile
  active: boolean
  onClick: () => void
}) {
  const total =
    profile.counts.skills +
    profile.counts.agents +
    profile.counts.commands +
    profile.counts.rules +
    profile.counts.hooks +
    profile.counts.plugins

  return (
    <button
      onClick={onClick}
      className={cn(
        'group relative mb-0.5 flex w-full cursor-pointer items-center rounded-lg px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        active
          ? 'bg-accent text-accent-foreground'
          : 'text-foreground/70 hover:bg-accent/40 hover:text-foreground',
      )}
    >
      {active && (
        <span className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-primary" />
      )}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-[13px] font-medium">{profile.name}</span>
          {profile.isDefault && (
            <span className="rounded bg-primary/15 px-1 py-px text-[9px] font-semibold uppercase tracking-wide text-primary">
              default
            </span>
          )}
        </div>
        <div className="flex items-center gap-2.5 text-[11px] text-muted-foreground">
          <span className="inline-flex items-center gap-1 tabular-nums">
            <Sparkles className="size-3" />
            {total}
          </span>
          <span className="inline-flex items-center gap-1">
            <KeyRound className="size-3" />
            {profile.authMethod || 'none'}
          </span>
          <span className="ml-auto tabular-nums">{timeAgo(profile.lastUsed)}</span>
        </div>
      </div>
    </button>
  )
}
