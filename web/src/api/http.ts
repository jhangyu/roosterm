import { ApiError, parseTabEnd, type Tab } from "./types"

// Backend error body: {"error":{op,tab?,message,kind,hint?}}. Anything else (e.g. a proxy page) is shown raw.
function errorFromBody(op: string, status: number, text: string, tabId: string | undefined, retryAfter: string | null): ApiError {
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    return new ApiError(op, status, null, text, tabId, retryAfter)
  }
  const e = typeof body === "object" && body !== null ? (body as Record<string, unknown>).error : undefined
  if (typeof e === "object" && e !== null) {
    const { op: serverOp, tab, message, kind, hint } = e as Record<string, unknown>
    if (typeof serverOp === "string" && typeof message === "string" && typeof kind === "string") {
      const detail = `${serverOp}: ${message}${typeof hint === "string" ? ` (hint: ${hint})` : ""}`
      return new ApiError(op, status, kind, detail, typeof tab === "string" ? tab : tabId, retryAfter)
    }
  }
  return new ApiError(op, status, null, text, tabId, retryAfter)
}

async function call(op: string, method: string, path: string, body?: unknown, tabId?: string): Promise<Response> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    throw new ApiError(op, 0, null, `network error: ${e instanceof Error ? e.message : String(e)}`, tabId)
  }
  if (!res.ok) throw errorFromBody(op, res.status, await res.text(), tabId, res.headers.get("Retry-After"))
  return res
}

const TAB_SHAPE =
  "{id: string, label: string, exited: boolean, exitCode: int|null, signal: int|null, reason: string (when exited), hidden: boolean}"

function parseTab(op: string, status: number, v: unknown): Tab {
  if (typeof v === "object" && v !== null) {
    const { id, label, exited, hidden } = v as Record<string, unknown>
    if (typeof id === "string" && typeof label === "string" && typeof exited === "boolean" && typeof hidden === "boolean") {
      if (!exited) return { id, label, hidden, end: null }
      const end = parseTabEnd(v)
      if (end !== undefined) return { id, label, hidden, end }
    }
  }
  throw new ApiError(op, status, null, `expected ${TAB_SHAPE}, got ${JSON.stringify(v)}`)
}

async function json(op: string, res: Response): Promise<unknown> {
  const text = await res.text()
  try {
    return JSON.parse(text)
  } catch {
    throw new ApiError(op, res.status, null, `expected JSON body, got ${JSON.stringify(text.slice(0, 200))}`)
  }
}

const tabPath = (id: string) => `/api/tabs/${encodeURIComponent(id)}`

export async function login(username: string, password: string): Promise<void> {
  await call("login", "POST", "/login", { username, password })
}

export async function logout(): Promise<void> {
  await call("logout", "POST", "/logout")
}

export async function listTabs(): Promise<Tab[]> {
  const op = "list tabs"
  const res = await call(op, "GET", "/api/tabs")
  const v = await json(op, res)
  if (!Array.isArray(v)) throw new ApiError(op, res.status, null, `expected array of ${TAB_SHAPE}, got ${JSON.stringify(v)}`)
  return v.map((t) => parseTab(op, res.status, t))
}

export async function createTab(request: { command?: string[]; label?: string }): Promise<Tab> {
  const op = "create tab"
  const res = await call(op, "POST", "/api/tabs", request)
  return parseTab(op, res.status, await json(op, res))
}

export async function renameTab(id: string, label: string): Promise<Tab> {
  const op = "rename tab"
  const res = await call(op, "PATCH", tabPath(id), { label }, id)
  return parseTab(op, res.status, await json(op, res))
}

// Hide or re-show a tab (PATCH /api/tabs/{id} {"hidden": bool}); the process is untouched.
export async function setTabHidden(id: string, hidden: boolean): Promise<Tab> {
  const op = hidden ? "hide tab" : "show tab"
  const res = await call(op, "PATCH", tabPath(id), { hidden }, id)
  return parseTab(op, res.status, await json(op, res))
}

export async function closeTab(id: string): Promise<void> {
  await call("close tab", "DELETE", tabPath(id), undefined, id)
}

export async function listSwarms(): Promise<string[]> {
  const op = "list swarms"
  const res = await call(op, "GET", "/api/swarms")
  const v = await json(op, res)
  if (!Array.isArray(v) || !v.every((s) => typeof s === "string")) {
    throw new ApiError(op, res.status, null, `expected string[], got ${JSON.stringify(v)}`)
  }
  return v
}
