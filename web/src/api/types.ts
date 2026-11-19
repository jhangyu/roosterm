export type TabEnd = { reason: string; exitCode: number | null; signal: number | null }
export type Tab = { id: string; label: string; end: TabEnd | null }

const isIntOrNull = (v: unknown): v is number | null => v === null || Number.isInteger(v)

// Shared by GET/POST/PATCH tab bodies and the WebSocket 4000 close reason.
export function parseTabEnd(v: unknown): TabEnd | undefined {
  if (typeof v !== "object" || v === null) return undefined
  const { reason, exitCode, signal } = v as Record<string, unknown>
  if (typeof reason !== "string" || !isIntOrNull(exitCode) || !isIntOrNull(signal)) return undefined
  return { reason, exitCode, signal }
}

export class ApiError extends Error {
  readonly op: string
  readonly status: number
  readonly kind: string | null
  readonly detail: string
  readonly tabId: string | undefined
  readonly retryAfter: string | null

  constructor(op: string, status: number, kind: string | null, detail: string, tabId?: string, retryAfter: string | null = null) {
    super(`${op}${tabId === undefined ? "" : ` (tab ${tabId})`}: HTTP ${status}${kind === null ? "" : ` ${kind}`}: ${detail}`)
    this.op = op
    this.status = status
    this.kind = kind
    this.detail = detail
    this.tabId = tabId
    this.retryAfter = retryAfter
  }
}
