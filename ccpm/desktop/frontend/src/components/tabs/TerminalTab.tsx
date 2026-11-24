import { useCallback, useEffect, useRef, useState } from 'react'
import { Terminal, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Plus, SquareTerminal, X } from 'lucide-react'
import { api } from '@/lib/api'
import { tildePath } from '@/lib/format'
import { useTheme, type Theme } from '@/lib/theme'
import { useGuarded } from '@/lib/useGuarded'
import { cn } from '@/lib/utils'
import { ConfirmModal } from '@/components/ui/Modal'
import type { TerminalChunk, TerminalSession } from '@/types'

// Hex, not the oklch theme tokens: xterm parses colors itself and only
// understands hex/rgb. Converted from globals.css; keep in step with it.
const SELECTION_DARK = '#e78a5359'
const XTERM_THEMES: Record<Theme, ITheme> = {
  graphite: {
    background: '#201e1d',
    foreground: '#e5e2e0',
    cursor: '#e78a53',
    cursorAccent: '#201e1d',
    selectionBackground: SELECTION_DARK,
  },
  midnight: {
    background: '#121113',
    foreground: '#c1c1c1',
    cursor: '#e78a53',
    cursorAccent: '#121113',
    selectionBackground: SELECTION_DARK,
  },
  light: {
    background: '#fdfcf9',
    foreground: '#191f2b',
    cursor: '#b54f05',
    cursorAccent: '#fdfcf9',
    selectionBackground: '#b54f0533',
    // xterm's default ANSI palette assumes a dark background; these would
    // otherwise be near-invisible on the light one.
    black: '#191f2b',
    white: '#6b7280',
    brightBlack: '#6b7280',
    brightWhite: '#374151',
    yellow: '#946200',
    brightYellow: '#7a5200',
  },
}

const FONT = '"JetBrains Mono", monospace'

/** Base64 to bytes. xterm decodes UTF-8 across writes, so a multibyte
 *  character split between two chunks still renders whole. */
