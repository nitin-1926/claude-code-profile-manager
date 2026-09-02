import { cn } from '@/lib/utils'

// A proper iOS-style switch: track + a clearly-contrasting knob.
export function Switch({
  on,
  disabled,
  onClick,
  label,
}: {
  on: boolean
  disabled?: boolean
  onClick: () => void
  label?: string
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
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
