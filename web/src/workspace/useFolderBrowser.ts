import { useCallback, useEffect, useRef, useState } from "react"
import { listDirs, type DirEntry } from "@/api/http"
import { ApiError, HTTP_STATUS } from "@/api/types"
import { loadMemory, parentPath, saveMemory, segments, withoutRecent, withRecent, type FolderMemory } from "./folders"

// Mockup 4b (docs/mockups/4b-folder-drill.html): drill-down folder navigator. The folder the breadcrumb ends in is
// the session's start folder.

type Listing =
  | { kind: "loading" }
  | { kind: "ready"; entries: DirEntry[] }
  | { kind: "denied"; message: string } // 403 permission_denied: still a valid start folder
  | { kind: "error"; message: string }
  | { kind: "gone" } // 404: the only state that blocks Open

type Drill = "in" | "up" | "none"

// Case-insensitive natural sort (v2 before v10).
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" })

const reason = (e: unknown) => (e instanceof Error ? e.message : String(e))

export type FolderBrowser = {
  path: string
  listing: Listing
  drill: Drill
  // The remembered folder that no longer exists when the dialog opened (its nearest parent is shown instead).
  notice: string | null
  // Why this device's folder memory cannot be read or written; the picker shows it.
  memoryError: string | null
  memory: FolderMemory
  filter: string
  setFilter: (filter: string) => void
  go: (path: string) => void
  up: () => void
  retry: () => void
  toggleHidden: () => void
  // Store the current folder (and the hidden-folder toggle) as this device's last used folder.
  // Returns the failure reason when storage fails (the caller keeps the dialog open so the notice is seen);
  // while memoryError is set the save is skipped, so the next call succeeds.
  remember: () => string | null
}

export function useFolderBrowser(open: boolean, onUnauthorized: () => void): FolderBrowser {
  const [path, setPath] = useState("")
  const [listing, setListing] = useState<Listing>({ kind: "loading" })
  const [drill, setDrill] = useState<Drill>("none")
  const [notice, setNotice] = useState<string | null>(null)
  const [memoryError, setMemoryError] = useState<string | null>(null)
  const [memory, setMemory] = useState<FolderMemory>({ recent: [], showHidden: false })
  const [filter, setFilter] = useState("")
  // Only the latest listing request may update the picker.
  const seq = useRef(0)

  // Lists `target`; with walkUp a missing folder is replaced by its nearest existing ancestor.
  // Resolves to the folder finally shown, or null when superseded by a newer request.
  const load = useCallback(
    async (target: string, nextDrill: Drill, walkUp: boolean): Promise<string | null> => {
      const id = ++seq.current
      setPath(target)
      setDrill(nextDrill)
      setFilter("")
      setListing({ kind: "loading" })
      let p = target
      for (;;) {
        try {
          const entries = await listDirs(p)
          if (id !== seq.current) return null
          setPath(p)
          setListing({ kind: "ready", entries: [...entries].sort((a, b) => collator.compare(a.name, b.name)) })
          return p
        } catch (e) {
          if (!(e instanceof ApiError)) throw e
          if (id !== seq.current) return null
          if (e.status === HTTP_STATUS.unauthorized) {
            onUnauthorized()
            return null
          }
          if (e.status === HTTP_STATUS.notFound && walkUp && p !== "") {
            p = parentPath(p)
            continue
          }
          setPath(p)
          if (e.status === HTTP_STATUS.notFound) setListing({ kind: "gone" })
          else if (e.status === HTTP_STATUS.forbidden && e.kind === "permission_denied") setListing({ kind: "denied", message: e.detail })
          else setListing({ kind: "error", message: e.message })
          return p
        }
      }
    },
    [onUnauthorized],
  )

  useEffect(() => {
    if (!open) return
    let remembered: FolderMemory = { recent: [], showHidden: false }
    try {
      remembered = loadMemory()
      setMemoryError(null)
    } catch (e) {
      setMemoryError(reason(e))
    }
    const start = remembered.recent[0] ?? ""
    setMemory(remembered)
    setNotice(null)
    void load(start, "none", true).then((shown) => {
      if (shown === null || shown === start) return
      setNotice(start)
      // The dead entry leaves Recent; storage is rewritten on the next Open.
      setMemory((m) => ({ ...m, recent: withoutRecent(m.recent, start) }))
    })
    return () => {
      seq.current++
    }
  }, [open, load])

  const go = (next: string) => {
    setNotice(null)
    void load(next, segments(next).length >= segments(path).length ? "in" : "up", false)
  }
  const remember = () => {
    if (memoryError !== null) return null
    try {
      saveMemory({ recent: withRecent(memory.recent, path), showHidden: memory.showHidden })
      return null
    } catch (e) {
      const message = reason(e)
      setMemoryError(message)
      return message
    }
  }
  return {
    path,
    listing,
    drill,
    notice,
    memoryError,
    memory,
    filter,
    setFilter,
    go,
    up: () => {
      setNotice(null)
      if (path !== "") void load(parentPath(path), "up", true)
    },
    retry: () => void load(path, "none", false),
    toggleHidden: () => setMemory((m) => ({ ...m, showHidden: !m.showHidden })),
    remember,
  }
}
