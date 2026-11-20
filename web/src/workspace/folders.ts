import type { DirEntry } from "@/api/http"

// Folder paths are relative to the host home ("" = home), as GET /api/dirs and POST /api/tabs take them.

export const segments = (path: string) => (path === "" ? [] : path.split("/"))
export const childPath = (path: string, name: string) => (path === "" ? name : `${path}/${name}`)
export const parentPath = (path: string) => segments(path).slice(0, -1).join("/")
export const displayPath = (path: string) => (path === "" ? "~" : `~/${path}`)
export const baseName = (path: string) => (path === "" ? "~" : path.slice(path.lastIndexOf("/") + 1))

// Mockup 4b: Recent holds at most 5 folders; the first one is the folder the dialog opens in.
const RECENT_MAX = 5
const STORE_KEY = "roosterm.folderPicker"

export type FolderMemory = { recent: string[]; showHidden: boolean }

// Shared safely: every update builds a new object or array.
export const EMPTY_MEMORY: FolderMemory = { recent: [], showHidden: false }

function parseMemory(raw: string): FolderMemory {
  const v: unknown = JSON.parse(raw)
  if (typeof v === "object" && v !== null) {
    const { recent, showHidden } = v as Record<string, unknown>
    if (Array.isArray(recent) && recent.every((p) => typeof p === "string") && typeof showHidden === "boolean") {
      return { recent: recent.slice(0, RECENT_MAX), showHidden }
    }
  }
  throw new Error(`${STORE_KEY}: expected {recent: string[], showHidden: boolean}, got ${raw}`)
}

// Per-device memory. Storage can be unavailable (private mode, quota) or hold another build's shape: both throw.
export function loadMemory(): FolderMemory {
  const raw = window.localStorage.getItem(STORE_KEY)
  return raw === null ? EMPTY_MEMORY : parseMemory(raw)
}

export function saveMemory(memory: FolderMemory): void {
  window.localStorage.setItem(STORE_KEY, JSON.stringify(memory))
}

export const withRecent = (recent: string[], path: string) => [path, ...recent.filter((p) => p !== path)].slice(0, RECENT_MAX)
export const withoutRecent = (recent: string[], path: string) => recent.filter((p) => p !== path)

export type MatchRange = { start: number; end: number }

const REGEXP_SPECIALS = /[.*+?^${}()|[\]\\/]/g // ES2023 lib: no RegExp.escape

// Case-insensitive match on the original name, so the range indexes the name the user sees.
export function matchRange(name: string, filter: string): MatchRange | null {
  const m = new RegExp(filter.replace(REGEXP_SPECIALS, "\\$&"), "iu").exec(name)
  return m === null ? null : { start: m.index, end: m.index + m[0].length }
}

export type FolderView = { hiddenEntries: DirEntry[]; autoHidden: boolean; showHidden: boolean; pool: DirEntry[]; matches: DirEntry[]; hiddenMatches: number }

// Dot folders show when asked for, when the filter starts with ".", or inside a dot folder. Prefix matches first, then inner matches.
export function folderView(entries: DirEntry[], path: string, filter: string, showHiddenPref: boolean): FolderView {
  const hiddenEntries = entries.filter((e) => e.hidden)
  const autoHidden = filter.startsWith(".") || segments(path).some((s) => s.startsWith("."))
  const showHidden = showHiddenPref || autoHidden
  const pool = showHidden ? entries : entries.filter((e) => !e.hidden)
  if (filter === "") return { hiddenEntries, autoHidden, showHidden, pool, matches: pool, hiddenMatches: 0 }
  const start = (e: DirEntry) => matchRange(e.name, filter)?.start
  const prefix = pool.filter((e) => start(e) === 0)
  const inner = pool.filter((e) => (start(e) ?? 0) > 0)
  const hiddenMatches = showHidden ? 0 : hiddenEntries.filter((e) => start(e) !== undefined).length
  return { hiddenEntries, autoHidden, showHidden, pool, matches: [...prefix, ...inner], hiddenMatches }
}
