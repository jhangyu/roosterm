import { useCallback, useEffect, useRef, useState } from "react"
import { closeTab, createTab, listTabs, logout, renameTab, setTabHidden } from "@/api/http"
import { ApiError, HTTP_STATUS, type CreateRequest, type Tab, type TabEnd } from "@/api/types"
import { AddIcon, ArrowClockwiseIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { onResume } from "@/lib/resume"
import { LoginScreen } from "@/screens/LoginScreen"
import { TerminalPane } from "@/terminal/TerminalPane"
import { BTN_PRIMARY } from "@/workspace/classes"
import { ErrorAlert } from "@/workspace/ErrorAlert"
import { NewSessionDialog } from "@/workspace/NewSessionDialog"
import { CloseConfirm, RenameDialog } from "@/workspace/SessionDialogs"
import { MainHead, PhoneBar, SessionRail } from "@/workspace/Sessions"
import { applyListing, hideTab, removeTab, restartRequest, showTab, withTitle, type Workspace } from "@/workspace/tabs"

type Auth = "checking" | "anonymous" | "signed-in"

const isGone = (e: unknown) => e instanceof ApiError && e.status === HTTP_STATUS.notFound

export default function App() {
  const [auth, setAuth] = useState<Auth>("checking")
  const [{ tabs, hidden, activeId }, setWs] = useState<Workspace>({ tabs: [], hidden: [], activeId: null })
  const [error, setError] = useState<ApiError | null>(null)
  // True from a terminal socket loss until the next successful API call: the pane's "Connection lost"
  // banner is then the one signal for an unreachable host, so network failures are not repeated as alerts.
  const linkDown = useRef(false)
  const [newOpen, setNewOpen] = useState(false)
  const [renaming, setRenaming] = useState<Tab | null>(null)
  const [confirming, setConfirming] = useState<Tab | null>(null)

  const signOut = useCallback(() => {
    setAuth("anonymous")
    setNewOpen(false)
    setRenaming(null)
    setConfirming(null)
  }, [])

  const run = useCallback(
    async (action: () => Promise<void>) => {
      try {
        await action()
        setError(null)
        linkDown.current = false
      } catch (e) {
        if (!(e instanceof ApiError)) throw e
        if (e.status === HTTP_STATUS.unauthorized) signOut()
        else if (!(e.status === HTTP_STATUS.networkFailure && linkDown.current)) setError(e)
      }
    },
    [signOut],
  )

  const refresh = useCallback(
    () =>
      run(async () => {
        const list = await listTabs()
        setWs((ws) => applyListing(ws, list))
        setAuth("signed-in")
      }),
    [run],
  )

  useEffect(() => {
    void refresh()
  }, [refresh])
  useEffect(() => onResume(() => void refresh()), [refresh])

  const create = (request: CreateRequest) =>
    void run(async () => {
      const tab = await createTab(request)
      setWs((ws) => showTab(ws, tab))
      setNewOpen(false)
    })

  const rename = (id: string, label: string) =>
    void run(async () => {
      const updated = await renameTab(id, label)
      setWs((ws) => ({ ...ws, tabs: ws.tabs.map((t) => (t.id === id ? updated : t)) }))
      setRenaming(null)
    })

  // Close exactly this id. 404 = another client already removed it: the goal state holds, drop the stale entry.
  const closeById = async (id: string) => {
    try {
      await closeTab(id)
    } catch (e) {
      if (!isGone(e)) throw e
    }
    setWs((ws) => removeTab(ws, id))
  }

  const close = (id: string) =>
    void run(async () => {
      await closeById(id)
      setConfirming(null)
    })

  // Hide keeps the process running; 404 = another client already removed it: drop the stale entry, as closeById does.
  const hide = (tab: Tab) =>
    void run(async () => {
      try {
        const updated = await setTabHidden(tab.id, true)
        setWs((ws) => hideTab(ws, updated))
      } catch (e) {
        if (!isGone(e)) throw e
        setWs((ws) => removeTab(ws, tab.id))
      }
    })

  const show = (tab: Tab) =>
    void run(async () => {
      const updated = await setTabHidden(tab.id, false)
      setWs((ws) => showTab(ws, updated))
      setNewOpen(false)
    })

  // Exited-panel Restart: open a session of the same kind and label, switch to it, then remove the exited one.
  const restart = (old: Tab) =>
    void run(async () => {
      const created = await createTab(restartRequest(old))
      // Create cannot set `renamed`; re-applying the label does, so the title is not appended to a custom label.
      const tab = old.renamed ? await renameTab(created.id, old.label) : created
      setWs((ws) => showTab(ws, tab))
      await closeById(old.id)
    })

  const select = (id: string) => setWs((ws) => ({ ...ws, activeId: id }))

  const socketLost = () => {
    linkDown.current = true
    void refresh()
  }

  // Closing a running session asks first; an exited one closes directly.
  const requestClose = (tab: Tab) => (tab.end === null ? setConfirming(tab) : close(tab.id))

  const signOutOfHost = () =>
    void run(async () => {
      await logout()
      signOut()
    })

  const markExited = useCallback((id: string, end: TabEnd) => {
    setWs((ws) => ({ ...ws, tabs: ws.tabs.map((t) => (t.id === id ? { ...t, end } : t)) }))
  }, [])

  if (auth === "anonymous") return <LoginScreen onSignedIn={() => void refresh()} />

  if (auth === "checking") {
    return (
      <div className="grid h-full place-items-center bg-(image:--mica) p-4 text-muted-foreground">
        {error === null ? (
          <p>Loading…</p>
        ) : (
          <div className="grid w-full max-w-[560px] justify-items-center gap-3.5">
            <ErrorAlert error={error} onDismiss={() => setError(null)} className="w-full" />
            <Button className={BTN_PRIMARY} onClick={() => void refresh()}>
              <ArrowClockwiseIcon />
              Retry
            </Button>
          </div>
        )}
      </div>
    )
  }

  const active = tabs.find((t) => t.id === activeId) ?? null
  const openNew = () => setNewOpen(true)
  return (
    // Mockup 3 screen 02 `.desk`: Mica page, 288px Acrylic rail + main column; phone: one column (screen 04).
    <div className="grid h-full grid-cols-1 grid-rows-[minmax(0,1fr)] bg-(image:--mica) text-foreground md:grid-cols-[288px_1fr]">
      <SessionRail tabs={tabs} activeId={activeId} onSelect={select} onNew={openNew} onHide={hide} onLogout={signOutOfHost} />
      <main className="relative z-10 flex min-h-0 min-w-0 flex-col md:px-7 md:pt-6 md:pb-7">
        <PhoneBar
          tabs={tabs}
          activeId={activeId}
          onSelect={select}
          onNew={openNew}
          onHide={hide}
          onLogout={signOutOfHost}
          onRename={setRenaming}
          onClose={requestClose}
        />
        {active !== null && <MainHead active={active} onRename={setRenaming} onClose={requestClose} />}
        {error !== null && <ErrorAlert error={error} onDismiss={() => setError(null)} className="mx-2.5 mb-3.5 w-auto md:mx-0" />}
        <div className="flex min-h-0 flex-1 flex-col px-2.5 md:px-0">
          {active !== null ? (
            <TerminalPane key={active.id} tab={active} onExited={markExited} onClose={close} onSocketLost={socketLost} onRestart={() => restart(active)} onTitle={(id, title) => setWs((ws) => withTitle(ws, id, title))} />
          ) : (
            // Not in mockup 3: empty state in the focused-card position.
            <div className="mb-2.5 grid flex-1 place-items-center rounded-lg bg-card shadow-8 md:mb-0">
              <div className="grid justify-items-center gap-1 p-6 text-center">
                <h2 className="text-subtitle1 font-semibold">No sessions</h2>
                <p className="mb-[18px] text-muted-foreground">Start a shell or attach to a swarm on this host.</p>
                <Button className={BTN_PRIMARY} onClick={openNew}>
                  <AddIcon />
                  New session
                </Button>
              </div>
            </div>
          )}
        </div>
      </main>
      <NewSessionDialog
        open={newOpen}
        onOpenChange={setNewOpen}
        onCreate={create}
        hidden={hidden}
        onShow={show}
        onUnauthorized={signOut}
      />
      <RenameDialog tab={renaming} onCancel={() => setRenaming(null)} onRename={rename} />
      <CloseConfirm tab={confirming} onCancel={() => setConfirming(null)} onConfirm={close} />
    </div>
  )
}
