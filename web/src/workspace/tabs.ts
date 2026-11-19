import type { Tab } from "@/api/types"

export type Workspace = { tabs: Tab[]; activeId: string | null }

// Apply a fresh server listing. The active tab stays active and in place even when another client removed it:
// silently focusing a different session would put that session's Close button under the user's pointer.
// The stale entry goes away when the user closes it (the server answers 404, see removeTab's caller).
export function applyListing(ws: Workspace, listed: Tab[]): Workspace {
  const at = ws.tabs.findIndex((t) => t.id === ws.activeId)
  if (at < 0) return { tabs: listed, activeId: listed[0]?.id ?? null }
  const active = ws.tabs[at]
  if (listed.some((t) => t.id === active.id)) return { tabs: listed, activeId: active.id }
  const pos = Math.min(at, listed.length)
  return { tabs: [...listed.slice(0, pos), active, ...listed.slice(pos)], activeId: active.id }
}

// Remove one tab by id; if it was active, focus its right neighbour, else its left one.
export function removeTab(ws: Workspace, id: string): Workspace {
  const at = ws.tabs.findIndex((t) => t.id === id)
  if (at < 0) return ws
  const tabs = ws.tabs.filter((t) => t.id !== id)
  if (ws.activeId !== id) return { tabs, activeId: ws.activeId }
  return { tabs, activeId: (tabs[at] ?? tabs[at - 1])?.id ?? null }
}
