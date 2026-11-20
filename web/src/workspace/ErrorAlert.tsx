import type { ApiError } from "@/api/types"
import { DismissIcon, ErrorCircleIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { ICON_BTN } from "./classes"

// Mockup 3 .banner (screen 05) in the danger palette: shows the ApiError message verbatim.
export function ErrorAlert({ error, onDismiss, className }: { error: ApiError; onDismiss: () => void; className?: string }) {
  return (
    <div
      role="alert"
      className={cn(
        "relative flex w-full shrink-0 items-center gap-3.5 rounded-lg bg-destructive-bg py-3 pr-3 pl-[18px] text-left text-body text-foreground shadow-2 inset-ring inset-ring-destructive-border",
        className,
      )}
    >
      <span className="grid text-destructive">
        <ErrorCircleIcon />
      </span>
      <div className="min-w-0 flex-1 text-body text-balance text-grey-26 md:text-pretty">
        <p className="break-all">
          <b className="mr-2 font-semibold text-foreground">{error.op} failed</b>
          {error.message}
        </p>
      </div>
      <Button variant="ghost" className={ICON_BTN} aria-label="Dismiss error" onClick={onDismiss}>
        <DismissIcon />
      </Button>
    </div>
  )
}
