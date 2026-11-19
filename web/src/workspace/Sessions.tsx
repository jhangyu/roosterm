import type { Tab } from "@/api/types"
import {
  AddIcon,
  DismissIcon,
  EditIcon,
  PeopleTeamIcon,
  ServerIcon,
  SignOutIcon,
  WindowConsoleIcon,
} from "@/components/icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { endLabel } from "@/terminal/pane-state"
import { BADGE, BADGE_IDLE, BADGE_RUN, ICON_BTN } from "./classes"

// ponytail: a tab created from the swarm dialog is labelled with its socket name; a renamed swarm tab shows the shell glyph.
const isSwarm = (tab: Tab) => tab.label.startsWith("claude-swarm-")

// .dot / .dot.live
function Dot({ live, className }: { live: boolean; className?: string }) {
  return <span className={cn("size-2 shrink-0 rounded-full bg-current", live && "animate-pulse", className)} />
}

// .badge.run "Running" / .badge.idle "Exited (…)"
export function SessionBadge({ tab, className }: { tab: Tab; className?: string }) {
  const running = tab.end === null
  return (
    <Badge className={cn(BADGE, running ? BADGE_RUN : BADGE_IDLE, className)}>
      <Dot live={running} />
      {tab.end === null ? "Running" : endLabel(tab.end)}
    </Badge>
  )
}

export type SessionsProps = {
  tabs: Tab[]
  activeId: string | null
  onSelect: (id: string) => void
  onNew: () => void
}

// Mockup 3 screen 02 `.rail`: brand, section label, session cards, dashed "New session", footer with sign-out.
export function SessionRail({ tabs, activeId, onSelect, onNew, onLogout }: SessionsProps & { onLogout: () => void }) {
  return (
    <nav
      aria-label="Sessions"
      className="relative z-10 hidden min-h-0 flex-col border-r border-foreground/6 bg-acrylic px-4 pt-6 pb-4 backdrop-blur-[30px] backdrop-saturate-125 md:flex"
    >
      <div className="flex items-center gap-3 px-2 pb-[22px]">
        <span className="grid size-9 place-items-center rounded-md bg-primary text-primary-foreground shadow-4">
          <WindowConsoleIcon />
        </span>
        <div>
          <b className="block text-subtitle2 font-semibold">Roosterm</b>
          <span className="flex items-center gap-1 text-caption text-muted-foreground">
            <ServerIcon className="size-4 shrink-0" />
            {window.location.hostname}
          </span>
        </div>
      </div>
      <div className="flex items-center justify-between px-2 pt-1.5 pb-2.5">
        <span className="text-caption font-semibold tracking-[.04em] text-muted-foreground uppercase">
          Sessions · {tabs.length}
        </span>
      </div>
      {/* -m/p keeps the active card's shadow-4 inside the scroll clip */}
      <div className="-m-2 min-h-0 overflow-y-auto p-2">
        <div className="grid gap-2">
          {tabs.map((tab) => {
            const active = tab.id === activeId
            const exited = tab.end !== null
            return (
              // .session / .session.active / .session.exited
              <Button
                key={tab.id}
                variant="ghost"
                aria-current={active ? "true" : undefined}
                onClick={() => onSelect(tab.id)}
                className={cn(
                  "relative grid h-auto grid-cols-[36px_1fr] items-start justify-items-start gap-3 rounded-md p-3 text-left text-body font-normal whitespace-normal text-foreground hover:bg-card/60 active:not-aria-[haspopup]:translate-y-0",
                  active &&
                    "bg-card shadow-4 hover:bg-card before:absolute before:top-[18px] before:bottom-[18px] before:left-0 before:w-[3px] before:rounded-full before:bg-primary",
                )}
              >
                <span
                  className={cn(
                    "grid size-9 place-items-center rounded-sm bg-grey-94 text-grey-26",
                    active && "bg-accent text-primary",
                    exited && !active && "bg-grey-96 text-grey-74",
                  )}
                >
                  {isSwarm(tab) ? <PeopleTeamIcon /> : <WindowConsoleIcon />}
                </span>
                <span className="min-w-0">
                  <span className={cn("block truncate font-semibold", exited && "text-grey-38")}>{tab.label}</span>
                  <span className="mt-0.5 block truncate font-mono text-caption text-muted-foreground">{tab.id}</span>
                  <SessionBadge tab={tab} className="mt-2" />
                </span>
              </Button>
            )
          })}
        </div>
        {/* .new-session */}
        <Button
          variant="outline"
          onClick={onNew}
          className="mt-3 h-11 w-full justify-start gap-2 rounded-md border-dashed border-grey-74 bg-card/40 px-4 text-body font-semibold text-grey-26 hover:border-grey-38 hover:bg-card/80 hover:text-grey-26"
        >
          <AddIcon />
          New session
        </Button>
      </div>
      {/* .rail-foot */}
      <div className="mt-auto flex items-center gap-2.5 border-t border-foreground/6 px-2 pt-3">
        <span className="flex-1 leading-4">
          Signed in
          <small className="block text-[11px] text-muted-foreground">Sessions keep running after sign-out</small>
        </span>
        <Button variant="ghost" className={ICON_BTN} aria-label="Sign out" title="Sign out" onClick={onLogout}>
          <SignOutIcon />
        </Button>
      </div>
    </nav>
  )
}

