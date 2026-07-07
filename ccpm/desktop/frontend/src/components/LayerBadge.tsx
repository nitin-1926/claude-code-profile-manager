import { cn } from '@/lib/utils'
import type { Layer } from '@/types'

const LABEL: Record<Layer, string> = {
  host: 'HOST',
  global: 'GLOBAL',
  profile: 'PROFILE',
}

// Provenance colors: host = amber (cascaded from ~/.claude), global = teal
// (~/.ccpm/share), profile = neutral (profile-local).
const STYLE: Record<Layer, string> = {
  host: 'bg-primary/15 text-primary',
  global: 'bg-secondary/15 text-secondary',
  profile: 'bg-muted text-muted-foreground',
}

export function LayerBadge({ layer, className }: { layer: Layer; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded px-1.5 py-px text-[9px] font-semibold uppercase tracking-wide',
        STYLE[layer],
        className,
      )}
    >
      {LABEL[layer]}
    </span>
  )
}

export function layerNote(layer: Layer): string {
  return layer === 'host' ? 'cascaded' : layer === 'global' ? 'shared' : 'profile-local'
}
