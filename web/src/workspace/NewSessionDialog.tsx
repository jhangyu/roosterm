import { useEffect, useState, type KeyboardEvent, type ReactNode } from "react"
import { RadioGroup as RadioGroupPrimitive } from "radix-ui"
import { listClaudePanes, listSwarms, type ClaudePanes } from "@/api/http"
import { ApiError, HTTP_STATUS, type ClaudePane, type CreateRequest, type Tab } from "@/api/types"
import { BotIcon, CheckmarkIcon, DismissIcon, EyeIcon, PeopleTeamIcon, PlugConnectedIcon, WindowConsoleIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { BTN, BTN_PRIMARY, ICON_BTN, KBD } from "@/components/classes"
import { FolderPicker, RecentFolders, StartsIn } from "./FolderPicker"
import { baseName } from "./folders"
import { CLAUDE_COMMAND, existingTab, swarmTab } from "./tabs"
import { useFolderBrowser } from "./useFolderBrowser"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (request: CreateRequest) => void
  hidden: Tab[]
  onShow: (tab: Tab) => void
  onUnauthorized: () => void
}

type Kind = "shell" | "claude" | "swarm" | "existing"
type Swarms = { kind: "loading" } | { kind: "ready"; names: string[] } | { kind: "error"; message: string }

type Existing = { kind: "loading" } | ({ kind: "ready" } & ClaudePanes) | { kind: "error"; message: string }

const KINDS: { value: Kind; icon: ReactNode; title: string; hint: string }[] = [
  { value: "claude", icon: <BotIcon />, title: "Claude Code", hint: "Runs claude in the folder below" },
  { value: "existing", icon: <PlugConnectedIcon />, title: "Existing Claude", hint: "Attach to Claude Code already running in tmux" },
  { value: "swarm", icon: <PeopleTeamIcon />, title: "Swarm", hint: "Attach to a running swarm socket" },
  { value: "shell", icon: <WindowConsoleIcon />, title: "Shell", hint: "Login shell in the folder below" },
]
const paneKey = (p: ClaudePane) => `${p.socket}:${p.id}`
const isKind = (v: string): v is Kind => KINDS.some((k) => k.value === v)

function Choice(props: { value: Kind; icon: ReactNode; title: string; hint: string }) {
  const id = `session-kind-${props.value}`
  return (
    // Mockup 4b .choice.v / .choice.v.selected
    <Label
      htmlFor={id}
      className={`group grid cursor-pointer grid-cols-[1fr_20px] grid-rows-[auto_auto] items-start gap-3 rounded-md border border-grey-88 bg-card p-3.5 text-left text-body leading-(--lh-body) font-normal transition-colors duration-150 hover:border-grey-74 has-[[data-state=checked]]:border-2 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-accent has-[[data-state=checked]]:p-[13px]`}
    >
      <span className="grid size-8 place-items-center rounded-sm bg-grey-94 text-grey-26 group-has-[[data-state=checked]]:bg-card group-has-[[data-state=checked]]:text-primary">
        {props.icon}
      </span>
      <RadioGroupItem id={id} value={props.value} />
      <span className="col-span-2">
        <b className="block font-semibold">{props.title}</b>
        <small className="text-caption text-muted-foreground">{props.hint}</small>
      </span>
    </Label>
  )
}

