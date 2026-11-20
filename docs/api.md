# roosterm HTTP/WebSocket API

This file is the living contract. Code wins on conflict, and this file must be fixed when they differ.

## Overview

All paths are same-origin. JSON bodies are UTF-8, `Content-Type: application/json`. Request bodies are limited to 64 KiB (413, kind `bad_request`). Unknown JSON fields are rejected (400).

Tabs are hc terminals carrying the label `roosterm=tab`; the title lives in label `roosterm.name`. roosterm stores no tab state. Terminals made outside roosterm are invisible to it.

### Cookie
`roosterm_session=<token>; Path=/; Max-Age=<session_ttl seconds>; HttpOnly; Secure; SameSite=Strict`
- `<token>`: 32 bytes from `crypto/rand`, base64url without padding (43 chars). The server stores only `sha256(token)` hex.
- Logout / expiry clear: same attributes with `Max-Age=0` and empty value.
- `Secure` is always set (the reverse proxy terminates TLS; browsers treat `http://localhost` as secure for cookies).

### Error body (every non-2xx JSON response)
```json
{"error":{"op":"tab.update","tab":"t7","message":"terminal.label t7: no terminal \"t7\"","kind":"not_found","hint":"see `hc ls --all`"}}
```
`op` always present; `tab` present when a tab is involved; `kind` and `hint` copied from hc errors when the error came from hc, else roosterm's own kind. Kinds and statuses:

| kind | HTTP status | when |
|---|---|---|
| `bad_request` | 400 | malformed JSON, unknown field, invalid value (message names field, expected, actual) |
| `bad_request` | 405 | a method other than GET/HEAD on a static (non-`/api/`, non-`/login`, non-`/logout`, non-`/ws/`) path |
| `bad_request` | 413 | request body over 64 KiB |
| `unauthorized` | 401 | missing/unknown/expired cookie; wrong credentials on `/login` |
| `not_found` | 404 | tab id unknown to hc, or exists but lacks label `roosterm=tab`; directory path missing or not a directory; unknown `/api/...` path or method (op `api`) |
| `outside_home` | 403 | directory path resolves (after symlinks) outside home |
| `permission_denied` | 403 | directory not readable |
| `too_many_attempts` | 429 | login locked; header `Retry-After: <seconds>` (integer, ≥1) |
| `core_offline` | 503 | hc socket not connected right now |
| any other hc kind (`finished`, `unavailable`, `invalid_params`, …) | 502 | hc returned a JSON-RPC error; message/kind/hint verbatim (hc `not_found` stays 404) |
| `internal` | 500 | roosterm bug or I/O failure (session file write, unresolvable home, etc.) |

hc-originated `message` = `<hc method>[ <tab>]: <hc message>`.

## Endpoints

| Method + path | Auth | Request | Success |
|---|---|---|---|
| `POST /login` | none | `{"username":string,"password":string}` (both non-empty) | `204`, `Set-Cookie` |
| `POST /logout` | cookie | none | `204`, clearing `Set-Cookie`; session removed from file |
| `GET /api/tabs` | cookie | none | `200` `Tab[]`, ordered by creation time ascending |
| `POST /api/tabs` | cookie | `{"command"?: string[], "label"?: string, "cwd"?: string, "swarm"?: string}` (`cwd` relative to home; `swarm` excludes `command` and `cwd`) | `201` `Tab` |
| `PATCH /api/tabs/{id}` | cookie | `{"label"?: string, "hidden"?: boolean}`, at least one (op `tab.update`; `label` sets `renamed`) | `200` `Tab` |
| `DELETE /api/tabs/{id}` | cookie | none | `204` |
| `GET /api/dirs?path=<rel>` | cookie | none | `200` `{"path": string, "entries": [{"name": string, "hidden": boolean}]}` |
| `GET /api/swarms` | cookie | none | `200` `string[]` (live tmux socket names starting with `claude-swarm-`, sorted) |
| `GET /ws/tabs/{id}?cols=N&rows=N` | cookie, checked **before** upgrade | WebSocket upgrade | see below |
| `GET /` and any other `GET` path | none | — | embedded static files; unknown paths serve `index.html` (SPA) |

Session check = `GET /api/tabs` (401 ⇒ show login). There is no whoami endpoint.

### `Tab`
```json
{"id":"t7","label":"zsh","exited":false,"exitCode":null,"signal":null,"reason":null,
 "hidden":false,"cwd":"projects/roosterm","title":"","renamed":false,"program":"zsh"}
```
- `id` = hc address. `label` = hc label `roosterm.name`. `exited` = hc `state == "finished"`.
- `exitCode`, `signal`, `reason` copied from the hc record (`reason` ∈ `exited`, `killed`, `server_stop`, `host_shutdown`, `holder_lost`, `stopped_by_restart`, or null while live).
- `hidden`: the tab is dropped from clients' tab strips; the process keeps running. Set by `PATCH`.
- `cwd`: hc cwd relative to home, `""` = home; a cwd outside home (not created by roosterm) stays absolute.
- `title`: the terminal title reported by hc.
- `renamed`: true once the user set `label` through `PATCH`.
- `program`: basename of `argv[0]`; `"claude"` marks a Claude Code tab.

