import type { Tab } from "@/api/types"

// tabs: the tab strip; hidden: sessions hidden on any client (listed in the New-session dialog for re-showing).
export type Workspace = { tabs: Tab[]; hidden: Tab[]; activeId: string | null }

// Apply a fresh server listing. The active tab stays active and in place even when another client removed or hid it:
// silently focusing a different session would put that session's Close button under the user's pointer.
// The stale entry goes away when the user closes it (the server answers 404, see removeTab's caller) or switches away
// and the next listing arrives.
export function applyListing(ws: Workspace, listed: Tab[]): Workspace {
  const hidden = listed.filter((t) => t.hidden)
  const shown = listed.filter((t) => !t.hidden)
  const at = ws.tabs.findIndex((t) => t.id === ws.activeId)
  if (at < 0) return { tabs: shown, hidden, activeId: shown[0]?.id ?? null }
  const active = ws.tabs[at]
  if (shown.some((t) => t.id === active.id)) return { tabs: shown, hidden, activeId: active.id }
  const pos = Math.min(at, shown.length)
  return { tabs: [...shown.slice(0, pos), active, ...shown.slice(pos)], hidden, activeId: active.id }
}

// Remove one tab by id; if it was active, focus its right neighbour, else its left one.
export function removeTab(ws: Workspace, id: string): Workspace {
  const at = ws.tabs.findIndex((t) => t.id === id)
  if (at < 0) return ws
  const tabs = ws.tabs.filter((t) => t.id !== id)
  if (ws.activeId !== id) return { ...ws, tabs }
  return { ...ws, tabs, activeId: (tabs[at] ?? tabs[at - 1])?.id ?? null }
}

// The server confirmed `tab` hidden: drop it from the strip like removeTab and list it as hidden.
export function hideTab(ws: Workspace, tab: Tab): Workspace {
  const next = removeTab(ws, tab.id)
  return { ...next, hidden: [...next.hidden.filter((t) => t.id !== tab.id), tab] }
}

// Put `tab` (new or re-shown) at the end of the strip, active, and out of the hidden list.
export function showTab(ws: Workspace, tab: Tab): Workspace {
  return {
    tabs: [...ws.tabs.filter((t) => t.id !== tab.id), tab],
    hidden: ws.hidden.filter((t) => t.id !== tab.id),
    activeId: tab.id,
  }
}
