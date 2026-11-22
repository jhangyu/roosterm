export type TabEnd = { reason: string; exitCode: number | null; signal: number | null }
// hidden: dropped from tab strips on every client, the process keeps running (re-shown from the New-session dialog).
// title: the program's own terminal title (Claude Code writes its session summary); program: basename of argv[0];
// renamed: the user set the label, so the title is never appended to it.
// cwd: start folder relative to the host home ("" = home); absolute when outside home (a tab not created by roosterm).
export type Tab = { id: string; label: string; hidden: boolean; end: TabEnd | null; title: string; program: string; renamed: boolean; cwd: string }

const isIntOrNull = (v: unknown): v is number | null => v === null || Number.isInteger(v)

// Shared by GET/POST/PATCH tab bodies and the WebSocket 4000 close reason.
export function parseTabEnd(v: unknown): TabEnd | undefined {
  if (typeof v !== "object" || v === null) return undefined
  const { reason, exitCode, signal } = v as Record<string, unknown>
  if (typeof reason !== "string" || !isIntOrNull(exitCode) || !isIntOrNull(signal)) return undefined
  return { reason, exitCode, signal }
}

// cwd: start folder relative to the host home ("" = home).
// pane: a Claude Code pane on a non-swarm tmux socket (GET /api/claude-panes).
export type ClaudePane = { socket: string; session: string; window: number; id: string; cwd: string }
export type CreateRequest = { swarm: string } | { pane: { socket: string; id: string }; label: string } | { command?: string[]; label?: string; cwd?: string }

// ApiError.status values the client reacts to (0 = the request never reached the host).
export const HTTP_STATUS = {
  networkFailure: 0,
  unauthorized: 401,
  forbidden: 403,
  notFound: 404,
  tooManyRequests: 429,
  badGateway: 502,
  serviceUnavailable: 503,
  gatewayTimeout: 504,
} as const

// The host cannot be reached: the request failed, or a reverse proxy in front answered for the dead host.
const HOST_UNREACHABLE_STATUSES: readonly number[] = [
  HTTP_STATUS.networkFailure,
  HTTP_STATUS.badGateway,
  HTTP_STATUS.serviceUnavailable,
  HTTP_STATUS.gatewayTimeout,
]
export const isHostUnreachable = (status: number): boolean => HOST_UNREACHABLE_STATUSES.includes(status)

export class ApiError extends Error {
  readonly op: string
  readonly status: number
  readonly kind: string | null
  readonly detail: string
  readonly retryAfter: string | null

  constructor(op: string, status: number, kind: string | null, detail: string, tabId?: string, retryAfter: string | null = null) {
    super(`${op}${tabId === undefined ? "" : ` (tab ${tabId})`}: HTTP ${status}${kind === null ? "" : ` ${kind}`}: ${detail}`)
    this.op = op
    this.status = status
    this.kind = kind
    this.detail = detail
    this.retryAfter = retryAfter
  }
}
