import type { ReactNode } from "react"
import { ArrowDownIcon, ArrowLeftIcon, ArrowRightIcon, ArrowUpIcon, KeyboardShiftIcon, KeyboardTabIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import type { SpecialKey } from "@/lib/keys"

type KeyBarProps = {
  disabled: boolean
  ctrlLatched: boolean
  onToggleCtrl: () => void
  onKey: (key: SpecialKey) => void
  onLetter: (letter: "c" | "d") => void
}

const ICON = "size-[18px]"
const KEY =
  "h-11 min-w-11 shrink-0 gap-1 rounded-sm border-0 bg-card px-2.5 text-[13px] font-semibold leading-none text-foreground shadow-2 hover:bg-card"

// Mockup 3 order: Esc, Tab, Shift+Tab, Ctrl (latching), C, D, divider, then the arrows.
export function KeyBar({ disabled, ctrlLatched, onToggleCtrl, onKey, onLetter }: KeyBarProps) {
  const key = (aria: string, onClick: () => void, content: ReactNode) => (
    <Button
      key={aria}
      type="button"
      variant="ghost"
      className={KEY}
      disabled={disabled}
      aria-label={aria}
      onPointerDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      {content}
    </Button>
  )
  return (
    <div
      role="toolbar"
      aria-label="Special keys"
      className="-mx-2.5 mt-2.5 flex shrink-0 gap-1.5 overflow-x-auto border-t border-foreground/[0.06] bg-acrylic p-2.5 backdrop-blur-[30px] [scrollbar-width:none] md:hidden"
    >
      {key("Escape", () => onKey("esc"), "Esc")}
      {key("Tab", () => onKey("tab"), <><KeyboardTabIcon className={ICON} />Tab</>)}
      {key("Shift+Tab", () => onKey("shift-tab"), <><KeyboardShiftIcon className={ICON} />Tab</>)}
      <Button
        type="button"
        variant="ghost"
        className={cn(
          KEY,
          "text-accent-foreground",
          ctrlLatched && "bg-primary text-primary-foreground hover:bg-primary",
        )}
        disabled={disabled}
        aria-pressed={ctrlLatched}
        onPointerDown={(e) => e.preventDefault()}
        onClick={onToggleCtrl}
      >
        Ctrl
      </Button>
      {key("C", () => onLetter("c"), "C")}
      {key("D", () => onLetter("d"), "D")}
      <span className="mx-0.5 my-1.5 w-px shrink-0 bg-foreground/[0.08]" aria-hidden />
      {key("Left", () => onKey("left"), <ArrowLeftIcon className={ICON} />)}
      {key("Up", () => onKey("up"), <ArrowUpIcon className={ICON} />)}
      {key("Down", () => onKey("down"), <ArrowDownIcon className={ICON} />)}
      {key("Right", () => onKey("right"), <ArrowRightIcon className={ICON} />)}
    </div>
  )
}
