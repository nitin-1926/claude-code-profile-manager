import type { ReactNode } from 'react'
import type { Profile } from '@/types'
import { shortDate, tildePath } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  Bot,
  FolderOpen,
  KeyRound,
  Layers,
  type LucideIcon,
  Plug,
  Puzzle,
  ScrollText,
  Sparkles,
  TerminalSquare,
  Webhook,
} from 'lucide-react'

export function OverviewTab({ profile }: { profile: Profile }) {
  return (
    <div className="px-6 py-5">
      <section>
        <SectionLabel>Assets</SectionLabel>
        <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-6">
          <StatCard icon={Sparkles} label="Skills" value={profile.counts.skills} />
          <StatCard icon={Bot} label="Agents" value={profile.counts.agents} />
          <StatCard icon={TerminalSquare} label="Commands" value={profile.counts.commands} />
          <StatCard icon={ScrollText} label="Rules" value={profile.counts.rules} />
          <StatCard icon={Webhook} label="Hooks" value={profile.counts.hooks} />
          <StatCard icon={Puzzle} label="Plugins" value={profile.counts.plugins} />
        </div>
      </section>

      <section className="mt-7">
        <SectionLabel>Details</SectionLabel>
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <DetailRow icon={FolderOpen} label="Profile directory" value={tildePath(profile.dir)} mono />
          <DetailRow icon={KeyRound} label="Auth method" value={profile.authMethod || '—'} />
          <DetailRow icon={Layers} label="Created" value={shortDate(profile.createdAt)} />
          <DetailRow icon={Plug} label="Last used" value={shortDate(profile.lastUsed)} last />
        </div>
      </section>
    </div>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </h2>
  )
}

function StatCard({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: number }) {
  const empty = value === 0
  return (
    <div className="rounded-xl border border-border bg-card p-3 transition-colors hover:border-primary/40">
      <div className="mb-2 flex size-7 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon className="size-4" />
      </div>
      <div className={cn('text-2xl font-semibold tabular-nums', empty && 'text-muted-foreground/40')}>
        {value}
      </div>
      <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
    </div>
  )
}

function DetailRow({
  icon: Icon,
  label,
  value,
  mono,
  last,
}: {
  icon: LucideIcon
  label: string
  value: string
  mono?: boolean
  last?: boolean
}) {
  return (
    <div className={cn('flex items-center gap-3 px-4 py-3', !last && 'border-b border-border')}>
      <Icon className="size-4 text-muted-foreground" />
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className={cn('ml-auto truncate text-sm', mono && 'font-mono text-xs')}>{value}</div>
    </div>
  )
}