type ActiveActions = { onRename: (tab: Tab) => void; onClose: (tab: Tab) => void }

// Mockup 3 screen 02 `.main-head`: title, status badge, spacer, actions.
export function MainHead({ active, onRename, onClose }: { active: Tab } & ActiveActions) {
  return (
    <div className="hidden items-center gap-3.5 px-1 pb-[18px] md:flex">
      <h2 className="truncate text-subtitle1 font-semibold">{active.label}</h2>
      <SessionBadge tab={active} />
      <span className="truncate font-mono text-caption text-muted-foreground">{active.id}</span>
      <span className="flex-1" />
      <Button variant="ghost" className={ICON_BTN} aria-label="Rename session" title="Rename session" onClick={() => onRename(active)}>
        <EditIcon />
      </Button>
      <Button variant="ghost" className={ICON_BTN} aria-label="Close session" title="Close session" onClick={() => onClose(active)}>
        <DismissIcon />
      </Button>
    </div>
  )
}

// Mockup 3 screen 04: `.p-appbar` (title + 44px icon buttons) and `.p-tabs` chips.
export function PhoneBar({
  tabs,
  activeId,
  onSelect,
  onNew,
  onLogout,
  onRename,
  onClose,
}: SessionsProps & { onLogout: () => void } & ActiveActions) {
  const active = tabs.find((t) => t.id === activeId) ?? null
  const phoneBtn = `${ICON_BTN} size-11`
  return (
    <div className="relative z-10 md:hidden">
      <div className="flex items-center gap-1 pt-1.5 pr-3 pb-2 pl-4">
        <div className="min-w-0 flex-1">
          <b className="block truncate text-subtitle2 font-semibold">{active?.label ?? "Roosterm"}</b>
          <small className="block truncate font-mono text-caption text-muted-foreground">
            {active?.id ?? window.location.hostname}
          </small>
        </div>
        {active !== null && (
          <>
            <Button variant="ghost" className={phoneBtn} aria-label="Rename session" onClick={() => onRename(active)}>
              <EditIcon />
            </Button>
            <Button variant="ghost" className={phoneBtn} aria-label="Close session" onClick={() => onClose(active)}>
              <DismissIcon />
            </Button>
          </>
        )}
        <Button variant="ghost" className={phoneBtn} aria-label="Sign out" onClick={onLogout}>
          <SignOutIcon />
        </Button>
      </div>
      <div className="flex gap-2 overflow-x-auto px-4 pb-2.5 [scrollbar-width:none]">
        {tabs.map((tab) => {
          const on = tab.id === activeId
          return (
            // .chip / .chip.on
            <Button
              key={tab.id}
              variant="ghost"
              aria-current={on ? "true" : undefined}
              onClick={() => onSelect(tab.id)}
              className={cn(
                "h-8 gap-1.5 rounded-full border-foreground/6 bg-card/60 px-3 text-[13px] font-semibold text-grey-26 hover:bg-card",
                on && "border-transparent bg-card text-foreground shadow-4",
              )}
            >
              {on && tab.end === null && <Dot live className="text-success" />}
              {tab.label}
            </Button>
          )
        })}
        {/* .chip.add */}
        <Button
          variant="ghost"
          aria-label="New session"
          onClick={onNew}
          className="size-8 rounded-full border-foreground/6 bg-card/60 p-0 text-grey-26 hover:bg-card"
        >
          <AddIcon className="size-4 shrink-0" />
        </Button>
      </div>
    </div>
  )
}
