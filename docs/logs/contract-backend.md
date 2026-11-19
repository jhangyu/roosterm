# Convergence contract — Roosterm backend

Frozen 2026-11-19. Only the user may change this file.

## End state
`roosterm` (Go, `host/`) runs against a real herdr-core on macOS and Linux, authenticates one user, and bridges HTTP/WebSocket tabs to hc terminals so that processes keep the same PID across client disconnects and roosterm restarts.

## In scope
1. One-time spike (spec section "開工前的一次性測試", items 1–5) with results in `docs/logs/` and the bridge spec `docs/hc-bridge.md`.
2. `host/`: config, `roosterm passwd`, `roosterm serve`, auth (argon2id, session file, rate limit), REST tab API, swarm socket listing, WebSocket ↔ hc bridge, hc reconnect via socket file watch, embedded static assets (placeholder page until the frontend lands).
3. `scripts/test.py`, `scripts/build.py`.
4. `deploy/` launchd plist and systemd user unit.

## Out of scope
- `web/` real UI (waits for the user's mockup choice), `app/` Flutter shell.
- Multi-user, push notifications, Windows hosts.

## Acceptance criteria
1. Spike items 1–3 pass, recorded in `docs/logs/` with commands, self-captured RC, decisive output. If any fails: stop and report to the user (fallback design is the user's call).
2. `python3 scripts/test.py` exits 0, total ≤40 s, reports 0 skipped, test dirs contain no sleep-like calls.
3. Live proof on macOS: with real hc, via curl/websocat — log in, create a tab, attach over WebSocket, run `echo $$` to get the shell PID, kill `roosterm` with SIGKILL, restart it, re-attach the same tab, `echo $$` prints the same PID. Requests without a valid cookie get 401 (HTTP and WebSocket upgrade).
4. Same live proof (criterion 3) passes on Linux in an OrbStack container or VM.
5. `GET /api/swarms` lists only `claude-swarm-*` sockets from the tmux socket dir; a tab created with `tmux -L <name> a` attaches to a live swarm server.
6. `roosterm` exits non-zero with the expected socket path in the message when hc is not running.
7. `deploy/` contains a launchd plist and a systemd user unit that start `roosterm serve`.

## Round budget
2 implementation rounds (user instruction). Unmet criteria after round 2 → stop and report the failure trace.

## Parking lot
- WebSocket Origin check vs the user's reverse proxy: if the proxy rewrites Host, every upgrade is rejected. Needs the user's proxy behaviour; may need an allowed-origins config key.
- Spec update: config gained a `shell` key (launchd has no `$SHELL`); spike overrides listed in the backend plan "Spec overrides" section.
- Red runs for Tasks 2, 6, 7, 8 only proved "no implementation → build fails"; mutation checks on auth, web, api scheduled after round 2.
