import { useEffect, useRef, useState } from 'react'
import { api } from './lib/api'
import type { Profile } from './types'
import { Sidebar } from './components/Sidebar'
import { ProfileView } from './components/ProfileView'
import { EmptyState } from './components/EmptyState'
import { TitleBar } from './components/TitleBar'
import { CliBanner } from './components/CliBanner'
import { UpdateToast } from './components/UpdateToast'
import { RefreshCw } from 'lucide-react'
import { cn } from './lib/utils'

type LoadState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; profiles: Profile[] }

export default function App() {
  const [state, setState] = useState<LoadState>({ status: 'loading' })
  const [selected, setSelected] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  // Bumped only by the title-bar button: it resets a tab's error boundary, so
  // the "hit refresh" advice on the error card actually does something.
  const [refreshCount, setRefreshCount] = useState(0)
  const refreshSeq = useRef(0)

  // Only the latest refresh applies: the watcher and a mutation can overlap,
  // and an older list resolving last would drop a just-cloned profile and
  // reset the selection to profiles[0].
  async function refresh() {
    const seq = ++refreshSeq.current
    setRefreshing(true)
    try {
      const profiles = await api.profiles.list()
      if (seq !== refreshSeq.current) return
      setState({ status: 'ready', profiles })
      setSelected((cur) => {
        if (cur && profiles.some((p) => p.name === cur)) return cur
        return profiles[0]?.name ?? null
      })
    } catch (e) {
      if (seq === refreshSeq.current) setState({ status: 'error', message: String(e) })
    } finally {
      if (seq === refreshSeq.current) setRefreshing(false)
    }
  }

  useEffect(() => {
    void refresh()
    // live-refresh when the CLI / Claude Code change profile state underneath us
    const off = api.onChanged(() => void refresh())
    return off
  }, [])

  const refreshButton = (
    <button
      onClick={() => {
        setRefreshCount((n) => n + 1)
        void refresh()
      }}
      title="Refresh"
      className="flex size-7 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <RefreshCw className={cn('size-3.5', refreshing && 'animate-spin')} />
    </button>
  )

  return (
    <div className="flex h-full w-full flex-col overflow-hidden">
      <TitleBar right={state.status === 'ready' && state.profiles.length > 0 ? refreshButton : undefined} />
      <CliBanner />
      <div className="flex min-h-0 flex-1">{renderBody()}</div>
      <UpdateToast />
    </div>
  )

  function renderBody() {
    if (state.status === 'loading') {
      return <CenteredNote>Loading profiles…</CenteredNote>
    }
    if (state.status === 'error') {
      return (
        <CenteredNote>
          <span className="text-destructive">Failed to read ~/.ccpm</span>
          <span className="mt-2 block max-w-md text-xs text-muted-foreground">{state.message}</span>
        </CenteredNote>
      )
    }
    if (state.profiles.length === 0) {
      return <EmptyState />
    }
    const active = state.profiles.find((p) => p.name === selected) ?? state.profiles[0]
    const names = state.profiles.map((p) => p.name)
    return (
      <>
        <Sidebar
          profiles={state.profiles}
          selected={active.name}
          onSelect={setSelected}
          names={names}
          onMutated={() => void refresh()}
        />
        <main className="flex min-w-0 flex-1 flex-col bg-background">
          <ProfileView
            profile={active}
            names={names}
            onMutated={refresh}
            onSelect={setSelected}
            refreshKey={refreshCount}
          />
        </main>
      </>
    )
  }
}

function CenteredNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full w-full items-center justify-center bg-background text-sm text-muted-foreground">
      <div className="text-center">{children}</div>
    </div>
  )
}
