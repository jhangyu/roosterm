# Visual review: built web UI vs mockup 3 (2026-11-19)

Reviewer: review-visual (fresh context). Contract criterion 5 of `docs/logs/contract-frontend.md`.

- Code state: web sources at `779b72b` (HEAD `44dbe10` only adds a docs log; `git diff 779b72b 44dbe10 -- web host/internal/web` is empty). Binary built with `python3 scripts/build.py` (RC=0).
- Mockup: `docs/mockups/3-calm-surfaces.html` via file://. App: `roosterm serve` on 127.0.0.1:18788 against the shared hc core.
- Browser: desktop Google Chrome headless (CDP) driven by Playwright (Python). Viewports: desktop 1280x800, phone 390x844 (mockup phone screen is 370 wide inside a 10px bezel; offsets compared from the left/right edges).
- Method: `getBoundingClientRect` / `getComputedStyle` per element, positions relative to the matching container (login card, rail, main column, dialog, phone screen, state stage). Mockup phone y values have the 36px fake status bar subtracted.
- Severity: BLOCKING = misaligned/off-centre >=2px, uneven spacing >=2px, wrong indentation, wrong font size/weight, wrong radius, missing/misplaced/wrongly styled mockup element. MINOR = <=1px, fixed-height boxes whose content stays centred, font rendering.
- Known accepted deviations (not reported): footer without Latency and "Ran ...", banner without Dismiss, swarm option copy, working directory / timers / pane counts, Sign out button, core-offline and tab-not-found states.

## Result

**10 BLOCKING, 10 MINOR, 5 need a lead decision.** The round does not close.

## Screenshots

| Screen | Mockup | App |
|---|---|---|
| 01 Login | `visual-review/01-login-mockup.png` | `visual-review/01-login-app.png` |
| 02 Desktop main | `visual-review/02-desktop-mockup.png` | `visual-review/02-desktop-app.png` |
| 03 New session dialog | `visual-review/03-dialog-mockup.png` | `visual-review/03-dialog-app-shell.png` (Shell selected), `visual-review/03-dialog-app-swarm-28-sockets.png` (Swarm selected, 28 sockets on this host) |
| 04 Phone 390px | `visual-review/04-phone-mockup.png` | `visual-review/04-phone-app.png` |
| 05 Disconnected (own roosterm SIGKILLed) | `visual-review/05-disconnected-mockup.png` | `visual-review/05-disconnected-app.png` |
| 06 Exited (`exit` typed in a shell) | `visual-review/06-exited-mockup.png` | `visual-review/06-exited-app.png` |

## BLOCKING differences

