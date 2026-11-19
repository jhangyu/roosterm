# Visual re-review 2: built web UI vs mockup 3 (2026-11-19)

Reviewer: review-visual-2 (fresh context). Contract criterion 5 of `docs/logs/contract-frontend.md`. Follows `docs/logs/2026-11-19-visual-review.md` (B1–B10).

- Code state: HEAD `5a6c3a5`, clean tree. Binary built with `python3 scripts/build.py` (RC=0) at that HEAD.
- Mockup: `docs/mockups/3-calm-surfaces.html` via file://. App: `roosterm serve` on 127.0.0.1:18791 against the shared hc core; three own shell sessions (t2y, t2z, t30).
- Browser: headless Chrome 158 over CDP, dependency-free Node driver. Viewports: desktop 1280x800, phone 390x844 (mobile + touch emulation). Focus emulation on, so `:focus-visible` matches like a focused window.
- Method: `getBoundingClientRect` / `getComputedStyle`, measured after two animation frames and after all finite CSS animations/transitions finished. Positions relative to the matching container: login card, rail, main column, dialog, phone screen (mockup: 36px status bar subtracted, offsets compared from both edges because the mockup screen is 370 wide), state stage / main column.
- Severity: BLOCKING = >=2px misalignment / off-centre / uneven spacing, wrong indentation, wrong font size/weight/line-height, wrong radius, missing/misplaced mockup element. MINOR = <=1px or rendering.
- Accepted deviations (not reported): footer without Latency and "Ran ...", banner without Dismiss, swarm copy, no Ctrl Shift T hint, head actions Rename/Close, phone Rename/Close/Sign out, per-kind glyphs, person-icon avatar, working dir / timers / pane counts, Sign out, core-offline and tab-not-found states.

## Result

**0 BLOCKING, 13 MINOR, 2 items for a lead decision** (rail footer copy, rail footer Settings button). All ten first-round BLOCKING items are fixed. No console errors on any screen (`console.error`, `error`, `unhandledrejection` collected from document start: empty on login, desktop, dialog, phone, exited, disconnected).

## Screenshots

| Screen | Mockup | App |
|---|---|---|
| 01 Login | `visual-review-2/01-login-mockup.png` | `visual-review-2/01-login-app.png` (Username focused) |
| 02 Desktop main | `visual-review-2/02-desktop-mockup.png` | `visual-review-2/02-desktop-app.png` |
| 03 New session dialog | `visual-review-2/03-dialog-mockup.png` | `visual-review-2/03-dialog-app.png` (Swarm selected, 28 sockets on this host) |
| 04 Phone 390px | `visual-review-2/04-phone-mockup.png` | `visual-review-2/04-phone-app.png` |
| 05 Disconnected (own roosterm SIGKILLed) | `visual-review-2/05-disconnected-mockup.png` | `visual-review-2/05-disconnected-app.png` |
| 06 Exited (`exit` typed in a shell) | `visual-review-2/06-exited-mockup.png` | `visual-review-2/06-exited-app.png` |

## First-round BLOCKING items, re-measured

