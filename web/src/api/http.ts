import { ApiError, HTTP_STATUS, parseTabEnd, type CreateRequest, type Tab } from "./types"

const FOREIGN_BODY_MAX_CHARS = 200
const HTML_TITLE = /<title[^>]*>([\s\S]*?)<\/title>/i

// A body the backend did not write (e.g. a proxy's HTML error page): the page <title>, else the first line, cut short.
function summarizeForeignBody(text: string): string {
  const title = HTML_TITLE.exec(text)?.[1].trim()
  const line = title || (text.trim().split("\n")[0] ?? "").trim()
  return line.length > FOREIGN_BODY_MAX_CHARS ? `${line.slice(0, FOREIGN_BODY_MAX_CHARS)}…` : line
}

// Backend error body: {"error":{op,tab?,message,kind,hint?}}. Anything else (e.g. a proxy page) is summarized.
function errorFromBody(op: string, status: number, text: string, tabId: string | undefined, retryAfter: string | null): ApiError {
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    return new ApiError(op, status, null, summarizeForeignBody(text), tabId, retryAfter)
  }
  const e = typeof body === "object" && body !== null ? (body as Record<string, unknown>).error : undefined
  if (typeof e === "object" && e !== null) {
    const { op: serverOp, tab, message, kind, hint } = e as Record<string, unknown>
    if (typeof serverOp === "string" && typeof message === "string" && typeof kind === "string") {
      const detail = `${serverOp}: ${message}${typeof hint === "string" ? ` (hint: ${hint})` : ""}`
      return new ApiError(op, status, kind, detail, typeof tab === "string" ? tab : tabId, retryAfter)
    }
  }
  return new ApiError(op, status, null, summarizeForeignBody(text), tabId, retryAfter)
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
    throw new ApiError(op, HTTP_STATUS.networkFailure, null, `network error: ${e instanceof Error ? e.message : String(e)}`, tabId)
  }
  if (!res.ok) throw errorFromBody(op, res.status, await res.text(), tabId, res.headers.get("Retry-After"))
  return res
}

const TAB_SHAPE =
  "{id: string, label: string, exited: boolean, exitCode: int|null, signal: int|null, reason: string (when exited), hidden: boolean, title: string, program: string, renamed: boolean, cwd: string}"

function parseTab(op: string, status: number, v: unknown): Tab {
  if (typeof v === "object" && v !== null) {
    const { id, label, exited, hidden, title, program, renamed, cwd } = v as Record<string, unknown>
    if (
      typeof id === "string" &&
      typeof label === "string" &&
      typeof exited === "boolean" &&
      typeof hidden === "boolean" &&
      typeof title === "string" &&
      typeof program === "string" &&
      typeof renamed === "boolean" &&
      typeof cwd === "string"
    ) {
      if (!exited) return { id, label, hidden, end: null, title, program, renamed, cwd }
      const end = parseTabEnd(v)
      if (end !== undefined) return { id, label, hidden, end, title, program, renamed, cwd }
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

// cwd: folder relative to the host home ("" = home); the host validates it like GET /api/dirs.
export async function createTab(request: CreateRequest): Promise<Tab> {
  const op = "create tab"
  const res = await call(op, "POST", "/api/tabs", request)
  return parseTab(op, res.status, await json(op, res))
}

async function patchTab(op: string, id: string, body: { label: string } | { hidden: boolean }): Promise<Tab> {
  const res = await call(op, "PATCH", tabPath(id), body, id)
  return parseTab(op, res.status, await json(op, res))
}

export const renameTab = (id: string, label: string): Promise<Tab> => patchTab("rename tab", id, { label })

// Hide or re-show a tab (PATCH /api/tabs/{id} {"hidden": bool}); the process is untouched.
export const setTabHidden = (id: string, hidden: boolean): Promise<Tab> => patchTab(hidden ? "hide tab" : "show tab", id, { hidden })

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

// Subfolders of a folder under the host home (GET /api/dirs?path=<rel>, "" = home).
export type DirEntry = { name: string; hidden: boolean }

export async function listDirs(path: string): Promise<DirEntry[]> {
  const op = "list folders"
  const res = await call(op, "GET", `/api/dirs?path=${encodeURIComponent(path)}`)
  const v = await json(op, res)
  const entries = typeof v === "object" && v !== null ? (v as Record<string, unknown>).entries : undefined
  if (!Array.isArray(entries) || !entries.every((e) => typeof e?.name === "string" && typeof e?.hidden === "boolean")) {
    throw new ApiError(op, res.status, null, `expected {path: string, entries: {name: string, hidden: boolean}[]}, got ${JSON.stringify(v)}`)
  }
  return entries.map((e: DirEntry) => ({ name: e.name, hidden: e.hidden }))
}
