import { createContext, useCallback, useContext, useState } from 'react'
import { cn } from '@/lib/utils'
import { CheckCircle2, Info, XCircle } from 'lucide-react'

type Kind = 'success' | 'error' | 'info'
interface Toast {
  id: number
  title: string
  desc?: string
  kind: Kind
}

const ToastCtx = createContext<(t: Omit<Toast, 'id'>) => void>(() => {})
export const useToast = () => useContext(ToastCtx)

let counter = 0

// `dock` renders at the bottom of the same stack (the update prompt), so a
// toast can never cover it: two independently fixed boxes in one corner did.
export function ToastProvider({ children, dock }: { children: React.ReactNode; dock?: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const push = useCallback((t: Omit<Toast, 'id'>) => {
    const id = ++counter
    setToasts((s) => [...s, { ...t, id }])
    // Errors carry multi-line CLI output worth reading; give them longer.
    setTimeout(() => setToasts((s) => s.filter((x) => x.id !== id)), t.kind === 'error' ? 10000 : 4500)
  }, [])

  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
        <div className="flex flex-col gap-2" aria-live="polite">
          {toasts.map((t) => (
            <div
              key={t.id}
              className="flex items-start gap-2.5 rounded-lg border border-border bg-popover p-3 shadow-lg"
            >
              <Icon kind={t.kind} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">{t.title}</div>
                {t.desc && (
                  <div
                    className={cn(
                      'mt-0.5 font-mono text-[11px] text-muted-foreground',
                      // Success stays one compact line; an error wraps (and
                      // scrolls past a few lines) so the CLI's message is readable.
                      t.kind === 'error' ? 'max-h-40 overflow-y-auto whitespace-pre-wrap break-words' : 'truncate',
                    )}
                  >
                    {t.desc}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
        {dock}
      </div>
    </ToastCtx.Provider>
  )
}

function Icon({ kind }: { kind: Kind }) {
  const cls = cn(
    'size-4 shrink-0 mt-0.5',
    kind === 'success' && 'text-secondary',
    kind === 'error' && 'text-destructive',
    kind === 'info' && 'text-primary',
  )
  if (kind === 'success') return <CheckCircle2 className={cls} />
  if (kind === 'error') return <XCircle className={cls} />
  return <Info className={cls} />
}
