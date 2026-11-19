# Convergence contract — Roosterm frontend

Frozen 2026-11-19. Only the user may change this file (the lead may append to the parking lot).

## End state
The web UI in `web/` (embedded in the `roosterm` binary) and the Flutter Android shell in `app/` let a user log in, open, use, and close terminal tabs against the real backend, and the web UI matches mockup 3 (`docs/mockups/3-calm-surfaces.html`) in layout, alignment, spacing, and indentation.

## Source of truth (user decision: mockup 3, "以 mockup 為主")
Where the client plan (`docs/superpowers/plans/2026-11-19-roosterm-client.md`) and mockup 3 differ, mockup 3 wins:
1. Layout: mockup 3's Mica page, Acrylic session rail with session cards (desktop), session chips (phone), terminal as one focused card, dialog, banners. Not the plan's top tab bar.
2. Tokens: colors, radius scale, type scale, font stacks are taken from mockup 3's `:root` block into `web/src/styles/tokens.css`.
3. Icons: the Fluent 2 pack (`~/ui-kits/icons/fluent2`, regular-20), the same icons mockup 3 uses. lucide is removed: no `lucide-react` dependency; shadcn components that import lucide are edited to use the Fluent 2 icons. One pack in the whole UI.
4. UI copy: English, as in mockup 3. States the mockup does not show (core offline, tab not found, protocol errors, lockout) use English in the same voice.
5. Key bar behaviour: as in mockup 3 (latching Ctrl, then C or D).

## In scope
- WP1–WP4 of the client plan (web), adjusted by the section above.
- WP5 Flutter app (`app/`).
- Round-level live acceptance L1–L7 of the client plan, run by a fresh verifier.
- Visual review: a fresh frontend reviewer compares the built UI with mockup 3.

## Out of scope
- Backend changes (`host/`), except deleting the `host/internal/web/dist/index.html` placeholder as the plan's WP1 says.
- Dark theme, push notifications, iOS.

## Acceptance criteria
1. `python3 scripts/build.py` RC=0; the built binary serves the web UI at `/`; `git status --porcelain` is empty after the build.
2. Live acceptance L1–L6d of the client plan pass against real `roosterm serve` + hc, recorded in `docs/logs/` (action, observed result, HEAD hash), with UI copy read as the English mockup-3 wording.
3. Grep gates: no `setInterval|setTimeout|sleep(` in `web/src` and `app/lib`; no color literals in `.ts/.tsx`; no `lucide` anywhere in `web/` (package.json, imports); every inline SVG icon comes from the Fluent 2 pack.
4. `flutter analyze` clean and `flutter build apk` RC=0 for `app/`; the APK installs on the attached Android device and shows the host list.
5. Visual review by a fresh frontend reviewer: for each of mockup 3's six screens (login, desktop main, new-session dialog, phone 390px, disconnected, exited), side-by-side screenshots of mockup vs built UI at the same viewport; every element aligned, centred, evenly spaced, and indented as in the mockup. Differences are listed with element, expected (mockup measurement), actual; the round closes when the reviewer reports no blocking difference.

## Round budget
2 rounds: round 1 = implementation + live acceptance + visual review; round 2 = fixes for blocking review findings only. Unmet criteria after round 2 → stop and report the failure trace.

## Parking lot
- WebSocket Origin check vs the user's reverse proxy (carried over from the backend contract): needs the user's answer on whether the proxy keeps the Host header.
- WP5 live load of a real host URL needs the user's HTTPS proxy URL.
