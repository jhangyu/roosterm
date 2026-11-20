import type { ReactNode } from "react"
import type { Tab } from "@/api/types"
import {
  AddIcon,
  DeleteIcon,
  DismissIcon,
  EditIcon,
  PeopleTeamIcon,
  PersonIcon,
  ServerIcon,
  SignOutIcon,
  WindowConsoleIcon,
} from "@/components/icons"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { endLabel } from "@/terminal/pane-state"
import { tabParts, tabText } from "./tabs"
import { BADGE, BADGE_IDLE, BADGE_RUN, ICON_BTN } from "./classes"
import { isSwarm } from "./tabs"

// .dot / .dot.live
function Dot({ live, className }: { live: boolean; className?: string }) {
  return <span className={cn("size-2 shrink-0 rounded-full bg-current", live && "animate-pulse", className)} />
}

// .badge.run "Running" / .badge.idle "Exited (…)"
function SessionBadge({ tab, className }: { tab: Tab; className?: string }) {
  const running = tab.end === null
  return (
    <Badge className={cn(BADGE, running ? BADGE_RUN : BADGE_IDLE, className)}>
      <Dot live={running} />
      {tab.end === null ? "Running" : endLabel(tab.end)}
    </Badge>
  )
}

type SessionsProps = {
  tabs: Tab[]
  activeId: string | null
  onSelect: (id: string) => void
  onNew: () => void
  onHide: (tab: Tab) => void
}

// "<folder> · <summary>": the folder keeps its name (capped), the summary truncates first (mockup 4b).
function TabLabel({ tab, folderMax = "max-w-[20ch]" }: { tab: Tab; folderMax?: string }) {
  const { folder, summary } = tabParts(tab)
  return (
    <span className="flex min-w-0" title={tabText(tab)}>
      <span className={cn("shrink-0 truncate", folderMax)}>{folder}</span>
      {summary !== "" && <span className="min-w-0 truncate font-normal whitespace-pre">{` · ${summary}`}</span>}
    </span>
  )
}

// Ghost icon button; `title` is the desktop tooltip, left off on phone.
function IconAction({
  label,
  title,
  className,
  onClick,
  children,
}: {
  label: string
  title?: string
  className: string
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Button variant="ghost" className={className} aria-label={label} title={title} onClick={onClick}>
      {children}
    </Button>
  )
}

function SignOutButton({ onLogout, className, title }: { onLogout: () => void; className: string; title?: string }) {
  return (
    <IconAction label="Sign out" title={title} className={className} onClick={onLogout}>
      <SignOutIcon />
    </IconAction>
  )
}

function RenameButton({ tab, onRename, className, title }: { tab: Tab; onRename: (tab: Tab) => void; className: string; title?: string }) {
  return (
    <IconAction label="Rename session" title={title} className={className} onClick={() => onRename(tab)}>
      <EditIcon />
    </IconAction>
  )
}

function CloseButton({ tab, onClose, className, title }: { tab: Tab; onClose: (tab: Tab) => void; className: string; title?: string }) {
  return (
    <IconAction label="Close session" title={title} className={className} onClick={() => onClose(tab)}>
      <DeleteIcon />
    </IconAction>
  )
}

function HideButton({
  tab,
  onHide,
  className,
  iconClass,
  title,
}: {
  tab: Tab
  onHide: (tab: Tab) => void
  className: string
  iconClass: string
  title?: string
}) {
  return (
    <IconAction label={`${HIDE_LABEL}: ${tab.label}`} title={title} className={className} onClick={() => onHide(tab)}>
      <DismissIcon className={iconClass} />
    </IconAction>
  )
}

const HIDE_LABEL = "Hide tab (session keeps running)"