| # | Item | Property | Expected (mockup) | Actual (app) | Delta | Status | Source |
|---|---|---|---|---|---|---|---|
| B1 | Mica page background | computed `background-image` | three radial gradients (blue 8% -10%, lilac 100% 110%, peach 70% 0%) | same three gradients on login, desktop and phone roots | 0 | fixed | `web/src/styles/tokens.css:40` |
| B2 | Terminal row pitch | xterm row height / pitch | desktop 20px; phone 17px | desktop 30 rows, every row 20px, pitch 20; phone 37 rows, every row 17px, pitch 17 (font 13px / 11.5px); no console error from the `rowHeightPx` spread | 0 | fixed | `web/src/terminal/metrics.ts`, `web/src/terminal/TerminalPane.tsx:120-125` |
| B3 | 20px caption line boxes | line-height / block height | rail host 20 (brand block 42), section block 36, list top 124; phone subtitle 20, title block 43 | 20 (42), 36, 124; 20, 43 | 0 | fixed | `web/src/workspace/Sessions.tsx` |
| B4 | Dialog close button | x / y in dialog; lead y | 496, 28; lead y 70 | 496, 28; lead y 70 | 0 | fixed | `web/src/workspace/NewSessionDialog.tsx` |
| B5 | Checked radio | fill / ring / dot | #fff fill, 1px #0f6cbd ring, 10px #0f6cbd dot centred | #fff fill, 1px #0f6cbd ring, 10x10 #0f6cbd dot at 5,5 in the 20px radio (tiles and socket list) | 0 | fixed | `web/src/components/ui/radio-group.tsx` |
| B6 | Swarm list with many sockets | dialog height / placement | dialog fits the viewport | 28 sockets: dialog 560x768 at y 16 (centred, 16 top and bottom), list 426px tall and scrolls inside; title, tiles, Cancel/Open on screen; list-to-actions gap 26 as in the mockup | 0 | fixed | `web/src/workspace/NewSessionDialog.tsx` |
| B7 | Banner stack when disconnected | alert count | one "Connection lost" banner, 14px above the card | one `[role=alert]` ("Connection lost"), 14px above the card | 0 | fixed | `web/src/App.tsx` |
| B8 | Exit panel Restart | background / border | #ffffff, 1px #d1d1d1 | #ffffff, 1px #d1d1d1 | 0 | fixed | `web/src/terminal/StatusBanner.tsx` ExitPanel |
| B9 | Rail footer | layout | avatar 32px, text x 66, one-line caption, footer 49px, 16px to rail bottom | avatar 32px at x 24, text x 66, caption one line (11/16), footer 49px, 16px to rail bottom; last button x 227 | 0 | fixed (copy: see lead decisions) | `web/src/workspace/Sessions.tsx:110-123` |
| B10 | Focused Username input | border colours | top/left #d1d1d1, bottom 2px #0f6cbd | top/left #d1d1d1, bottom 2px #0f6cbd | 0 | fixed | `web/src/workspace/classes.ts:17` |

## Extra checks requested

| Check | Expected | Actual | Verdict |
|---|---|---|---|
| Disconnected banner copy | "studio-mac stopped responding 40 s ago. Your session is still running on the host; output will resume from where you left off." | "127.0.0.1 stopped responding 6 s ago. Your session is still running on the host. Output will resume when the connection returns." The counter advanced from 0 to 6 s. No raw close code. | matches the mockup wording pattern; last clause reworded (MINOR M13) |
| Restart button | white fill | white fill, #d1d1d1 border (B8) | pass |
| Rail footer caption | mockup: name "jhangyu", caption "Chrome · macOS" (who is signed in, from which client) | "Signed in", caption "Sessions keep running" | geometry identical; copy is a deviation in meaning (lead decision L1) |

## Fresh pass: geometry that matches (no difference)

- 01 Login: card 420x515, centred in the viewport (430, 142.5), padding 40/40/32, radius 28, shadow-28; mark 44, title y 104 24/32 600, subtitle y 140 with 16px icon, labels y 188/272 14/20 600, inputs y 214/298 40px radius 8, eye button 36px at x 342, Sign in 44px at y 366, footer rule y 432, foot text x 64 y 451. All within 0.5px.
- 02 Desktop: rail 288, padding 24/16/16; mark 36 at 24,27; brand text x 72; section label y 94; cards at y 124/226/328, 94px, pitch 102; active card shadow-4 and 3px indicator 18px from top and bottom; New session 44px, 12px below the list; footer as B9. Main padding 24/28/28; head 54px, gaps title→badge 14 and badge→path 14; last head button 4px from the head end; terminal card radius 16, shadow-8, 28px right and bottom gap; terminal padding 22/28, xterm screen at 28,22; footer padding 10/20, first item x 48, last item 20px from the card edge.
- 03 Dialog: 560 wide, padding 28/28/24, radius 28, shadow-64; title 20/28 600 at 28,28; tiles 246x94 at x 28/286 (gap 12), padding 16 (15 selected), glyph 40, radio 20 at x 495; tiles→list label 22, label→list 8; socket rows padding 14/16, radio x 45, name x 79 13px 600; actions right-aligned, gap 8.
- 04 Phone: app bar 58px, padding 6/12/8/16, title 16/22 at 16,6.5, subtitle 12/20, title block 43, last button 12px from the right edge, 44px buttons; chips 32px at x 16/72.2, gap 8; terminal card 10px side margins, 10px above the key bar, padding 14; key bar padding 10, keys at x 10/60/130.8/201.6/251.6/301.6, divider 353.6, arrows 362.6–512.6, 44px keys.
- 05 Disconnected: banner padding 12/12/12/18, radius 16, warning fill + inset border + shadow-2, icon x 18 and vertically centred, title x 52 from the banner edge, Reconnect vertically centred (14px top/bottom in a 64px banner), 14px to the card; terminal opacity 0.45 + grayscale(1); footer Disconnected badge at x 48, last item 20px from the edge. The banner text wraps to two lines (banner 64 vs 60) because the main column is 936 wide next to the rail, while the mockup state stage has no rail (1184 wide).
- 06 Exited: exit panel horizontally centred (offset 0), 72px above the card bottom, padding 14/14/14/20, radius 16, shadow-16; Restart and Close session 36px, 8px radius, gap 16 group spacing as in the mockup; footer items as on screen 02.