// Mockup 4b: desktop dialog / phone bottom sheet; Shell and Claude Code start in the picker's folder, Swarm attaches to a socket.
export function NewSessionDialog({ open, onOpenChange, onCreate, hidden, onShow, onUnauthorized }: Props) {
  const [kind, setKind] = useState<Kind>("claude")
  const [swarms, setSwarms] = useState<Swarms>({ kind: "loading" })
  const [picked, setPicked] = useState<string | null>(null)
  const [existing, setExisting] = useState<Existing>({ kind: "loading" })
  const [pickedPane, setPickedPane] = useState<string | null>(null)
  const folders = useFolderBrowser(open, onUnauthorized)

  useEffect(() => {
    if (!open) return
    let current = true
    setKind("claude")
    setSwarms({ kind: "loading" })
    setPicked(null)
    setExisting({ kind: "loading" })
    setPickedPane(null)
    listClaudePanes().then(
      (found) => {
        if (!current) return
        setExisting({ kind: "ready", ...found })
        setPickedPane(found.panes[0] ? paneKey(found.panes[0]) : null)
      },
      (e: unknown) => {
        if (!(e instanceof ApiError)) throw e
        if (e.status === HTTP_STATUS.unauthorized) onUnauthorized()
        else if (current) setExisting({ kind: "error", message: e.message })
      },
    )
    listSwarms().then(
      (names) => {
        if (!current) return
        setSwarms({ kind: "ready", names })
        setPicked(names[0] ?? null)
      },
      (e: unknown) => {
        if (!(e instanceof ApiError)) throw e
        if (e.status === HTTP_STATUS.unauthorized) onUnauthorized()
        else if (current) setSwarms({ kind: "error", message: e.message })
      },
    )
    return () => {
      current = false
    }
  }, [open, onUnauthorized])

  const pane = existing.kind === "ready" ? existing.panes.find((p) => paneKey(p) === pickedPane) : undefined
  const listsSession = kind === "swarm" || kind === "existing"
  const canOpen = kind === "swarm" ? picked !== null : kind === "existing" ? pane !== undefined : folders.listing.kind !== "gone"
  const submit = () => {
    if (!canOpen) return
    if (kind === "existing") {
      if (pane !== undefined) onCreate(existingTab(pane))
      return
    }
    if (kind === "swarm") {
      if (picked !== null) onCreate(swarmTab(picked))
      return
    }
    // A failed save keeps the dialog open: App closes it on create, which would hide the notice.
    if (folders.remember() !== null) return
    onCreate(kind === "claude" ? { command: CLAUDE_COMMAND, cwd: folders.path } : { cwd: folders.path })
  }
  // Ctrl/Cmd+Enter = primary, wherever focus is in the dialog.
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      submit()
    }
  }

  const folder = baseName(folders.path)
  const primary = kind === "swarm" ? "Open swarm" : kind === "existing" ? "Open existing Claude" : kind === "claude" ? `Open Claude Code in ${folder}` : `Open shell in ${folder}`
  const noteOf = (l: Swarms | Existing, count: number) => (l.kind === "ready" ? `${count} found` : l.kind === "loading" ? "Loading…" : "Failed")
  const listNote = noteOf(swarms, swarms.kind === "ready" ? swarms.names.length : 0)
  const existingNote = noteOf(existing, existing.kind === "ready" ? existing.panes.length : 0)
  const setKindFrom = (v: string) => {
    if (isKind(v)) setKind(v)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* Desktop: 560 px dialog, the body scrolls inside 100dvh - 32. Phone (< md): full-height bottom sheet 48 px below the top. */}
      <DialogContent
        onKeyDown={onKeyDown}
        className="flex flex-col gap-0 bg-popover text-body shadow-64 ring-0 sm:max-w-none max-md:top-12 max-md:bottom-0 max-md:left-0 max-md:w-full max-md:max-w-none max-md:translate-x-0 max-md:translate-y-0 max-md:rounded-t-xl max-md:rounded-b-none max-md:p-0 md:max-h-[calc(100dvh-2rem)] md:w-[760px] md:max-w-[calc(100%-2rem)] md:rounded-xl md:p-7 md:pb-6"
      >
        <div className="mx-auto mt-2 mb-1 h-1 w-8 shrink-0 rounded-full bg-grey-82 md:hidden" aria-hidden="true" />
        <div className="mb-1.5 flex shrink-0 items-start justify-between max-md:mb-0 max-md:items-center max-md:pr-2 max-md:pl-5">
          <DialogTitle className="text-subtitle1 font-semibold">New session</DialogTitle>
          <DialogClose asChild>
            <Button variant="ghost" className={`${ICON_BTN} max-md:size-11`} aria-label="Close">
              <DismissIcon />
            </Button>
          </DialogClose>
        </div>
        <DialogDescription className="mb-[22px] shrink-0 text-body text-muted-foreground max-md:sr-only">
          Start a shell or Claude Code on {window.location.hostname}, attach to a running swarm, or attach to Claude Code already running in tmux.
        </DialogDescription>

        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain max-md:px-4 max-md:pt-2">
          <RadioGroup aria-label="Session type" value={kind} onValueChange={setKindFrom} className="mb-[22px] hidden grid-cols-4 gap-3 md:grid">
            {KINDS.map((k) => (
              <Choice key={k.value} {...k} />
            ))}
          </RadioGroup>
          {/* Phone: one 48 px segmented row (M3 segmented button shape, Fluent colours). */}
          <RadioGroupPrimitive.Root
            aria-label="Session type"
            value={kind}
            onValueChange={setKindFrom}
            className="mb-3.5 grid h-(--picker-seg-h) shrink-0 grid-cols-4 overflow-hidden rounded-full border border-grey-74 md:hidden"
          >
            {KINDS.map((k) => (
              <RadioGroupPrimitive.Item
                key={k.value}
                value={k.value}
                className="inline-flex min-w-0 items-center justify-center gap-1 border-grey-74 bg-card px-1.5 text-[13px] font-semibold whitespace-nowrap outline-none not-first:border-l focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-foreground active:bg-grey-92 data-[state=checked]:bg-brand-160 data-[state=checked]:text-brand-60"
              >
                {kind === k.value && <CheckmarkIcon className="size-4 shrink-0" />}
                {k.title}
              </RadioGroupPrimitive.Item>
            ))}
          </RadioGroupPrimitive.Root>

          {kind === "existing" ? (
            <>
              <div className="mb-2 flex items-baseline justify-between">
                <b className="font-semibold">Claude Code running in tmux</b>
                <small className="text-caption text-muted-foreground">{existingNote}</small>
              </div>
              <div className="mb-[22px] max-h-60 shrink-0 overflow-y-auto rounded-md border border-grey-88">
                {existing.kind === "loading" && <p className="px-4 py-3.5 text-muted-foreground">Loading tmux panes…</p>}
                {existing.kind === "error" && <p className="break-all px-4 py-3.5 text-destructive">{existing.message}</p>}
                {existing.kind === "ready" && existing.panes.length === 0 && (
                  <p className="px-4 py-3.5 text-muted-foreground">No Claude Code panes in tmux on this host</p>
                )}
                {existing.kind === "ready" &&
                  existing.errors.map((e) => (
                    <p key={e.socket} className="break-all border-t border-grey-94 px-4 py-3.5 text-destructive first:border-t-0">
                      {e.socket}: {e.message}
                    </p>
                  ))}
                {existing.kind === "ready" && existing.panes.length > 0 && (
                  <RadioGroup aria-label="Claude Code pane" value={pickedPane ?? ""} onValueChange={setPickedPane} className="gap-0">
                    {existing.panes.map((p) => {
                      const key = paneKey(p)
                      const id = `pane-${key}`
                      return (
                        <Label
                          key={key}
                          htmlFor={id}
                          className="grid cursor-pointer grid-cols-[20px_1fr] items-center gap-3.5 border-t border-grey-94 px-4 py-3.5 text-body font-normal first:border-t-0 hover:bg-grey-98 has-[[data-state=checked]]:bg-accent"
                        >
                          <RadioGroupItem id={id} value={key} />
                          <span className="min-w-0">
                            <b className="block truncate font-semibold">{baseName(p.cwd)}</b>
                            <code className="block truncate font-mono text-caption text-muted-foreground">
                              {p.socket} · {p.session}:{p.window} · {p.id}
                            </code>
                          </span>
                        </Label>
                      )
                    })}
                  </RadioGroup>
                )}
              </div>
            </>
          ) : kind === "swarm" ? (
            <>
              <div className="mb-2 flex items-baseline justify-between">
                <b className="font-semibold">Available swarm sockets</b>
                <small className="text-caption text-muted-foreground">{listNote}</small>
              </div>
              <div className="mb-[22px] max-h-60 shrink-0 overflow-y-auto rounded-md border border-grey-88">
                {swarms.kind === "loading" && <p className="px-4 py-3.5 text-muted-foreground">Loading swarm sockets…</p>}
                {swarms.kind === "error" && <p className="break-all px-4 py-3.5 text-destructive">{swarms.message}</p>}
                {swarms.kind === "ready" && swarms.names.length === 0 && (
                  <p className="px-4 py-3.5 text-muted-foreground">No claude-swarm-* sessions on this host</p>
                )}
                {swarms.kind === "ready" && swarms.names.length > 0 && (
                  <RadioGroup aria-label="Swarm socket" value={picked ?? ""} onValueChange={setPicked} className="gap-0">
                    {swarms.names.map((name) => {
                      const id = `swarm-${name}`
                      return (
                        // .sock / .sock.selected
                        <Label
                          key={name}
                          htmlFor={id}
                          className="grid cursor-pointer grid-cols-[20px_1fr] items-center gap-3.5 border-t border-grey-94 px-4 py-3.5 text-body font-normal first:border-t-0 hover:bg-grey-98 has-[[data-state=checked]]:bg-accent"
                        >
                          <RadioGroupItem id={id} value={name} />
                          <code className="font-mono text-[13px] font-semibold">{name}</code>
                        </Label>
                      )
                    })}
                  </RadioGroup>
                )}
              </div>
            </>
          ) : (
            <>
              <div className="mb-2 flex items-baseline justify-between gap-3 max-md:hidden">
                <b className="font-semibold">Start in</b>
                {folders.memory.recent.length > 0 && <small className="text-caption text-muted-foreground">Remembered from your last session</small>}
              </div>
              <RecentFolders b={folders} />
              <FolderPicker b={folders} className="shrink-0 max-md:min-h-80 max-md:flex-1" />
              <StartsIn b={folders} className="mx-0.5 mt-2.5 mb-[22px] shrink-0 max-md:mt-2 max-md:mb-4" />
            </>
          )}

          {/* Not in mockup 3: tabs hidden on any client; Show puts one back in the strip and attaches to it. */}
          {hidden.length > 0 && (
            <div className="mb-6 shrink-0 max-md:mb-2">
              <div className="mb-2 flex items-baseline justify-between">
                <b className="font-semibold">Hidden sessions</b>
                <small className="text-caption text-muted-foreground">{hidden.length}</small>
              </div>
              <div className="max-h-40 overflow-y-auto rounded-md border border-grey-88">
                {hidden.map((tab) => (
                  <div key={tab.id} className="flex items-center gap-3 border-t border-grey-94 py-1.5 pr-1.5 pl-4 first:border-t-0">
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-semibold">{tab.label}</span>
                      <span className="block truncate font-mono text-caption text-muted-foreground">{tab.id}</span>
                    </span>
                    <Button variant="outline" className={BTN} onClick={() => onShow(tab)}>
                      <EyeIcon />
                      Show
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center justify-end gap-2 max-md:mt-2.5 max-md:border-t max-md:border-grey-94 max-md:px-4 max-md:pt-3 max-md:pb-[max(24px,env(safe-area-inset-bottom))]">
          {!listsSession && (
            <span className="mr-auto flex items-center gap-1.5 text-caption whitespace-nowrap text-muted-foreground max-md:hidden">
              <kbd className={KBD}>↵</kbd>in <kbd className={KBD}>⌫</kbd>up
            </span>
          )}
          <DialogClose asChild>
            <Button variant="outline" className={`${BTN} max-md:h-12`}>
              Cancel
            </Button>
          </DialogClose>
          <Button className={`${BTN_PRIMARY} block min-w-0 truncate max-md:h-12 max-md:flex-1 md:max-w-80`} disabled={!canOpen} onClick={submit}>
            {primary}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
