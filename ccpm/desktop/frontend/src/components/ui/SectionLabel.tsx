import type { ReactNode } from 'react'

/** Small uppercase heading above a tab section. */
export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
      {children}
    </h2>
  )
}
