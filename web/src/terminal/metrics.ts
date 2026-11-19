// Desktop vs phone switch: Tailwind `md` (768px), the same width the key bar and card padding use.
export const DESKTOP_QUERY = "(min-width: 768px)"

export type TerminalMetrics = { fontSize: number; rowHeightPx: number }

function readNumber(name: string): number {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  const n = Number(v)
  if (v === "" || !Number.isFinite(n)) {
    throw new Error(`terminal metrics: expected ${name} in web/src/styles/tokens.css to be a unitless number, got ${JSON.stringify(v)}`)
  }
  return n
}

export const readPhoneMetrics = (): TerminalMetrics => ({
  fontSize: readNumber("--terminal-font-size-phone"),
  rowHeightPx: readNumber("--terminal-row-height-phone"),
})

// xterm's `lineHeight` multiplies the measured font cell height (font dependent), so a fixed ratio cannot give the
// mockup's px row pitch. Measure the cell like xterm does (device-pixel ceil), then derive the ratio for a target row px.
const ROW_ROUNDING_GUARD = 0.001 // keeps floor(cell * ratio) from landing one device px under the target

export function lineHeightForRow(fontFamily: string, fontSize: number, rowPx: number): number {
  const probe = document.createElement("span")
  probe.textContent = "W"
  probe.style.cssText = `position:absolute;visibility:hidden;white-space:pre;line-height:normal;font-family:${fontFamily};font-size:${fontSize}px`
  document.body.appendChild(probe)
  const dpr = window.devicePixelRatio
  const cell = Math.ceil(probe.getBoundingClientRect().height * dpr) / dpr
  probe.remove()
  return rowPx / cell + ROW_ROUNDING_GUARD
}
