// Mockup 3 control recipes (docs/mockups/3-calm-surfaces.html "Controls"), layered over shadcn Button/Input/Badge.

// .btn: 36px high, 0 16px, corner-small, 600 body text, 1px input border on card.
export const BTN =
  "h-9 gap-2 rounded-sm border-input bg-card px-4 text-body font-semibold text-foreground hover:bg-muted"
// .btn.primary
export const BTN_PRIMARY =
  "h-9 gap-2 rounded-sm border-transparent bg-primary px-4 text-body font-semibold text-primary-foreground hover:bg-primary-hover"
// .btn.danger-outline
export const BTN_DANGER =
  "h-9 gap-2 rounded-sm border-destructive-border bg-card px-4 text-body font-semibold text-destructive hover:bg-destructive-bg"
// .icon-btn: 36x36, corner-small, grey-26 glyph, 5% foreground hover.
export const ICON_BTN = "size-9 rounded-sm text-grey-26 hover:bg-foreground/5 hover:text-foreground"

// .field input: 40px, 0 12px, corner-small, grey-38 bottom stroke, Fluent focus underline.
export const FIELD_INPUT =
  "h-10 rounded-sm border-input border-b-grey-38 bg-card px-3 text-body focus-visible:border-b-2 focus-visible:border-b-primary focus-visible:ring-0"

// .badge base + .run / .idle
export const BADGE = "h-[22px] gap-1.5 rounded-full border-0 px-2.5 text-caption font-semibold leading-none"
export const BADGE_RUN = "bg-success-bg text-success inset-ring inset-ring-success-border"
export const BADGE_IDLE = "bg-grey-96 text-grey-38 inset-ring inset-ring-grey-88"

// .dialog: 560px, corner-extra-large, shadow-64, 28px 28px 24px.
export const DIALOG =
  "w-[560px] max-w-[calc(100%-2rem)] gap-0 rounded-xl bg-popover p-7 pb-6 text-body shadow-64 ring-0 sm:max-w-[560px]"