### `POST /api/tabs`
- `command` omitted ⇒ argv `[config.shell, "-l"]`. Present ⇒ non-empty array of non-empty strings, used verbatim as argv.
- `cwd` omitted or `""` ⇒ home. Otherwise validated exactly like `GET /api/dirs` `path` (relative, no `..`, resolves inside home, must be a directory). The resolved directory is the hc cwd.
- `swarm` = a live socket name from `GET /api/swarms`. It excludes `command` and `cwd` (400 `swarm: expected no "command" or "cwd" together with "swarm", got both`). A name not starting with `claude-swarm-` or containing `/` gives 400 (`swarm: expected a socket name starting with "claude-swarm-" and without "/", got "<name>"`). The host attaches with `tmux -S <socket dir>/<name> a [-t <window with most claude panes>]`; a well-formed name with no live socket gives 500.
- `label` omitted ⇒ the swarm name for a swarm tab, else `"~"` when the resolved cwd is home, else the base name of the resolved cwd. Label: 1–64 characters, no control characters (400 otherwise).
- hc call: `terminal.run {argv, cwd, size: {cols:80, rows:24}, labels: {"roosterm":"tab","roosterm.name":label}, no_parent: true}`. The first attach resizes it.

### `PATCH /api/tabs/{id}`
`label` (same rules as above) sets `roosterm.name` and marks the tab `renamed`. `hidden` sets or clears the hide mark. Tabs not carrying `roosterm=tab` give 404.

### `DELETE /api/tabs/{id}`
Live ⇒ `terminal.kill` then `terminal.rm`; finished ⇒ `terminal.rm`.

### `GET /api/dirs?path=<rel>`
Lists the sub-directories of `path` (relative to home; empty = home), sorted by name; `hidden` is true for dot-names. Symlinks are listed only when they resolve to a directory inside home. Errors: 400 absolute path, `..` segment or NUL byte; 403 `outside_home` or `permission_denied`; 404 missing or not a directory.

### Login rate limit
After 5 consecutive failures every `/login` returns 429 for 60 s; a success resets the count. Requests during lockout do not extend it and are not checked against the password. The limit is global (single user), not per IP.

## WebSocket `/ws/tabs/{id}?cols=N&rows=N`

Pre-upgrade (plain HTTP responses with the error body): 401 without valid cookie; 400 if `cols`/`rows` missing or not in 1..1000. Origin check: coder/websocket default (Origin host must equal request Host; no Origin header is accepted).

After upgrade:

| Direction | Frame | Content |
|---|---|---|
| client → server | binary | keystroke/paste bytes, forwarded verbatim as hc DATA (max message 1 MiB) |
| client → server | text | `{"type":"resize","cols":N,"rows":N}` (1..1000) → hc RESIZE |
| server → client | binary | terminal output. The first binary message after (re)attach is `\x1b[3J` (clear scrollback) followed by the full screen paint; write it as-is, no client reset needed |
| server → client | text | `{"type":"core","state":"offline"}` — hc core gone; tab stays open, input is dropped |
| server → client | text | `{"type":"core","state":"online"}` — re-attached; a full-screen binary message follows |
| server → client | text | `{"type":"error","op":"tab.input","tab":"t7","kind":"core_offline","message":"…"}` — one per client binary message dropped while offline; also sent before every 4400 (`op` `tab.input`, kind `bad_request`) and 4500 (`op` `tab.attach`, hc kind) close |
| server → client | ping | every `ping_interval` (config); browsers answer automatically |

Close codes (server → client):

| code | reason text | meaning |
|---|---|---|
| 4000 | JSON `{"reason":"exited","exitCode":7,"signal":null}` | program ended (also sent immediately when the tab is already finished at connect) |
| 4004 | `tab t7 not found` | id unknown or not a roosterm tab |
| 4400 | `bad message: <detail>` | invalid text frame (bad JSON, unknown type, size out of range) |
| 4500 | hc message, truncated to 123 bytes | hc returned an error on attach |
| 1001 | `server shutdown` | roosterm is stopping |

The WebSocket stays open while the hc core is offline; roosterm re-attaches server-side (`core` messages). The client reconnects only on its own `online`/`visibilitychange` signals.

Multiple sockets on one tab are allowed; the last one that attached/typed/resized sets the size (hc rule).

## Config file

`~/.config/roosterm/config.toml` (`--config <path>` overrides; `$XDG_CONFIG_HOME/roosterm/config.toml` when that variable is set). All keys required except `hc_socket`; unknown keys are rejected; `shell` must be an absolute path to an executable file.

```toml
listen = "127.0.0.1:8787"
hc_socket = ""          # empty: hc's own rule, docs/hc-bridge.md §1
ping_interval = "25s"
session_ttl = "720h0m0s"
shell = "/bin/zsh"
username = "me"
password_hash = "$argon2id$v=19$m=65536,t=3,p=4$<salt b64>$<key b64>"
```
Sessions file: `sessions.json` in the same directory as the config file, mode 0600.
