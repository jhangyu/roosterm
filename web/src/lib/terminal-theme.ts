import type { ITheme } from "@xterm/xterm"

const THEME_VARS = {
  background: "--term-background",
  foreground: "--term-foreground",
  cursor: "--term-cursor",
  selectionBackground: "--term-selection",
  black: "--term-black", brightBlack: "--term-bright-black",
  red: "--term-red", brightRed: "--term-bright-red",
  green: "--term-green", brightGreen: "--term-bright-green",
  yellow: "--term-yellow", brightYellow: "--term-bright-yellow",
  blue: "--term-blue", brightBlue: "--term-bright-blue",
  magenta: "--term-magenta", brightMagenta: "--term-bright-magenta",
  cyan: "--term-cyan", brightCyan: "--term-bright-cyan",
  white: "--term-white", brightWhite: "--term-bright-white",
} as const satisfies Partial<Record<keyof ITheme, string>>

type TerminalMetrics = { fontSize: number; rowHeightPx: number }
type TerminalLook = { theme: ITheme; fontFamily: string; desktop: TerminalMetrics; phone: TerminalMetrics }

function readVar(style: CSSStyleDeclaration, name: string): string {
  const v = style.getPropertyValue(name).trim()
  if (v === "") throw new Error(`terminal look: expected CSS variable ${name} in web/src/styles/tokens.css, found none`)
  return v
}

function readNumber(style: CSSStyleDeclaration, name: string): number {
  const v = readVar(style, name)
  const n = Number(v)
  if (!Number.isFinite(n)) throw new Error(`terminal look: expected ${name} to be a unitless number, got ${JSON.stringify(v)}`)
  return n
}

export function readTerminalLook(): TerminalLook {
  const style = getComputedStyle(document.documentElement)
  const theme: ITheme = {}
  for (const [key, name] of Object.entries(THEME_VARS)) theme[key as keyof typeof THEME_VARS] = readVar(style, name)
  return {
    theme,
    fontFamily: readVar(style, "--app-font-mono"),
    desktop: { fontSize: readNumber(style, "--terminal-font-size"), rowHeightPx: readNumber(style, "--terminal-row-height") },
    phone: { fontSize: readNumber(style, "--terminal-font-size-phone"), rowHeightPx: readNumber(style, "--terminal-row-height-phone") },
  }
}
