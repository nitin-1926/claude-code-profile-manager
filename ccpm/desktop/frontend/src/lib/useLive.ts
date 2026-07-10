import { useCallback, useEffect, useState } from 'react'
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

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const load = useCallback(() => {
    fetcher()
      .then((d) => {
        setData(d)
        setError(null)
      })
      // Don't swallow failures silently — log and expose so consumers can show
      // an error instead of an indefinite "Loading…".
      .catch((e) => {
        console.error('useLive: fetch failed', e)
        setError(String(e))
      })
  }, deps)

  useEffect(() => {
    load()
    const off = api.onChanged(load)
    return off
  }, [load])

  return [data, load, error]
}
