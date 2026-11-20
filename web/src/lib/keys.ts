const SPECIAL_KEYS = ["esc", "tab", "shift-tab", "ctrl-c", "ctrl-d", "up", "down", "left", "right"] as const
export type SpecialKey = (typeof SPECIAL_KEYS)[number]

const ESC = "\x1b"

export function keySequence(key: SpecialKey, applicationCursor: boolean): string {
  const arrow = applicationCursor ? `${ESC}O` : `${ESC}[`
  switch (key) {
    case "esc": return ESC
    case "tab": return "\t"
    case "shift-tab": return `${ESC}[Z`
    case "ctrl-c": return "\x03"
    case "ctrl-d": return "\x04"
    case "up": return `${arrow}A`
    case "down": return `${arrow}B`
    case "right": return `${arrow}C`
    case "left": return `${arrow}D`
  }
}
