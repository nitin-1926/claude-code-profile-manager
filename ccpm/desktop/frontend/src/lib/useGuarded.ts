import { useToast } from '@/components/ui/Toast'

// useGuarded wraps a handler that may be async: a rejected Wails bridge call
// (backend down, Go panic) would otherwise become an unhandled rejection and
// the button would silently do nothing.
//
// Lives here rather than inside Modal.tsx so surfaces that are not modals can
// use it without importing an overlay component — the alternative is each one
// re-inlining the try/catch, which is how the bug this prevents comes back.
export function useGuarded(action: string) {
  const toast = useToast()
  return (fn: () => void) => () => {
    try {
      const r = fn() as unknown
      if (r instanceof Promise) {
        r.catch((e: unknown) =>
          toast({ kind: 'error', title: `${action} failed`, desc: String(e) }),
        )
      }
    } catch (e) {
      toast({ kind: 'error', title: `${action} failed`, desc: String(e) })
    }
  }
}