## MINOR differences

| # | Screen | Element | Property | Expected | Actual | Delta | Source |
|---|---|---|---|---|---|---|---|
| M1 | 02 | Session card content (glyph, name, badge) | x / y in card | glyph 12,12; badge y 60 | glyph 13,13; badge y 59 | ±1px (1px transparent Button border; meta is block 16px vs mockup inline 14px) | `web/src/workspace/Sessions.tsx:78-100` |
| M2 | 03 | Dialog "N found" note | line-height | 20 | 16 | 0px visual (baseline aligned) | `web/src/workspace/NewSessionDialog.tsx` list label |
| M3 | 01 | Sign in button text | line-height | 20 | 22 | 0px (fixed 44px, centred) | `web/src/screens/LoginScreen.tsx` |
| M4 | 02, 06 | Badge | padding | 0 10px | 2px 10px | 0px (fixed 22px height) | `web/src/components/ui/badge.tsx` |
| M5 | 06 | Exit panel | height | 90 | 88 | -2px, still centred with 72px bottom gap (caption line 16 vs 20) | `web/src/terminal/StatusBanner.tsx` ExitPanel `small` |
| M6 | 06 | Main head on the exited screen | height | 46 (no buttons in the mockup stage) | 54 (Rename/Close as on screen 02) | card top 74 → 78 | consistent with screen 02 |
| M7 | 02, 05 | xterm scrollbar | presence | none | grey scrollbar track at the right of the terminal | extra visual | xterm default viewport |
| M8 | 02, 06 | Terminal cursor | colour / after exit | #242424 block | #0f6cbd block, still drawn after the process exited | colour | `web/src/styles/tokens.css` `--term-cursor` |
| M9 | 02 | Rail brand mark icon | placement | mockup draws it left/dark (mockup cascade bug) | centred white icon | app is the intended design | none |
| M10 | 03 | Dialog lead text | copy / lines | "…on studio-mac, or attach read-only to a running Claude swarm." (2 lines) | "…on 127.0.0.1, or attach to a running Claude swarm." (1 line) | everything below 20px higher; all spacings identical | `web/src/workspace/NewSessionDialog.tsx` |
| M11 | 03 | Dialog close button on open | focus ring | none | ring drawn (Radix auto-focus; the driver opened the dialog with a script click, so `:focus-visible` may not match after a real mouse click) | rendering, unverified with a real pointer | `web/src/components/ui/dialog.tsx` |
| M12 | 06 | Exit panel caption | copy | "The shell ended normally. Scrollback is kept until you close this session." | "Scrollback is kept until you close this session." | first sentence missing, panel 468 vs 592 wide, still centred | `web/src/terminal/StatusBanner.tsx` ExitPanel |
| M13 | 05 | Banner detail | copy | "…; output will resume from where you left off." | "… Output will resume when the connection returns." | wording | `web/src/terminal/StatusBanner.tsx` disconnected case |

## Lead decisions (not counted as BLOCKING)

| # | Screen | Mockup element | App | Note |
|---|---|---|---|---|
| L1 | 02, 03, 05, 06 | Rail footer name "jhangyu" + caption "Chrome · macOS" (who is signed in and from which client) | "Signed in" + "Sessions keep running" | Geometry matches (B9). The caption is a different message: the mockup states the client, the app states a policy. A client caption can be derived in the browser (user agent); the username has no endpoint. Accept as a copy deviation or ask for the client caption. |
| L2 | 02, 03, 05, 06 | Rail footer Settings button (gear, 36px, left of Sign out) | absent; Sign out sits at the mockup's Sign out position (x 227) | No settings feature exists, so a gear would be a dead control. Same class as the accepted "no Ctrl Shift T hint". By the strict rubric this is a missing mockup element; I recommend accepting it with the other omissions. |