// Mockup 3 screen 02 `.rail`: brand, section label, session cards, dashed "New session", footer with sign-out.
export function SessionRail({ tabs, activeId, onSelect, onNew, onHide, onLogout }: SessionsProps & { onLogout: () => void }) {
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
          <span className="flex items-center gap-1 text-caption leading-5 text-muted-foreground">
            <ServerIcon className="size-4 shrink-0" />
            {window.location.hostname}
          </span>
        </div>
      </div>
      <div className="flex items-center justify-between px-2 pt-1.5 pb-2.5">
        <span className="text-caption leading-5 font-semibold tracking-[.04em] text-muted-foreground uppercase">
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
              // Not in mockup 3: the card's top-right hide X (a sibling, since buttons cannot nest).
              <div key={tab.id} className="group relative">
                {/* .session / .session.active / .session.exited */}
                <Button
                  variant="ghost"
                  aria-current={active ? "true" : undefined}
                  onClick={() => onSelect(tab.id)}
                  className={cn(
                    "relative grid h-auto w-full grid-cols-[36px_1fr] items-start justify-items-start gap-3 rounded-md p-3 text-left text-body font-normal whitespace-normal text-foreground hover:bg-card/60 active:not-aria-[haspopup]:translate-y-0",
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
                  <span className="min-w-0 pr-6">
                    <span className={cn("block font-semibold", exited && "text-grey-38")}><TabLabel tab={tab} /></span>
                    <span className="mt-0.5 block truncate font-mono text-caption text-muted-foreground">{tab.id}</span>
                    <SessionBadge tab={tab} className="mt-2" />
                  </span>
                </Button>
                <HideButton
                  tab={tab}
                  onHide={onHide}
                  title={HIDE_LABEL}
                  iconClass="size-4 shrink-0"
                  className={cn(
                    ICON_BTN,
                    "absolute top-2 right-2 size-7 opacity-0 group-hover:opacity-100 focus-visible:opacity-100",
                    active && "opacity-100",
                  )}
                />
              </div>
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
        {/* .avatar: no username endpoint exists, so the person glyph stands in for initials */}
        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-brand-150 text-brand-60">
          <PersonIcon className="size-4 shrink-0" />
        </span>
        <span className="min-w-0 flex-1 leading-4">
          Signed in
          <small className="block truncate text-[11px] text-muted-foreground">Sessions keep running</small>
        </span>
        <SignOutButton onLogout={onLogout} className={ICON_BTN} title="Sign out" />
      </div>
    </nav>
  )
}

type ActiveActions = { onRename: (tab: Tab) => void; onClose: (tab: Tab) => void }

// Mockup 3 screen 02 `.main-head`: title, status badge, spacer, actions.
export function MainHead({ active, onRename, onClose }: { active: Tab } & ActiveActions) {
  return (
    <div className="hidden items-center gap-3.5 px-1 pb-[18px] md:flex">
      <h2 className="min-w-0 text-subtitle1 font-semibold" title={tabText(active)}><TabLabel tab={active} /></h2>
      <SessionBadge tab={active} />
      <span className="truncate font-mono text-caption leading-5 text-muted-foreground">{active.id}</span>
      <span className="flex-1" />
      <RenameButton tab={active} onRename={onRename} className={ICON_BTN} title="Rename session" />
      <CloseButton tab={active} onClose={onClose} className={ICON_BTN} title="Close session" />
    </div>
  )
}

// Mockup 3 screen 04: `.p-appbar` (title + 44px icon buttons) and `.p-tabs` chips.
export function PhoneBar({
  tabs,
  activeId,
  onSelect,
  onNew,
  onHide,
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
          <b className="block text-subtitle2 font-semibold">{active ? <TabLabel tab={active} /> : "Roosterm"}</b>
          {/* inline caption on a body line box, as the mockup's `.title small` (line 21px) */}
          <div className="truncate">
            <small className="font-mono text-caption leading-5 text-muted-foreground">{active?.id ?? window.location.hostname}</small>
          </div>
        </div>
        {active !== null && (
          <>
            <RenameButton tab={active} onRename={onRename} className={phoneBtn} />
            <CloseButton tab={active} onClose={onClose} className={phoneBtn} />
          </>
        )}
        <SignOutButton onLogout={onLogout} className={phoneBtn} />
      </div>
      <div className="flex gap-2 overflow-x-auto px-4 pb-2.5 [scrollbar-width:none]">
        {tabs.map((tab) => {
          const on = tab.id === activeId
          return (
            // .chip / .chip.on, plus a hide X (not in mockup 3) as a sibling button inside the chip outline
            <span
              key={tab.id}
              className={cn(
                "flex h-8 shrink-0 items-center rounded-full border border-foreground/6 bg-card/60 text-grey-26 hover:bg-card",
                on && "border-transparent bg-card text-foreground shadow-4",
              )}
            >
              <Button
                variant="ghost"
                aria-current={on ? "true" : undefined}
                onClick={() => onSelect(tab.id)}
                className="h-8 gap-1.5 rounded-full border-0 bg-transparent pr-1 pl-3 text-[13px] font-semibold text-inherit hover:bg-transparent"
              >
                {on && tab.end === null && <Dot live className="text-success" />}
                <TabLabel tab={tab} folderMax="max-w-[12ch]" />
              </Button>
              <HideButton
                tab={tab}
                onHide={onHide}
                iconClass="size-3.5 shrink-0"
                className="mr-1 size-6 rounded-full p-0 text-grey-38 hover:bg-foreground/5 hover:text-foreground"
              />
            </span>
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
