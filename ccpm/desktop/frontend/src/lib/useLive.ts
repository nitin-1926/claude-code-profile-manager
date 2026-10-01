import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from './api'

// Fetches data and re-fetches when deps change OR the Go watcher fires a change
// event. Returns [data, reload]. reload() lets a tab refresh right after its own
// mutation without waiting for the debounced watcher.
export function useLive<T>(
  fetcher: () => Promise<T>,
  deps: unknown[],
): [T | null, () => void, string | null] {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Only the latest fetch may apply. A watcher tick and a reload() after a
  // mutation overlap within one profile, and without this whichever resolves
  // last wins, so an older snapshot can overwrite a newer one.
  const seq = useRef(0)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const load = useCallback(() => {
    const n = ++seq.current
    fetcher()
      .then((d) => {
        if (n !== seq.current) return
        setData(d)
        setError(null)
      })
      // Don't swallow failures silently — log and expose so consumers can show
      // an error instead of an indefinite "Loading…".
      .catch((e) => {
        console.error('useLive: fetch failed', e)
        if (n === seq.current) setError(String(e))
      })
  }, deps)

  useEffect(() => {
    // Clear on a deps change (i.e. a profile switch) before the new fetch
    // resolves. Without this the previous profile's rows stay on screen while
    // the action buttons already target the newly-selected one.
    setData(null)
    setError(null)
    load()
    const off = api.onChanged(load)
    return off
  }, [load])

  return [data, load, error]
}
