// Desktop vs phone switch: Tailwind `md` (768px), the same width the key bar and card padding use.
export const DESKTOP_QUERY = "(min-width: 768px)"

export type TerminalMetrics = { fontSize: number; lineHeight: number }

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
  lineHeight: readNumber("--terminal-line-height-phone"),
})
