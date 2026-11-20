// Desktop vs phone switch: Tailwind `md` (768px), the same width the key bar and card padding use.
export const DESKTOP_QUERY = "(min-width: 768px)"

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