| # | Screen | Element | Property | Expected (mockup) | Actual (app) | Delta | Likely source |
|---|---|---|---|---|---|---|---|
| B1 | 01, 02, 03, 04 | Mica page background | computed `background-image` | three radial gradients over #f3f3f3 (blue top-left, lilac bottom-right, peach top) | `none`; flat #f3f3f3 from body | gradients missing | `web/src/styles/tokens.css:40` (`--mica` ends in the colour `#f3f3f3`, which is invalid inside `background-image`, so `bg-(image:--mica)` computes to none); used at `web/src/App.tsx:116,136`, `web/src/screens/LoginScreen.tsx:43` |
| B2 | 02, 05, 06 (desktop) and 04 (phone) | Terminal text rows | row pitch | desktop 13px/20px; phone 11.5px/17px | desktop 13px/23px; phone 11.5px/20px | +3px per row | `web/src/styles/tokens.css:84-88` (`--terminal-line-height` 1.5385, phone 1.4783) read by `web/src/terminal/TerminalPane.tsx` / `metrics.ts`; xterm multiplies the font cell height (about 15px at 13px, 13.5px at 11.5px), not the font size |
| B3 | 02, 04 | Caption lines that the mockup keeps on a 20px line box | line-height / height | rail host line h 20; "SESSIONS" label h 20 (section block 36); phone app-bar subtitle line 20 (title block 43) | 16; 16 (section 32); 16 (title block 38) | rail brand block 64 -> 60, session list top 124 -> 116 (-8px from rail top); phone title-to-subtitle gap 3px tighter | `web/src/workspace/Sessions.tsx:54` (host), `:61` (section label), `:163` (phone subtitle); add `leading-5` |
| B4 | 03 | Dialog close button and head | x / y / head height | close button at content top-right: x 496, y 28 in the dialog; head 36 tall; lead text y 70 | x 500, y 24; head 32; lead y 66 | close +4px right, -4px up; everything below the head 4px higher | `web/src/workspace/NewSessionDialog.tsx:103` (`-mt-1 -mr-1`) |
| B5 | 03 | Checked radio (session type tiles and socket list) | fill | white fill, 1px primary ring, 10px primary dot | fill rgb(15,108,189): a solid blue disc, the dot is invisible | wrong style | `web/src/components/ui/radio-group.tsx:26` (`data-checked:bg-primary`) not overridden by `RADIO` in `web/src/workspace/NewSessionDialog.tsx:34` |
| B6 | 03 | Swarm socket list | dialog height / placement | list sits inside a 560px dialog centred in the viewport (mockup dialog 496px tall) | with 28 sockets on this host the dialog is 1711px tall, top at y -455 in an 800px viewport; title, choice tiles and Cancel / Open are off-screen and unreachable | dialog overflows the viewport | `web/src/workspace/NewSessionDialog.tsx:128` (list container has no max height or scroll) |
| B7 | 05 | Banner stack above the card | element count | one "Connection lost" banner, 14px above the card | an extra error banner "list tabs failed - list tabs: HTTP 0: network error: Failed to fetch" above the "Connection lost" banner | extra element; card pushed down 74px | `web/src/App.tsx:152` (`onSocketLost` -> `refresh`) and `:149` (ErrorAlert) |
| B8 | 06 | Exit panel "Restart" button | background / border | `.btn`: #ffffff fill, 1px #d1d1d1 border | #f3f3f3 fill, 1px #e0e0e0 border | wrong fill | `web/src/terminal/StatusBanner.tsx:115` (`variant="outline"` gives `bg-background`); `BTN` in `web/src/workspace/classes.ts` is the mockup recipe |
| B9 | 02, 03, 05, 06 | Rail footer | layout | avatar 32px + name + one-line client caption, text x 66; footer 49px | no avatar; caption "Sessions keep running after sign-out" wraps to two lines ("sign-" / "out"); text x 24; footer 61px | footer +12px, text -42px | `web/src/workspace/Sessions.tsx:113-117` (the backend has no endpoint that returns the username; the wrap can be fixed in the client) |
| B10 | 01 | Focused Username input | border colours | top/right/left #d1d1d1, bottom 2px #0f6cbd (Fluent focus underline) | all four sides #0f6cbd, bottom 2px | wrong focus style | `web/src/workspace/classes.ts:17` (`FIELD_INPUT`) does not override `focus-visible:border-ring` from `web/src/components/ui/input.tsx:10` |

## Checked and matching (no difference)

