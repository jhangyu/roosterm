import type { ApiError } from "@/api/types"
import { DismissIcon, ErrorCircleIcon } from "@/components/icons"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { BANNER, BANNER_TONES, ICON_BTN } from "@/components/classes"

// Mockup 3 .banner (screen 05) in the danger palette: shows the ApiError message verbatim.
export function ErrorAlert({ error, onDismiss, className }: { error: ApiError; onDismiss: () => void; className?: string }) {
  return (
    <div role="alert" className={cn(BANNER, BANNER_TONES.error, "relative w-full text-left text-body text-foreground", className)}>
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
