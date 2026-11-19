import type { TabEnd } from "@/api/types"
import type { SocketEnd } from "@/api/tab-socket"

export type PaneState =
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "core-offline"; notice: string | null }
  | { kind: "ended"; end: SocketEnd }

// An exited or removed tab has nothing to re-attach to; every other end can be retried.
export const canReconnect = (s: PaneState) =>
  s.kind === "ended" && s.end.kind !== "exited" && s.end.kind !== "not-found"

export function endLabel(end: TabEnd): string {
  if (end.exitCode !== null) return `Exited (code ${end.exitCode})`
  if (end.signal !== null) return `Exited (signal ${end.signal})`
  return `Exited (${end.reason})`
}
