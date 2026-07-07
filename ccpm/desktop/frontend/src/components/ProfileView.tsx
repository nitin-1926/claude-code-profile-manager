import { useState } from 'react'
import type { CmdResult, Profile } from '@/types'
import { tildePath, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'
import { useToast } from '@/components/ui/Toast'
import { PromptModal, ConfirmModal } from '@/components/ui/Modal'
import { validateProfileName } from '@/lib/validate'
import { Copy, FolderOpen, KeyRound, type LucideIcon, Pencil, Play, Trash2 } from 'lucide-react'
import { OverviewTab } from './tabs/OverviewTab'
import { CascadeTab } from './tabs/CascadeTab'
import { UsageTab } from './tabs/UsageTab'
import { HealthTab } from './tabs/HealthTab'
import { AssetsTab } from './tabs/AssetsTab'
import { McpPluginsTab } from './tabs/McpPluginsTab'
import { PermissionsTab } from './tabs/PermissionsTab'
import { SettingsTab } from './tabs/SettingsTab'
import { ErrorBoundary } from './ui/ErrorBoundary'

const TABS = [
  { id: 'Overview', enabled: true },
  { id: 'Cascade', enabled: true },
  { id: 'Assets', enabled: true },
  { id: 'MCP & Plugins', enabled: true },
  { id: 'Permissions', enabled: true },
  { id: 'Settings', enabled: true },
  { id: 'Usage', enabled: true },
  { id: 'Health', enabled: true },
] as const

type Dialog = null | 'clone' | 'rename' | 'delete'

export function ProfileView({
  profile,
  names,
  onMutated,
  onSelect,
}: {
  profile: Profile
  names: string[]
  onMutated: () => void
  onSelect: (name: string) => void
}) {
  const [tab, setTab] = useState<string>('Overview')
  const [dialog, setDialog] = useState<Dialog>(null)
  const toast = useToast()

  function report(action: string, r: CmdResult) {
    if (r.ok) toast({ kind: 'success', title: `${action} succeeded`, desc: r.output.split('\n')[0] })
    else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
    onMutated()
  }

  async function launch() {
    const r = await api.mutate.launch(profile.name)
    if (r.ok) toast({ kind: 'info', title: 'Launching in Terminal', desc: r.output })
    else toast({ kind: 'error', title: 'Launch failed', desc: r.error || r.output })
  }

  async function openFolder() {
    const r = await api.mutate.openFolder(profile.name)
    if (!r.ok) toast({ kind: 'error', title: 'Could not open folder', desc: r.error })
  }

  return (
    <div className="flex h-full flex-col">
      <header className="shrink-0 px-6 pt-4">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold tracking-tight">{profile.name}</h1>
          <span
            className={cn(
              'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium',
              profile.authMethod ? 'bg-secondary/15 text-secondary' : 'bg-muted text-muted-foreground',
            )}
          >
            <KeyRound className="size-3" />
            {profile.authMethod || 'no auth'}
          </span>
          {profile.isDefault && (
            <span className="rounded-full bg-primary/15 px-2 py-0.5 text-[11px] font-medium text-primary">
              default
            </span>
          )}

          <div className="ml-auto flex items-center gap-1.5">
            <ActionButton primary icon={Play} label="Run" title="Launch Claude Code with this profile" onClick={launch} />
            <ActionButton icon={Copy} label="Clone" title="Clone this profile" onClick={() => setDialog('clone')} />
            <IconButton icon={FolderOpen} title="Open profile folder" onClick={openFolder} />
            <IconButton icon={Pencil} title="Rename profile" onClick={() => setDialog('rename')} />
            <IconButton icon={Trash2} title="Delete profile" danger onClick={() => setDialog('delete')} />
          </div>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">
          last used {timeAgo(profile.lastUsed)} · {tildePath(profile.dir)}
        </p>

        <nav className="mt-4 flex gap-0.5 border-b border-border">
          {TABS.map((t) => {
            const active = t.id === tab
            return (
              <button
                key={t.id}
                disabled={!t.enabled}
                onClick={() => t.enabled && setTab(t.id)}
                className={cn(
                  'relative -mb-px rounded-t-md px-3 py-1.5 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  active
                    ? 'font-medium text-foreground'
                    : t.enabled
                      ? 'cursor-pointer text-muted-foreground hover:text-foreground'
                      : 'cursor-default text-muted-foreground/40',
                )}
              >
                {t.id}
                {active && <span className="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-primary" />}
              </button>
            )
          })}
        </nav>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <ErrorBoundary resetKey={`${profile.name}:${tab}`}>
          {tab === 'Overview' && <OverviewTab profile={profile} />}
          {tab === 'Cascade' && <CascadeTab profile={profile.name} />}
          {tab === 'Assets' && <AssetsTab profile={profile.name} onMutated={onMutated} />}
          {tab === 'MCP & Plugins' && <McpPluginsTab profile={profile.name} onMutated={onMutated} />}
          {tab === 'Permissions' && <PermissionsTab profile={profile.name} onMutated={onMutated} />}
          {tab === 'Settings' && <SettingsTab profile={profile.name} onMutated={onMutated} />}
          {tab === 'Usage' && <UsageTab profile={profile.name} />}
          {tab === 'Health' && <HealthTab />}
        </ErrorBoundary>
      </div>

      {/* dialogs */}
      <PromptModal
        open={dialog === 'clone'}
        title={`Clone "${profile.name}"`}
        label="New profile name"
        placeholder="e.g. work-staging"
        confirmLabel="Clone"
        validate={(v) => validateProfileName(v, names)}
        onCancel={() => setDialog(null)}
        onConfirm={async (dst) => {
          setDialog(null)
          report('Clone', await api.mutate.clone(profile.name, dst))
          onSelect(dst)
        }}
      />
      <PromptModal
        open={dialog === 'rename'}
        title={`Rename "${profile.name}"`}
        label="New name"
        initial={profile.name}
        confirmLabel="Rename"
        validate={(v) => (v === profile.name ? 'Enter a different name' : validateProfileName(v, names))}
        onCancel={() => setDialog(null)}
        onConfirm={async (next) => {
          setDialog(null)
          report('Rename', await api.mutate.rename(profile.name, next))
          onSelect(next)
        }}
      />
      <ConfirmModal
        open={dialog === 'delete'}
        title={`Delete "${profile.name}"`}
        message="This removes the profile, its assets, and its keychain credentials. This cannot be undone."
        requireText={profile.name}
        confirmLabel="Delete profile"
        onCancel={() => setDialog(null)}
        onConfirm={async () => {
          setDialog(null)
          report('Delete', await api.mutate.remove(profile.name))
        }}
      />
    </div>
  )
}

function ActionButton({
  icon: Icon,
  label,
  title,
  primary,
  onClick,
}: {
  icon: LucideIcon
  label: string
  title: string
  primary?: boolean
  onClick: () => void
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      className={cn(
        'inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        primary
          ? 'bg-primary text-primary-foreground hover:bg-primary/90'
          : 'border border-border text-foreground hover:bg-accent',
      )}
    >
      <Icon className="size-3.5" />
      {label}
    </button>
  )
}

function IconButton({
  icon: Icon,
  title,
  danger,
  onClick,
}: {
  icon: LucideIcon
  title: string
  danger?: boolean
  onClick: () => void
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      className={cn(
        'inline-flex size-7 cursor-pointer items-center justify-center rounded-md border border-border text-muted-foreground transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        danger ? 'hover:bg-destructive/15 hover:text-destructive' : 'hover:bg-accent hover:text-foreground',
      )}
    >
      <Icon className="size-3.5" />
    </button>
  )
}
