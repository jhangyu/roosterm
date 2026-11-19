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

export type TerminalLook = { theme: ITheme; fontFamily: string; fontSize: number; lineHeight: number }

export function readTerminalLook(): TerminalLook {
  const css = getComputedStyle(document.documentElement)
  const read = (name: string) => {
    const v = css.getPropertyValue(name).trim()
    if (v === "") throw new Error(`terminal look: expected CSS variable ${name} in web/src/styles/tokens.css, found none`)
    return v
  }
  const num = (name: string) => {
    const v = read(name)
    const n = Number(v)
    if (!Number.isFinite(n)) throw new Error(`terminal look: expected ${name} to be a unitless number, got ${JSON.stringify(v)}`)
    return n
  }
  const theme: ITheme = {}
  for (const [key, name] of Object.entries(THEME_VARS)) theme[key as keyof typeof THEME_VARS] = read(name)
  return {
    theme,
    fontFamily: read("--app-font-mono"),
    fontSize: num("--terminal-font-size"),
    lineHeight: num("--terminal-line-height"),
  }
}
