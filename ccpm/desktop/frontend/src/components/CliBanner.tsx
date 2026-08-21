import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import { AlertTriangle, ArrowUpRight, X } from 'lucide-react'

const INSTALL_URL = 'https://github.com/nitin-1926/claude-code-profile-manager#install'

// CliBanner surfaces a single upfront warning when the ccpm CLI is missing from
// PATH — the desktop app shells out to it for every write action, so without it
// those actions fail one-by-one. Driven by the existing health.doctor() signal
// (HealthResult.available); dismissible for the session.
export function CliBanner() {
  const [missing, setMissing] = useState(false)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    // A failed/again-unavailable check should never surface as an error itself.
    api.health
      .doctor()
      .then((r) => setMissing(!r.available))
      .catch(() => setMissing(true))
  }, [])

  if (!missing || dismissed) return null

  return (
    <div
      className="flex items-center gap-2.5 border-b border-primary/40 bg-primary/10 px-4 py-2 text-xs text-foreground"
      role="alert"
    >
      <AlertTriangle className="size-3.5 shrink-0 text-primary" />
      <span className="min-w-0 flex-1">
        The <span className="font-mono">ccpm</span> CLI wasn&apos;t found on your PATH — write actions will fail until
        it&apos;s installed.
      </span>
      <button
        onClick={() => api.openURL(INSTALL_URL)}
        className="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-md px-2 py-1 font-medium text-primary transition-colors hover:bg-primary/15"
      >
        Install instructions
        <ArrowUpRight className="size-3" />
      </button>
      <button
        onClick={() => setDismissed(true)}
        title="Dismiss"
        className="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
