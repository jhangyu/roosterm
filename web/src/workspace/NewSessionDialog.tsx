import { useEffect, useState, type ReactNode } from "react"
import { listSwarms } from "@/api/http"
import { ApiError, type Tab } from "@/api/types"
import { DismissIcon, PeopleTeamIcon, WindowConsoleIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { BTN, BTN_PRIMARY, DIALOG, ICON_BTN } from "./classes"

export type CreateRequest = { command?: string[]; label?: string }

// ponytail: the API's Tab has no command, so a swarm tab is recognised by its socket-name label (as created by this dialog);
// a renamed swarm tab is treated as a shell. Add a command field to Tab if that matters.
export const isSwarm = (tab: Tab) => tab.label.startsWith("claude-swarm-")

// Same kind and label as the given tab: swarm => re-attach to its socket; shell => server default shell, same label.
export const restartRequest = (tab: Tab): CreateRequest => (isSwarm(tab) ? swarmTab(tab.label) : { label: tab.label })

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (request: CreateRequest) => void
  onUnauthorized: () => void
}

type Kind = "shell" | "swarm"
type Swarms = { kind: "loading" } | { kind: "ready"; names: string[] } | { kind: "error"; message: string }

// Swarm tab request (backend plan, HTTP/WS API: POST /api/tabs); the socket name doubles as the tab label.
export const swarmTab = (name: string): CreateRequest => ({ command: ["tmux", "-L", name, "a"], label: name })

// .radio: 20px ring, grey-38, primary when checked.
const RADIO =
  "size-5 border-grey-38 bg-card shadow-none data-[state=checked]:border-primary [&_[data-slot=radio-group-indicator]]:size-5 [&_[data-slot=radio-group-indicator]>span]:size-2.5 [&_[data-slot=radio-group-indicator]>span]:bg-primary"

function Choice(props: { value: Kind; icon: ReactNode; title: string; hint: string }) {
  const id = `session-kind-${props.value}`
  return (
    // .choice / .choice.selected
    <Label
      htmlFor={id}
      className={`grid cursor-pointer grid-cols-[40px_1fr_20px] items-start gap-3 rounded-md border border-grey-88 bg-card p-4 text-left text-body leading-(--lh-body) font-normal hover:border-grey-74 has-[[data-state=checked]]:border-2 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-accent has-[[data-state=checked]]:p-[15px] group`}
    >
      <span
        className={"grid size-10 place-items-center rounded-sm bg-grey-94 text-grey-26 group-has-[[data-state=checked]]:bg-card group-has-[[data-state=checked]]:text-primary"}
      >
        {props.icon}
      </span>
      <span>
        <b className="block font-semibold">{props.title}</b>
        <small className="text-caption text-muted-foreground">{props.hint}</small>
      </span>
      <RadioGroupItem id={id} value={props.value} className={RADIO} />
    </Label>
  )
}

// Mockup 3 screen 03: shell vs. swarm choice tiles, then the swarm socket list.
export function NewSessionDialog({ open, onOpenChange, onCreate, onUnauthorized }: Props) {
  const [kind, setKind] = useState<Kind>("shell")
  const [swarms, setSwarms] = useState<Swarms>({ kind: "loading" })
  const [picked, setPicked] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    let current = true
    setKind("shell")
    setSwarms({ kind: "loading" })
    setPicked(null)
    listSwarms().then(
      (names) => {
        if (!current) return
        setSwarms({ kind: "ready", names })
        setPicked(names[0] ?? null)
      },
      (e: unknown) => {
        if (!(e instanceof ApiError)) throw e
        if (e.status === 401) onUnauthorized()
        else if (current) setSwarms({ kind: "error", message: e.message })
      },
    )
    return () => {
      current = false
    }
  }, [open, onUnauthorized])

  const canOpen = kind === "shell" || picked !== null
  const submit = () => {
    if (kind === "shell") onCreate({})
    else if (picked !== null) onCreate(swarmTab(picked))
  }

  const listNote =
    swarms.kind === "ready" ? `${swarms.names.length} found` : swarms.kind === "loading" ? "Loading…" : "Failed"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={DIALOG} showCloseButton={false}>
        <div className="mb-1.5 flex items-start justify-between">
          <DialogTitle className="text-subtitle1 font-semibold">New session</DialogTitle>
          <DialogClose asChild>
            <Button variant="ghost" className={`${ICON_BTN} -mt-1 -mr-1`} aria-label="Close">
              <DismissIcon />
            </Button>
          </DialogClose>
        </div>
        <DialogDescription className="mb-[22px] text-body text-muted-foreground">
          Start a fresh shell on {window.location.hostname}, or attach to a running Claude swarm.
        </DialogDescription>

        <RadioGroup
          aria-label="Session type"
          value={kind}
          onValueChange={(v) => setKind(v === "swarm" ? "swarm" : "shell")}
          className="mb-[22px] grid grid-cols-1 gap-3 sm:grid-cols-2"
        >
          <Choice value="shell" icon={<WindowConsoleIcon />} title="Shell" hint="New login shell on this host" />
          <Choice value="swarm" icon={<PeopleTeamIcon />} title="Swarm" hint="Attach to a running tmux swarm socket" />
        </RadioGroup>

        {kind === "swarm" && (
          <>
            <div className="mb-2 flex items-baseline justify-between">
              <b className="font-semibold">Available swarm sockets</b>
              <small className="text-caption text-muted-foreground">{listNote}</small>
            </div>
            <div className="mb-[26px] overflow-hidden rounded-md border border-grey-88">
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
                        className={`grid cursor-pointer grid-cols-[20px_1fr] items-center gap-3.5 px-4 py-3.5 text-body font-normal hover:bg-grey-98 border-t border-grey-94 first:border-t-0 has-[[data-state=checked]]:bg-accent`}
                      >
                        <RadioGroupItem id={id} value={name} className={RADIO} />
                        <code className="font-mono text-[13px] font-semibold">{name}</code>
                      </Label>
                    )
                  })}
                </RadioGroup>
              )}
            </div>
          </>
        )}

        <div className="flex justify-end gap-2">
          <DialogClose asChild>
            <Button variant="outline" className={BTN}>
              Cancel
            </Button>
          </DialogClose>
          <Button className={BTN_PRIMARY} disabled={!canOpen} onClick={submit}>
            {kind === "shell" ? "Open shell" : "Open swarm"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
