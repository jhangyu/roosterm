export type SpecialKey = "esc" | "tab" | "shift-tab" | "up" | "down" | "left" | "right" | "ctrl-c" | "ctrl-d"

const ESC = "\x1b"

export function keySequence(key: SpecialKey, applicationCursor: boolean): string {
  const arrow = applicationCursor ? `${ESC}O` : `${ESC}[`
  switch (key) {
    case "esc": return ESC
    case "tab": return "\t"
    case "shift-tab": return `${ESC}[Z`
    case "up": return `${arrow}A`
    case "down": return `${arrow}B`
    case "right": return `${arrow}C`
    case "left": return `${arrow}D`
    case "ctrl-c": return "\x03"
    case "ctrl-d": return "\x04"
  }
}
