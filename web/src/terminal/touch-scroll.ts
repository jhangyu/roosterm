import type { Terminal } from "@xterm/xterm"

// xterm 6 has no touch scrolling (its VS Code scrollbar never registers touch gestures), so a
// one-finger vertical drag is turned into one line per row of finger travel. Scrollback scrolls
// directly; otherwise each line becomes a one-line wheel event on the touched element, and xterm
// does what it does for a desktop wheel: report it to a mouse-tracking app, or send arrow keys
// in the alternate buffer.
export function attachTouchScroll(term: Terminal, el: HTMLElement): () => void {
  let lastY: number | null = null
  let pending = 0 // finger travel in px not yet turned into whole lines

  const rowHeight = () => {
    const screen = term.element?.querySelector(".xterm-screen")
    if (!(screen instanceof HTMLElement)) throw new Error(`touch scroll: expected .xterm-screen in an opened terminal, got ${screen}`)
    return screen.clientHeight / term.rows
  }

  const scroll = (lines: number, target: EventTarget, clientX: number, clientY: number) => {
    if (term.buffer.active.type === "normal" && term.modes.mouseTrackingMode === "none") {
      term.scrollLines(lines)
      return
    }
    for (let i = 0; i < Math.abs(lines); i++) {
      target.dispatchEvent(
        new WheelEvent("wheel", { deltaY: Math.sign(lines), deltaMode: WheelEvent.DOM_DELTA_LINE, clientX, clientY, bubbles: true, cancelable: true }),
      )
    }
  }

  const start = (ev: TouchEvent) => {
    lastY = ev.touches.length === 1 ? ev.touches[0].clientY : null
    pending = 0
  }
  const move = (ev: TouchEvent) => {
    if (lastY === null || ev.touches.length !== 1) return
    const t = ev.touches[0]
    pending += lastY - t.clientY
    lastY = t.clientY
    const lines = Math.trunc(pending / rowHeight())
    if (lines === 0) return
    pending -= lines * rowHeight()
    scroll(lines, t.target, t.clientX, t.clientY)
  }
  const end = () => {
    lastY = null
  }

  el.addEventListener("touchstart", start, { passive: true })
  el.addEventListener("touchmove", move, { passive: true })
  el.addEventListener("touchend", end, { passive: true })
  el.addEventListener("touchcancel", end, { passive: true })
  return () => {
    el.removeEventListener("touchstart", start)
    el.removeEventListener("touchmove", move)
    el.removeEventListener("touchend", end)
    el.removeEventListener("touchcancel", end)
  }
}
