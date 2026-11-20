import type { ReactNode } from "react"
import { ArrowClockwiseIcon, CheckmarkCircleIcon, DeleteIcon, PlugDisconnectedIcon, ServerIcon, WarningIcon } from "@/components/icons"
import { BANNER, BANNER_TONES, BTN, BTN_PRIMARY } from "@/components/classes"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import type { TabEnd } from "@/api/types"
import { endLabel, formatTime, type PaneState } from "./pane-state"

const ICON_TONES = { warn: "text-warning", error: "text-destructive" }

function Banner(props: { tone: keyof typeof BANNER_TONES; icon: ReactNode; title: string; detail?: string; action?: ReactNode }) {
  return (
    <div role="alert" className={cn(BANNER, BANNER_TONES[props.tone], "mb-3.5")}>
      <span className={cn("grid", ICON_TONES[props.tone])}>{props.icon}</span>
      <div className="min-w-0 flex-1">
        <b className="mr-2 font-semibold">{props.title}</b>
        {props.detail === undefined ? null : (
          <span className="break-words text-grey-26">{props.detail}</span>
        )}
      </div>
      {props.action}
    </div>
  )
}

type Props = { state: PaneState; onReconnect: () => void }

// Rendered above the terminal card. The exited state is the card overlay `ExitPanel`, not a banner.
export function StatusBanner({ state, onReconnect }: Props) {
  const reconnect = (
    <Button type="button" className={BTN_PRIMARY} onClick={onReconnect}>
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
          title={end.badMessage ? "Bad message" : "Host core error"}
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
          detail={`${location.hostname} stopped responding at ${formatTime(end.at)}. Your session is still running on the host. Output will resume when the connection returns.`}
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
        <Button type="button" variant="outline" className={BTN} onClick={onRestart}>
          Restart
        </Button>
      )}
      <Button type="button" className={BTN_PRIMARY} onClick={onClose}>
        <DeleteIcon />
        Close session
      </Button>
    </div>
  )
}
