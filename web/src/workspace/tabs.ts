import type { ClaudePane, CreateRequest, Tab } from "@/api/types"
import { baseName } from "./folders"

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

// Claude Code runs `claude` in the chosen folder (contract: client sends command ["claude"]; host reports program "claude").
const CLAUDE_PROGRAM = "claude"
export const CLAUDE_COMMAND = [CLAUDE_PROGRAM]

// ponytail: the API's Tab has no command, so a swarm tab is recognised by its socket-name label (as created by the New-session dialog);
// a renamed swarm tab is treated as a shell. Add a command field to Tab if that matters.
export const isSwarm = (tab: Tab) => tab.label.startsWith("claude-swarm-")

// Swarm tab request (docs/api.md: POST /api/tabs); the host labels it with the socket name.
export const swarmTab = (name: string): CreateRequest => ({ swarm: name })

// Existing-Claude tab request: the label "<folder> <socket>:<pane id>" is how restartRequest recognises it again
// (ponytail: a renamed tab is treated as a shell, like swarm tabs).
export const existingTab = (pane: ClaudePane): CreateRequest => ({
  pane: { socket: pane.socket, id: pane.id },
  label: `${baseName(pane.cwd)} ${pane.socket}:${pane.id}`,
})
const EXISTING_LABEL = /^.* (\S+):(%\d+)$/

// Same kind, folder and label as the given tab: swarm => re-attach to its socket; existing Claude => re-attach to its pane; Claude Code => `claude` in the same
// folder; shell => server default shell in the same folder. The caller restores `renamed` (create cannot set it).
const TMUX_PROGRAM = "tmux"
function existingRestart(tab: Tab): CreateRequest {
  const m = EXISTING_LABEL.exec(tab.label)
  if (m === null) throw new Error(`existing Claude tab: expected label "<folder> <socket>:%<n>", got ${JSON.stringify(tab.label)}`)
  return { pane: { socket: m[1], id: m[2] }, label: tab.label }
}

export const restartRequest = (tab: Tab): CreateRequest =>
  isSwarm(tab)
    ? swarmTab(tab.label)
    : tab.program === TMUX_PROGRAM && EXISTING_LABEL.test(tab.label)
      ? existingRestart(tab)
      : tab.program === CLAUDE_PROGRAM
      ? { command: CLAUDE_COMMAND, cwd: tab.cwd, label: tab.label }
      : { cwd: tab.cwd, label: tab.label }

// Tab label split for display: the folder part never truncates before the summary does.
// Only a not-renamed Claude Code tab with a non-empty title gets a summary (shells set titles too, so they stay folder-only).
export function tabParts(tab: Tab): { folder: string; summary: string } {
  const summary = !tab.renamed && tab.program === CLAUDE_PROGRAM ? tab.title : ""
  return { folder: tab.label, summary }
}

export const tabText = (tab: Tab): string => {
  const { folder, summary } = tabParts(tab)
  return summary === "" ? folder : `${folder} · ${summary}`
}

// Replace one tab by id with change(tab).
export const updateTab = (ws: Workspace, id: string, change: (t: Tab) => Tab): Workspace => ({
  ...ws,
  tabs: ws.tabs.map((t) => (t.id === id ? change(t) : t)),
})

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
