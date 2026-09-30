import { useRef, useState } from 'react'
import type { CmdResult } from '@/types'
import { useToast } from '@/components/ui/Toast'

// useCommand runs one CLI write at a time and reports it as a toast. `after`
// runs once the write settles (typically reload + onMutated).
//
// The in-flight guard is a ref, not the `busy` state: two Enter presses in one
// tick both see busy === false, and that is how a tab used to fire two
// concurrent `ccpm` writes against the same settings file. A call made while
// another is running is dropped and resolves false.
//
// Resolves true only when the command succeeded, so callers clear a draft on
// success and keep it (for a retry) on failure.
export function useCommand(after?: () => void) {
  const [busy, setBusy] = useState(false)
  const running = useRef(false)
  const toast = useToast()

  async function run(action: string, call: () => Promise<CmdResult>): Promise<boolean> {
    if (running.current) return false
    running.current = true
    setBusy(true)
    try {
      const r = await call()
      if (r.ok) toast({ kind: 'success', title: `${action} succeeded`, desc: r.output.split('\n')[0] })
      else toast({ kind: 'error', title: `${action} failed`, desc: (r.error || r.output).split('\n')[0] })
      return r.ok
    } catch (e) {
      // A rejected Wails bridge call (backend down, Go panic) must not become
      // an unhandled rejection behind a button that silently did nothing.
      toast({ kind: 'error', title: `${action} failed`, desc: String(e) })
      return false
    } finally {
      running.current = false
      setBusy(false)
      after?.()
    }
  }

  return { busy, run }
}
