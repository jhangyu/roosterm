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
  return raw === null ? { recent: [], showHidden: false } : parseMemory(raw)
}

export function saveMemory(memory: FolderMemory): void {
  window.localStorage.setItem(STORE_KEY, JSON.stringify(memory))
}

export const withRecent = (recent: string[], path: string) => [path, ...recent.filter((p) => p !== path)].slice(0, RECENT_MAX)
export const withoutRecent = (recent: string[], path: string) => recent.filter((p) => p !== path)
