# Live web acceptance L1-L7 (2026-11-19)

HEAD 779b72b6ce89f5cdfbcd95cd5459b922798b5f64, working tree clean before and after (`git status --porcelain` empty).
Setup: `build/roosterm` built by `python3 scripts/build.py`, config from `roosterm passwd` (user `me`), `listen = "127.0.0.1:18787"`, run as `hc run --bg --name verify-web build/roosterm serve --config ...` against the running hc core. Browser: desktop Google Chrome 158 (headless=new) driven over the Chrome DevTools Protocol by a dependency-free node script (nothing added to `web/package.json`). Desktop = 1440x900. Phone = CDP device emulation 390x844, mobile, touch. Each driver script's exit code was captured right after it (`RC=$?` into its log). Wait signals used: `hc wait`, DOM MutationObserver, CDP events. Real-time waits only for the 60 s login lockout (business time).
Note: `hc wait --output listening` returned reason `timeout`/`output` without a "listening" line in roosterm's output, but the next `curl /` returned 200, so readiness was confirmed by the curl (HTTP 200).

## L1 build + serve: PASS
- `python3 scripts/build.py` -> `wrote build/roosterm`, RC=0.
- `ls host/internal/web/dist/index.html` -> exists (427 bytes, references `/assets/index-M507KUvG.js` and `/assets/index-CRwOrni3.css`).
- `git status --porcelain host/internal/web/dist` -> empty. `git status --porcelain` (whole tree) -> empty.
- `curl http://127.0.0.1:18787/` -> HTTP 200, body is the built `<!doctype html>... <title>Roosterm</title> ... /assets/index-*.js`. Chrome rendered the login screen from it.

## L2 login / lockout / logout: PASS
- Login screen text: `Sign in to Roosterm | 127.0.0.1:18787 · terminal portal | Username | Password | Sign in | Your sessions keep running on this host after you sign out. Sign-in is rate limited.`
- Wrong password x5 (each time after clean limiter state): alert `Wrong username or password. login: HTTP 401 unauthorized: login: wrong username or password` (contains `unauthorized` plus the server message). 6th attempt: `Too many attempts. Try again in 60 seconds. login: HTTP 429 too_many_attempts: login: too many failed logins: retry in 60 s`. (Limiter: 5 failures -> 60 s lockout, `host/internal/auth/ratelimit.go`.) An earlier aborted driver run had already triggered one lockout, the counted sequence above is the clean re-run, a second wrong attempt during lockout also returned 429 with the remaining seconds.
- After the lockout expired, correct password -> workspace: `Roosterm | 127.0.0.1 | SESSIONS · 4 | tmux t13 Exited (code 1) | zsh t1b Running | Claude Code t1c Running | build t1d Running | New session | Signed in | Sessions keep running after sign-out`. Cookie: `roosterm_session` HttpOnly, Secure, SameSite=Strict, set on http://127.0.0.1.
- Sign out (button `Sign out`) -> login screen, cookies for the origin `[]`. `Page.reload` -> still the login screen.

## L3 new tab / shell / claude: PASS
- `New session` dialog text: `New session | Start a fresh shell on 127.0.0.1, or attach to a running Claude swarm. | Shell | New login shell on this host | Swarm | Attach to a running tmux swarm socket | Cancel | Open shell`. Open shell -> tab t1f, prompt `jhangyu@JhangyudeMac-Studio-3 ~ %`.
- `echo $$` -> `51552`. `cd /tmp && pwd` -> `/tmp`, prompt became `... /tmp %`.
- `claude` -> Claude Code TUI rendered in the pane: yellow rule, `Accessing workspace: /private/tmp`, `Quick safety check: Is this a project you created or one you trust? ...`, `❯ No, exit`, `Yes, I trust this folder`, `Enter to confirm · Esc to cancel`; footer `Connected 131 × 27 UTF-8`. Enter on `No, exit` returned to the shell prompt.

## L4 key bar (phone viewport 390px; the bar is `md:hidden`): PASS
- Bar buttons in order: `Escape, Tab, Shift+Tab, Ctrl, C, D, Left, Up, Down, Right`.
- `cat -v`, typed `X`, then bar Escape, Tab, Shift+Tab, Up, Down, Left, Right, then `Y` + Enter: output `X^[<TAB>^[[Z^[[A^[[B^[[D^[[CY` (Esc `^[`, Tab a literal tab, Shift+Tab `^[[Z`, arrows `^[[A ^[[B ^[[D ^[[C`).
- Ctrl (aria-pressed true while latched) then C on the running `cat -v`: `^C` and the prompt returned, Ctrl unlatched afterwards (aria-pressed false). `sleep 100` typed in the remote shell, Ctrl + C: `^C`, prompt returned, `echo $$` -> `51552` (same shell).
- Ctrl then D on the empty prompt: overlay `Exited (code 0) | Scrollback is kept until you close this session. | Restart | Close session`, all key bar buttons disabled. Click `Close session`: tab t1f removed from the rail and from `GET /api/tabs` (list afterwards t13,t1b,t1c,t1d). (The driver script l4b exited RC=1 only because its last assertion wrongly expected no exit panel while the pane had switched to the already-exited tab t13, the removal itself was confirmed by the rail, 4 sessions, and the API.)

