# roosterm design

## Goal
Use Claude Code and any shell program in a full terminal from a browser or the Android app, over HTTPS to your own host. Neither a client disconnect nor a roosterm restart stops the running programs.

## Non-goals
- Multiple accounts or permission levels; Windows hosts.
- Push notifications, chat UI, `claude -p` structured output, any AI-assistant feature: roosterm is a terminal shell and never parses Claude Code.
- Its own output replay buffer: hc repaints the screen on attach.

## Architecture
```
browser / Flutter WebView ──HTTPS (user's reverse proxy)──▶ roosterm (one Go binary) ──Unix socket, JSON-RPC──▶ herdr-core (hc)
```
- `host/`: login, REST for tabs, one WebSocket per tab (`/ws/tabs/{id}`), embedded web files (`go:embed`). It holds no process and keeps no tab state.
- `web/`: React + Vite + xterm.js client; the key bar and reconnect logic live here and are shared with the app.
- `app/`: Flutter WebView plus a host list; the WebView keeps the cookie and navigation is restricted to the host origin.
- hc (pinned 0.1.21, see `docs/hc-bridge.md`): owns every PTY and runs `$SHELL -l`, `claude`, `tmux -L claude-swarm-<pid> a`.
- A tab is an hc terminal labelled `roosterm=tab`; the hc terminal list is the only tab store.
- Swarm shortcut: list `claude-swarm-*` sockets under `${TMUX_TMPDIR:-/tmp}/tmux-<uid>`.
- Auth: one account, argon2id hash in `config.toml`, session cookie (HttpOnly, Secure, SameSite=Strict), session file mode 0600, failed-login rate limit in two buckets (known-device cookie vs anonymous, 429 with Retry-After), cross-origin check on state-changing requests (Go `net/http` CrossOriginProtection) and required JSON Content-Type, sessions dropped at start when the password hash changed.
- No Docker, no database, no Node at run time. Details: `docs/api.md`, `docs/hc-bridge.md`.

## Process survival
Programs live in hc, not in roosterm. Closing the browser, restarting or crashing roosterm, or SIGKILL of the hc core leaves programs and PIDs unchanged (the hc holder restarts the core). Only `hc server stop` ends them.

## Reconnect
- A reconnect is a fresh `terminal.attach`; hc sends the full screen again.
- The web client reconnects on browser `online` and `visibilitychange`, never on a timer.
- roosterm watches the hc socket directory (fsnotify) and re-dials when the socket reappears. Open WebSockets stay up and get `core` offline/online messages.
- WebSocket pings every `ping_interval` keep proxies from cutting idle connections.

## Errors
- Fail loudly, never degrade silently. Messages name the operation, the tab id, the expected and the actual value.
- Startup without a reachable hc socket exits non-zero.
- hc errors pass through to the client; a program exit closes the WebSocket with code 4000 and the exit code.
- Time is injectable, so tests use a fake clock.