function decode(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

/** The profile's embedded terminals. Sessions live in Go and survive this tab
 *  unmounting (a profile switch remounts every tab); closing one here is the
 *  only thing that ends it, short of quitting the app. */
export function TerminalTab({ profile, focus }: { profile: string; focus: string | null }) {
  const [sessions, setSessions] = useState<TerminalSession[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [active, setActive] = useState<string | null>(focus)
  const [confirming, setConfirming] = useState<TerminalSession | null>(null)
  // Disables "New terminal" while Start is pending; a double click otherwise
  // started two sessions.
  const [starting, setStarting] = useState(false)
  const guard = useGuarded('Terminal')

  useEffect(() => {
    if (focus) setActive(focus)
  }, [focus])

  // Drops a response from a superseded list call, so an older list can never
  // land after a newer one.
  const gen = useRef(0)
  const load = useCallback(() => {
    const mine = ++gen.current
    api.terminal
      .list()
      .then((all) => {
        if (gen.current !== mine) return
        setSessions(all.filter((s) => s.profile === profile))
        setError(null)
      })
      .catch((e) => {
        if (gen.current === mine) setError(String(e))
      })
  }, [profile])

  useEffect(() => {
    load()
    return api.terminal.onChanged(load)
  }, [load])

  const startNew = guard(async () => {
    if (starting) return
    setStarting(true)
    try {
      const s = await api.terminal.start(profile, '', [])
      setActive(s.id)
    } finally {
      setStarting(false)
    }
  })

  const close = async (s: TerminalSession) => {
    setSessions((prev) => prev?.filter((x) => x.id !== s.id) ?? prev)
    await api.terminal.close(s.id)
  }

  const current = sessions?.find((s) => s.id === active) ?? sessions?.at(-1) ?? null

  if (error) return <div className="px-6 py-5 text-sm text-destructive">{error}</div>
  if (!sessions) return <div className="px-6 py-5 text-sm text-muted-foreground">Loading terminals…</div>

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border px-4 py-1.5">
        {sessions.map((s) => (
          <div
            key={s.id}
            className={cn(
              'group flex shrink-0 items-center rounded-md text-xs transition-colors',
              s.id === current?.id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent/50',
            )}
          >
            <button
              type="button"
              onClick={() => setActive(s.id)}
              title={`ccpm run ${s.profile} in ${s.cwd}`}
              className="flex cursor-pointer items-center gap-1.5 rounded-md py-1 pl-2 pr-1 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <span
                className={cn('size-1.5 rounded-full', s.running ? 'bg-secondary' : 'bg-muted-foreground/40')}
                aria-label={s.running ? 'running' : 'exited'}
              />
              {s.title}
            </button>
            <button
              type="button"
              title="Close terminal"
              aria-label={`Close terminal ${s.title}`}
              onClick={guard(() => (s.running ? setConfirming(s) : close(s)))}
              className="mr-0.5 inline-flex size-5 cursor-pointer items-center justify-center rounded opacity-60 hover:bg-accent hover:opacity-100 focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="size-3" />
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={startNew}
          disabled={starting}
          title={`Start claude with the ${profile} profile`}
          className="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
        >
          <Plus className="size-3.5" />
          New terminal
        </button>
      </div>

      {current ? (
        <>
          <div className="flex shrink-0 items-center gap-2 px-4 py-1 text-[11px] text-muted-foreground">
            <span className="shrink-0 whitespace-nowrap font-mono">ccpm run {current.profile}</span>
            <span className="min-w-0 truncate">{tildePath(current.cwd)}</span>
            {!current.running && (
              <span className="ml-auto shrink-0">exited with code {current.exitCode}</span>
            )}
          </div>
          <div className="min-h-0 flex-1 px-2 pb-2">
            <TerminalView key={current.id} id={current.id} />
          </div>
        </>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <SquareTerminal className="size-8 text-muted-foreground/60" />
          <p className="text-sm text-muted-foreground">No terminals for {profile} yet.</p>
          <button
            type="button"
            onClick={startNew}
            disabled={starting}
            className="inline-flex cursor-pointer items-center gap-1.5 rounded-md bg-primary px-2.5 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
          >
            <Plus className="size-3.5" />
            New terminal
          </button>
        </div>
      )}

      <ConfirmModal
        open={confirming !== null}
        title="End this session?"
        message="Claude Code is still running in this terminal. Closing it stops the process."
        confirmLabel="Close terminal"
        onCancel={() => setConfirming(null)}
        onConfirm={async () => {
          const s = confirming
          setConfirming(null)
          if (s) await close(s)
        }}
      />
    </div>
  )
}

/** One xterm bound to one Go session. Unmounting disposes the view only. */
function TerminalView({ id }: { id: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const { theme } = useTheme()
  // Read at mount; later changes go through the effect below, so a theme
  // switch restyles the terminal instead of rebuilding it.
  const themeRef = useRef(theme)
  themeRef.current = theme

  useEffect(() => {
    let disposed = false
    let teardown = () => {}
    // xterm measures the cell size once, at open. Opening before the font has
    // loaded sizes every cell for the fallback font.
    document.fonts
      .load(`12px ${FONT}`)
      .catch(() => undefined)
      .then(() => {
        if (!disposed && ref.current) teardown = mount(ref.current)
      })

    function mount(el: HTMLDivElement): () => void {
      const term = new Terminal({
        fontFamily: FONT,
        fontSize: 12,
        scrollback: 10000,
        cursorBlink: true,
        theme: XTERM_THEMES[themeRef.current],
      })
      const fit = new FitAddon()
      term.loadAddon(fit)
      term.open(el)
      termRef.current = term

      // Live chunks are held until the scrollback has been written. From then
      // on writtenSeq is the byte count on screen, and a chunk at or below it
      // (already in the scrollback, or delivered twice) is dropped.
      let writtenSeq: number | null = null
      let pending: TerminalChunk[] = []
      let earlyExit: number | null = null
      let exited = false
      const showExit = (code: number) => {
        if (exited) return
        exited = true
        term.write(`\r\n\x1b[2m[process exited with code ${code}]\x1b[0m\r\n`)
      }

      const writeChunk = (c: TerminalChunk) => {
        if (writtenSeq === null || c.seq <= writtenSeq) return
        term.write(decode(c.data))
        writtenSeq = c.seq
      }
      const offData = api.terminal.onData(id, (c) => {
        if (writtenSeq === null) pending.push(c)
        else writeChunk(c)
      })
      const offExit = api.terminal.onExit(id, (code) => {
        if (writtenSeq === null) earlyExit = code
        else showExit(code)
      })
      const input = term.onData((d) => {
        if (!exited) api.terminal.write(id, d).catch(() => undefined)
      })
      // Shift+Enter and Option+Enter insert a newline in Claude Code instead of
      // submitting: send ESC CR, the sequence its /terminal-setup binds. xterm
      // would send a bare CR for both. Returning false skips xterm for every
      // phase of the key (keydown, keypress, keyup); only keydown sends.
      term.attachCustomKeyEventHandler((e) => {
        if (e.key !== 'Enter' || !(e.shiftKey || e.altKey) || e.metaKey || e.ctrlKey) return true
        if (e.type === 'keydown') {
          e.preventDefault()
          if (!exited) api.terminal.write(id, '\x1b\r').catch(() => undefined)
        }
        return false
      })
      const resize = term.onResize(({ cols, rows }) => {
        api.terminal.resize(id, cols, rows).catch(() => undefined)
      })
      const refit = () => {
        if (el.clientWidth > 0 && el.clientHeight > 0) fit.fit()
      }
      const observer = new ResizeObserver(refit)
      observer.observe(el)
      refit()

      api.terminal
        .attach(id)
        .then((a) => {
          if (disposed) return
          term.write(decode(a.data))
          writtenSeq = a.seq
          pending.forEach(writeChunk)
          pending = []
          if (!a.session.running) showExit(a.session.exitCode)
          else if (earlyExit !== null) showExit(earlyExit)
          // onResize fires only on a change; the PTY may still be at its
          // starting 80x24 or at another view's size.
          api.terminal.resize(id, term.cols, term.rows).catch(() => undefined)
          term.focus()
        })
        .catch((e) => {
          if (!disposed) term.write(`\r\n[could not attach: ${String(e)}]\r\n`)
        })

      return () => {
        offData()
        offExit()
        input.dispose()
        resize.dispose()
        observer.disconnect()
        term.dispose()
        termRef.current = null
      }
    }

    return () => {
      disposed = true
      teardown()
    }
  }, [id])

  useEffect(() => {
    if (termRef.current) termRef.current.options.theme = XTERM_THEMES[theme]
  }, [theme])

  return (
    <div
      className="h-full w-full overflow-hidden rounded-md p-2"
      style={{ background: XTERM_THEMES[theme].background }}
    >
      <div ref={ref} className="h-full w-full" />
    </div>
  )
}
