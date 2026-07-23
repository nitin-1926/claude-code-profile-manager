import { ThemeToggle } from './ThemeToggle'

// Full-width draggable chrome strip. The macOS traffic lights overlay its
// left edge (Wails can't reposition them — issue #4227), so this strip stays
// EMPTY on the left: nothing sits beside the lights, so nothing can look
// misaligned with them. Brand lives in the sidebar below; global actions sit
// on the right, far from the lights.
export function TitleBar({ right }: { right?: React.ReactNode }) {
  return (
    <header
      className="flex h-10 shrink-0 items-center border-b border-border bg-sidebar pr-3 pl-20"
      style={{ '--wails-draggable': 'drag' } as React.CSSProperties}
    >
      <div
        className="ml-auto flex items-center gap-1.5"
        style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}
      >
        {right}
        <ThemeToggle />
      </div>
    </header>
  )
}
