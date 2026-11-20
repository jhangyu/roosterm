import type { ReactNode } from "react"
import { CheckmarkCircleIcon, WarningIcon } from "@/components/icons"
import { formatTime, isDetached, type PaneState } from "./pane-state"

export type FooterInfo = {
  cols: number
  rows: number
  scrollbackLines: number
  lastOutput: Date | null // local clock of the last received frame, captured when the pane left `live`
  endedAt: Date | null // local clock when the tab's exit was observed
}

// Mockup 3 `.badge.lost`: 22px high, 10px padding, 6px gap, full radius, caption 600, inset 1px warning border.
function LostBadge({ label }: { label: string }) {
  return (
    <span className="inline-flex h-[22px] items-center gap-1.5 rounded-full bg-warning-bg px-2.5 text-xs leading-none font-semibold text-warning inset-ring inset-ring-warning-border">
      <WarningIcon className="size-4" />
      {label}
    </span>
  )
}

// Mockup 3 `.term-status`: gap 18, padding 10/20, top border grey-94, grey-98 fill, caption 12/16 muted.
// Desktop only: the phone card has no footer in the mockup.
export function StatusFooter({ state, info }: { state: PaneState; info: FooterInfo }) {
  const size = <span>{`${info.cols} × ${info.rows}`}</span>
  let items: ReactNode
  let right: ReactNode
  if (isDetached(state)) {
    const label = state.kind === "core-offline" ? "Core offline" : "Disconnected"
    items = <><LostBadge label={label} />{info.lastOutput && <span>{`Last output ${formatTime(info.lastOutput)}`}</span>}{size}</>
    right = <span>Input paused until reconnected</span>
  } else if (state.kind === "ended") {
    items = <>{info.endedAt && <span>{`Ended ${formatTime(info.endedAt)}`}</span>}{size}</>
    right = <span>Read-only</span>
  } else if (state.kind === "live") {
    items = (
      <>
        <span className="flex items-center gap-1.5 font-semibold text-success">
          <CheckmarkCircleIcon className="size-4" />
          Connected
        </span>
        {size}
        <span>UTF-8</span>
      </>
    )
    right = <span>{`Scrollback ${info.scrollbackLines.toLocaleString("en-US")} lines`}</span>
  } else {
    items = <><span>Connecting…</span>{size}</>
    right = null
  }
  return (
    <div className="hidden items-center gap-[18px] border-t border-grey-94 bg-grey-98 px-5 py-2.5 text-xs leading-4 text-muted-foreground md:flex">
      {items}
      <span className="flex-1" />
      {right}
    </div>
  )
}
