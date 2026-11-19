import type { ReactNode } from "react"
import { ArrowClockwiseIcon, CheckmarkCircleIcon, DeleteIcon, PlugDisconnectedIcon, ServerIcon, WarningIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import type { TabEnd } from "@/api/types"
import { endLabel, type PaneState } from "./pane-state"

// Mockup 3 `.banner`: padding 12/12/12/18, gap 14, radius-lg, 1px inset border + shadow-2, 14px below.
const BANNER =
  "mb-3.5 flex shrink-0 items-center gap-3.5 rounded-lg py-3 pr-3 pl-[18px]"
const TONES = {
  warn: "bg-warning-bg shadow-[inset_0_0_0_1px_var(--warning-border),var(--elev-2)]",
  error: "bg-destructive-bg shadow-[inset_0_0_0_1px_var(--destructive-border),var(--elev-2)]",
}
const ICON_TONES = { warn: "text-warning", error: "text-destructive" }
// Mockup 3 `.btn`: 36px high, 16px padding, 8px gap, radius-sm, 14/20 semibold.
const ACTION = "h-9 gap-2 rounded-sm px-4 text-sm font-semibold"

function Banner(props: { tone: keyof typeof TONES; icon: ReactNode; title: string; detail?: string; action?: ReactNode }) {
  return (
    <div role="alert" className={cn(BANNER, TONES[props.tone])}>
      <span className={cn("grid", ICON_TONES[props.tone])}>{props.icon}</span>
      <div className="min-w-0 flex-1">
        <b className="mr-2 font-semibold">{props.title}</b>
        {props.detail === undefined ? null : (
          <span className="break-words text-[color:var(--grey-26)]">{props.detail}</span>
        )}
      </div>
      {props.action}
    </div>
  )
}

type Props = { state: PaneState; disconnectedAt: number | null; onReconnect: () => void }

const MS_PER_SECOND = 1000

// Rendered above the terminal card. The exited state is the card overlay `ExitPanel`, not a banner.
export function StatusBanner({ state, disconnectedAt, onReconnect }: Props) {
  const reconnect = (
    <Button type="button" className={ACTION} onClick={onReconnect}>
      <ArrowClockwiseIcon />
      Reconnect
    </Button>
  )
  switch (state.kind) {
    case "live":
      return null
    case "connecting":
      return <p className="mb-3.5 shrink-0 px-1 text-muted-foreground">Connecting…</p>
    case "core-offline":
      return (
        <Banner
          tone="warn"
          icon={<ServerIcon />}
          title="Host core offline"
          detail={state.notice ?? "Input is paused until the core is back online."}
        />
      )
  }
  const end = state.end
  switch (end.kind) {
    case "exited":
      return null
    case "not-found":
      return <Banner tone="error" icon={<WarningIcon />} title="Session not found" detail={end.reason} />
    case "server-error":
      return (
        <Banner
          tone="error"
          icon={<WarningIcon />}
          title={end.code === 4400 ? "Bad message" : "Host core error"}
          detail={end.detail}
          action={reconnect}
        />
      )
    case "server-shutdown":
      return (
        <Banner
          tone="warn"
          icon={<ServerIcon />}
          title="Host is stopping"
          detail="The host service is shutting down. Reconnect once it is back."
          action={reconnect}
        />
      )
    case "disconnected":
      return (
        <Banner
          tone="warn"
          icon={<PlugDisconnectedIcon />}
          title="Connection lost"
          detail={`${location.hostname} stopped responding ${disconnectedAt === null ? 0 : Math.max(0, Math.round((Date.now() - disconnectedAt) / MS_PER_SECOND))} s ago. Your session is still running on the host. Output will resume when the connection returns.`}
          action={reconnect}
        />
      )
    case "protocol-error":
      return <Banner tone="error" icon={<WarningIcon />} title="Protocol error" detail={end.message} action={reconnect} />
  }
}

// Mockup 3 `.exit-panel`: centred over the card, padding 14/14/14/20, gap 16, radius-lg, shadow-16.
export function ExitPanel({ end, onClose, onRestart }: { end: TabEnd; onClose: () => void; onRestart?: () => void }) {
  return (
    <div
      role="status"
      className="absolute bottom-[72px] left-1/2 z-10 flex -translate-x-1/2 items-center gap-4 rounded-lg bg-card py-3.5 pr-3.5 pl-5 shadow-16"
    >
      <span className="grid text-success">
        <CheckmarkCircleIcon />
      </span>
      <span>
        <b className="block font-semibold">{endLabel(end)}</b>
        <small className="text-xs text-muted-foreground">Scrollback is kept until you close this session.</small>
      </span>
      {onRestart && (
        <Button type="button" variant="outline" className={cn(ACTION, "border-input bg-card")} onClick={onRestart}>
          Restart
        </Button>
      )}
      <Button type="button" className={ACTION} onClick={onClose}>
        <DeleteIcon />
        Close session
      </Button>
    </div>
  )
}