## L5 disconnect / reconnect: PASS (network toggle emulated)
- New shell tab t1j, `echo $$` -> `52986`.
- `kill -9 47890` (the roosterm process listening on 18787, RC=0): banner `Connection lost` / `Your session is still running on the host. close 1006`, `Reconnect` button, footer `Disconnected | Last output 21:10:50 | ... | Input paused until reconnected`. A second alert `list tabs failed ... network error: Failed to fetch` also showed (see defects/observations).
- Restarted as `hc run --bg --name verify-web2 ...` (curl 200), clicked `Reconnect`: pane live, footer `Connected`, `echo $$` -> `52986` (same PID), earlier scrollback redrawn.
- Auto-reconnect: `kill -9 53382` again, banner `Connection lost` shown, restarted (`verify-web3`, curl 200), then CDP `Network.emulateNetworkConditions` offline then online (fires the window `online` event) with no click on Reconnect: banner cleared by itself, `echo $$` -> `52986`. Limitation: the machine's real network was not toggled, the offline/online event was emulated through CDP.

## L6 swarm: PASS
- Started `hc run --bg --name verify-swarm tmux -L claude-swarm-verify new-session -s s`.
- Swarm choice in New session: list header `28 found`, DOM list equals `GET /api/swarms` (28 names, same order), every name starts with `claude-swarm-`, includes `claude-swarm-verify`.
- Picked `claude-swarm-verify` -> `Open swarm`: new tab t1h labelled `claude-swarm-verify`, header `claude-swarm-verify | Running | t1h`, pane shows tmux status line `[s] 0:zsh*   "JhangyudeMac-Studio-3" 21:10 19-11-26`.

## L6b hc core restart: PASS (run last)
- Phone viewport, tab t1j open (`echo $$` -> 52986), MutationObserver armed in the page, `review-visual-opus` told CORE_RESTART_NOW / CORE_RESTART_DONE.
- `hc server restart` RC=0 (core kept the terminals, holder terminals 9).
- Observed during the restart: banner `Host core offline | Input is paused until the core is back online.`, footer `Core offline | Last output 21:11:27 | 131 × 27 | Input paused until reconnected`, all key bar buttons disabled at that moment (`offlineKeybarDisabled: true`), and `connectionLost: false` (the socket never showed `Connection lost`, it stayed open).
- Banner then cleared by itself, key bar enabled again, footer `Connected`, screen redrawn, `echo $$` -> `52986` (same PID as before).

## L6c killed tab from a second window: PASS
- Window 1 on tab t1h (swarm). Window 2 (second page, same cookies): selected t1h, X (`Close session`) -> confirm `Close “claude-swarm-verify”? | Closing the session ends the program running in it.` -> `Close session`.
- Window 1 pane switched to the exit panel `Exited (code 1) | Scrollback is kept until you close this session. | Restart | Close session`, footer `Ended 21:10:34 ... Read-only`. Text `Protocol error` absent. Window 1's rail still listed t1h as exited, while `GET /api/tabs` no longer contained it.

## L6d unknown tab: PASS
- From the page: `new WebSocket('ws://127.0.0.1:18787/ws/tabs/nope?cols=80&rows=24')` -> close `{"code":4004,"reason":"tab nope not found","clean":true}`.

## L7 grep gates: PASS
- `grep -rnE "setInterval|setTimeout|sleep\(" web/src app/lib` -> no output, grep RC=1 (no match).
- `grep -rnE "#[0-9a-fA-F]{3,8}\b|oklch\(|rgb\(" web/src --include='*.tsx' --include='*.ts'` -> no output, RC=1.
- `grep -rni lucide web` (excluding node_modules, dist) -> no files, RC=1. `web/package.json` has no `lucide`, no other icon package.
- Fluent 2 check: the only file with `<svg` is `web/src/components/icons.tsx` (28 icons). A script compared each icon's path string with `~/ui-kits/icons/fluent2/regular-20/*.svg`: 28 of 28 match, 0 not in pack.

## Defects / observations
- None blocking. Observation: after the roosterm kill, an extra alert `list tabs failed list tabs: HTTP 0: network error: Failed to fetch` appears above the `Connection lost` banner (raw network error text, not mockup-style copy).
- Observation: in L6c the closing window keeps showing the removed tab in the rail as `Exited (code 1)` until the user clicks `Close session` (which then closed a tab that no longer existed). In my run that click removed the next active tab t13, a stale exited `tmux` tab left from the earlier backend gate (not created by this run; it was already `exited`, no running program).

## Cleanup
Tabs created by this run: t1f (closed in L4), t1h (removed in L6c), t1j (`DELETE /api/tabs/t1j` -> 204). Remaining tabs t1b, t1c, t1d are not mine and were not touched. roosterm killed (last one `hc kill verify-web3`), `tmux -L claude-swarm-verify kill-server` RC=0 (`no server running` afterwards), verify-chrome killed, ports 18787 and 9333 no longer listening. hc core left running, `hc server stop` never run.
