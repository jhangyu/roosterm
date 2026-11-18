# Roamgate research (as of 2026-11-19)

## Identity

| Candidate | URL | Stars | Last push | License | Purpose |
|---|---|---|---|---|---|
| **powerfooI/roamgate (chosen, upstream)** | https://github.com/powerfooI/roamgate | 328 (gh api) | 2026-11-18 (gh api; clone HEAD bf3879f, 2026-11-17) | MIT | Browser/PWA client for **Herdr** (terminal multiplexer for coding agents). Created 2026-08-07. Site: https://roamgate.dev |
| Epsirom/roamgate, piqusy, nmindz, dinamicapps, srp, shayani, caio-withhexagon | github.com/<user>/roamgate | not checked | not checked | not checked | Forks/mirrors of the same project (WebSearch result titles only) |
| herdrdev/herdr (the required backend, separate project) | https://github.com/herdrdev/herdr | 51,722 | 2026-11-07 | Apache-2.0 | Rust terminal multiplexer, "the runtime your coding agents live on" (GitHub API, README) |

No other project named Roamgate found. The name does not refer to a Claude-Code-specific portal: it is a client of Herdr. Unrelated Claude-Code-on-Android projects surfaced by search (not examined, details unverified): ishaquehassan/claude-remote-terminal (Flutter, WebSocket PTY), karan-suthar03/claude-remote (Tailscale + APK).

## Architecture (source: roamgate-src/docs/ARCHITECTURE.md)
- Browser (React+Vite, xterm.js) <-same-origin HTTP/WebSocket-> Bridge (Bun+TS, one compiled executable) <-Unix sockets-> **Herdr server** (`herdr.sock` NDJSON control, `herdr-client.sock` binary terminal traffic). ARCHITECTURE.md:9-20, 410-413.
- Herdr owns the PTYs and renders the terminal surface; xterm just displays what Herdr rendered (ARCHITECTURE.md:20, 73). Kitty graphics not shown (:96).
- No third-party relay or account found; auth is local password/token, cookie 30 days (SECURITY.md:19-33). Remote hosts via SSH tunnel runtimes (ARCHITECTURE.md:346-361), optional.
- Disconnect: browser close leaves Herdr untouched; endpoint stream stays warm, replays last frame (ARCHITECTURE.md:35-37, 75-79). Herdr README: server keeps terminals running when client closes; after server/machine restart "the original processes do not survive" (https://github.com/herdrdev/herdr README).
- Extras beyond terminal: files, git diff/review, agent chat view, "Ranger" assistant that runs `claude -p` (ARCHITECTURE.md:263-285), Web Push.
- Android: PWA only (README.md:137-145); no native app found.

## Grading
| Req | Grade | Evidence |
|---|---|---|
| R1 one binary + host tools | **partial** | Roamgate is one binary (ARCHITECTURE.md:410), but requires a running Herdr server, a second Rust binary run as user service (README.md:16; DEPLOYMENT.md:76-83). No Docker. |
| R2 HTTP(S)/WS via own reverse proxy | **yes** | Same-origin HTTP/WS (ARCHITECTURE.md:11); reverse-proxied clients explicitly mentioned (DEPLOYMENT.md:255; SECURITY.md:62-64). No ready proxy config/WS-header docs found (未查到). |
| R3 browser + Android app | **partial** | Browser yes; Android via PWA only (README.md:137-145). No Flutter/native app. |
| R4 full TUI fidelity + arbitrary shell | **partial / unverified** | Panes are real terminals (herdr README: "doesn't wrap or replace [agents]; it owns their terminals"). But there is an extra emulator layer (Herdr renders, xterm displays; ARCHITECTURE.md:73, 96). Claude Code TUI fidelity through it not tested. |
| R5 multi-tab; `tmux -L claude-swarm-<pid> a` | **partial / unverified** | Tabs/panes/workspaces yes (FEATURES.md:7-30). Nested tmux inside a Herdr pane: 未查到 in Roamgate docs/code or Herdr docs; Roamgate source mentions no tmux. Likely works (a pane is a real terminal) but untested. |
| R6 same PID after disconnect; survives host server restart | **partial** | Client disconnect: yes (ARCHITECTURE.md:75-79, Herdr README). Roamgate bridge restart: yes, "Disconnecting stops the runtime... never Herdr" (ARCHITECTURE.md:37). **Herdr server restart/crash: no**, processes do not survive (Herdr README). Whether Herdr itself is as robust as needed to keep the claude PID alive: unverified. |
| R7 not its own AI harness | **partial** | Terminal around agents, yes; but ships Ranger assistant (spawns `claude -p`), chat view, git review (ARCHITECTURE.md:263-285). Ranger is described as experimental; whether it can be disabled: 未查到. |

## Contradictions with the user's description
- Roamgate is not a standalone Claude-Code remote portal; it is a front-end that needs Herdr as the session layer. The "one binary" claim holds only if Herdr counts as an existing host tool; it is not a typical host tool.
- Session survival relies on Herdr, and Herdr's own restart kills processes: the swarm-lead constraint (lead claude must never die) depends on Herdr server uptime.

## Recommendation
**(a) Use as-is, but only after a one-hour spike; otherwise (c) borrow ideas.** Roamgate covers R2, most of R3/R4/R5/R6 in practice and is MIT, active, single-user, no relay. It fails R1/R3/R7 strictly (needs Herdr, PWA not native, extra features). Spike pass criteria (not run, research only): (1) start `claude` in a Herdr pane via Roamgate, check TUI rendering; (2) in another tab run `tmux -L claude-swarm-<pid> a`; (3) close browser, reopen, compare `claude` PID; (4) restart the Roamgate service, compare PID. If 1-4 pass, Roosterm is unnecessary. If the user insists on strict R1/R3/R7 (no Herdr dependency, native app), Roamgate is not suitable as-is and its bridge/PWA design is only reference material.

Confidence: medium (documentation and architecture read; nothing built or run; Herdr internals read only from README/summary pages).

## Sources
- https://github.com/powerfooI/roamgate (cloned, commit bf3879f); README.md, FEATURES.md, SECURITY.md, docs/ARCHITECTURE.md, docs/DEPLOYMENT.md in `scratch/research/roamgate-src`
- GitHub API repos/powerfooI/roamgate, repos/herdrdev/herdr (2026-11-19)
- https://github.com/herdrdev/herdr README; https://herdr.dev/docs
- WebSearch results 2026-11-19 (forks, other projects)
