# roosterm

*[繁體中文版本 / Traditional Chinese](README.zh-TW.md)*

roosterm is a self-hosted web terminal: it gives you full terminals (a login shell, Claude Code, any program) in a browser or in an Android app, over HTTPS to your own machine. Closing the browser or restarting roosterm does not stop the running programs.
<!-- evidence: docs/design.md:4 goal statement; host/cmd/roosterm/main.go:19-22 the two subcommands passwd and serve; app/README.md:3 Android WebView shell -->

roosterm is a terminal shell only: it never parses Claude Code, and has no chat UI, push notifications, multiple accounts or Windows host support.
<!-- evidence: docs/design.md:6-9 non-goals -->

### Key Features

- **Programs outlive clients and the server.** Every PTY is owned by herdr-core (`hc`), not by roosterm. A browser close, a roosterm restart or crash, or a killed hc core leaves programs and PIDs unchanged; only `hc server stop` ends them.
  <!-- evidence: docs/design.md:24-25; host/cmd/roosterm/serve.go:57 roosterm only dials hc through hc.Start -->
- **No tab state in roosterm.** A tab is an hc terminal carrying the label `roosterm=tab`; the hc terminal list is the only tab store. Terminals created outside roosterm are invisible to it.
  <!-- evidence: docs/design.md:19; docs/api.md:9 -->
- **Three kinds of new session.** Shell, Claude Code, or attach to a running Claude Code swarm (a `claude-swarm-*` tmux socket).
  <!-- evidence: web/src/workspace/NewSessionDialog.tsx:25 type Kind = "shell" | "claude" | "swarm"; docs/api.md:50 GET /api/swarms; web/src/workspace/tabs.ts:22 CLAUDE_PROGRAM -->
- **Mobile-friendly key bar.** Esc, Tab, Shift-Tab and the arrow keys are sent as terminal sequences, honouring application-cursor mode.
  <!-- evidence: web/src/lib/keys.ts:1-16; web/src/terminal/KeyBar.tsx -->
- **Reconnect without timers.** A reconnect is a fresh attach and hc repaints the whole screen. The web client reconnects when the browser comes back online or the page returns to the foreground; the server re-dials hc when its socket reappears.
  <!-- evidence: web/src/lib/resume.ts:1-10; docs/design.md:27-31; host/internal/hc/supervisor.go:11,34 fsnotify watcher -->
- **Single self-contained binary.** The web client is embedded in the Go binary; no Docker, database or Node is needed at run time.
  <!-- evidence: host/internal/web/static.go:15 //go:embed all:dist; docs/design.md:22 -->
- **Folder picker and tab management.** Browse sub-directories under your home to start a tab there; rename a tab or hide it from the tab strip while its process keeps running.
  <!-- evidence: docs/api.md:49 GET /api/dirs, :47 PATCH /api/tabs/{id} (label, hidden); web/src/workspace/FolderPicker.tsx -->

### Contents

