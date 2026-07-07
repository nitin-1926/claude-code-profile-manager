import { Component, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'

interface Props {
  children: ReactNode
  /** Changing this resets the boundary (e.g. the active tab id). */
  resetKey?: string
}
interface State {
  error: Error | null
}

// Catches render errors in a tab so a single broken view shows an inline error
// card instead of blanking the whole window.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidUpdate(prev: Props) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-full items-center justify-center p-8">
          <div className="max-w-md rounded-xl border border-destructive/40 bg-card p-5 text-center">
            <AlertTriangle className="mx-auto mb-3 size-6 text-destructive" />
            <div className="text-sm font-medium">This view hit an error</div>
            <p className="mt-1 font-mono text-[11px] leading-relaxed text-muted-foreground">
              {this.state.error.message}
            </p>
            <p className="mt-3 text-xs text-muted-foreground">
              Switch tabs and back, or hit refresh. The rest of the app is fine.
            </p>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
