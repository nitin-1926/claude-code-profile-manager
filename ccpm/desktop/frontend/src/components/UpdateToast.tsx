import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import type { UpdateInfo, UpdateProgress } from '@/types'
import { cn } from '@/lib/utils'
import { ArrowUpRight, Download, Loader2, Sparkles, X } from 'lucide-react'

const PHASE_LABEL: Record<string, string> = {
  downloading: 'Downloading update…',
  extracting: 'Preparing…',
  installing: 'Installing…',
  relaunching: 'Relaunching…',
}

// UpdateToast checks GitHub for a newer desktop build a few seconds after launch
// and, if one exists, offers a one-click in-place update (bottom-right). The Go
// updater downloads + verifies + swaps the bundle and relaunches — no re-drag.
export function UpdateToast() {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [dismissed, setDismissed] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [progress, setProgress] = useState<UpdateProgress | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    // Delay so the check never competes with first paint. Network/API failures
    // are swallowed — a missed update check should never surface as an error.
    const t = setTimeout(() => {
      api.updater
        .check()
        .then((u) => {
          if (u.available) setInfo(u)
        })
        .catch(() => {})
    }, 2500)
    const off = api.onUpdateProgress((p) => setProgress(p))
    return () => {
      clearTimeout(t)
      off()
    }
  }, [])

  if (!info || !info.available || dismissed) return null

  async function install() {
    setInstalling(true)
    setError(null)
    try {
      // Resolves after the swap helper is spawned; the app then quits + relaunches.
      await api.updater.install()
    } catch (e) {
      setError(String(e))
      setInstalling(false)
    }
  }

  return (
    <div className="fixed bottom-4 right-4 z-50 w-80" aria-live="polite">
      <div className="overflow-hidden rounded-xl border border-primary/40 bg-popover shadow-xl">
        <div className="flex items-start gap-2.5 p-3.5">
          <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <Sparkles className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">Update available</span>
              {!installing && (
                <button
                  onClick={() => setDismissed(true)}
                  title="Later"
                  className="ml-auto flex size-5 cursor-pointer items-center justify-center rounded text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <X className="size-3.5" />
                </button>
              )}
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">
              <span className="font-mono">v{info.current}</span>
              <span className="mx-1">→</span>
              <span className="font-mono text-foreground">v{info.latest}</span>
            </div>

            {error && <div className="mt-2 text-xs text-destructive">{error}</div>}

            {installing ? (
              <div className="mt-3">
                <div className="mb-1.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                  <Loader2 className="size-3.5 animate-spin" />
                  {PHASE_LABEL[progress?.phase ?? 'downloading'] ?? 'Working…'}
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-primary transition-all duration-200"
                    style={{ width: `${progress?.percent ?? 5}%` }}
                  />
                </div>
              </div>
            ) : (
              <div className="mt-3 flex items-center gap-2">
                <button
                  onClick={() => void install()}
                  className="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <Download className="size-3.5" />
                  Update now
                </button>
                {info.url && (
                  <button
                    onClick={() => api.openURL(info.url)}
                    className="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
                  >
                    What&apos;s new
                    <ArrowUpRight className="size-3" />
                  </button>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
