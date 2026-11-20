import { parseTabEnd, type TabEnd } from "./types"

// Wire constants owned by docs/api.md ("WebSocket").
const CLOSE_EXITED = 4000
const CLOSE_NOT_FOUND = 4004
const CLOSE_BAD_MESSAGE = 4400
const CLOSE_HC_ERROR = 4500
const CLOSE_GOING_AWAY = 1001
const resizeMessage = (cols: number, rows: number) => JSON.stringify({ type: "resize", cols, rows })

export type CoreState = "offline" | "online"

export type SocketEnd =
  | { kind: "exited"; end: TabEnd }
  | { kind: "not-found"; reason: string }
  | { kind: "server-error"; badMessage: boolean; detail: string }
  | { kind: "server-shutdown" }
  | { kind: "disconnected"; at: Date }
  | { kind: "protocol-error"; message: string }

type TabSocketHandlers = {
  onOpen: () => void
  onOutput: (data: Uint8Array) => void
  onCore: (state: CoreState) => void
  onServerError: (message: string) => void
  onEnd: (end: SocketEnd) => void
}

export type TabSocket = {
  sendInput: (data: string) => void
  sendResize: (cols: number, rows: number) => void
  close: () => void
}

const encoder = new TextEncoder()

type Control = { kind: "core"; state: CoreState } | { kind: "error"; message: string } | { kind: "invalid"; message: string }

function parseControl(tabId: string, text: string): Control {
  const invalid: Control = { kind: "invalid", message: `tab ${tabId}: unexpected text frame ${JSON.stringify(text.slice(0, 200))}` }
  let v: unknown
  try {
    v = JSON.parse(text)
  } catch {
    return invalid
  }
  if (typeof v !== "object" || v === null) return invalid
  const m = v as Record<string, unknown>
  if (m.type === "core" && (m.state === "offline" || m.state === "online")) return { kind: "core", state: m.state }
  if (m.type === "error" && typeof m.message === "string") {
    return { kind: "error", message: `${String(m.op)} ${String(m.tab)}: ${String(m.kind)}: ${m.message}` }
  }
  return invalid
}

function classifyClose(tabId: string, code: number, reason: string, lastError: string | null, at: Date): SocketEnd {
  switch (code) {
    case CLOSE_EXITED: {
      let parsed: unknown
      try {
        parsed = JSON.parse(reason)
      } catch {
        parsed = undefined
      }
      const end = parseTabEnd(parsed)
      if (end === undefined) {
        return {
          kind: "protocol-error",
          message: `tab ${tabId}: close ${CLOSE_EXITED} expected reason {"reason","exitCode","signal"}, got ${JSON.stringify(reason)}`,
        }
      }
      return { kind: "exited", end }
    }
    case CLOSE_NOT_FOUND:
      return { kind: "not-found", reason }
    case CLOSE_BAD_MESSAGE:
    case CLOSE_HC_ERROR:
      return { kind: "server-error", badMessage: code === CLOSE_BAD_MESSAGE, detail: lastError ?? reason }
    case CLOSE_GOING_AWAY:
      return { kind: "server-shutdown" }
    default:
      return { kind: "disconnected", at }
  }
}

export function openTabSocket(tabId: string, size: { cols: number; rows: number }, handlers: TabSocketHandlers): TabSocket {
  const scheme = location.protocol === "https:" ? "wss:" : "ws:"
  const query = new URLSearchParams({ cols: String(size.cols), rows: String(size.rows) })
  const ws = new WebSocket(`${scheme}//${location.host}/ws/tabs/${encodeURIComponent(tabId)}?${query}`)
  ws.binaryType = "arraybuffer"
  let ended = false
  let lastError: string | null = null
  const end = (e: SocketEnd) => {
    if (ended) return
    ended = true
    handlers.onEnd(e)
  }

  ws.onopen = () => handlers.onOpen()
  ws.onmessage = (ev) => {
    if (ev.data instanceof ArrayBuffer) {
      handlers.onOutput(new Uint8Array(ev.data))
      return
    }
    const c = parseControl(tabId, String(ev.data))
    switch (c.kind) {
      case "core":
        handlers.onCore(c.state)
        return
      case "error":
        lastError = c.message
        handlers.onServerError(c.message)
        return
      case "invalid":
        end({ kind: "protocol-error", message: c.message })
        ws.close()
        return
    }
  }
  ws.onclose = (ev) => end(classifyClose(tabId, ev.code, ev.reason, lastError, new Date()))

  const requireOpen = (what: string) => {
    if (ws.readyState !== WebSocket.OPEN) {
      throw new Error(`tab ${tabId}: ${what} expected socket state OPEN (1), got ${ws.readyState}`)
    }
  }
  return {
    sendInput: (data) => {
      requireOpen("send input")
      ws.send(encoder.encode(data))
    },
    sendResize: (cols, rows) => {
      requireOpen("send resize")
      ws.send(resizeMessage(cols, rows))
    },
    close: () => {
      ended = true // client-initiated: no onEnd
      ws.close()
    },
  }
}
