import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import type { Tab, TabEnd } from "@/api/types"
import { openTabSocket, type CoreState, type SocketEnd, type TabSocket } from "@/api/tab-socket"
import { keySequence, type SpecialKey } from "@/lib/keys"
import { onResume } from "@/lib/resume"
import { readTerminalLook } from "@/lib/terminal-theme"
import { cn } from "@/lib/utils"
import { KeyBar } from "./KeyBar"
import { DESKTOP_QUERY, readPhoneMetrics } from "./metrics"
import { ExitPanel, StatusBanner } from "./StatusBanner"
import { StatusFooter, type FooterInfo } from "./StatusFooter"
import { canReconnect, type PaneState } from "./pane-state"

const SCROLLBACK_LINES = 5000
const CTRL_LETTER_OFFSET = 64 // "C" (67) - 64 = 0x03 (^C)
const LETTER = /^[a-zA-Z]$/

export type TerminalPaneProps = {
  tab: Tab
  onExited: (tabId: string, end: TabEnd) => void
  onClose: (tabId: string) => void
  onSocketLost: () => void
  onRestart?: () => void
}

export function TerminalPane({ tab, onExited, onClose, onSocketLost, onRestart }: TerminalPaneProps) {
  const hostRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const socketRef = useRef<TabSocket | null>(null)
  const initial: PaneState =
    tab.end === null ? { kind: "connecting" } : { kind: "ended", end: { kind: "exited", end: tab.end } }
  const [state, setState] = useState<PaneState>(initial)
  const [ctrlLatched, setCtrlLatched] = useState(false)
  const [footer, setFooter] = useState<FooterInfo>({ cols: 0, rows: 0, scrollbackLines: 0, lastOutput: null, endedAt: null })
  const lastOutputRef = useRef<Date | null>(null)
  // Socket callbacks and the resume listener outlive renders; they read the latest state/props via refs.
  const stateRef = useRef<PaneState>(initial)
  const ctrlRef = useRef(false)
  const propsRef = useRef({ onExited, onSocketLost })
  useLayoutEffect(() => {
    propsRef.current = { onExited, onSocketLost }
  })

  const setCtrl = (latched: boolean) => {
    ctrlRef.current = latched
    setCtrlLatched(latched)
  }

  const setPane = (s: PaneState) => {
    stateRef.current = s
    setState(s)
    if (s.kind !== "live") setCtrl(false)
    if (s.kind === "core-offline" || s.kind === "ended") {
      const now = new Date()
      setFooter((f) => ({ ...f, lastOutput: lastOutputRef.current, endedAt: s.kind === "ended" && s.end.kind === "exited" ? now : f.endedAt }))
    }
    const term = termRef.current
    if (term !== null) term.options.disableStdin = s.kind !== "live"
  }

  const handleEnd = (end: SocketEnd) => {
    socketRef.current = null
    setPane({ kind: "ended", end })
    switch (end.kind) {
      case "exited":
        propsRef.current.onExited(tab.id, end.end)
        return
      case "disconnected":
      case "not-found":
      case "server-shutdown":
        propsRef.current.onSocketLost()
        return
      case "server-error":
      case "protocol-error":
        return
    }
  }

  const handleCore = (core: CoreState) => {
    setPane(core === "offline" ? { kind: "core-offline", notice: null } : { kind: "live" })
  }

  // Shown only while the core is offline; otherwise the socket attaches it to the 4400/4500 close that follows.
  const handleServerError = (message: string) => {
    if (stateRef.current.kind === "core-offline") setPane({ kind: "core-offline", notice: message })
  }

  const connect = () => {
    const term = termRef.current
    if (term === null) throw new Error(`tab ${tab.id}: connect expected a created terminal, got none`)
    socketRef.current?.close()
    setPane({ kind: "connecting" })
    socketRef.current = openTabSocket(tab.id, { cols: term.cols, rows: term.rows }, {
      onOpen: () => {
        setPane({ kind: "live" })
        term.focus()
      },
      onOutput: (data) => {
        lastOutputRef.current = new Date()
        term.write(data)
      },
      onCore: handleCore,
      onServerError: handleServerError,
      onEnd: handleEnd,
    })
  }

  // Mount once per tab (parent uses key={tab.id}). connect/handleEnd read only refs and tab.id,
  // so the first-render closures stay correct for the component's lifetime.
  useEffect(() => {
    const host = hostRef.current
    if (host === null) throw new Error(`tab ${tab.id}: expected terminal host element, got none`)
    const look = readTerminalLook()
    const phone = readPhoneMetrics()
    const desktop = window.matchMedia(DESKTOP_QUERY)
    const metrics = () => (desktop.matches ? look : phone)
    const term = new Terminal({
      ...look,
      fontSize: metrics().fontSize,
      lineHeight: metrics().lineHeight,
      cursorBlink: true,
      scrollback: SCROLLBACK_LINES,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()
    termRef.current = term
    const syncSize = () => setFooter((f) => ({ ...f, cols: term.cols, rows: term.rows }))
    syncSize()
    const applyMetrics = () => {
      term.options.fontSize = metrics().fontSize
      term.options.lineHeight = metrics().lineHeight
      fit.fit()
    }
    desktop.addEventListener("change", applyMetrics)
    const written = term.onWriteParsed(() =>
      setFooter((f) => (f.scrollbackLines === term.buffer.active.baseY ? f : { ...f, scrollbackLines: term.buffer.active.baseY })),
    )
    term.options.disableStdin = stateRef.current.kind !== "live"
    const data = term.onData((d) => {
      if (stateRef.current.kind !== "live") return
      const socket = socketRef.current
      if (socket === null) throw new Error(`tab ${tab.id}: input expected a live socket, got none`)
      // Latched Ctrl turns the next letter into its control byte, whether it came from the key bar or the keyboard.
      const latched = ctrlRef.current
      if (latched) setCtrl(false)
      socket.sendInput(latched && LETTER.test(d) ? String.fromCharCode(d.toUpperCase().charCodeAt(0) - CTRL_LETTER_OFFSET) : d)
    })
    // Not live ⇒ skip; the next attach carries the current size in its URL, so nothing is lost.
    const resize = term.onResize(({ cols, rows }) => {
      syncSize()
      if (stateRef.current.kind === "live") socketRef.current?.sendResize(cols, rows)
    })
    const observer = new ResizeObserver(() => fit.fit())
    observer.observe(host)
    if (stateRef.current.kind === "connecting") connect()
    const stopResume = onResume(() => {
      if (canReconnect(stateRef.current)) connect()
    })
    return () => {
      stopResume()
      observer.disconnect()
      data.dispose()
      resize.dispose()
      written.dispose()
      desktop.removeEventListener("change", applyMetrics)
      socketRef.current?.close()
      socketRef.current = null
      term.dispose()
      termRef.current = null
    }
  }, [])

  const input = (text: string) => {
    const term = termRef.current
    if (term === null) throw new Error(`tab ${tab.id}: key input expected a terminal, got none`)
    term.input(text, true)
  }
  const sendKey = (key: SpecialKey) => {
    const term = termRef.current
    if (term === null) throw new Error(`tab ${tab.id}: key ${key} expected a terminal, got none`)
    input(keySequence(key, term.modes.applicationCursorKeysMode))
  }

  // Mockup 3 `.term-card.dim`: frozen, greyed terminal while detached (not while connecting or when exited).
  const dimmed = state.kind === "core-offline" || (state.kind === "ended" && state.end.kind !== "exited")

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <StatusBanner state={state} onReconnect={connect} />
      <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg bg-[var(--term-background)] shadow-8">
        <div
          className={cn(
            "min-h-0 flex-1 overflow-hidden p-3.5 md:px-7 md:py-[22px]",
            dimmed && "opacity-45 grayscale",
          )}
        >
          <div ref={hostRef} className="h-full w-full" />
        </div>
        {dimmed && <div className="pointer-events-none absolute inset-0 bg-[color:var(--grey-98)]/25" />}
        <StatusFooter state={state} info={footer} />
        {state.kind === "ended" && state.end.kind === "exited" && (
          <ExitPanel end={state.end.end} onClose={() => onClose(tab.id)} onRestart={onRestart} />
        )}
      </div>
      <KeyBar
        disabled={state.kind !== "live"}
        ctrlLatched={ctrlLatched}
        onToggleCtrl={() => setCtrl(!ctrlRef.current)}
        onKey={sendKey}
        onLetter={input}
      />
    </div>
  )
}