- [Architecture](#architecture)
- [Requirements](#requirements)
- [Build](#build)
- [First-time setup](#first-time-setup)
- [Configuration](#configuration)
- [Run as a service](#run-as-a-service)
- [Using the clients](#using-the-clients)
- [Security notes](#security-notes)
- [API](#api)
- [Development and tests](#development-and-tests)
- [Project layout](#project-layout)

### Architecture

```
browser / Flutter WebView ──HTTPS (your reverse proxy)──▶ roosterm (one Go binary) ──Unix socket, JSON-RPC──▶ herdr-core (hc)
```

- `host/` is the Go server: login, REST for tabs, one WebSocket per tab (`/ws/tabs/{id}`), and the embedded web files. It holds no process and keeps no tab state.
- `web/` is a React + Vite + xterm.js client. The key bar and reconnect logic live here and are shared with the app.
- `app/` is a Flutter WebView shell plus a host list; the WebView keeps the session cookie.
- hc (pinned to 0.1.21) owns every PTY and runs your shell, `claude`, or `tmux a` for swarm tabs.
<!-- evidence: docs/design.md:11-22; docs/hc-bridge.md:1 hc 0.1.21; web/package.json:15-16 react and @xterm/xterm dependencies -->

roosterm does not terminate TLS itself: it listens on plain HTTP (default `127.0.0.1:8787`) and expects a reverse proxy for HTTPS.
<!-- evidence: host/internal/config/config.go:17 DefaultListen; host/cmd/roosterm/serve.go:71 net.Listen("tcp") with no TLS; docs/api.md:15 "the reverse proxy terminates TLS" -->

### Requirements

- A running herdr-core (`hc` 0.1.21) on the host. roosterm exits non-zero at startup if the hc socket is unreachable. roosterm does not install hc.
  <!-- evidence: docs/design.md:18,35; deploy/README.md:3 -->
- macOS or Linux host (Windows hosts are a non-goal). Service scripts exist for launchd (macOS) and systemd (Linux).
  <!-- evidence: docs/design.md:7; scripts/service.py:2,176-177; deploy/systemd/roosterm.service -->
- To build: Go 1.26.4 (`host/go.mod`), Node and npm for the web client, Python 3 for the scripts.
  <!-- evidence: host/go.mod:3; scripts/build.py:39-45 npm ci, npm run build, go build -->
- To build the Android app: the Flutter SDK (Dart `^3.12.2`).
  <!-- evidence: app/pubspec.yaml:8; app/README.md:5-8 -->

### Build

```sh
python3 scripts/build.py                        # build/roosterm for this machine
python3 scripts/build.py --target linux/arm64   # build/roosterm-linux-arm64
```

The script runs `npm ci` and `npm run build` for the web client, then `go build -trimpath` with `CGO_ENABLED=0`. `--target` takes `GOOS/GOARCH`.
<!-- evidence: scripts/build.py:3-5,29,31,37-45 -->

Android app (debug-signed, for sideloading):

```sh
cd app
flutter analyze
flutter build apk
```
<!-- evidence: app/README.md:5-8; docs/logs/2026-11-21-refactor2.md "Android release stays debug-signed" -->

### First-time setup

1. Install `hc` and `roosterm` into `~/.local/bin` (the service units expect that path).
2. From your login shell run `roosterm passwd`. It prompts for a username, a password and a repeat, hashes the password with argon2id, and writes `~/.config/roosterm/config.toml` (mode 0600) with defaults and your current `$SHELL`.
3. Run `roosterm serve`, or install the service below.

<!-- evidence: deploy/README.md:3; host/cmd/roosterm/passwd.go:38-64 prompts, hash, config.Save; host/cmd/roosterm/passwd.go:76-81 defaults and $SHELL; host/internal/config/config.go:119-120 Save writes mode 0600 -->

Both subcommands accept `--config PATH`; the default is `$XDG_CONFIG_HOME/roosterm/config.toml`, else `~/.config/roosterm/config.toml`. Running `passwd` on an existing config replaces only the username and hash.
<!-- evidence: host/cmd/roosterm/main.go:47; host/internal/config/config.go:50-57; host/cmd/roosterm/passwd.go:69-71,62-63 -->

### Configuration

`config.toml` keys (all required except `hc_socket`; unknown keys are rejected at load):

| Key | Default written by `passwd` | Meaning |
|---|---|---|
| `listen` | `127.0.0.1:8787` | HTTP listen address |
| `hc_socket` | empty | hc socket path; empty means hc's own default location rule |
| `ping_interval` | `25s` | WebSocket ping interval, keeps proxies from cutting idle connections |
| `session_ttl` | `720h0m0s` | Session cookie and session lifetime |
| `shell` | your `$SHELL` | Absolute path to an executable; used as `shell -l` for new shell tabs |
| `username` | set by `passwd` | The single login user |
| `password_hash` | set by `passwd` | argon2id hash |

<!-- evidence: host/internal/config/config.go:17-19 defaults, :39-45 toml keys, :71-72 unknown keys rejected, :105-108 absolute executable shell; docs/api.md:120-130 example file; docs/api.md:70 command omitted => [config.shell, "-l"] -->

Sessions are stored in `sessions.json` next to the config file (mode 0600).
<!-- evidence: host/internal/config/config.go:61 SessionsPath; host/internal/auth/session.go:23,55 -->

### Run as a service

Neither service ever stops hc: **never run `hc server stop` on a live host, it ends every program in every tab.**
<!-- evidence: deploy/README.md:5; scripts/service.py:11 -->

macOS (launchd), after putting `hc` and `roosterm` in `~/.local/bin`:

```sh
python3 scripts/service.py install    # render the plist into ~/Library/LaunchAgents and load it
python3 scripts/service.py status     # loaded, pid, last exit code, whether the port answers
python3 scripts/service.py restart    # also: start, stop, uninstall
```

The log is `~/Library/Logs/roosterm.log`. The plist starts a login shell (`/bin/zsh -lc`); if yours is not zsh, edit `deploy/launchd/dev.roosterm.serve.plist` and run `install` again.
<!-- evidence: scripts/service.py:4-9,22,26,27,92-94; deploy/README.md:9-17 -->

Linux (systemd user units; `herdr-core.service` runs `hc server start`, `roosterm.service` requires it):

```sh
mkdir -p ~/.config/systemd/user
cp deploy/systemd/herdr-core.service deploy/systemd/roosterm.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now herdr-core.service roosterm.service
loginctl enable-linger "$USER"   # start at boot without a login session
```

Log: `journalctl --user -u roosterm.service`.
<!-- evidence: deploy/README.md:21-30; deploy/systemd/herdr-core.service:6-7; deploy/systemd/roosterm.service:2-8 -->

### Using the clients

**Web client.** Open the HTTPS URL of your reverse proxy and sign in. The sidebar lists your sessions; the new-session dialog offers Shell, Claude, or Swarm and a folder picker. Sessions can be renamed or hidden, and a finished session can be restarted or closed. Closing a tab kills a live process.
<!-- evidence: web/src/screens/LoginScreen.tsx; web/src/workspace/Sessions.tsx; web/src/workspace/NewSessionDialog.tsx:25-31; docs/api.md:79-81 DELETE kills live then removes; web/src/terminal/TerminalPane.tsx -->

**Android app.** Add a host (a name and an `https://` URL; plain `http` is rejected because the session cookie is `Secure`) and open it in the WebView. Android only.
<!-- evidence: app/lib/hosts.dart:10-20; app/lib/host_page.dart:26-27 WebViewController; app/README.md:3 -->

### Security notes

- One account, argon2id password hash in `config.toml`. Login rate limit: 5 failures lock `/login` for 60 s (HTTP 429 with `Retry-After`); the limit is global, not per IP, and a successful login resets it.
  <!-- evidence: host/internal/auth/ratelimit.go:8-10,13,23-24; docs/api.md:85-86 -->
- Session cookie `roosterm_session`: HttpOnly, Secure, SameSite=Strict; the server stores only the SHA-256 of the token. The sessions file must be mode 0600 or the server refuses it.
  <!-- evidence: host/internal/auth/cookie.go:10-12; docs/api.md:12,13; host/internal/auth/session.go:55 -->
- What is exposed: tab REST endpoints, one WebSocket per tab, and directory listing under your home only (paths resolving outside home, including through symlinks, are refused with 403). Anyone who logs in gets a full shell as the roosterm user, so put it behind HTTPS and a strong password.
  <!-- evidence: docs/api.md:30,83 outside_home -->
- Programs started in a tab get a scrubbed environment: `TMUX`, `TMUX_PANE` and `CLAUDE*` variables inherited from the hc core are unset.
  <!-- evidence: host/internal/api/scrub.go:5-9 -->

### API

The HTTP and WebSocket contract (endpoints, cookie, error body, close codes) is in [docs/api.md](docs/api.md). The hc protocol notes are in [docs/hc-bridge.md](docs/hc-bridge.md) and the design summary in [docs/design.md](docs/design.md).
<!-- evidence: docs/api.md:1-3; docs/hc-bridge.md:1-3; docs/design.md:1 -->

### Development and tests

```sh
python3 scripts/test.py                          # gofmt, go vet, go test -race, with time budgets
cd web && npx tsc -b && npx oxlint src           # web type check and lint
cd app && flutter analyze                        # Android shell analysis
```

`scripts/test.py` fails if sleep-like text appears in test code, any test exceeds 1 s, the suite exceeds 40 s, or any test is skipped. The web client has no committed tests.
<!-- evidence: scripts/test.py:14-20,28-37,54-63,93-106; web/package.json:10 lint script; docs/logs/2026-11-21-refactor2.md "Evidence" section; docs/logs/2026-11-21-refactor2.md "no committed web test" -->

### Project layout

```
host/cmd/roosterm/   main, serve, passwd
host/internal/       api, apierr, atomicfile, auth, bridge, config, hc, swarm, web
web/src/             api, components, lib, screens, terminal, workspace, styles
app/lib/             Flutter host list and WebView pages
deploy/              systemd units, launchd plist, deploy/README.md
docs/                design.md, api.md, hc-bridge.md, logs/
scripts/             build.py, service.py, test.py
```
<!-- evidence: host/cmd/roosterm/ listing; host/internal/ listing; web/src listing; app/lib listing; deploy/ listing -->
