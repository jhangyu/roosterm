# Contract: web UI tweaks (2026-11-20)

End state: the web terminal is dark grey with macOS Terminal colours, scrolls by touch on phones, tabs can be hidden and re-shown without killing the process, and the phone key bar shows the arrows before Tab.

## In scope
1. Terminal theme: dark grey background, ANSI 16 palette matching macOS Terminal.app ("Basic"/"Pro"-style dark), set in `web/src/styles/tokens.css` `--term-*` variables. Chrome outside the terminal keeps its current light design.
2. Touch scrolling: a one-finger vertical drag on the terminal scrolls it on phones (normal buffer: scrollback; alternate buffer / mouse-tracking apps: wheel events, as xterm.js does for a mouse wheel).
3. Hide tab: each tab gets a "close" control (X) that hides the tab only; the process keeps running on the host. Hidden state is stored server-side as a terminal label so every device agrees. The New-session dialog lists hidden sessions and can re-show one. Existing "Close session" (kill) stays.
4. Key bar order: Esc, Left, Up, Down, Right, divider, Tab, Shift+Tab, Ctrl, C, D.

## Out of scope
Flutter app code changes, new keys, light-theme toggle, restyling non-terminal chrome.

## Acceptance
1. `--term-background` is a dark grey (not white); all 16 `--term-*` colours and fg/cursor/selection are defined; text stays readable (fg vs bg contrast >= 7:1).
2. On a touch device (or Chrome devtools touch emulation) a vertical drag over the terminal scrolls scrollback; tap-to-focus still opens the keyboard.
3. Hide → tab disappears from the tab strip on all clients after list refresh; process still listed by host core; New-session dialog shows it; re-show → tab back and attached with live output. Go test covers hide/unhide label round-trip and list filtering. `python3 scripts/test.py` green.
4. KeyBar renders in the order above.
5. `scripts/build.py` succeeds on the combined state on main.

Round budget: 3.