- 01 Login: card 420x515, centred (horizontal and vertical offset 0 in the stage/viewport), padding 40/40/32, radius 28, shadow-28; mark 44px, title 24/32 600, subtitle row with 16px icon and 6px gap, labels 14/20 600, inputs 40px radius 8, eye button 36px at right 2px, button 44px full width, footer rule 22px below with 18px padding. Every x/y/w/h matches within 0.5px.
- 02 Desktop: rail 288px, padding 24/16/16; session cards 12px padding, 36px glyph, 12px gap, 8px between cards, 102px pitch, active card shadow-4 with 3px indicator at top/bottom 18px; New session button 44px, 12px below the list; main column padding 24/28/28; head 14px gap, 18px below; title 20/28 600; terminal card radius 16, shadow-8, right and bottom gap 28; terminal padding 22/28; footer padding 10/20, 18px gap between items, caption 12/16.
- 03 Dialog: 560px, centred (offset 0,0), padding 28/28/24, radius 28; choice tiles 2 columns, 12px gap, padding 16 (15 when selected with 2px border), glyph 40px, radio 20px; list label 8px above the list; actions right-aligned with 8px gap.
- 04 Phone: app bar padding 6/12/8/16 and 44px buttons (right gap 12); chips 32px, 8px gap, x 16/72/201 identical; terminal card 10px side margins, 10px above the key bar; key bar keys x and widths identical (10, 60, 131, 202, 252, 302, divider 354, arrows 363-513), 44px keys, 6px gap.
- 05 Disconnected: banner padding 12/12/12/18, 14px icon-to-text gap, 14px above the card, radius 16; dimmed terminal opacity 0.45 + grayscale(1), 25% white overlay; footer Disconnected badge and items at the same x as the mockup.
- 06 Exited: exit panel horizontally centred on the card (offset 0), 72px above the card bottom, padding 14/14/14/20, gap 16, shadow-16; button group spacing identical.

## MINOR differences

| # | Screen | Element | Expected | Actual | Delta | Likely source |
|---|---|---|---|---|---|---|
| M1 | 02 | Session card content (glyph, name, meta, badge) | x 12, y 12 inside the card | x 13, y 13 | +1px (1px transparent border from the shadcn Button base) | `web/src/workspace/Sessions.tsx:78-85` |
| M2 | 02, 03, 06 | Main-head path; dialog "N found" note | line-height 20 | 16 | 0px visual offset (centred / baseline aligned) | `web/src/workspace/Sessions.tsx:134`, `web/src/workspace/NewSessionDialog.tsx` list label |
| M3 | 01 | Sign in button text | line-height 20 | 22 | 0px (fixed 44px height, text centred) | `web/src/screens/LoginScreen.tsx` |
| M4 | 01 | Input vertical padding | 0 12px | 4px 12px | 0px (fixed 40px height, text centred) | `web/src/components/ui/input.tsx:10` |
| M5 | 02, 06 | Badge vertical padding | 0 10px | 2px 10px | 0px (fixed 22px height) | `web/src/components/ui/badge.tsx` |
| M6 | 06 | Exit panel height | 90 | 88 | -2px; panel stays centred with 72px bottom gap (caption line box 16 vs 20) | `web/src/terminal/StatusBanner.tsx` ExitPanel `small` |
| M7 | 06 | Main head height on the exited screen | 46 (mockup head has no buttons here) | 54 (rename/close buttons, same as screen 02) | card top 74 -> 78 | consistent with screen 02 |
| M8 | 02 | xterm scrollbar | none drawn | grey scrollbar on the right of the terminal card | extra visual | xterm default viewport |
| M9 | 02, 06 | Terminal cursor | block in #242424 | block in #0f6cbd; also stays drawn after the process exited | colour | `web/src/styles/tokens.css:95` (`--term-cursor`) |
| M10 | 02 | Rail brand mark icon | mockup draws it left-aligned and dark (mockup cascade bug: `.rail-brand span {display:flex}` also matches `.mark`) | centred white icon | app is the intended design | none |

## Need a lead decision (mockup elements absent, same class as the accepted omissions)

| # | Screen | Mockup element | App | Note |
|---|---|---|---|---|
| D1 | 02 | `Ctrl Shift T` key hint in "New session" | absent | no such shortcut is implemented; showing the hint without the binding would be wrong |
| D2 | 02 | Head actions Copy / Scrollback / More | Rename / Close | placement, size (36px) and right gap (32px) match the mockup |
| D3 | 04 | Title chevron (session sheet), Hide keyboard, More | no chevron; Rename / Close / Sign out | 44px buttons, right gap 12 match |
| D4 | 02 | Per-kind glyphs (bot, eye) | console / people-team | the API does not return the command |
| D5 | 05 | Banner text "studio-mac stopped responding 40 s ago ... output will resume ..." | "Your session is still running on the host. close 1006" | copy shows a raw WebSocket close code |
