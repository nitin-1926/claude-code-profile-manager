import { useCallback, useEffect, useState } from 'react'
import { api } from './api'

// Fetches data and re-fetches when deps change OR the Go watcher fires a change
// event. Returns [data, reload]. reload() lets a tab refresh right after its own
// mutation without waiting for the debounced watcher.
export function useLive<T>(fetcher: () => Promise<T>, deps: unknown[]): [T | null, () => void] {
  const [data, setData] = useState<T | null>(null)

  // eslint-disable-next-line react-hooks/exhaustive-deps
  const load = useCallback(() => {
    fetcher()
      .then(setData)
      .catch(() => {})
  }, deps)

  useEffect(() => {
    load()
    const off = api.onChanged(load)
    return off
  }, [load])

  return [data, load]
}
