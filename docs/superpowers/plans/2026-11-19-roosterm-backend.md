# Roosterm Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use podium:team-spawn (recommended) or podium:team-fable to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `roosterm` (Go, `host/`) authenticates one user and bridges HTTP/WebSocket tabs to herdr-core (`hc`) terminals so programs keep their PID across client disconnects, roosterm restarts and hc core restarts.

**Architecture:** One Go binary. `internal/hc` speaks hc's framed JSON-RPC over the Unix socket (one shared connection, many attach channels) and a supervisor re-dials when the socket file re-appears. `internal/web` serves login, the session gate and the embedded static page; `internal/api` serves REST tabs/swarms; `internal/bridge` turns one WebSocket into one hc attach stream. Tabs are hc terminals carrying the label `roosterm=tab`; roosterm stores no tab state.

**Tech Stack:** Go 1.26.4 stdlib (`net/http` 1.22+ patterns, `crypto/rand`, `embed`), `github.com/coder/websocket` v1.8.15, `github.com/fsnotify/fsnotify` v1.10.1, `github.com/BurntSushi/toml` v1.6.0, `golang.org/x/crypto` v0.59.0 (argon2), `golang.org/x/term` v0.48.0; `golang.org/x/sys` v0.50.0 comes in indirectly. Versions read from proxy.golang.org `@latest` on 2026-11-19; APIs checked with `go doc` at those versions and context7 `/coder/websocket`. All code in this plan was extracted from this file into a scratch tree, compiled, and run: `scripts/test.py` → 47 tests, 0 skipped, 2.5 s, PASS; `hc` and `bridge` packages stable over 20 `-race` repetitions; `go mod tidy` result as in Task 9 step 4. Python 3 stdlib for `scripts/`.

**Decisions (hard-to-reverse only):**
- *Keystrokes go as DATA on the attach channel, not `terminal.send`.* Alternatives: `terminal.send` (spec text), `terminal.keys`. Driver: spike — `terminal.send` pastes text and presses Enter (`docs/hc-bridge.md` §4); raw bytes are the only path that supports arbitrary TUI input. Drawback: none known; this corrects the spec.
- *WebSocket stays open while the hc core is offline; roosterm re-attaches server-side.* Alternatives: close with a code and let the client reconnect. Driver: spec forbids timer retries in the client and the client cannot know when hc returns; the server sees the socket file event. Drawback: the bridge holds one goroutine per open tab during an outage.
- *Tab ownership by label `roosterm=tab`; title in label `roosterm.name`.* Alternatives: list every hc terminal; hc `name`. Driver: hc terminals started by agents (`hc run`) would flood the tab bar; hc `name` allows only `[a-z0-9_.-]{1,32}`. Drawback: terminals made outside roosterm are invisible to it.
- *Login rate limit is global (single user), not per IP.* Alternatives: per-IP with trusted `X-Forwarded-For`. Driver: single account behind the user's reverse proxy; every request has the proxy's IP. Drawback: an attacker can keep the owner locked out for the lockout window.
- *Config gains `shell` (not in the spec's field list).* Driver: under launchd `SHELL` is unset, so "`$SHELL -l`" has no value at serve time; `roosterm passwd` records `$SHELL` once from the user's interactive shell.

Plan gate: `/Users/jhangyu/.claude/plugins/cache/jhangyu/podium/1.1.1/protocols/review.md` §L0 Plan gate.

## Spec overrides (the spike or this plan replaces spec text; `docs/hc-bridge.md` wins)

| Spec text (`docs/superpowers/specs/2026-11-19-roosterm-design.md`) | This plan | Source |
|---|---|---|
| line 72: keyboard binary frame → `terminal.send` | binary frame → DATA frame on the attach channel | `docs/hc-bridge.md` §4 "terminal.send — NOT used" |
| line 72: window size via JSON control message | client still sends JSON `{"type":"resize"}`; the server sends an hc RESIZE frame | `docs/hc-bridge.md` §3, §4 |
| line 70: `DELETE /api/tabs/{id}` → `terminal.kill` | live → `terminal.kill` then `terminal.rm`; finished → `terminal.rm` (a killed record would otherwise stay in the list 24 h) | `docs/hc-bridge.md` §4 terminal.rm |
| line 64: socket dir `$TMUX_TMPDIR`, else `/tmp/tmux-<uid>` | `${TMUX_TMPDIR:-/tmp}/tmux-<uid>` (tmux appends `tmux-<uid>` to `TMUX_TMPDIR` too) | man tmux; spike report "Spec contradictions" 2 |
| line 101: config fields | adds `shell` (see Decisions) | — |
| line 108: `internal/{auth,hc,web}` | `internal/{apierr,config,auth,hc,hc/hctest,swarm,web,api,bridge}` (one package per work package, no shared files between parallel tasks) | this plan |
| line 102: hc autostart "after spike item 4" | launchd/systemd unit runs idempotent `hc server start`, then `roosterm serve`; never `hc server stop` | spike log "Autostart"; `docs/hc-bridge.md` §5 |

## Global Constraints

- Spec: `docs/superpowers/specs/2026-11-19-roosterm-design.md`. Contract: `docs/logs/contract-backend.md` (frozen). hc wire facts: `docs/hc-bridge.md` (authoritative over the spec where they differ).
- herdr-core pinned at 0.1.21, protocol generation 1. hello must report `generation == 1`, `framed == true`, features ⊇ {`attach`, `events`} (`docs/hc-bridge.md` §2).
- Go module path `roosterm/host`, directory `host/`, `go 1.26.4`. No other third-party modules than the Tech Stack list.
- macOS and Linux, one code path: no `runtime.GOOS` switches, no build tags.
- No sleep, no polling loops, no timer-based retries in product or test code. Wait on channels, fsnotify events, or context cancellation. Time as business data (session expiry, lockout, ping interval) uses an injected clock or a config value. IO caps (`context.WithTimeout` around one Ping, HTTP shutdown grace, `ReadHeaderTimeout`) are fail-fast limits, not waits.
- No silent fallback: missing required data → error. Every error message names the operation, the affected object (tab id, path, method) and expected vs actual.
- No swallowed errors: `_ =` only on a `Close`/`CloseNow`/`SetDeadline` whose failure cannot change the outcome, with a `ponytail:` or explanatory comment on that line or the line above.
- No dead code, no commented-out code, no to-do markers, no constant feature flags. Magic values are named constants with semantic names.
- Tests: entry `python3 scripts/test.py`; whole script ≤ 40 s, each test ≤ 1 s, `t.Parallel()` in every test, hermetic (no network, no real clock in assertions, no real hc). Fake hc over a Unix socket (`internal/hc/hctest`). Socket paths in tests come from `os.MkdirTemp("", "rt")` (macOS limits Unix socket paths to 104 bytes; `t.TempDir()` is too long). Test files and `hctest` must not contain `time.Sleep` or the word `sleep` (any case).
- Kept tests (spec "測試" section) only: auth (argon2id verify, cookie attributes, session expiry, rate limit, WS upgrade rejected without cookie), bridge (hc wire codec, WS↔JSON-RPC and REST↔JSON-RPC translation, exit code propagation, hc offline behavior), swarm scan. Config and CLI get run-once checks that are not committed (Task 1 step 6, Task 9 step 6).
- Shared-tree git: no stash (including `stash list`), reset, `checkout --`, clean, `git add -A`. Commit with `git add <own paths>` then `git commit -m "<msg>" -- <own paths>`; no `;` in `-m`. Write files with the Write/Edit tools, not shell heredocs.
- Build output only in `build/` (gitignored, Task 4). Go's own cache is outside the repo.
- After writing any Go file: `cd host && gofmt -w <files>` (the plan's code is correct but not column-aligned; `scripts/test.py` fails on `gofmt -l` output).

---

## HTTP/WS API (contract owned by this plan; the client plan consumes it)

All paths are same-origin. JSON bodies are UTF-8, `Content-Type: application/json`. Max request body 64 KiB (413, kind `bad_request`, above).

### Cookie
`roosterm_session=<token>; Path=/; Max-Age=<session_ttl seconds>; HttpOnly; Secure; SameSite=Strict`
- `<token>`: 32 bytes from `crypto/rand`, base64url without padding (43 chars). The server stores only `sha256(token)` hex.
- Logout / expiry clear: same attributes with `Max-Age=0` and empty value.
- `Secure` is always set (the reverse proxy terminates TLS; browsers treat `http://localhost` as secure for cookies).

### Error body (every non-2xx JSON response)
```json
{"error":{"op":"tab.rename","tab":"t7","message":"terminal.label t7: no terminal \"t7\"","kind":"not_found","hint":"see `hc ls --all`"}}
```
`op` always present; `tab` present when a tab is involved; `kind` and `hint` copied from hc errors when the error came from hc, else roosterm's own kind. Kinds and statuses:

| kind | HTTP status | when |
|---|---|---|
| `bad_request` | 400 | malformed JSON, unknown field, invalid value (message names field, expected, actual) |
| `bad_request` | 405 | a method other than GET/HEAD on a static (non-`/api/`, non-`/login`, non-`/logout`, non-`/ws/`) path |
| `bad_request` | 413 | request body over 64 KiB |
| `unauthorized` | 401 | missing/unknown/expired cookie; wrong credentials on `/login` |
| `not_found` | 404 | tab id unknown to hc, or exists but lacks label `roosterm=tab`; unknown `/api/...` path or method (op `api`) |
| `too_many_attempts` | 429 | login locked; header `Retry-After: <seconds>` (integer, ≥1) |
| `core_offline` | 503 | hc socket not connected right now |
| any other hc kind (`finished`, `unavailable`, `invalid_params`, …) | 502 | hc returned a JSON-RPC error; message/kind/hint verbatim |
| `internal` | 500 | roosterm bug or I/O failure (session file write etc.) |

hc-originated `message` = `<hc method>[ <tab>]: <hc message>`.

### Endpoints

| Method + path | Auth | Request | Success |
|---|---|---|---|
| `POST /login` | none | `{"username":string,"password":string}` | `204`, `Set-Cookie` |
| `POST /logout` | cookie | none | `204`, clearing `Set-Cookie`; session removed from file |
| `GET /api/tabs` | cookie | none | `200` `Tab[]`, ordered by `created_ms` ascending |
| `POST /api/tabs` | cookie | `{"command"?: string[], "label"?: string, "cwd"?: string}` (`cwd` relative to home) | `201` `Tab` |
| `PATCH /api/tabs/{id}` | cookie | `{"label"?: string, "hidden"?: boolean}`, at least one (op `tab.update`; `label` sets `renamed`) | `200` `Tab` |
| `GET /api/dirs?path=<rel>` | cookie | none | `200` `{"path": string, "entries": [{"name": string, "hidden": boolean}]}`; 400 bad path, 403 outside home or permission denied, 404 missing |
| `DELETE /api/tabs/{id}` | cookie | none | `204` |
| `GET /api/swarms` | cookie | none | `200` `string[]` (socket names, sorted, e.g. `"claude-swarm-4242"`) |
| `GET /ws/tabs/{id}?cols=N&rows=N` | cookie, checked **before** upgrade | WebSocket upgrade | see below |
| `GET /` and any other `GET` path | none | — | embedded static files; unknown paths serve `index.html` (SPA) |

Session check = `GET /api/tabs` (401 ⇒ show login). There is no whoami endpoint.

`Tab`:
```json
{"id":"t7","label":"zsh","exited":false,"exitCode":null,"signal":null,"reason":null}
```
- `id` = hc address. `label` = hc label `roosterm.name`. `exited` = hc `state == "finished"`. `exitCode`, `signal`, `reason` copied from the hc record (`reason` ∈ `exited`, `killed`, `server_stop`, `host_shutdown`, `holder_lost`, `stopped_by_restart`, or null while live).

`POST /api/tabs` rules:
- `command` omitted ⇒ argv `[config.shell, "-l"]`. Present ⇒ non-empty array of non-empty strings, used verbatim as argv (swarm tab: `["tmux","-L","claude-swarm-4242","a"]`).
- `label` omitted ⇒ `filepath.Base(argv[0])`. Label: 1–64 chars, no control chars (400 otherwise).
- hc call: `terminal.run {argv, cwd: $HOME of roosterm, size: {cols:80, rows:24}, labels: {"roosterm":"tab","roosterm.name":label}, no_parent: true}`. The first attach resizes it.

`DELETE /api/tabs/{id}`: live ⇒ `terminal.kill` then `terminal.rm`; finished ⇒ `terminal.rm`.

Login rate limit: after `login_max_failures = 5` consecutive failures, every `/login` returns 429 for `login_lockout = 60 s` (constants `auth.MaxFailures`, `auth.Lockout`); a success resets the count. Requests during lockout do not extend it and are not checked against the password.

### WebSocket `/ws/tabs/{id}?cols=N&rows=N`

Pre-upgrade (plain HTTP responses with the error body): 401 without valid cookie; 400 if `cols`/`rows` missing or not in 1..1000. Origin check: coder/websocket default (Origin host must equal request Host; no Origin header is accepted).

After upgrade:

| Direction | Frame | Content |
|---|---|---|
| client → server | binary | keystroke/paste bytes, forwarded verbatim as hc DATA (max message 1 MiB) |
| client → server | text | `{"type":"resize","cols":N,"rows":N}` (1..1000) → hc RESIZE |
| server → client | binary | terminal output. The first binary message after (re)attach is the full screen (starts with `\x18\x1b[!p`); write it as-is, no client reset needed |
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

The client reconnects only on its own `online`/`visibilitychange` signals (spec). While a socket is open, hc restarts are handled by the server (`core` messages).

Multiple sockets on one tab are allowed; the last one that attached/typed/resized sets the size (hc rule).

### Config file `~/.config/roosterm/config.toml` (`--config <path>` overrides; `$XDG_CONFIG_HOME/roosterm/config.toml` when that variable is set)
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

---

## Work packages

| Round | Task | Package | Model | Owned files | Depends on |
|---|---|---|---|---|---|
| 1 | 1 | Module scaffold, config, API error type | sonnet | `host/go.mod`, `host/go.sum`, `host/internal/config/config.go`, `host/internal/apierr/apierr.go` | — |
| 1 | 2 | Auth: argon2id, sessions file, rate limit, cookie | sonnet | `host/internal/auth/*` | Task 1 step 1 (go.mod) |
| 1 | 3 | hc client, attach streams, supervisor with socket watch, fake hc | sonnet | `host/internal/hc/*`, `host/internal/hc/hctest/*` | Task 1 step 1 |
| 1 | 4 | Swarm scan, `scripts/test.py`, `scripts/build.py`, `.gitignore` | sonnet | `host/internal/swarm/*`, `scripts/test.py`, `scripts/build.py`, `.gitignore` | Task 1 step 1; `test.py` full run after Tasks 1–3 commit |
| 1 | 5 | Deploy units | sonnet | `deploy/**` | — |
| 2 | 6 | Web shell: login/logout, session middleware, static embed | sonnet | `host/internal/web/*`, `host/internal/web/dist/index.html` | 1, 2 |
| 2 | 7 | REST API: tabs, swarms | sonnet | `host/internal/api/*` | 1, 3, 4 |
| 2 | 8 | WebSocket ↔ hc bridge | sonnet | `host/internal/bridge/*` | 1, 3 |
| 2 | 9 | `cmd/roosterm` (`serve`, `passwd`), module tidy | sonnet | `host/cmd/roosterm/*`; `host/go.mod`, `host/go.sum` (tidy only) | 1, 2, 3, 4; build step waits for the Task 6, 7, 8 commits |

Every task's complete code is in its steps; an implementer copies it, runs the named commands, and STOPs and reports if a command's output differs from the expected output.

Round 1 start rule: Task 1 commits `host/go.mod` + `host/go.sum` first (step 1, minutes); Tasks 2–4 write files at once and run `go test` after that commit lands. Task 4's last step (`scripts/test.py` over the whole module) runs after Tasks 1–3 have committed. Round 2: Task 9 writes its files at once; its build and run-once steps start when Tasks 6, 7, 8 have committed.

No two tasks in a round share a file. `host/go.mod`/`go.sum`: Task 1 in round 1, Task 9 in round 2. Every other task that finds it needs a module change STOPs and reports (all modules are pinned in Task 1).

### Contract acceptance mapping

| Contract criterion | Satisfied by |
|---|---|
| 1 spike items 1–3 | Done: `docs/logs/2026-11-19-spike-hc.md` (commit d64498d) |
| 2 `python3 scripts/test.py` exit 0, ≤40 s, 0 skipped, no sleep | Task 4 (runner + checks + injected-violation proof); tests from Tasks 2, 3, 4, 6, 7, 8; re-run in Task 9 step 7 |
| 3 live proof macOS | Tasks 3, 6, 7, 8, 9; executed in "Post-round-2 gate" G1 |
| 4 live proof Linux | Same + Task 4 `build.py --target linux/arm64`; gate G2 |
| 5 `/api/swarms` lists only `claude-swarm-*`; swarm tab attaches | Task 4 (`TestListOnlyClaudeSwarmSockets`), Task 7 (`TestSwarmsEndpoint`, `TestCreateTabCommandAndLabel`), gate G3 |
| 6 non-zero exit with socket path when hc is down | Task 3 (`TestDialMissingSocketNamesPath`), Task 9 (step 6 run-once), gate G4 |
| 7 launchd plist + systemd user unit start `roosterm serve` | Task 5 |

---

## Round 1

### Task 1: Module scaffold, config, API error type

**Files:**
- Create: `host/go.mod`, `host/go.sum`
- Create: `host/internal/config/config.go`
- Create: `host/internal/apierr/apierr.go`
- Run-once, not committed: `host/internal/config/config_test.go` (step 6 deletes it)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `package config`
    - `type Config struct { Listen string; HCSocket string; PingInterval time.Duration; SessionTTL time.Duration; Shell string; Username string; PasswordHash string }` — `HCSocket` is the raw value (may be `""`).
    - `const DefaultListen = "127.0.0.1:8787"`, `const DefaultPingInterval = 25 * time.Second`, `const DefaultSessionTTL = 720 * time.Hour`
    - `func DefaultPath() (string, error)` — `$XDG_CONFIG_HOME/roosterm/config.toml`, else `$HOME/.config/roosterm/config.toml`; error if both are empty.
    - `func Load(path string) (Config, error)` — error wraps the read error (`errors.Is(err, fs.ErrNotExist)` works).
    - `func Save(path string, c Config) error`
    - `func SessionsPath(configPath string) string` — `filepath.Join(filepath.Dir(configPath), "sessions.json")`
    - `func (c Config) SocketPath() string` — `c.HCSocket` if non-empty, else `DefaultSocket(os.Getenv, os.Getuid(), dirExists)`.
    - `func DefaultSocket(getenv func(string) string, uid int, dirExists func(string) bool) string`
  - `package apierr`
    - `type Error struct { Status int \`json:"-"\`; Op string \`json:"op"\`; Tab string \`json:"tab,omitempty"\`; Message string \`json:"message"\`; Kind string \`json:"kind"\`; Hint string \`json:"hint,omitempty"\` }`
    - `func (e *Error) Error() string` — `"<op> <tab>: <message> (kind <kind>)"`, tab part omitted when empty.
    - `func Write(w http.ResponseWriter, e *Error)` — sets `Content-Type: application/json`, writes `e.Status`, body `{"error":<e>}`.
    - `func WriteJSON(w http.ResponseWriter, status int, v any)` — success bodies.
    - `const MaxBody = 64 << 10`
    - `func DecodeJSON(w http.ResponseWriter, r *http.Request, op string, v any) *Error` — exactly one JSON object, unknown fields rejected; > `MaxBody` → 413; other malformed → 400; both kind `bad_request`; nil on success.
    - Kind constants: `KindBadRequest = "bad_request"`, `KindUnauthorized = "unauthorized"`, `KindNotFound = "not_found"`, `KindTooManyAttempts = "too_many_attempts"`, `KindCoreOffline = "core_offline"`, `KindInternal = "internal"`.

**Behavior:**
- `Load`: decode with `toml.DecodeFile` into a struct of string fields; `md.Undecoded()` non-empty → error listing the unknown keys. Required (non-empty): `listen`, `ping_interval`, `session_ttl`, `shell`, `username`, `password_hash`. `hc_socket` may be empty. Durations via `time.ParseDuration`, must be > 0. `shell` must be absolute and `os.Stat` must show a regular file with any execute bit. Each error: `config <path>: <key>: expected <what>, got <actual>`.
- `Save`: `os.MkdirAll(dir, 0o700)`, write `<path>.tmp` with mode 0600 via `toml.NewEncoder`, chmod 0600, then `os.Rename`. Durations written with `Duration.String()` (`720h0m0s`).
- `DefaultSocket` (hc rule, `docs/hc-bridge.md` §1): `XDG_RUNTIME_DIR` set → `$XDG_RUNTIME_DIR/herdr-core/default.sock`; else `/run/user/<uid>` exists → `/run/user/<uid>/herdr-core/default.sock`; else `<XDG_STATE_HOME or $HOME/.local/state>/herdr-core/default/default.sock`.

**Constraints:**
- Only `github.com/BurntSushi/toml` beyond stdlib in `config`; `apierr` stdlib only.
- go.mod pins exactly the Tech Stack versions.

**Acceptance criteria:**
- [ ] `cd host && go list -m all` lists `github.com/coder/websocket v1.8.15`, `github.com/fsnotify/fsnotify v1.10.1`, `github.com/BurntSushi/toml v1.6.0`, `golang.org/x/crypto v0.59.0`, `golang.org/x/term v0.48.0`.
- [ ] Run-once (step 6): `cd host && go test ./internal/config/ -run 'TestLoadRejectsUnknownKey|TestLoadRoundTrip|TestDefaultSocketRule' -count=1 -v` passes; output and RC go into the report; the test file is then deleted and never committed.
- [ ] `cd host && go vet ./internal/config/ ./internal/apierr/` exits 0; `gofmt -l internal/config internal/apierr` prints nothing.
- [ ] `git ls-files host/internal/config` lists only `config.go`.

**Steps:**

- [ ] **Step 1: Create the module and commit it (other tasks wait for this commit)**

```bash
mkdir -p host && cd host
go mod init roosterm/host
go mod edit -go=1.26.4
go get github.com/coder/websocket@v1.8.15 github.com/fsnotify/fsnotify@v1.10.1 github.com/BurntSushi/toml@v1.6.0 golang.org/x/crypto/argon2@v0.59.0 golang.org/x/term@v0.48.0
go list -m all
cd ..
git add host/go.mod host/go.sum
git commit -m "build: host Go module with pinned dependencies" -- host/go.mod host/go.sum
```
Expected: `go list -m all` prints `roosterm/host` plus the five modules at the pinned versions and `golang.org/x/sys v0.50.0`. All requires are marked `// indirect` until code imports them; Task 9 tidies that. `go get` must use the package path `golang.org/x/crypto/argon2` so go.sum also covers `golang.org/x/sys`.

- [ ] **Step 2: Write `host/internal/apierr/apierr.go`**

```go
// Package apierr is the JSON error body shared by every roosterm HTTP endpoint.
package apierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
)

const (
	KindBadRequest      = "bad_request"
	KindUnauthorized    = "unauthorized"
	KindNotFound        = "not_found"
	KindTooManyAttempts = "too_many_attempts"
	KindCoreOffline     = "core_offline"
	KindInternal        = "internal"
)

// MaxBody is the largest accepted JSON request body.
const MaxBody = 64 << 10

type Error struct {
	Status  int    `json:"-"`
	Op      string `json:"op"`
	Tab     string `json:"tab,omitempty"`
	Message string `json:"message"`
	Kind    string `json:"kind"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string {
	if e.Tab == "" {
		return fmt.Sprintf("%s: %s (kind %s)", e.Op, e.Message, e.Kind)
	}
	return fmt.Sprintf("%s %s: %s (kind %s)", e.Op, e.Tab, e.Message, e.Kind)
}

// Write sends e as {"error": e} with status e.Status.
func Write(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	if err := json.NewEncoder(w).Encode(struct {
		Error *Error `json:"error"`
	}{e}); err != nil {
		log.Printf("write error body for %s: %v", e.Op, err)
	}
}

// DecodeJSON reads exactly one JSON object with only known fields into v.
// Bodies over MaxBody give 413; anything else malformed gives 400.
func DecodeJSON(w http.ResponseWriter, r *http.Request, op string, v any) *Error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Op: op, Kind: KindBadRequest,
				Message: fmt.Sprintf("request body: expected at most %d bytes, got more", MaxBody)}
		}
		return &Error{Status: http.StatusBadRequest, Op: op, Kind: KindBadRequest,
			Message: fmt.Sprintf("request body: expected one JSON object with known fields, got %v", err)}
	}
	if dec.More() {
		return &Error{Status: http.StatusBadRequest, Op: op, Kind: KindBadRequest,
			Message: "request body: expected one JSON object, got trailing data"}
	}
	return nil
}

// WriteJSON sends v as a JSON body with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write JSON body (status %d): %v", status, err)
	}
}
```

- [ ] **Step 3: Write the run-once config test `host/internal/config/config_test.go`**

```go
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/config"
)

func TestLoadRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "listen = \"127.0.0.1:1\"\nping_interval = \"1s\"\nsession_ttl = \"1h\"\nshell = \"/bin/sh\"\nusername = \"me\"\npassword_hash = \"x\"\nbogus = 1\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("Load: expected error naming key bogus, got %v", err)
	}
}

func TestLoadRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	want := config.Config{Listen: "127.0.0.1:9", HCSocket: "", PingInterval: 25 * time.Second,
		SessionTTL: 720 * time.Hour, Shell: "/bin/sh", Username: "me", PasswordHash: "$argon2id$v=19$m=64,t=1,p=1$AAAA$AAAA"}
	if err := config.Save(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode: expected 0600, got %#o", info.Mode().Perm())
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip: expected %+v, got %+v", want, got)
	}
}

func TestDefaultSocketRule(t *testing.T) {
	t.Parallel()
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		name   string
		getenv func(string) string
		runDir bool
		want   string
	}{
		{"xdg runtime", env(map[string]string{"XDG_RUNTIME_DIR": "/r", "HOME": "/h"}), true, "/r/herdr-core/default.sock"},
		{"run user", env(map[string]string{"HOME": "/h"}), true, "/run/user/501/herdr-core/default.sock"},
		{"state home", env(map[string]string{"HOME": "/h", "XDG_STATE_HOME": "/s"}), false, "/s/herdr-core/default/default.sock"},
		{"home default", env(map[string]string{"HOME": "/h"}), false, "/h/.local/state/herdr-core/default/default.sock"},
	}
	for _, c := range cases {
		got := config.DefaultSocket(c.getenv, 501, func(string) bool { return c.runDir })
		if got != c.want {
			t.Errorf("%s: expected %s, got %s", c.name, c.want, got)
		}
	}
}
```

- [ ] **Step 4: Run it to see it fail**

Run: `cd host && go test ./internal/config/ -count=1`
Expected: FAIL to build, `no non-test Go files` or `undefined: config.Load`.

- [ ] **Step 5: Write `host/internal/config/config.go`**

```go
// Package config reads and writes ~/.config/roosterm/config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	DefaultListen       = "127.0.0.1:8787"
	DefaultPingInterval = 25 * time.Second
	DefaultSessionTTL   = 720 * time.Hour
)

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
	execBits os.FileMode = 0o111
)

type Config struct {
	Listen       string
	HCSocket     string // raw value, may be ""
	PingInterval time.Duration
	SessionTTL   time.Duration
	Shell        string
	Username     string
	PasswordHash string
}

type fileConfig struct {
	Listen       string `toml:"listen"`
	HCSocket     string `toml:"hc_socket"`
	PingInterval string `toml:"ping_interval"`
	SessionTTL   string `toml:"session_ttl"`
	Shell        string `toml:"shell"`
	Username     string `toml:"username"`
	PasswordHash string `toml:"password_hash"`
}

// DefaultPath is $XDG_CONFIG_HOME/roosterm/config.toml, else $HOME/.config/roosterm/config.toml.
func DefaultPath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "roosterm", "config.toml"), nil
	}
	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("config path: expected HOME or XDG_CONFIG_HOME to be set, got both empty")
	}
	return filepath.Join(home, ".config", "roosterm", "config.toml"), nil
}

// SessionsPath is sessions.json next to the config file.
func SessionsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "sessions.json")
}

func Load(path string) (Config, error) {
	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return Config{}, fmt.Errorf("config %s: unknown keys %v: expected only listen, hc_socket, ping_interval, session_ttl, shell, username, password_hash", path, keys)
	}
	for _, f := range []struct{ key, val string }{
		{"listen", fc.Listen}, {"ping_interval", fc.PingInterval}, {"session_ttl", fc.SessionTTL},
		{"shell", fc.Shell}, {"username", fc.Username}, {"password_hash", fc.PasswordHash},
	} {
		if f.val == "" {
			return Config{}, fmt.Errorf("config %s: %s: expected a non-empty value, got \"\"", path, f.key)
		}
	}
	ping, err := positiveDuration(path, "ping_interval", fc.PingInterval)
	if err != nil {
		return Config{}, err
	}
	ttl, err := positiveDuration(path, "session_ttl", fc.SessionTTL)
	if err != nil {
		return Config{}, err
	}
	if err := checkShell(path, fc.Shell); err != nil {
		return Config{}, err
	}
	return Config{Listen: fc.Listen, HCSocket: fc.HCSocket, PingInterval: ping, SessionTTL: ttl,
		Shell: fc.Shell, Username: fc.Username, PasswordHash: fc.PasswordHash}, nil
}

func positiveDuration(path, key, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config %s: %s: expected a positive duration like \"25s\", got %q", path, key, v)
	}
	return d, nil
}

func checkShell(path, shell string) error {
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("config %s: shell: expected an absolute path, got %q", path, shell)
	}
	info, err := os.Stat(shell)
	if err != nil {
		return fmt.Errorf("config %s: shell: expected an executable file, got %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&execBits == 0 {
		return fmt.Errorf("config %s: shell: expected an executable regular file, got mode %v", path, info.Mode())
	}
	return nil
}

// Save writes c atomically (tmp file 0600 + rename), creating the directory 0700.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	encErr := toml.NewEncoder(f).Encode(fileConfig{
		Listen: c.Listen, HCSocket: c.HCSocket, PingInterval: c.PingInterval.String(),
		SessionTTL: c.SessionTTL.String(), Shell: c.Shell, Username: c.Username, PasswordHash: c.PasswordHash,
	})
	closeErr := f.Close()
	if encErr != nil || closeErr != nil {
		return fmt.Errorf("config save %s: encode: %v, close: %v", path, encErr, closeErr)
	}
	if err := os.Chmod(tmp, fileMode); err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	return nil
}

// SocketPath is hc_socket if set, else hc's own default rule.
func (c Config) SocketPath() string {
	if c.HCSocket != "" {
		return c.HCSocket
	}
	return DefaultSocket(os.Getenv, os.Getuid(), dirExists)
}

// DefaultSocket applies hc's rule (docs/hc-bridge.md §1).
func DefaultSocket(getenv func(string) string, uid int, dirExists func(string) bool) string {
	if d := getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "herdr-core", "default.sock")
	}
	if run := fmt.Sprintf("/run/user/%d", uid); dirExists(run) {
		return filepath.Join(run, "herdr-core", "default.sock")
	}
	state := getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(getenv("HOME"), ".local", "state")
	}
	return filepath.Join(state, "herdr-core", "default", "default.sock")
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
```

- [ ] **Step 6: Run the run-once test, record it, delete it**

```bash
cd host
gofmt -w internal/config internal/apierr
go test ./internal/config/ -run 'TestLoadRejectsUnknownKey|TestLoadRoundTrip|TestDefaultSocketRule' -count=1 -v; echo RC=$?
go vet ./internal/config/ ./internal/apierr/; echo VET_RC=$?
gofmt -l internal/config internal/apierr
rm internal/config/config_test.go
```
Expected: three `--- PASS` lines, `ok  roosterm/host/internal/config`, `RC=0`, `VET_RC=0`, `gofmt -l` prints nothing. Paste the PASS lines and both RCs into the report.

- [ ] **Step 7: Commit**

```bash
git add host/internal/config/config.go host/internal/apierr/apierr.go
git commit -m "feat(host): config file and API error body" -- host/internal/config/config.go host/internal/apierr/apierr.go
git status --porcelain host/internal/config host/internal/apierr
```
Expected: the status command prints nothing.

### Task 2: Auth

**Files:**
- Create: `host/internal/auth/password.go`, `session.go`, `ratelimit.go`, `cookie.go`
- Test: `host/internal/auth/auth_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces (`package auth`):
  - `type Clock func() time.Time`
  - `type Cost struct { Time uint32; MemoryKiB uint32; Threads uint8 }`; `var DefaultCost = Cost{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}`
  - `func NewSalt() ([]byte, error)` — 16 bytes, `crypto/rand`.
  - `func HashPassword(password string, salt []byte) string` — `HashPasswordCost(password, salt, DefaultCost)`.
  - `func HashPasswordCost(password string, salt []byte, c Cost) string` — PHC `$argon2id$v=19$m=<KiB>,t=<n>,p=<n>$<salt>$<key>` (raw std base64, no padding). Tests use a cheap `Cost` so argon2 stays fast under `-race`.
  - `func VerifyPassword(phc, password string) (bool, error)` — error only for a malformed hash string (message names the part that failed).
  - `type Sessions struct` (mutex-guarded)
  - `func OpenSessions(path string, ttl time.Duration, now Clock) (*Sessions, error)`
  - `func (s *Sessions) Create() (string, error)`
  - `func (s *Sessions) Valid(token string) bool`
  - `func (s *Sessions) Delete(token string) error`
  - `const MaxFailures = 5`, `const Lockout = 60 * time.Second`
  - `type Limiter struct` (mutex-guarded)
  - `func NewLimiter(now Clock) *Limiter`
  - `func (l *Limiter) Allow() (retryAfter time.Duration, ok bool)`
  - `func (l *Limiter) Fail()`
  - `func (l *Limiter) Succeed()`
  - `const CookieName = "roosterm_session"`
  - `func SessionCookie(token string, ttl time.Duration) *http.Cookie`
  - `func ClearCookie() *http.Cookie`

**Behavior:**
- argon2id: key length 32, salt length 16 (named constants). `VerifyPassword` parses params from the PHC string (does not assume `DefaultCost`) and compares with `subtle.ConstantTimeCompare`. Version other than 19 → error.
- Sessions file JSON `{"sessions":[{"hash":"<sha256 hex>","expires_ms":<unix ms>}]}`. `OpenSessions`: missing file → empty store; existing file with mode bits beyond 0600 → error `sessions <path>: expected mode 0600, got <mode>`; broken JSON → error (never reset). Expired entries are dropped from memory on open. `Create`: 32 random bytes, base64url raw → token; store sha256; purge expired; write file atomically (tmp 0600 + rename); on write failure the entry is removed and the error returned. `Valid`: entry exists and `now() < expires`. `Delete`: remove entry (unknown token → no error), persist.
- Limiter: counts consecutive failures; on the `MaxFailures`th failure records `lockedUntil = now + Lockout`. `Allow`: if `now < lockedUntil` → `(lockedUntil-now, false)`; once the lockout passes, failures reset to 0. `Succeed` resets.
- Cookies: `Path "/"`, `HttpOnly`, `Secure`, `SameSite http.SameSiteStrictMode`, `MaxAge int(ttl.Seconds())`; clear has `MaxAge -1` (net/http writes `Max-Age=0`) and empty value.

**Constraints:**
- `golang.org/x/crypto/argon2` only; no bcrypt/scrypt. Clock injected everywhere; no `time.Now` in `host/internal/auth`.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/auth/ -count=1 -race -v` passes and runs `TestVerifyPassword`, `TestVerifyPasswordMalformed`, `TestSessionCookieAttributes`, `TestSessionExpiry`, `TestSessionsSurviveReopen`, `TestSessionsFileMode`, `TestLimiterLockout`.
- [ ] `grep -n 'time.Now' host/internal/auth/*.go; echo RC=$?` → `RC=1`.
- [ ] `cd host && go vet ./internal/auth/ && gofmt -l internal/auth` → exit 0, no output.

**Steps:**

- [ ] **Step 1: Write the failing test `host/internal/auth/auth_test.go`**

```go
package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/auth"
)

// cheap keeps argon2 fast under -race; VerifyPassword reads params from the hash.
var cheap = auth.Cost{Time: 1, MemoryKiB: 64, Threads: 1}

var salt = []byte("0123456789abcdef")

func TestVerifyPassword(t *testing.T) {
	t.Parallel()
	h := auth.HashPasswordCost("pw", salt, cheap)
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("hash prefix: got %s", h)
	}
	if ok, err := auth.VerifyPassword(h, "pw"); err != nil || !ok {
		t.Fatalf("right password: expected true, nil; got %v, %v", ok, err)
	}
	if ok, err := auth.VerifyPassword(h, "nope"); err != nil || ok {
		t.Fatalf("wrong password: expected false, nil; got %v, %v", ok, err)
	}
	if auth.DefaultCost != (auth.Cost{Time: 3, MemoryKiB: 65536, Threads: 4}) {
		t.Fatalf("DefaultCost: got %+v", auth.DefaultCost)
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                                      "password hash:",
		"$bcrypt$v=19$m=64,t=1,p=1$AAAA$AAAA":   "password hash:",
		"$argon2id$v=18$m=64,t=1,p=1$AAAA$AAAA": "version",
		"$argon2id$v=19$m=0,t=1,p=1$AAAA$AAAA":  "params",
		"$argon2id$v=19$m=64,t=1,p=1$!!!!$AAAA": "salt",
		"$argon2id$v=19$m=64,t=1,p=1$AAAA$":     "key",
	}
	for phc, want := range cases {
		_, err := auth.VerifyPassword(phc, "pw")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("VerifyPassword(%q): expected error containing %q, got %v", phc, want, err)
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	t.Parallel()
	c := auth.SessionCookie("tok", 2*time.Hour).String()
	for _, want := range []string{"roosterm_session=tok", "Path=/", "Max-Age=7200", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(c, want) {
			t.Errorf("session cookie %q: missing %q", c, want)
		}
	}
	clear := auth.ClearCookie().String()
	for _, want := range []string{"roosterm_session=", "Max-Age=0", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(clear, want) {
			t.Errorf("clear cookie %q: missing %q", clear, want)
		}
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newClock() *fakeClock { return &fakeClock{t: time.Unix(1_800_000_000, 0)} }

func TestSessionExpiry(t *testing.T) {
	t.Parallel()
	clk := newClock()
	s, err := auth.OpenSessions(filepath.Join(t.TempDir(), "sessions.json"), time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 {
		t.Fatalf("token length: expected 43, got %d", len(tok))
	}
	if !s.Valid(tok) {
		t.Fatal("fresh token: expected valid")
	}
	if s.Valid("garbage") {
		t.Fatal("unknown token: expected invalid")
	}
	clk.t = clk.t.Add(time.Hour)
	if s.Valid(tok) {
		t.Fatal("token at expiry: expected invalid")
	}
}

func TestSessionsSurviveReopen(t *testing.T) {
	t.Parallel()
	clk := newClock()
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Valid(tok) {
		t.Fatal("after reopen: expected valid")
	}
	if err := s2.Delete(tok); err != nil {
		t.Fatal(err)
	}
	if err := s2.Delete("unknown"); err != nil {
		t.Fatalf("delete unknown: expected nil, got %v", err)
	}
	s3, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Valid(tok) {
		t.Fatal("after delete and reopen: expected invalid")
	}
}

func TestSessionsFileMode(t *testing.T) {
	t.Parallel()
	clk := newClock()
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("sessions mode: expected 0600, got %#o", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.OpenSessions(path, time.Hour, clk.now); err == nil || !strings.Contains(err.Error(), "expected mode 0600") {
		t.Fatalf("0644 file: expected mode error, got %v", err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.OpenSessions(path, time.Hour, clk.now); err == nil {
		t.Fatal("broken JSON: expected error, got nil")
	}
}

func TestLimiterLockout(t *testing.T) {
	t.Parallel()
	clk := newClock()
	l := auth.NewLimiter(clk.now)
	for i := 0; i < auth.MaxFailures-1; i++ {
		l.Fail()
	}
	if _, ok := l.Allow(); !ok {
		t.Fatal("after 4 failures: expected allowed")
	}
	l.Fail()
	if d, ok := l.Allow(); ok || d != auth.Lockout {
		t.Fatalf("after 5 failures: expected (60s, false), got (%v, %v)", d, ok)
	}
	clk.t = clk.t.Add(30 * time.Second)
	if d, ok := l.Allow(); ok || d != 30*time.Second {
		t.Fatalf("30s into lockout: expected (30s, false), got (%v, %v)", d, ok)
	}
	clk.t = clk.t.Add(30 * time.Second)
	if _, ok := l.Allow(); !ok {
		t.Fatal("after lockout: expected allowed")
	}
	l.Fail()
	if _, ok := l.Allow(); !ok {
		t.Fatal("one failure after lockout: expected allowed (count reset)")
	}
	for i := 0; i < auth.MaxFailures-2; i++ {
		l.Fail()
	}
	l.Succeed()
	l.Fail()
	if _, ok := l.Allow(); !ok {
		t.Fatal("after success: expected count reset")
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd host && go test ./internal/auth/ -count=1`
Expected: FAIL to build (`no non-test Go files` or `undefined: auth.HashPasswordCost`).

- [ ] **Step 3: Write `host/internal/auth/password.go`**

```go
// Package auth holds the single-user login: argon2id hash, session file, rate limit, cookie.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Clock is injected wherever auth needs the time.
type Clock func() time.Time

// Cost is the argon2id parameter set written into a PHC string.
type Cost struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultCost is RFC 9106's second recommended option (64 MiB, t=3, p=4).
var DefaultCost = Cost{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}

const (
	argonKeyLen = 32
	saltLen     = 16
	phcParts    = 6 // "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
)

var phcB64 = base64.RawStdEncoding

func NewSalt() ([]byte, error) {
	s := make([]byte, saltLen)
	if _, err := rand.Read(s); err != nil {
		return nil, fmt.Errorf("new salt: %w", err)
	}
	return s, nil
}

// HashPassword hashes with DefaultCost.
func HashPassword(password string, salt []byte) string {
	return HashPasswordCost(password, salt, DefaultCost)
}

// HashPasswordCost returns $argon2id$v=19$m=<KiB>,t=<n>,p=<n>$<salt>$<key> (raw std base64).
func HashPasswordCost(password string, salt []byte, c Cost) string {
	key := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, c.MemoryKiB, c.Time, c.Threads,
		phcB64.EncodeToString(salt), phcB64.EncodeToString(key))
}

// VerifyPassword checks password against a PHC string. The error is only for a malformed hash.
func VerifyPassword(phc, password string) (bool, error) {
	parts := strings.Split(phc, "$")
	if len(parts) != phcParts || parts[0] != "" || parts[1] != "argon2id" {
		return false, fmt.Errorf("password hash: expected $argon2id$v=19$m=<n>,t=<n>,p=<n>$<salt>$<key>, got %d $-separated parts", len(parts))
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, fmt.Errorf("password hash version: expected v=%d, got %q", argon2.Version, parts[2])
	}
	var c Cost
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &c.MemoryKiB, &c.Time, &c.Threads); err != nil ||
		c.MemoryKiB == 0 || c.Time == 0 || c.Threads == 0 {
		return false, fmt.Errorf("password hash params: expected m=<n>,t=<n>,p=<n> all > 0, got %q", parts[3])
	}
	salt, err := phcB64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, fmt.Errorf("password hash salt: expected non-empty raw base64, got error %v", err)
	}
	key, err := phcB64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false, fmt.Errorf("password hash key: expected non-empty raw base64, got error %v", err)
	}
	got := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}
```

- [ ] **Step 4: Write `host/internal/auth/session.go`**

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	tokenLen                    = 32
	sessionFileMode fs.FileMode = 0o600
)

// Sessions stores sha256(token) → expiry in a JSON file. Safe for concurrent use.
type Sessions struct {
	mu      sync.Mutex
	path    string
	ttl     time.Duration
	now     Clock
	entries map[string]int64 // sha256 hex → expires unix ms
}

type sessionFile struct {
	Sessions []sessionEntry `json:"sessions"`
}

type sessionEntry struct {
	Hash      string `json:"hash"`
	ExpiresMS int64  `json:"expires_ms"`
}

// OpenSessions loads path (missing file = empty store). A file readable by others or with broken JSON is an error.
func OpenSessions(path string, ttl time.Duration, now Clock) (*Sessions, error) {
	s := &Sessions{path: path, ttl: ttl, now: now, entries: map[string]int64{}}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sessions %s: %w", path, err)
	}
	if perm := info.Mode().Perm(); perm&^sessionFileMode != 0 {
		return nil, fmt.Errorf("sessions %s: expected mode 0600, got %#o", path, perm)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sessions %s: %w", path, err)
	}
	var f sessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("sessions %s: expected {\"sessions\":[...]} JSON, got %w", path, err)
	}
	nowMS := now().UnixMilli()
	for _, e := range f.Sessions {
		if e.ExpiresMS > nowMS {
			s.entries[e.Hash] = e.ExpiresMS
		}
	}
	return s, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create returns a new token (43 chars base64url) and persists its hash.
func (s *Sessions) Create() (string, error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	h := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	nowMS := now.UnixMilli()
	for k, exp := range s.entries {
		if exp <= nowMS {
			delete(s.entries, k)
		}
	}
	s.entries[h] = now.Add(s.ttl).UnixMilli()
	if err := s.saveLocked(); err != nil {
		delete(s.entries, h)
		return "", err
	}
	return token, nil
}

func (s *Sessions) Valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.entries[hashToken(token)]
	return ok && s.now().UnixMilli() < exp
}

// Delete removes token; an unknown token is not an error.
func (s *Sessions) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := hashToken(token)
	if _, ok := s.entries[h]; !ok {
		return nil
	}
	delete(s.entries, h)
	return s.saveLocked()
}

func (s *Sessions) saveLocked() error {
	f := sessionFile{Sessions: []sessionEntry{}}
	for h, exp := range s.entries {
		f.Sessions = append(f.Sessions, sessionEntry{Hash: h, ExpiresMS: exp})
	}
	slices.SortFunc(f.Sessions, func(a, b sessionEntry) int { return strings.Compare(a.Hash, b.Hash) })
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("sessions %s: encode: %w", s.path, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, sessionFileMode); err != nil {
		return fmt.Errorf("sessions %s: write: %w", s.path, err)
	}
	if err := os.Chmod(tmp, sessionFileMode); err != nil {
		return fmt.Errorf("sessions %s: chmod: %w", s.path, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("sessions %s: rename: %w", s.path, err)
	}
	return nil
}
```

- [ ] **Step 5: Write `host/internal/auth/ratelimit.go`**

```go
package auth

import (
	"sync"
	"time"
)

const (
	MaxFailures = 5
	Lockout     = 60 * time.Second
)

// Limiter is the global login limiter (single user behind one proxy IP).
type Limiter struct {
	mu          sync.Mutex
	now         Clock
	failures    int
	lockedUntil time.Time
}

func NewLimiter(now Clock) *Limiter { return &Limiter{now: now} }

// Allow reports whether a login attempt may be checked; when locked it returns the time left.
func (l *Limiter) Allow() (retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.lockedUntil) {
		return l.lockedUntil.Sub(now), false
	}
	if !l.lockedUntil.IsZero() {
		l.failures = 0
		l.lockedUntil = time.Time{}
	}
	return 0, true
}

func (l *Limiter) Fail() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.failures >= MaxFailures {
		l.lockedUntil = l.now().Add(Lockout)
	}
}

func (l *Limiter) Succeed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = 0
	l.lockedUntil = time.Time{}
}
```

- [ ] **Step 6: Write `host/internal/auth/cookie.go`**

```go
package auth

import (
	"net/http"
	"time"
)

const CookieName = "roosterm_session"

func SessionCookie(token string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}

// ClearCookie expires the session cookie (MaxAge -1 is written as Max-Age=0).
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}
```

- [ ] **Step 7: Run the tests to see them pass**

```bash
cd host
gofmt -w internal/auth
go test ./internal/auth/ -count=1 -race -v; echo RC=$?
go vet ./internal/auth/; echo VET_RC=$?
gofmt -l internal/auth
cd .. && grep -n 'time.Now' host/internal/auth/*.go; echo GREP_RC=$?
```
Expected: seven `--- PASS` lines (the seven names in the acceptance criteria), `RC=0`, `VET_RC=0`, no gofmt output, `GREP_RC=1`.

- [ ] **Step 8: Commit**

```bash
git add host/internal/auth/password.go host/internal/auth/session.go host/internal/auth/ratelimit.go host/internal/auth/cookie.go host/internal/auth/auth_test.go
git commit -m "feat(host): argon2id login, session file, rate limit, cookie" -- host/internal/auth
git status --porcelain host/internal/auth
```
Expected: the status command prints nothing.

### Task 3: hc client, attach streams, supervisor, fake hc

**Files:**
- Create: `host/internal/hc/proto.go` (frame codec), `types.go`, `client.go`, `stream.go`, `supervisor.go`
- Create: `host/internal/hc/hctest/fake.go`
- Test: `host/internal/hc/client_test.go`, `host/internal/hc/supervisor_test.go` (package `hc_test`), `host/internal/hc/supervisor_internal_test.go` (package `hc`)

**Interfaces:**
- Consumes: wire format in `docs/hc-bridge.md` §2–§6.
- Produces (`package hc`):
  - `type Frame struct { Channel uint32; Kind byte; Payload []byte }`; `const KindData byte = 0`, `KindCredit = 1`, `KindClose = 2`, `KindResize = 3`
  - `func WriteFrame(w io.Writer, f Frame) error`, `func ReadFrame(r io.Reader) (Frame, error)`, `func ParseResize(p []byte) (Size, error)` (exported for `hctest`)
  - `type Size struct { Cols uint16 \`json:"cols"\`; Rows uint16 \`json:"rows"\` }`
  - `type Terminal struct { Address string \`json:"address"\`; Argv []string \`json:"argv"\`; Labels map[string]string \`json:"labels"\`; State string \`json:"state"\`; Reason *string \`json:"reason"\`; ExitCode *int \`json:"exit_code"\`; Signal *int \`json:"signal"\`; PID *int \`json:"pid"\`; Cwd string \`json:"cwd"\`; CreatedMS int64 \`json:"created_ms"\`; Cols uint16 \`json:"cols"\`; Rows uint16 \`json:"rows"\` }`
  - `const StateLive = "live"`, `const StateFinished = "finished"`
  - `const LabelOwner = "roosterm"`, `const LabelOwnerTab = "tab"`, `const LabelName = "roosterm.name"`; `func IsTab(t Terminal) bool` — `t.Labels[LabelOwner] == LabelOwnerTab` (tab ownership, shared by Tasks 7 and 8)
  - `const KindNotFound = "not_found"`, `const KindFinished = "finished"` (hc error kinds Tasks 7 and 8 branch on)
  - `type RunParams struct { Argv []string \`json:"argv"\`; Cwd string \`json:"cwd"\`; Size Size \`json:"size"\`; Labels map[string]string \`json:"labels"\`; NoParent bool \`json:"no_parent"\` }`
  - `type RPCError struct { Method string; Code int; Message string; Kind string; Hint string }` with `Error() string` = `"hc <method>: <message> (kind <kind>)"`
  - `type Exit struct { Reason string \`json:"reason"\`; ExitCode *int \`json:"exit_code"\`; Signal *int \`json:"signal"\` }` with `Error() string` = `"program ended: reason <r>, exit code <n|none>, signal <n|none>"`
  - `var ErrCoreGone = errors.New("hc core connection closed")`, `var ErrCoreOffline = errors.New("hc core offline")`, `var ErrDetached = errors.New("hc stream detached")`
  - `func Dial(ctx context.Context, socketPath string) (*Client, error)`
  - `func (c *Client) Call(ctx context.Context, method string, params, result any) error`
  - `func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error)`
  - `func (c *Client) List(ctx context.Context, labels map[string]string) ([]Terminal, error)` — sends `all: true`; never returns a nil slice on success
  - `func (c *Client) Get(ctx context.Context, id string) (Terminal, error)`
  - `func (c *Client) Label(ctx context.Context, id string, labels map[string]string) (Terminal, error)`
  - `func (c *Client) Kill(ctx context.Context, id string) (Terminal, error)`
  - `func (c *Client) Remove(ctx context.Context, id string) error`
  - `func (c *Client) Attach(ctx context.Context, id string, size Size) (*Stream, error)`
  - `func (c *Client) Done() <-chan struct{}` — closed when the Unix connection ends.
  - `func (c *Client) Close() error`
  - `func (s *Stream) Recv(ctx context.Context) ([]byte, error)` — next DATA payload; at stream end returns `*Exit` (core CLOSE with reason), `ErrDetached` (after our `Close`) or an error wrapping `ErrCoreGone` (connection ended).
  - `func (s *Stream) Credit(n int) error`
  - `func (s *Stream) Write(p []byte) error` — DATA
  - `func (s *Stream) Resize(size Size) error` — RESIZE
  - `func (s *Stream) Close() error` — detach (CLOSE, empty); no-op when the stream already ended
  - Every write error from `Credit`/`Write`/`Resize`/`Close`/`Call` wraps `ErrCoreGone` (a failed Unix write closes the connection).
  - `func Start(ctx context.Context, socketPath string) (*Supervisor, error)`
  - `func (s *Supervisor) Client() (*Client, error)` — current client or an error wrapping `ErrCoreOffline`
  - `func (s *Supervisor) Await(ctx context.Context) (*Client, error)` — blocks until online or ctx done
  - `func (s *Supervisor) Close() error`
- Produces (`package hctest`, test helper imported by Tasks 7 and 8):
  - `type Fake struct { Path string }` (plus unexported state)
  - `func Start(t testing.TB) *Fake` — socket at `<os.MkdirTemp("", "rt")>/hc.sock`, cleanup via `t.Cleanup`.
  - `func (f *Fake) Stop()` — close listener and every connection, remove the socket file (models crash/stop). Idempotent.
  - `func (f *Fake) Restart(t testing.TB)` — listen again on `f.Path`; terminals survive.
  - `func (f *Fake) Seed(term hc.Terminal)` — add a terminal record as-is.
  - `func (f *Fake) Get(id string) (hc.Terminal, bool)`
  - `func (f *Fake) Exit(id string, code int)` — mark finished (`reason "exited"`), send CLOSE with reason JSON to every attach stream of `id`.
  - `func (f *Fake) Input(id string) <-chan []byte` — DATA payloads received from viewers (buffered 64).
  - `func (f *Fake) Resized(id string) <-chan hc.Size` — RESIZE frames received (buffered 64).
  - `func (f *Fake) FailNext(method string, e hc.RPCError)` — the next call of `method` returns this error (`Code` 0 means -32000).
  - `func Paint(id string) []byte` — first-paint bytes the fake sends: `"\x18\x1b[!p" + "screen:" + id`.

**Behavior:**
- One Unix connection per `Client`. `Dial`: `net.Dialer.DialContext("unix", path)`; on failure return `fmt.Errorf("hc not running: %w; expected a listening herdr-core socket at %s (start it with \"hc server start\")", err, path)` — the wrapped `*net.OpError` already includes `dial unix <path>`. Then send plain hello `{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true,"generation":1}}\n`, read one `\n` line (ctx end unblocks the read via a past deadline), check `generation == 1`, `framed == true`, `features` contains `attach` and `events`; mismatch → error `hc hello at <path>: expected <field> <want>, got <got>`.
- A reader goroutine owns the read side: channel 0 DATA → JSON-RPC reply routed by `id` to the waiting `Call`; other channels → that stream's queue (mutex + slice + notify channel; unbounded is safe because hc stops at the 256 KiB credit window — marked `ponytail:`). CLOSE from core: payload non-empty → parse `Exit`, deliver as stream end; empty → reply to our detach; `Recv` then returns `ErrDetached`. Unknown channel → ignore frame (stream already closed by us). Read error/EOF → mark closed, close `Done()` **first**, then fail all pending calls and end all streams with an error wrapping `ErrCoreGone`. (Done first: a caller that retries via `Supervisor.Client` must already see offline.)
- Writes serialized by one mutex (frames must not interleave).
- `Call` result decoding: JSON-RPC `error` → `*RPCError{Method, Code, Message, Kind: data.kind, Hint: data.hint}`. ctx done → return `ctx.Err()` wrapped with method name; the reply, if it arrives later, is dropped.
- `Attach`: the pending call is flagged; the reader, when it routes the attach reply, creates and registers the stream under the client lock before it reads the next frame, so the first-paint frame is never lost.
- Credit: bridge calls `Credit(len(p))` after writing each payload to the WebSocket.
- Supervisor: `Start` dials once; dial failure → return the `Dial` error (criterion 6). Then it creates an fsnotify watcher on `filepath.Dir(socketPath)`. State: `cur *Client` plus `online chan struct{}` (closed while online). `currentLocked()` flips to offline (cur = nil, new open `online` channel) whenever `cur.Done()` is closed; `Client`, `Await` and the loop all go through it, so no caller ever sees a dead client as online. Loop: select on ctx, `cur.Done()`, watcher events, watcher errors. Each event whose `filepath.Clean(event.Name) == filepath.Clean(socketPath)` (any op) while offline triggers one `Dial`; success → `cur = c`, close `online`; failure → stay offline and `log.Printf("hc reconnect: %v", err)`. Events while online are ignored (macOS repeats spurious `CREATE`). Watcher error → log and keep watching. ponytail: one dial per event; a dial landing between a new core's bind and listen waits for the next event on that name (`docs/hc-bridge.md` §6).
- Fake: speaks the real format: plain hello line reply `{"jsonrpc":"2.0","id":<id>,"result":{"generation":1,"framed":true,"features":["framed","attach","events"],"version":"0.1.21","core":{"id":"fake","name":"fake"},"methods":[]}}`, then frames. Listening binds `<Path>.tmp` and renames it to `Path`, so the socket file appears only when it already accepts (no bind→listen gap in tests). Methods: `terminal.run` (assign `t1`, `t2`, …, `pid` 1000+n, `created_ms` n, state live, store labels/argv/cwd/size), `terminal.list` (filter by all given labels; `all` honored), `terminal.get`, `terminal.label` (merge, `""` deletes), `terminal.kill` (reply with the finished record, reason `killed`, signal 1, then CLOSE `{"exit_code":null,"finished_ms":0,"reason":"killed","signal":1}` to streams), `terminal.rm` (finished only; live → error kind `invalid_params`), `terminal.attach` (unknown → `-32000` kind `not_found` message `no terminal "<id>"`; finished → kind `finished` message `<id> has finished`; else record size, reply `{"channel":n,"automatic_input":true,"resize_notifications":false,"terminal":{…}}` then DATA `Paint(id)`). Stream DATA from viewer → copied to `Input(id)` and echoed back as DATA. RESIZE → record size and send to `Resized(id)`. CLOSE from viewer → empty CLOSE reply.

**Constraints:**
- Only stdlib + fsnotify in `hc`; `hctest` stdlib + `hc`.
- No goroutine leaks: `Client.Close` waits for the reader; `Supervisor.Close` stops the loop, closes the watcher and the client; `Fake.Stop` waits for its goroutines.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/hc/... -count=1 -race -v` passes and runs `TestFrameRoundTrip`, `TestDialMissingSocketNamesPath`, `TestDialRejectsWrongGeneration`, `TestCallReturnsRPCError`, `TestAttachFirstPaintThenEcho`, `TestStreamExitDeliversCode`, `TestStreamEndsWhenCoreGone`, `TestSupervisorOfflineThenReconnect`, `TestSupervisorIgnoresOtherFiles`.
- [ ] `cd host && go test ./internal/hc/... -count=20 -race` passes (flake check; not part of `scripts/test.py`).
- [ ] `TestDialMissingSocketNamesPath` asserts the error string contains the socket path and `hc server start`, for both `Dial` and `Start`.
- [ ] `grep -rniE 'sleep' host/internal/hc; echo RC=$?` → `RC=1`.

**Steps:**

- [ ] **Step 1: Write the failing tests**

`host/internal/hc/client_test.go`:

```go
package hc_test

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

func tabParams() hc.RunParams {
	return hc.RunParams{Argv: []string{"/bin/sh"}, Cwd: "/", Size: hc.Size{Cols: 80, Rows: 24},
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: "sh"}, NoParent: true}
}

func dialFake(t *testing.T) (*hctest.Fake, *hc.Client) {
	t.Helper()
	f := hctest.Start(t)
	c, err := hc.Dial(t.Context(), f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("client close: %v", err)
		}
	})
	return f, c
}

func TestFrameRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	in := hc.Frame{Channel: 7, Kind: hc.KindResize, Payload: []byte{1, 2, 3, 4}}
	if err := hc.WriteFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	want := []byte{7, 0, 0, 0, 3, 4, 0, 0, 0, 1, 2, 3, 4}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("wire bytes: expected %v, got %v", want, buf.Bytes())
	}
	out, err := hc.ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.Channel != 7 || out.Kind != hc.KindResize || !bytes.Equal(out.Payload, in.Payload) {
		t.Fatalf("round trip: expected %+v, got %+v", in, out)
	}
}

func TestDialMissingSocketNamesPath(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "none.sock")
	_, err = hc.Dial(t.Context(), path)
	if err == nil {
		t.Fatal("Dial missing socket: expected error, got nil")
	}
	for _, want := range []string{path, "hc server start"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Dial error %q: missing %q", err, want)
		}
	}
	if _, err := hc.Start(t.Context(), path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("Start missing socket: expected error naming %s, got %v", path, err)
	}
}

func TestDialRejectsWrongGeneration(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "g.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			return
		}
		conn.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{"generation":2,"framed":true,"features":["attach","events"]}}` + "\n"))
	}()
	_, err = hc.Dial(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "expected generation 1, got 2") {
		t.Fatalf("Dial: expected generation error, got %v", err)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	t.Parallel()
	_, c := dialFake(t)
	_, err := c.Get(t.Context(), "t999")
	var re *hc.RPCError
	if !errors.As(err, &re) {
		t.Fatalf("Get unknown: expected *hc.RPCError, got %v", err)
	}
	if re.Method != "terminal.get" || re.Code != -32000 || re.Kind != hc.KindNotFound || re.Message != `no terminal "t999"` || re.Hint == "" {
		t.Fatalf("RPCError fields: got %+v", re)
	}
}

func TestAttachFirstPaintThenEcho(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Recv(ctx)
	if err != nil || !bytes.Equal(p, hctest.Paint(term.Address)) {
		t.Fatalf("first Recv: expected paint, got %q, %v", p, err)
	}
	if err := st.Credit(len(p)); err != nil {
		t.Fatal(err)
	}
	if err := st.Write([]byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	if got := <-f.Input(term.Address); string(got) != "ls\r" {
		t.Fatalf("fake input: expected ls\\r, got %q", got)
	}
	if p, err := st.Recv(ctx); err != nil || string(p) != "ls\r" {
		t.Fatalf("echo: expected ls\\r, got %q, %v", p, err)
	}
	if err := st.Resize(hc.Size{Cols: 132, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	if got := <-f.Resized(term.Address); got != (hc.Size{Cols: 132, Rows: 40}) {
		t.Fatalf("fake resize: got %+v", got)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); !errors.Is(err, hc.ErrDetached) {
		t.Fatalf("after Close: expected ErrDetached, got %v", err)
	}
}

func TestStreamExitDeliversCode(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	f.Exit(term.Address, 7)
	_, err = st.Recv(ctx)
	var ex *hc.Exit
	if !errors.As(err, &ex) || ex.Reason != "exited" || ex.ExitCode == nil || *ex.ExitCode != 7 || ex.Signal != nil {
		t.Fatalf("Recv after exit: expected *hc.Exit{exited, 7, nil}, got %v", err)
	}
	_, err = c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	var re *hc.RPCError
	if !errors.As(err, &re) || re.Kind != hc.KindFinished {
		t.Fatalf("Attach finished: expected kind finished, got %v", err)
	}
}

func TestStreamEndsWhenCoreGone(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	f.Stop()
	if _, err := st.Recv(ctx); !errors.Is(err, hc.ErrCoreGone) {
		t.Fatalf("Recv after core stop: expected ErrCoreGone, got %v", err)
	}
	<-c.Done()
	if _, err := c.Get(ctx, term.Address); !errors.Is(err, hc.ErrCoreGone) {
		t.Fatalf("Get after core stop: expected ErrCoreGone, got %v", err)
	}
}
```

`host/internal/hc/supervisor_test.go`:

```go
package hc_test

import (
	"bytes"
	"errors"
	"testing"

	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

func TestSupervisorOfflineThenReconnect(t *testing.T) {
	t.Parallel()
	f := hctest.Start(t)
	ctx := t.Context()
	sup, err := hc.Start(ctx, f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sup.Close(); err != nil {
			t.Errorf("supervisor close: %v", err)
		}
	})
	c1, err := sup.Client()
	if err != nil {
		t.Fatal(err)
	}
	term, err := c1.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	f.Stop()
	<-c1.Done()
	if _, err := sup.Client(); !errors.Is(err, hc.ErrCoreOffline) {
		t.Fatalf("Client while core down: expected ErrCoreOffline, got %v", err)
	}
	f.Restart(t)
	c2, err := sup.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c2 == c1 {
		t.Fatal("Await: expected a new client after restart")
	}
	st, err := c2.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if p, err := st.Recv(ctx); err != nil || !bytes.Equal(p, hctest.Paint(term.Address)) {
		t.Fatalf("re-attach first paint: got %q, %v", p, err)
	}
}
```

`host/internal/hc/supervisor_internal_test.go`:

```go
package hc

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestSupervisorIgnoresOtherFiles(t *testing.T) {
	t.Parallel()
	const sock = "/run/user/501/herdr-core/default.sock"
	cases := []struct {
		name string
		want bool
	}{
		{sock, true},
		{"/run/user/501/herdr-core/./default.sock", true},
		{"/run/user/501/herdr-core/default.sock.tmp", false},
		{"/run/user/501/herdr-core/core.json", false},
		{"/run/user/501/herdr-core", false},
	}
	for _, c := range cases {
		if got := matchesSocket(fsnotify.Event{Name: c.name, Op: fsnotify.Create}, sock); got != c.want {
			t.Errorf("matchesSocket(%q): expected %v, got %v", c.name, c.want, got)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd host && go test ./internal/hc/... -count=1`
Expected: FAIL to build (`no non-test Go files` / `undefined: hc.Frame`, `undefined: matchesSocket`).

- [ ] **Step 3: Write `host/internal/hc/proto.go`**

```go
// Package hc speaks herdr-core's framed JSON-RPC over its Unix socket (docs/hc-bridge.md).
package hc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Frame kinds (docs/hc-bridge.md §3).
const (
	KindData   byte = 0
	KindCredit byte = 1
	KindClose  byte = 2
	KindResize byte = 3
)

const (
	frameHeaderLen         = 9
	maxFramePayload        = 16 << 20
	rpcChannel      uint32 = 0
)

// Frame is [channel u32 LE][kind u8][len u32 LE][payload].
type Frame struct {
	Channel uint32
	Kind    byte
	Payload []byte
}

func WriteFrame(w io.Writer, f Frame) error {
	buf := make([]byte, frameHeaderLen+len(f.Payload))
	binary.LittleEndian.PutUint32(buf[0:4], f.Channel)
	buf[4] = f.Kind
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(f.Payload)))
	copy(buf[frameHeaderLen:], f.Payload)
	_, err := w.Write(buf)
	return err
}

func ReadFrame(r io.Reader) (Frame, error) {
	var h [frameHeaderLen]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Frame{}, err
	}
	f := Frame{Channel: binary.LittleEndian.Uint32(h[0:4]), Kind: h[4]}
	n := binary.LittleEndian.Uint32(h[5:9])
	if n > maxFramePayload {
		return Frame{}, fmt.Errorf("hc frame on channel %d: expected payload <= %d bytes, got %d", f.Channel, maxFramePayload, n)
	}
	f.Payload = make([]byte, n)
	if _, err := io.ReadFull(r, f.Payload); err != nil {
		return Frame{}, fmt.Errorf("hc frame on channel %d: payload: %w", f.Channel, err)
	}
	return f, nil
}

// resizePayload is u16 LE cols, u16 LE rows.
func resizePayload(s Size) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:2], s.Cols)
	binary.LittleEndian.PutUint16(b[2:4], s.Rows)
	return b
}

// ParseResize decodes a RESIZE payload (used by the fake core).
func ParseResize(p []byte) (Size, error) {
	if len(p) != 4 {
		return Size{}, fmt.Errorf("hc resize payload: expected 4 bytes, got %d", len(p))
	}
	return Size{Cols: binary.LittleEndian.Uint16(p[0:2]), Rows: binary.LittleEndian.Uint16(p[2:4])}, nil
}
```

- [ ] **Step 4: Write `host/internal/hc/types.go`**

```go
package hc

import (
	"errors"
	"fmt"
)

type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Terminal is the hc terminal record (fields roosterm reads).
type Terminal struct {
	Address   string            `json:"address"`
	Argv      []string          `json:"argv"`
	Labels    map[string]string `json:"labels"`
	State     string            `json:"state"`
	Reason    *string           `json:"reason"`
	ExitCode  *int              `json:"exit_code"`
	Signal    *int              `json:"signal"`
	PID       *int              `json:"pid"`
	Cwd       string            `json:"cwd"`
	CreatedMS int64             `json:"created_ms"`
	Cols      uint16            `json:"cols"`
	Rows      uint16            `json:"rows"`
}

const (
	StateLive     = "live"
	StateFinished = "finished"
)

// Tab ownership labels.
const (
	LabelOwner    = "roosterm"
	LabelOwnerTab = "tab"
	LabelName     = "roosterm.name"
)

// hc error kinds roosterm branches on.
const (
	KindNotFound = "not_found"
	KindFinished = "finished"
)

func IsTab(t Terminal) bool { return t.Labels[LabelOwner] == LabelOwnerTab }

type RunParams struct {
	Argv     []string          `json:"argv"`
	Cwd      string            `json:"cwd"`
	Size     Size              `json:"size"`
	Labels   map[string]string `json:"labels"`
	NoParent bool              `json:"no_parent"`
}

// RPCError is a JSON-RPC error returned by hc.
type RPCError struct {
	Method  string
	Code    int
	Message string
	Kind    string
	Hint    string
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("hc %s: %s (kind %s)", e.Method, e.Message, e.Kind)
}

// Exit is the CLOSE reason hc sends when a program ends.
type Exit struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exit_code"`
	Signal   *int   `json:"signal"`
}

func (e *Exit) Error() string {
	return fmt.Sprintf("program ended: reason %s, exit code %s, signal %s", e.Reason, optInt(e.ExitCode), optInt(e.Signal))
}

func optInt(p *int) string {
	if p == nil {
		return "none"
	}
	return fmt.Sprint(*p)
}

var (
	ErrCoreGone    = errors.New("hc core connection closed")
	ErrCoreOffline = errors.New("hc core offline")
	ErrDetached    = errors.New("hc stream detached")
)
```

- [ ] **Step 5: Write `host/internal/hc/client.go`**

```go
package hc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"
)

const (
	protocolGeneration = 1
	jsonrpcVersion     = "2.0"
)

var requiredFeatures = []string{"attach", "events"}

// Client is one Unix connection to hc: calls on channel 0, attach streams on other channels.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
	path string

	wmu sync.Mutex // frames must not interleave

	mu      sync.Mutex
	closed  bool
	nextID  uint64
	pending map[uint64]*pendingCall
	streams map[uint32]*Stream

	done      chan struct{}
	closeOnce sync.Once
}

type pendingCall struct {
	method   string
	terminal string // set for terminal.attach: the reader registers the stream on the reply
	attach   bool
	reply    chan callReply // buffered 1
}

type callReply struct {
	result json.RawMessage
	stream *Stream
	err    error
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	ID     *uint64         `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Kind string `json:"kind"`
			Hint string `json:"hint"`
		} `json:"data"`
	} `json:"error"`
}

// Dial connects to the hc socket and performs the plain-mode hello.
func Dial(ctx context.Context, socketPath string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("hc not running: %w; expected a listening herdr-core socket at %s (start it with \"hc server start\")", err, socketPath)
	}
	r := bufio.NewReader(conn)
	if err := hello(ctx, conn, r, socketPath); err != nil {
		// ponytail: hello already failed; the close error adds nothing.
		_ = conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, r: r, path: socketPath, pending: map[uint64]*pendingCall{},
		streams: map[uint32]*Stream{}, done: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func hello(ctx context.Context, conn net.Conn, r *bufio.Reader, path string) error {
	stop := context.AfterFunc(ctx, func() {
		// Unblocks the hello read when ctx ends; the deadline error is reported below.
		_ = conn.SetDeadline(time.Unix(1, 0))
	})
	defer stop()
	req, err := json.Marshal(rpcRequest{JSONRPC: jsonrpcVersion, ID: 0, Method: "hello",
		Params: map[string]any{"framed": true, "generation": protocolGeneration}})
	if err != nil {
		return fmt.Errorf("hc hello at %s: encode: %w", path, err)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("hc hello at %s: write: %w", path, err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("hc hello at %s: read: %w", path, err)
	}
	var resp struct {
		Result *struct {
			Generation int      `json:"generation"`
			Framed     bool     `json:"framed"`
			Features   []string `json:"features"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("hc hello at %s: expected a JSON-RPC line, got %q: %w", path, line, err)
	}
	if resp.Result == nil {
		return fmt.Errorf("hc hello at %s: expected a result, got error %s", path, resp.Error)
	}
	if resp.Result.Generation != protocolGeneration {
		return fmt.Errorf("hc hello at %s: expected generation %d, got %d", path, protocolGeneration, resp.Result.Generation)
	}
	if !resp.Result.Framed {
		return fmt.Errorf("hc hello at %s: expected framed true, got false", path)
	}
	for _, f := range requiredFeatures {
		if !slices.Contains(resp.Result.Features, f) {
			return fmt.Errorf("hc hello at %s: expected feature %q, got features %v", path, f, resp.Result.Features)
		}
	}
	return nil
}

// Done is closed when the Unix connection has ended.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close ends the connection and waits for the reader to stop.
func (c *Client) Close() error {
	err := c.conn.Close()
	<-c.done
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (c *Client) readLoop() {
	c.shutdown(c.readFrames())
}

func (c *Client) readFrames() error {
	for {
		f, err := ReadFrame(c.r)
		if err != nil {
			return err
		}
		if f.Channel == rpcChannel {
			if f.Kind == KindData {
				c.routeReply(f.Payload)
			}
			continue
		}
		c.mu.Lock()
		s := c.streams[f.Channel]
		if s != nil && f.Kind == KindClose {
			delete(c.streams, f.Channel)
		}
		c.mu.Unlock()
		if s == nil {
			continue // channel already detached by us
		}
		switch f.Kind {
		case KindData:
			s.push(f.Payload)
		case KindClose:
			if len(f.Payload) == 0 {
				s.end(ErrDetached)
				continue
			}
			var e Exit
			if err := json.Unmarshal(f.Payload, &e); err != nil {
				s.end(fmt.Errorf("hc stream %s: close reason %q: %w", s.terminal, f.Payload, err))
				continue
			}
			s.end(&e)
		}
	}
}

func (c *Client) routeReply(payload []byte) {
	var resp rpcResponse
	if err := json.Unmarshal(payload, &resp); err != nil || resp.ID == nil {
		return // not a reply (notification) or not JSON: nothing waits for it
	}
	c.mu.Lock()
	pc := c.pending[*resp.ID]
	delete(c.pending, *resp.ID)
	if pc == nil {
		c.mu.Unlock()
		return // caller gave up (ctx done)
	}
	rep := callReply{result: resp.Result}
	switch {
	case resp.Error != nil:
		rep.err = &RPCError{Method: pc.method, Code: resp.Error.Code, Message: resp.Error.Message,
			Kind: resp.Error.Data.Kind, Hint: resp.Error.Data.Hint}
	case pc.attach:
		var a struct {
			Channel uint32 `json:"channel"`
		}
		if err := json.Unmarshal(resp.Result, &a); err != nil || a.Channel == rpcChannel {
			rep.err = fmt.Errorf("hc %s %s: expected result with a non-zero channel, got %s", pc.method, pc.terminal, resp.Result)
			break
		}
		// Registered before the reader reads the next frame, so the first paint is not lost.
		rep.stream = newStream(c, a.Channel, pc.terminal)
		c.streams[a.Channel] = rep.stream
	}
	c.mu.Unlock()
	pc.reply <- rep
}

func (c *Client) shutdown(cause error) {
	c.closeOnce.Do(func() {
		gone := fmt.Errorf("%w (%s: %v)", ErrCoreGone, c.path, cause)
		c.mu.Lock()
		c.closed = true
		pending, streams := c.pending, c.streams
		c.pending, c.streams = map[uint64]*pendingCall{}, map[uint32]*Stream{}
		c.mu.Unlock()
		// ponytail: the reader already failed; closing is cleanup only.
		_ = c.conn.Close()
		close(c.done) // before waking callers, so Supervisor.Client sees offline when they retry
		for _, pc := range pending {
			pc.reply <- callReply{err: fmt.Errorf("hc %s: %w", pc.method, gone)}
		}
		for _, s := range streams {
			s.end(gone)
		}
	})
}

func (c *Client) writeFrame(f Frame) error {
	c.wmu.Lock()
	err := WriteFrame(c.conn, f)
	c.wmu.Unlock()
	if err != nil {
		// A failed write means the connection is unusable; closing it lets the reader end every stream.
		_ = c.conn.Close()
		return fmt.Errorf("hc write channel %d: %w (%v)", f.Channel, ErrCoreGone, err)
	}
	return nil
}

// Call sends one JSON-RPC request and decodes the result into result (nil: ignore result).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	_, err := c.call(ctx, method, "", false, params, result)
	return err
}

func (c *Client) call(ctx context.Context, method, terminal string, attach bool, params, result any) (*Stream, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("hc %s: %w", method, ErrCoreGone)
	}
	c.nextID++
	id := c.nextID
	pc := &pendingCall{method: method, terminal: terminal, attach: attach, reply: make(chan callReply, 1)}
	c.pending[id] = pc
	c.mu.Unlock()

	msg, err := json.Marshal(rpcRequest{JSONRPC: jsonrpcVersion, ID: id, Method: method, Params: params})
	if err != nil {
		c.forget(id)
		return nil, fmt.Errorf("hc %s: encode params: %w", method, err)
	}
	if err := c.writeFrame(Frame{Channel: rpcChannel, Kind: KindData, Payload: msg}); err != nil {
		c.forget(id)
		return nil, fmt.Errorf("hc %s: %w", method, err)
	}
	select {
	case rep := <-pc.reply:
		if rep.err != nil {
			return nil, rep.err
		}
		if result != nil {
			if err := json.Unmarshal(rep.result, result); err != nil {
				return nil, fmt.Errorf("hc %s: decode result %s: %w", method, rep.result, err)
			}
		}
		return rep.stream, nil
	case <-ctx.Done():
		// ponytail: an attach reply arriving after this is dropped and its channel stays open on the
		// hc side until the connection ends; only happens when the caller's ctx ends mid-attach.
		c.forget(id)
		return nil, fmt.Errorf("hc %s: %w", method, ctx.Err())
	}
}

func (c *Client) forget(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

type terminalRef struct {
	Terminal string `json:"terminal"`
}

func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.run", p, &t)
	return t, err
}

// List returns live and finished terminals carrying all given labels.
func (c *Client) List(ctx context.Context, labels map[string]string) ([]Terminal, error) {
	var res struct {
		Terminals []Terminal `json:"terminals"`
	}
	err := c.Call(ctx, "terminal.list", map[string]any{"all": true, "labels": labels}, &res)
	if err != nil {
		return nil, err
	}
	if res.Terminals == nil {
		res.Terminals = []Terminal{}
	}
	return res.Terminals, nil
}

func (c *Client) Get(ctx context.Context, id string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.get", terminalRef{id}, &t)
	return t, err
}

// Label merges labels; a value "" removes that key.
func (c *Client) Label(ctx context.Context, id string, labels map[string]string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.label", map[string]any{"terminal": id, "labels": labels}, &t)
	return t, err
}

func (c *Client) Kill(ctx context.Context, id string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.kill", terminalRef{id}, &t)
	return t, err
}

func (c *Client) Remove(ctx context.Context, id string) error {
	return c.Call(ctx, "terminal.rm", terminalRef{id}, nil)
}

// Attach opens a viewer stream; the first Recv returns the full-screen first paint.
func (c *Client) Attach(ctx context.Context, id string, size Size) (*Stream, error) {
	return c.call(ctx, "terminal.attach", id, true,
		map[string]any{"terminal": id, "cols": size.Cols, "rows": size.Rows}, nil)
}
```

- [ ] **Step 6: Write `host/internal/hc/stream.go`**

```go
package hc

import (
	"context"
	"encoding/binary"
	"sync"
)

// Stream is one attach channel.
type Stream struct {
	c        *Client
	channel  uint32
	terminal string

	mu     sync.Mutex
	queue  [][]byte // ponytail: unbounded, safe because hc stops sending at its 256 KiB credit window
	endErr error
	notify chan struct{} // buffered 1
}

func newStream(c *Client, channel uint32, terminal string) *Stream {
	return &Stream{c: c, channel: channel, terminal: terminal, notify: make(chan struct{}, 1)}
}

func (s *Stream) push(p []byte) {
	s.mu.Lock()
	s.queue = append(s.queue, p)
	s.mu.Unlock()
	s.signal()
}

func (s *Stream) end(err error) {
	s.mu.Lock()
	if s.endErr == nil {
		s.endErr = err
	}
	s.mu.Unlock()
	s.signal()
}

func (s *Stream) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Recv returns the next DATA payload. Queued data is delivered before the end error:
// *Exit (program ended), ErrDetached (after Close) or an error wrapping ErrCoreGone.
func (s *Stream) Recv(ctx context.Context) ([]byte, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			p := s.queue[0]
			s.queue[0] = nil
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return p, nil
		}
		if s.endErr != nil {
			err := s.endErr
			s.mu.Unlock()
			return nil, err
		}
		s.mu.Unlock()
		select {
		case <-s.notify:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Credit tells hc that n bytes from this channel were consumed.
func (s *Stream) Credit(n int) error {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(n))
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindCredit, Payload: b})
}

// Write sends keystroke bytes as DATA.
func (s *Stream) Write(p []byte) error {
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindData, Payload: p})
}

func (s *Stream) Resize(size Size) error {
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindResize, Payload: resizePayload(size)})
}

// Close detaches (empty CLOSE). A stream that already ended needs no detach.
func (s *Stream) Close() error {
	s.mu.Lock()
	ended := s.endErr != nil
	s.mu.Unlock()
	if ended {
		return nil
	}
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindClose})
}
```

- [ ] **Step 7: Write `host/internal/hc/supervisor.go`**

```go
package hc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Supervisor keeps one Client and re-dials when the socket file changes while offline.
type Supervisor struct {
	path    string
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	stopped chan struct{}

	mu     sync.Mutex
	cur    *Client       // nil while offline
	online chan struct{} // closed while online; replaced on each transition to offline
}

// Start dials once (failure is returned: hc not running) and then watches the socket directory.
// ponytail: dial before watch; a core that dies and re-binds inside that microsecond window is
// noticed only on the next event for the socket name.
func Start(ctx context.Context, socketPath string) (*Supervisor, error) {
	c, err := Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("hc supervisor %s: new watcher: %w", socketPath, err), c.Close())
	}
	dir := filepath.Dir(socketPath)
	if err := w.Add(dir); err != nil {
		return nil, errors.Join(fmt.Errorf("hc supervisor %s: watch %s: %w", socketPath, dir, err), w.Close(), c.Close())
	}
	online := make(chan struct{})
	close(online)
	lctx, cancel := context.WithCancel(ctx)
	s := &Supervisor{path: socketPath, watcher: w, cancel: cancel, stopped: make(chan struct{}), cur: c, online: online}
	go s.loop(lctx)
	return s, nil
}

// currentLocked returns the live client or nil, flipping to offline when the client's connection ended.
func (s *Supervisor) currentLocked() *Client {
	if s.cur != nil && isClosed(s.cur.Done()) {
		s.cur = nil
		s.online = make(chan struct{})
	}
	return s.cur
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Client returns the current client, or ErrCoreOffline.
func (s *Supervisor) Client() (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.currentLocked(); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("hc %s: %w", s.path, ErrCoreOffline)
}

// Await blocks until a client is online or ctx ends.
func (s *Supervisor) Await(ctx context.Context) (*Client, error) {
	for {
		s.mu.Lock()
		c := s.currentLocked()
		ch := s.online
		s.mu.Unlock()
		if c != nil {
			return c, nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func matchesSocket(ev fsnotify.Event, socketPath string) bool {
	return filepath.Clean(ev.Name) == filepath.Clean(socketPath)
}

func (s *Supervisor) loop(ctx context.Context) {
	defer close(s.stopped)
	for {
		s.mu.Lock()
		var done <-chan struct{}
		if c := s.currentLocked(); c != nil {
			done = c.Done()
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-done:
			// next iteration's currentLocked flips to offline
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if matchesSocket(ev, s.path) {
				s.redial(ctx)
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("hc watch %s: %v", filepath.Dir(s.path), err)
		}
	}
}

// redial dials once if offline. Events while online are ignored (macOS repeats CREATE).
func (s *Supervisor) redial(ctx context.Context) {
	s.mu.Lock()
	c := s.currentLocked()
	s.mu.Unlock()
	if c != nil {
		return
	}
	nc, err := Dial(ctx, s.path)
	if err != nil {
		log.Printf("hc reconnect: %v", err)
		return
	}
	s.mu.Lock()
	s.cur = nc
	close(s.online)
	s.mu.Unlock()
}

// Close stops watching and closes the current client.
func (s *Supervisor) Close() error {
	s.cancel()
	<-s.stopped
	err := s.watcher.Close()
	s.mu.Lock()
	c := s.cur
	s.cur = nil
	s.online = make(chan struct{}) // Await callers wait for their ctx from now on
	s.mu.Unlock()
	if c != nil {
		err = errors.Join(err, c.Close())
	}
	return err
}
```

- [ ] **Step 8: Write `host/internal/hc/hctest/fake.go`**

```go
// Package hctest is an in-process fake herdr-core speaking the real wire format over a Unix socket.
package hctest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"roosterm/host/internal/hc"
)

const (
	firstPID   = 1000
	chanBuffer = 64
	errCode    = -32000
	sigHUP     = 1
)

// Paint is the first-paint payload the fake sends on every attach.
func Paint(id string) []byte { return []byte("\x18\x1b[!p" + "screen:" + id) }

type Fake struct {
	Path string

	mu      sync.Mutex
	ln      net.Listener
	conns   map[*fakeConn]struct{}
	terms   map[string]hc.Terminal
	runs    int
	fail    map[string]hc.RPCError
	inputs  map[string]chan []byte
	resizes map[string]chan hc.Size
	wg      sync.WaitGroup
}

type fakeConn struct {
	conn     net.Conn
	wmu      sync.Mutex
	streams  map[uint32]string // channel → terminal id, guarded by Fake.mu
	nextChan uint32
}

// Start listens at <os.MkdirTemp("", "rt")>/hc.sock (short path: macOS limits socket paths to 104 bytes).
func Start(t testing.TB) *Fake {
	t.Helper()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fake{Path: filepath.Join(dir, "hc.sock"), conns: map[*fakeConn]struct{}{}, terms: map[string]hc.Terminal{},
		fail: map[string]hc.RPCError{}, inputs: map[string]chan []byte{}, resizes: map[string]chan hc.Size{}}
	t.Cleanup(func() {
		f.Stop()
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("hctest cleanup %s: %v", dir, err)
		}
	})
	f.listen(t)
	return f
}

// listen binds a temp name and renames it to Path, so the socket file appears only once it accepts
// (a watcher's dial on the CREATE event never meets a bound-but-not-listening socket).
func (f *Fake) listen(t testing.TB) {
	t.Helper()
	tmp := f.Path + ".tmp"
	ln, err := net.Listen("unix", tmp)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := os.Rename(tmp, f.Path); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.ln = ln
	f.mu.Unlock()
	f.wg.Add(1)
	go f.accept(ln)
}

// Restart listens again on Path after Stop (terminals survive, like a real core restart).
func (f *Fake) Restart(t testing.TB) {
	t.Helper()
	f.listen(t)
}

// Stop closes the listener and every connection and removes the socket file (core crash/stop).
func (f *Fake) Stop() {
	f.mu.Lock()
	ln := f.ln
	f.ln = nil
	conns := f.conns
	f.conns = map[*fakeConn]struct{}{}
	f.mu.Unlock()
	if ln == nil {
		return
	}
	if err := ln.Close(); err != nil {
		panic(fmt.Sprintf("hctest stop: close listener: %v", err))
	}
	for fc := range conns {
		// ponytail: the peer may have closed first; either way the connection is gone.
		_ = fc.conn.Close()
	}
	if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		panic(fmt.Sprintf("hctest stop: remove %s: %v", f.Path, err))
	}
	f.wg.Wait()
}

func (f *Fake) Seed(term hc.Terminal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terms[term.Address] = term
}

func (f *Fake) Get(id string) (hc.Terminal, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.terms[id]
	return t, ok
}

// FailNext makes the next call of method return e.
func (f *Fake) FailNext(method string, e hc.RPCError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[method] = e
}

// Input yields DATA payloads viewers sent to id.
func (f *Fake) Input(id string) <-chan []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inputLocked(id)
}

func (f *Fake) inputLocked(id string) chan []byte {
	ch, ok := f.inputs[id]
	if !ok {
		ch = make(chan []byte, chanBuffer)
		f.inputs[id] = ch
	}
	return ch
}

// Resized yields RESIZE frames viewers sent to id.
func (f *Fake) Resized(id string) <-chan hc.Size {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resizedLocked(id)
}

func (f *Fake) resizedLocked(id string) chan hc.Size {
	ch, ok := f.resizes[id]
	if !ok {
		ch = make(chan hc.Size, chanBuffer)
		f.resizes[id] = ch
	}
	return ch
}

// Exit finishes id with reason "exited" and sends the CLOSE reason to every attach stream of id.
func (f *Fake) Exit(id string, code int) {
	reason := "exited"
	f.finish(id, hc.Terminal{Reason: &reason, ExitCode: &code})
}

func (f *Fake) finish(id string, end hc.Terminal) hc.Terminal {
	f.mu.Lock()
	t := f.terms[id]
	t.State = hc.StateFinished
	t.Reason, t.ExitCode, t.Signal = end.Reason, end.ExitCode, end.Signal
	f.terms[id] = t
	type target struct {
		fc *fakeConn
		ch uint32
	}
	var targets []target
	for fc := range f.conns {
		for ch, tid := range fc.streams {
			if tid == id {
				targets = append(targets, target{fc, ch})
				delete(fc.streams, ch)
			}
		}
	}
	f.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"exit_code": t.ExitCode, "finished_ms": 0, "reason": t.Reason, "signal": t.Signal})
	if err != nil {
		panic(err)
	}
	for _, tg := range targets {
		tg.fc.write(hc.Frame{Channel: tg.ch, Kind: hc.KindClose, Payload: payload})
	}
	return t
}

func (f *Fake) accept(ln net.Listener) {
	defer f.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed by Stop
		}
		fc := &fakeConn{conn: conn, streams: map[uint32]string{}}
		f.mu.Lock()
		f.conns[fc] = struct{}{}
		f.mu.Unlock()
		f.wg.Add(1)
		go f.serve(fc)
	}
}

func (fc *fakeConn) write(fr hc.Frame) {
	fc.wmu.Lock()
	defer fc.wmu.Unlock()
	// ponytail: a write to a connection the viewer already closed is expected in teardown.
	_ = hc.WriteFrame(fc.conn, fr)
}

func (f *Fake) serve(fc *fakeConn) {
	defer f.wg.Done()
	defer func() {
		_ = fc.conn.Close() // ponytail: connection is finished either way
		f.mu.Lock()
		delete(f.conns, fc)
		f.mu.Unlock()
	}()
	r := bufio.NewReader(fc.conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var hello struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(line, &hello); err != nil || hello.Method != "hello" {
		return
	}
	reply := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"generation":1,"framed":true,"features":["framed","attach","events"],"version":"0.1.21","core":{"id":"fake","name":"fake"},"methods":[]}}`+"\n", hello.ID)
	fc.wmu.Lock()
	_, err = fc.conn.Write([]byte(reply))
	fc.wmu.Unlock()
	if err != nil {
		return
	}
	for {
		fr, err := hc.ReadFrame(r)
		if err != nil {
			return
		}
		f.handle(fc, fr)
	}
}

func (f *Fake) handle(fc *fakeConn, fr hc.Frame) {
	if fr.Channel == 0 {
		if fr.Kind == hc.KindData {
			f.rpc(fc, fr.Payload)
		}
		return
	}
	f.mu.Lock()
	id, ok := fc.streams[fr.Channel]
	if ok && fr.Kind == hc.KindClose {
		delete(fc.streams, fr.Channel)
	}
	var input chan []byte
	var resized chan hc.Size
	if ok {
		input, resized = f.inputLocked(id), f.resizedLocked(id)
	}
	f.mu.Unlock()
	if !ok {
		return
	}
	switch fr.Kind {
	case hc.KindData:
		input <- fr.Payload
		fc.write(hc.Frame{Channel: fr.Channel, Kind: hc.KindData, Payload: fr.Payload}) // echo
	case hc.KindResize:
		size, err := hc.ParseResize(fr.Payload)
		if err != nil {
			panic(err)
		}
		f.mu.Lock()
		t := f.terms[id]
		t.Cols, t.Rows = size.Cols, size.Rows
		f.terms[id] = t
		f.mu.Unlock()
		resized <- size
	case hc.KindClose:
		fc.write(hc.Frame{Channel: fr.Channel, Kind: hc.KindClose})
	}
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Kind string `json:"kind"`
		Hint string `json:"hint,omitempty"`
	} `json:"data"`
}

func newErr(kind, msg, hint string) *rpcErr {
	e := &rpcErr{Code: errCode, Message: msg}
	e.Data.Kind, e.Data.Hint = kind, hint
	return e
}

func notFound(id string) *rpcErr {
	return newErr(hc.KindNotFound, fmt.Sprintf("no terminal %q", id), "see `hc ls --all`")
}

func (f *Fake) rpc(fc *fakeConn, payload []byte) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		panic(fmt.Sprintf("hctest: bad request %q: %v", payload, err))
	}
	var p struct {
		hc.RunParams
		Terminal string            `json:"terminal"`
		All      bool              `json:"all"`
		Cols     uint16            `json:"cols"`
		Rows     uint16            `json:"rows"`
		Labels   map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		panic(fmt.Sprintf("hctest: bad params %q: %v", req.Params, err))
	}
	result, rerr, after := f.dispatch(fc, req.Method, p.RunParams, p.Terminal, p.All, p.Labels, hc.Size{Cols: p.Cols, Rows: p.Rows})
	msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rerr != nil {
		msg["error"] = rerr
	} else {
		msg["result"] = result
	}
	b, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	fc.write(hc.Frame{Channel: 0, Kind: hc.KindData, Payload: b})
	if after != nil {
		after()
	}
}

// dispatch returns the result or error, and an action to run after the reply is written.
func (f *Fake) dispatch(fc *fakeConn, method string, run hc.RunParams, id string, all bool, labels map[string]string, size hc.Size) (any, *rpcErr, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.fail[method]; ok {
		delete(f.fail, method)
		code := e.Code
		if code == 0 {
			code = errCode
		}
		re := newErr(e.Kind, e.Message, e.Hint)
		re.Code = code
		return nil, re, nil
	}
	switch method {
	case "terminal.run":
		f.runs++
		pid := firstPID + f.runs
		// run.Labels is shadowed by the outer "labels" field; labels carries it.
		t := hc.Terminal{Address: fmt.Sprintf("t%d", f.runs), Argv: run.Argv, Labels: map[string]string{}, State: hc.StateLive,
			PID: &pid, Cwd: run.Cwd, CreatedMS: int64(f.runs), Cols: run.Size.Cols, Rows: run.Size.Rows}
		for k, v := range labels {
			t.Labels[k] = v
		}
		f.terms[t.Address] = t
		return t, nil, nil
	case "terminal.list":
		out := []hc.Terminal{}
		for _, t := range f.terms {
			if t.State == hc.StateFinished && !all {
				continue
			}
			match := true
			for k, v := range labels {
				if t.Labels[k] != v {
					match = false
				}
			}
			if match {
				out = append(out, t)
			}
		}
		return map[string]any{"terminals": out, "hidden_finished": 0, "machines": []any{}}, nil, nil
	}
	t, ok := f.terms[id]
	if !ok {
		return nil, notFound(id), nil
	}
	switch method {
	case "terminal.get":
		return t, nil, nil
	case "terminal.label":
		merged := map[string]string{}
		for k, v := range t.Labels {
			merged[k] = v
		}
		for k, v := range labels {
			if v == "" {
				delete(merged, k)
			} else {
				merged[k] = v
			}
		}
		t.Labels = merged
		f.terms[id] = t
		return t, nil, nil
	case "terminal.kill":
		reason, sig := "killed", sigHUP
		// finish takes f.mu itself, so it runs after the reply, outside this lock
		t.State, t.Reason, t.ExitCode, t.Signal = hc.StateFinished, &reason, nil, &sig
		return t, nil, func() { f.finish(id, hc.Terminal{Reason: &reason, Signal: &sig}) }
	case "terminal.rm":
		if t.State != hc.StateFinished {
			return nil, newErr("invalid_params", fmt.Sprintf("%s is live; kill it first", id), ""), nil
		}
		delete(f.terms, id)
		return map[string]string{"removed": id}, nil, nil
	case "terminal.attach":
		if t.State == hc.StateFinished {
			return nil, newErr(hc.KindFinished, id+" has finished", "read what it left: hc read "+id), nil
		}
		fc.nextChan++
		ch := fc.nextChan
		fc.streams[ch] = id
		t.Cols, t.Rows = size.Cols, size.Rows
		f.terms[id] = t
		return map[string]any{"channel": ch, "automatic_input": true, "resize_notifications": false, "terminal": t}, nil,
			func() { fc.write(hc.Frame{Channel: ch, Kind: hc.KindData, Payload: Paint(id)}) }
	}
	return nil, newErr("invalid_params", "hctest: method not implemented: "+method, ""), nil
}
```

Note on `terminal.kill`: the reply goes out before `finish` runs, and `finish` re-reads the record under the lock, so the DELETE path (Task 7) sees `finished` on its following `terminal.rm`: `rpc` runs `after()` before it reads the next frame on that connection.

- [ ] **Step 9: Run the tests to see them pass, then the flake check**

```bash
cd host
gofmt -w internal/hc
go vet ./internal/hc/...; echo VET_RC=$?
go test ./internal/hc/... -count=1 -race -v; echo RC=$?
go test ./internal/hc/... -count=20 -race; echo FLAKE_RC=$?
gofmt -l internal/hc
cd .. && grep -rniE 'sleep' host/internal/hc; echo GREP_RC=$?
```
Expected: nine `--- PASS` lines (names in the acceptance criteria), `VET_RC=0`, `RC=0`, `FLAKE_RC=0`, no gofmt output, `GREP_RC=1`.

- [ ] **Step 10: Commit**

```bash
git add host/internal/hc/proto.go host/internal/hc/types.go host/internal/hc/client.go host/internal/hc/stream.go host/internal/hc/supervisor.go host/internal/hc/hctest/fake.go host/internal/hc/client_test.go host/internal/hc/supervisor_test.go host/internal/hc/supervisor_internal_test.go
git commit -m "feat(host): hc framed JSON-RPC client, supervisor, fake core" -- host/internal/hc
git status --porcelain host/internal/hc
```
Expected: the status command prints nothing.

### Task 4: Swarm scan and scripts

**Files:**
- Create: `host/internal/swarm/swarm.go`, `host/internal/swarm/swarm_test.go`
- Create: `scripts/test.py`, `scripts/build.py`
- Modify: `.gitignore` (add `build/`)

**Interfaces:**
- Produces (`package swarm`):
  - `const Prefix = "claude-swarm-"`
  - `func Dir(getenv func(string) string, uid int) string` — `<TMUX_TMPDIR or /tmp>/tmux-<uid>`
  - `func List(dir string) ([]string, error)` — names of entries that are Unix sockets (`fs.ModeSocket`) and start with `Prefix`, sorted. `dir` missing → `[]string{}`, nil (no tmux server has run; truthful empty). Other errors → `fmt.Errorf("swarm list %s: %w", dir, err)`.
- Produces (`scripts/`):
  - `python3 scripts/test.py` — exit 0 only if all checks pass.
  - `python3 scripts/build.py [--target <goos>/<goarch>]` — writes `build/roosterm` (or `build/roosterm-<goos>-<goarch>` with `--target`).

**Behavior:**
- Swarm: never connects to the sockets (other live Claude sessions own them).
- `test.py` (stdlib only), from any cwd (paths from `__file__`):
  1. Forbidden-word scan: every `host/**/*_test.go` and every file under `host/internal/hc/hctest/` matched against regex `(?i)\bsleep\b|time\.Sleep` → any hit: print `file:line` and exit 1.
  2. `gofmt -l .` (cwd `host`) must print nothing; `go vet ./...` must exit 0.
  3. `go test ./... -json -count=1 -race -timeout 30s` (cwd `host`, default package parallelism). Parse events: per-test `Elapsed` from `pass`/`fail` actions; count `skip` actions that carry a `Test` field (package-level `[no test files]` skips have none).
  4. One budget `SUITE_BUDGET_S = 40` for the whole script wall time (each subprocess gets the remaining time as its timeout). Fail (exit 1) if: go test RC ≠ 0, wall time > budget, any test `Elapsed` > `TEST_BUDGET_S = 1.0`, skipped > 0, zero tests ran. Always print: `tests: N`, `skipped: N`, `wall: X s`, and the 5 slowest tests.
- `build.py`: if `web/package.json` exists → `npm ci --prefix web` then `npm run build --prefix web` (the client plan sets Vite `outDir` to `host/internal/web/dist`); else print `web/: no package.json, embedding host/internal/web/dist as committed`. Then `go build -trimpath -o build/<name> ./cmd/roosterm` (cwd `host`, `CGO_ENABLED=0`, `GOOS`/`GOARCH` from `--target`). Non-zero RC of any step → print the command and exit with that RC. Bad `--target` → argparse error, RC 2.

**Constraints:**
- Python stdlib only, no platform branches. Named constants for budgets.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/swarm/ -count=1 -race -v` runs `TestListOnlyClaudeSwarmSockets`, `TestListMissingDir`, `TestDir` and passes.
- [ ] After Tasks 1–3 have committed: `python3 scripts/test.py; echo RC=$?` → `RC=0`, output contains `skipped: 0`.
- [ ] Injected violation (red before green): write `host/internal/swarm/inject_test.go` containing `package swarm_test` and the comment line `// sleep`; `python3 scripts/test.py; echo RC=$?` → `RC=1` and output names `host/internal/swarm/inject_test.go:3`; `rm host/internal/swarm/inject_test.go`; rerun → `RC=0`. Never commit the injected file.
- [ ] `grep -n '^build/$' .gitignore` → 1 match.
- [ ] `python3 scripts/build.py --target bad; echo RC=$?` → `RC=2` (full builds are checked in Task 9, which owns `cmd/roosterm`).

**Steps:**

- [ ] **Step 1: Write the failing test `host/internal/swarm/swarm_test.go`**

```go
package swarm_test

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"roosterm/host/internal/swarm"
)

func TestListOnlyClaudeSwarmSockets(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	for _, name := range []string{"claude-swarm-9", "default", "claude-swarm-10"} {
		ln, err := net.Listen("unix", filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
	}
	if err := os.WriteFile(filepath.Join(dir, "claude-swarm-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := swarm.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude-swarm-10", "claude-swarm-9"}
	if !slices.Equal(got, want) {
		t.Fatalf("List: expected %v, got %v", want, got)
	}
}

func TestListMissingDir(t *testing.T) {
	t.Parallel()
	got, err := swarm.List(filepath.Join(t.TempDir(), "absent"))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("List missing dir: expected [], nil; got %#v, %v", got, err)
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if got := swarm.Dir(getenv, 501); got != "/tmp/tmux-501" {
		t.Fatalf("Dir without TMUX_TMPDIR: got %s", got)
	}
	env["TMUX_TMPDIR"] = "/var/tmux"
	if got := swarm.Dir(getenv, 501); got != "/var/tmux/tmux-501" {
		t.Fatalf("Dir with TMUX_TMPDIR: got %s", got)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd host && go test ./internal/swarm/ -count=1`
Expected: FAIL to build (`no non-test Go files` or `undefined: swarm.List`).

- [ ] **Step 3: Write `host/internal/swarm/swarm.go`**

```go
// Package swarm lists Claude agent-team tmux servers (claude-swarm-* sockets). It never connects to them.
package swarm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const Prefix = "claude-swarm-"

// Dir is tmux's socket directory: ${TMUX_TMPDIR:-/tmp}/tmux-<uid> (man tmux).
func Dir(getenv func(string) string, uid int) string {
	base := getenv("TMUX_TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, fmt.Sprintf("tmux-%d", uid))
}

// List returns the names of Unix sockets in dir that start with Prefix, sorted.
// A missing dir means no tmux server has run: empty list, no error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("swarm list %s: %w", dir, err)
	}
	names := []string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), Prefix) && e.Type()&fs.ModeSocket != 0 {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
```

- [ ] **Step 4: Run the swarm tests to see them pass**

```bash
cd host && gofmt -w internal/swarm && go vet ./internal/swarm/ && go test ./internal/swarm/ -count=1 -race -v; echo RC=$?
```
Expected: three `--- PASS` lines, `RC=0`.

- [ ] **Step 5: Write `scripts/test.py`**

```python
#!/usr/bin/env python3
"""Roosterm test entry: forbidden-word scan, gofmt, go vet, go test with time budgets.

Run from anywhere: python3 scripts/test.py. Exit 0 only if every check passes.
"""
import json
import pathlib
import re
import subprocess
import sys
import time
from typing import NoReturn

ROOT = pathlib.Path(__file__).resolve().parent.parent
HOST = ROOT / "host"
SUITE_BUDGET_S = 40.0  # whole script wall time (contract criterion 2)
TEST_BUDGET_S = 1.0  # any single test
SLOWEST_SHOWN = 5
GO_TEST_TIMEOUT = "30s"  # go's own hang guard: dumps goroutines before the budget kill
FORBIDDEN = re.compile(r"(?i)\bsleep\b|time\.Sleep")


def fail(msg: str) -> NoReturn:
    print(f"FAIL: {msg}")
    sys.exit(1)


def scan_forbidden() -> None:
    files = sorted(HOST.rglob("*_test.go")) + sorted((HOST / "internal" / "hc" / "hctest").rglob("*.go"))
    hits = []
    for f in files:
        for n, line in enumerate(f.read_text(encoding="utf-8").splitlines(), 1):
            if FORBIDDEN.search(line):
                hits.append(f"{f.relative_to(ROOT)}:{n}: {line.strip()}")
    if hits:
        fail("sleep-like text in test code:\n" + "\n".join(hits))
    print(f"forbidden-word scan: {len(files)} files clean")


def run(cmd: list[str], timeout: float) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(cmd, cwd=HOST, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        fail(f"{' '.join(cmd)}: exceeded the {SUITE_BUDGET_S:.0f} s suite budget")


def remaining(start: float) -> float:
    left = SUITE_BUDGET_S - (time.monotonic() - start)
    if left <= 0:
        fail(f"suite budget {SUITE_BUDGET_S:.0f} s used up before the next step")
    return left


def main() -> None:
    start = time.monotonic()
    scan_forbidden()
    fmt = run(["gofmt", "-l", "."], remaining(start))
    if fmt.returncode != 0 or fmt.stdout.strip():
        fail(f"gofmt -l (rc {fmt.returncode}) lists unformatted files:\n{fmt.stdout}{fmt.stderr}")
    vet = run(["go", "vet", "./..."], remaining(start))
    if vet.returncode != 0:
        fail(f"go vet rc {vet.returncode}:\n{vet.stdout}{vet.stderr}")
    test = run(["go", "test", "./...", "-json", "-count=1", "-race", "-timeout", GO_TEST_TIMEOUT], remaining(start))
    elapsed: dict[str, float] = {}
    failed, skipped, output = [], [], []
    for line in test.stdout.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            output.append(line)
            continue
        if ev.get("Action") == "output":
            output.append(ev.get("Output", "").rstrip("\n"))
        name = ev.get("Test")
        if not name:
            continue
        key = f"{ev['Package']}.{name}"
        action = ev.get("Action")
        if action in ("pass", "fail"):
            elapsed[key] = ev.get("Elapsed", 0.0)
        if action == "fail":
            failed.append(key)
        if action == "skip":
            skipped.append(key)
    wall = time.monotonic() - start
    slowest = sorted(elapsed.items(), key=lambda kv: kv[1], reverse=True)[:SLOWEST_SHOWN]
    print(f"tests: {len(elapsed)}")
    print(f"skipped: {len(skipped)}")
    print(f"wall: {wall:.1f} s (budget {SUITE_BUDGET_S:.0f} s)")
    print("slowest:")
    for key, secs in slowest:
        print(f"  {secs:.2f} s  {key}")
    problems = []
    if test.returncode != 0:
        problems.append(f"go test rc {test.returncode}; failed: {failed}\n" + "\n".join(output[-60:]) + test.stderr)
    if wall > SUITE_BUDGET_S:
        problems.append(f"wall {wall:.1f} s > {SUITE_BUDGET_S:.0f} s")
    over = [f"{k} {s:.2f} s" for k, s in elapsed.items() if s > TEST_BUDGET_S]
    if over:
        problems.append(f"tests over {TEST_BUDGET_S} s: {over}")
    if skipped:
        problems.append(f"skipped tests: {skipped}")
    if not elapsed:
        problems.append("zero tests ran")
    if problems:
        fail("\n".join(problems))
    print("PASS")


if __name__ == "__main__":
    main()
```

The first run after a dependency change compiles the race runtime and may exceed 40 s; that run fails loudly with the budget message. Warm the cache once with `cd host && go test -race -count=1 -run '^$' ./...` before the recorded run (compilation is not test time; the recorded run must still be ≤ 40 s whole-script).

- [ ] **Step 6: Write `scripts/build.py`**

```python
#!/usr/bin/env python3
"""Build roosterm into build/: the web client (when web/package.json exists), then the Go binary.

python3 scripts/build.py                        -> build/roosterm (host platform)
python3 scripts/build.py --target linux/arm64   -> build/roosterm-linux-arm64
"""
import argparse
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
HOST = ROOT / "host"
WEB = ROOT / "web"
BUILD = ROOT / "build"


def step(cmd: list[str], cwd: pathlib.Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(cmd), flush=True)
    rc = subprocess.run(cmd, cwd=cwd, env=env).returncode
    if rc != 0:
        print(f"FAIL: {' '.join(cmd)} (cwd {cwd}) exited {rc}")
        sys.exit(rc)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", help="GOOS/GOARCH, e.g. linux/arm64")
    args = ap.parse_args()
    env = dict(os.environ, CGO_ENABLED="0")
    name = "roosterm"
    if args.target:
        parts = args.target.split("/")
        if len(parts) != 2 or not all(parts):
            ap.error(f"--target: expected GOOS/GOARCH like linux/arm64, got {args.target!r}")
        env["GOOS"], env["GOARCH"] = parts
        name = f"roosterm-{parts[0]}-{parts[1]}"
    if (WEB / "package.json").exists():
        step(["npm", "ci", "--prefix", str(WEB)], ROOT)
        step(["npm", "run", "build", "--prefix", str(WEB)], ROOT)
    else:
        print("web/: no package.json, embedding host/internal/web/dist as committed")
    BUILD.mkdir(exist_ok=True)
    out = BUILD / name
    step(["go", "build", "-trimpath", "-o", str(out), "./cmd/roosterm"], HOST, env)
    print(f"wrote {out.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 7: Add `build/` to `.gitignore`**

Append one line `build/` to `.gitignore` with the Edit tool (the file currently contains `scratch/`). Check: `grep -n '^build/$' .gitignore` → one match.

- [ ] **Step 8: Prove the runner goes red, then green (after Tasks 1–3 have committed)**

1. Write `host/internal/swarm/inject_test.go` with the Write tool, content exactly:
   ```go
   package swarm_test

   // sleep
   ```
2. `python3 scripts/test.py; echo RC=$?` → `FAIL: sleep-like text in test code:` then `host/internal/swarm/inject_test.go:3: // sleep`, `RC=1`.
3. `rm host/internal/swarm/inject_test.go`
4. `cd host && go test -race -count=1 -run '^$' ./... && cd ..` (cache warm-up), then `python3 scripts/test.py; echo RC=$?` → `skipped: 0`, `PASS`, `RC=0`.
5. `python3 scripts/build.py --target bad; echo RC=$?` → `RC=2`.

Paste both runner outputs (red and green) and the RCs into the report.

- [ ] **Step 9: Commit**

```bash
git add host/internal/swarm/swarm.go host/internal/swarm/swarm_test.go scripts/test.py scripts/build.py .gitignore
git commit -m "feat: swarm socket scan, test and build scripts" -- host/internal/swarm scripts/test.py scripts/build.py .gitignore
git status --porcelain host/internal/swarm scripts .gitignore
```
Expected: the status command prints nothing (`inject_test.go` is gone).

### Task 5: Deploy units

**Files:**
- Create: `deploy/launchd/dev.roosterm.serve.plist`
- Create: `deploy/systemd/herdr-core.service`, `deploy/systemd/roosterm.service`
- Create: `deploy/README.md`

**Interfaces:**
- Consumes: `roosterm serve` (Task 9), `hc server start` (idempotent, spike log "Autostart").
- Produces: files a user copies to `~/Library/LaunchAgents/` or `~/.config/systemd/user/`.

**Behavior:**
- launchd: `Label dev.roosterm.serve`; `ProgramArguments` = `/bin/zsh`, `-lc`, `"$HOME/.local/bin/hc" server start && exec "$HOME/.local/bin/roosterm" serve` (a login shell so the hc core inherits the login environment; absolute hc path because `-lc` does not read `.zshrc`; README says to replace `/bin/zsh` with the user's login shell); `RunAtLoad true`; `KeepAlive` `{SuccessfulExit false}` (restart roosterm after a crash, not after a clean exit); `StandardOutPath`/`StandardErrorPath` `/Users/USERNAME/Library/Logs/roosterm.log` (launchd does not expand `~`; README gives a `sed` line to fill in the user name).
- systemd `herdr-core.service`: `Type=oneshot`, `RemainAfterExit=yes`, `ExecStart=%h/.local/bin/hc server start`, `[Install] WantedBy=default.target`. No `ExecStop` (hc server stop would end every program; README states this).
- systemd `roosterm.service`: `Requires=herdr-core.service`, `After=herdr-core.service`, `ExecStart=%h/.local/bin/roosterm serve`, `Restart=on-failure`, `[Install] WantedBy=default.target`.
- README: install commands for both, `loginctl enable-linger $USER` for boot without login, warning "never `hc server stop` on a live host", log locations.

**Acceptance criteria:**
- [ ] `plutil -lint deploy/launchd/dev.roosterm.serve.plist` → `OK`.
- [ ] In OrbStack `roosterm-spike`: `systemd-analyze --user verify deploy/systemd/herdr-core.service deploy/systemd/roosterm.service; echo RC=$?` → `RC=0`; when it is non-zero, the only accepted messages are about the missing `ExecStart` binaries `%h/.local/bin/roosterm` (record the full output either way).
- [ ] `grep -c 'roosterm" serve\|roosterm serve' deploy/launchd/dev.roosterm.serve.plist deploy/systemd/roosterm.service` → ≥1 each.
- [ ] `grep -rn 'server stop' deploy/launchd deploy/systemd; echo RC=$?` → `RC=1`.

**Steps:**

- [ ] **Step 1: Write `deploy/launchd/dev.roosterm.serve.plist`**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>dev.roosterm.serve</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/zsh</string>
		<string>-lc</string>
		<string>"$HOME/.local/bin/hc" server start &amp;&amp; exec "$HOME/.local/bin/roosterm" serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>/Users/USERNAME/Library/Logs/roosterm.log</string>
	<key>StandardErrorPath</key>
	<string>/Users/USERNAME/Library/Logs/roosterm.log</string>
</dict>
</plist>
```

Run: `plutil -lint deploy/launchd/dev.roosterm.serve.plist` → `deploy/launchd/dev.roosterm.serve.plist: OK`.

- [ ] **Step 2: Write `deploy/systemd/herdr-core.service`**

```ini
[Unit]
Description=herdr-core terminal holder (hc)

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%h/.local/bin/hc server start

[Install]
WantedBy=default.target
```

- [ ] **Step 3: Write `deploy/systemd/roosterm.service`**

```ini
[Unit]
Description=roosterm web terminal server
Requires=herdr-core.service
After=herdr-core.service

[Service]
ExecStart=%h/.local/bin/roosterm serve
Restart=on-failure

[Install]
WantedBy=default.target
```

- [ ] **Step 4: Write `deploy/README.md`**

````markdown
# Deploy roosterm

roosterm needs a running herdr-core (`hc`). Both units below run `hc server start` (idempotent) before `roosterm serve`. Install `hc` and `roosterm` into `~/.local/bin` first, then run `roosterm passwd` once from your login shell.

**Never run `hc server stop` on a live host: it ends every program in every tab.** Neither unit stops hc.

## macOS (launchd)

```sh
sed "s#/Users/USERNAME#$HOME#g" deploy/launchd/dev.roosterm.serve.plist > ~/Library/LaunchAgents/dev.roosterm.serve.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/dev.roosterm.serve.plist
```

The plist starts a login shell (`/bin/zsh -lc`) so hc inherits your login environment. If your login shell is not zsh, replace `/bin/zsh` in the copied plist. launchd restarts roosterm after a crash, not after a clean exit. Log: `~/Library/Logs/roosterm.log`.

Stop roosterm (hc keeps running): `launchctl bootout gui/$(id -u)/dev.roosterm.serve`.

## Linux (systemd user units)

```sh
mkdir -p ~/.config/systemd/user
cp deploy/systemd/herdr-core.service deploy/systemd/roosterm.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now herdr-core.service roosterm.service
loginctl enable-linger "$USER"   # start at boot without a login session
```

Log: `journalctl --user -u roosterm.service`. Stop roosterm (hc keeps running): `systemctl --user stop roosterm.service`. Stopping `herdr-core.service` does not stop hc (the unit has no `ExecStop`).
````

- [ ] **Step 5: Verify**

```bash
plutil -lint deploy/launchd/dev.roosterm.serve.plist; echo PLUTIL_RC=$?
grep -c 'roosterm" serve\|roosterm serve' deploy/launchd/dev.roosterm.serve.plist deploy/systemd/roosterm.service
grep -rn 'server stop' deploy/launchd deploy/systemd; echo STOP_RC=$?
orb -m roosterm-spike bash -c 'cd /Users/'"$USER"'/project/roosterm && systemd-analyze --user verify deploy/systemd/herdr-core.service deploy/systemd/roosterm.service; echo VERIFY_RC=$?'
```
Expected: `PLUTIL_RC=0`, both grep counts ≥ 1, `STOP_RC=1`, `VERIFY_RC=0` or only messages naming the missing `roosterm`/`hc` binaries under `%h/.local/bin`. If `orb` cannot reach the repo path (OrbStack mounts the macOS home at the same path by default), copy the two unit files into the VM with `orb push` and verify there; record which way was used.

- [ ] **Step 6: Commit**

```bash
git add deploy/launchd/dev.roosterm.serve.plist deploy/systemd/herdr-core.service deploy/systemd/roosterm.service deploy/README.md
git commit -m "deploy: launchd plist and systemd user units" -- deploy
git status --porcelain deploy
```
Expected: the status command prints nothing.

---

## Round 2

### Task 6: Web shell — login, logout, session middleware, static embed

**Files:**
- Create: `host/internal/web/server.go`, `host/internal/web/login.go`, `host/internal/web/static.go`
- Create: `host/internal/web/dist/index.html` (placeholder page; the client plan's Vite build overwrites `dist/`)
- Test: `host/internal/web/web_test.go`

**Interfaces:**
- Consumes: `auth.Sessions` (`Valid`, `Create`, `Delete`), `auth.Limiter` (`Allow`, `Fail`, `Succeed`), `auth.VerifyPassword`, `auth.SessionCookie`, `auth.ClearCookie`, `auth.CookieName`; tests also `auth.OpenSessions`, `auth.NewLimiter`, `auth.HashPasswordCost`, `auth.Cost`, `auth.MaxFailures`, `auth.Lockout` (Task 2); `apierr.Error`, `apierr.Write`, `apierr.DecodeJSON`, kind constants (Task 1).
- Produces (`package web`):
  - `type Options struct { Username string; PasswordHash string; SessionTTL time.Duration; Sessions *auth.Sessions; Limiter *auth.Limiter; API http.Handler; TabSocket http.Handler }`
  - `func New(o Options) http.Handler`

**Behavior:**
- Routes on one `http.ServeMux`: `POST /login` (public), `POST /logout` (session), `/api/` → `o.API` (session), `GET /ws/tabs/{id}` → `o.TabSocket` (session; the mux sets path value `id`, which the middleware passes through unchanged), `/` → static (public). Static is registered as `/` (all methods), not `GET /`: in Go 1.22+ routing `GET /` and `/api/` conflict (neither is more specific) and `Handle` panics.
- Session middleware (unexported `requireSession`): read cookie `auth.CookieName`; missing or `!Sessions.Valid` → `apierr.Write` 401 kind `unauthorized`, op `session`, message `missing or expired session cookie`. It runs before any upgrade, so a WebSocket handshake without a cookie gets HTTP 401 and the handler is never called.
- `POST /login`: `Limiter.Allow` first; false → 429, header `Retry-After` = `max(1, ceil(retryAfter.Seconds()))`, kind `too_many_attempts`, message names the seconds; the body is not read. Then `apierr.DecodeJSON` (op `login`), both fields non-empty (400). Credentials: `subtle.ConstantTimeCompare` on username **and** `VerifyPassword` (always both, no early exit); mismatch → `Limiter.Fail`, 401 kind `unauthorized`, message `wrong username or password`. `VerifyPassword` error → 500 kind `internal`. Success → `Limiter.Succeed`, `Sessions.Create` (error → 500), `http.SetCookie(w, auth.SessionCookie(token, o.SessionTTL))`, 204.
- `POST /logout`: `Sessions.Delete(token)` (error → 500), `http.SetCookie(w, auth.ClearCookie())`, 204.
- Static: `//go:embed all:dist`; `fs.Sub(dist, "dist")`; method other than GET/HEAD → 405 kind `bad_request` op `static`; path cleaned; existing regular file → `http.ServeFileFS`; missing or directory → `index.html` (SPA routes); other stat error → 500.

**Constraints:** stdlib + internal packages only.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/web/ -count=1 -race -v` runs and passes `TestLoginSetsCookieAttributes`, `TestLoginWrongPassword`, `TestLoginRateLimited`, `TestAPIRequiresSession`, `TestWebSocketUpgradeRejectedWithoutCookie`, `TestExpiredSessionRejected`, `TestLogoutClearsSession`.
- [ ] `TestWebSocketUpgradeRejectedWithoutCookie` sends `Connection: Upgrade`, `Upgrade: websocket`, `Sec-WebSocket-Version: 13`, `Sec-WebSocket-Key` without cookie → status 401 and the `TabSocket` handler records zero calls.
- [ ] `grep -n 'time.Now' host/internal/web/*.go; echo RC=$?` → `RC=1`.

**Steps:**

- [ ] **Step 1: Write the failing test `host/internal/web/web_test.go`**

```go
package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/auth"
	"roosterm/host/internal/web"
)

const sessionTTL = time.Hour

type env struct {
	t        *testing.T
	h        http.Handler
	now      time.Time
	apiCalls int
	wsCalls  int
}

// newEnv wires web.New with a fixed clock and counting API/TabSocket handlers.
// Requests go through ServeHTTP directly (one goroutine), so plain fields are race-free.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, now: time.Unix(1_800_000_000, 0)}
	clock := func() time.Time { return e.now }
	sessions, err := auth.OpenSessions(filepath.Join(t.TempDir(), "sessions.json"), sessionTTL, clock)
	if err != nil {
		t.Fatal(err)
	}
	hash := auth.HashPasswordCost("pw", []byte("0123456789abcdef"), auth.Cost{Time: 1, MemoryKiB: 64, Threads: 1})
	e.h = web.New(web.Options{Username: "me", PasswordHash: hash, SessionTTL: sessionTTL, Sessions: sessions,
		Limiter: auth.NewLimiter(clock),
		API:     http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.apiCalls++ }),
		TabSocket: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e.wsCalls++
			if r.PathValue("id") != "t1" {
				t.Errorf("TabSocket path value id: expected t1, got %q", r.PathValue("id"))
			}
		}),
	})
	return e
}

func (e *env) do(method, target, body string, cookie *http.Cookie, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) login(user, pass string) *httptest.ResponseRecorder {
	b, err := json.Marshal(map[string]string{"username": user, "password": pass})
	if err != nil {
		e.t.Fatal(err)
	}
	return e.do("POST", "/login", string(b), nil, nil)
}

func (e *env) loggedIn() *http.Cookie {
	e.t.Helper()
	rec := e.login("me", "pw")
	if rec.Code != http.StatusNoContent {
		e.t.Fatalf("login: expected 204, got %d %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return &http.Cookie{Name: c.Name, Value: c.Value}
		}
	}
	e.t.Fatal("login: no session cookie")
	return nil
}

func errKind(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Op   string `json:"op"`
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body, err)
	}
	if body.Error.Op == "" {
		t.Fatalf("error body %q: missing op", rec.Body)
	}
	return body.Error.Kind
}

func TestLoginSetsCookieAttributes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.login("me", "pw")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login: expected 204, got %d %s", rec.Code, rec.Body)
	}
	sc := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"roosterm_session=", "Path=/", "Max-Age=3600", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(sc, want) {
			t.Errorf("Set-Cookie %q: missing %q", sc, want)
		}
	}
	if c := rec.Result().Cookies(); len(c) != 1 || len(c[0].Value) != 43 {
		t.Fatalf("session cookie: expected one 43-char token, got %v", c)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, c := range [][2]string{{"me", "nope"}, {"you", "pw"}} {
		rec := e.login(c[0], c[1])
		if rec.Code != http.StatusUnauthorized || errKind(t, rec) != "unauthorized" {
			t.Errorf("login %v: expected 401 unauthorized, got %d %s", c, rec.Code, rec.Body)
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("login %v: expected no cookie", c)
		}
	}
	if rec := e.do("POST", "/login", `{"username":"me"}`, nil, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("login without password: expected 400, got %d", rec.Code)
	}
	if rec := e.do("POST", "/login", `{"username":"me","password":"pw","x":1}`, nil, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("login with unknown field: expected 400, got %d", rec.Code)
	}
}

func TestLoginRateLimited(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for i := 0; i < auth.MaxFailures; i++ {
		if rec := e.login("me", "nope"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: expected 401, got %d", i+1, rec.Code)
		}
	}
	rec := e.login("me", "pw")
	if rec.Code != http.StatusTooManyRequests || errKind(t, rec) != "too_many_attempts" {
		t.Fatalf("locked login: expected 429 too_many_attempts, got %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After: expected 60, got %q", got)
	}
	e.now = e.now.Add(auth.Lockout)
	if rec := e.login("me", "pw"); rec.Code != http.StatusNoContent {
		t.Fatalf("after lockout: expected 204, got %d", rec.Code)
	}
}

func TestAPIRequiresSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, c := range []*http.Cookie{nil, {Name: auth.CookieName, Value: "bogus"}} {
		rec := e.do("GET", "/api/tabs", "", c, nil)
		if rec.Code != http.StatusUnauthorized || errKind(t, rec) != "unauthorized" {
			t.Fatalf("cookie %v: expected 401, got %d", c, rec.Code)
		}
	}
	if e.apiCalls != 0 {
		t.Fatalf("API handler: expected 0 calls without session, got %d", e.apiCalls)
	}
	if rec := e.do("GET", "/api/tabs", "", e.loggedIn(), nil); rec.Code != http.StatusOK || e.apiCalls != 1 {
		t.Fatalf("with session: expected 200 and 1 API call, got %d and %d", rec.Code, e.apiCalls)
	}
}

func TestWebSocketUpgradeRejectedWithoutCookie(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	h := http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Sec-Websocket-Version": {"13"},
		"Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="}}
	rec := e.do("GET", "/ws/tabs/t1?cols=80&rows=24", "", nil, h)
	if rec.Code != http.StatusUnauthorized || e.wsCalls != 0 {
		t.Fatalf("upgrade without cookie: expected 401 and 0 handler calls, got %d and %d", rec.Code, e.wsCalls)
	}
	if rec := e.do("GET", "/ws/tabs/t1?cols=80&rows=24", "", e.loggedIn(), h); e.wsCalls != 1 {
		t.Fatalf("upgrade with cookie: expected 1 handler call, got %d (status %d)", e.wsCalls, rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.loggedIn()
	e.now = e.now.Add(sessionTTL)
	if rec := e.do("GET", "/api/tabs", "", c, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: expected 401, got %d", rec.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.loggedIn()
	rec := e.do("POST", "/logout", "", c, nil)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: expected 204 with Max-Age=0 cookie, got %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	if rec := e.do("GET", "/api/tabs", "", c, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout: expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd host && go test ./internal/web/ -count=1`
Expected: FAIL to build (`undefined: web.New`).

- [ ] **Step 3: Write `host/internal/web/dist/index.html`**

```html
<!doctype html><title>Roosterm</title><p>Roosterm host is running. The web client is not built yet.</p>
```

- [ ] **Step 4: Write `host/internal/web/server.go`**

```go
// Package web is roosterm's HTTP shell: login/logout, the session gate, and the embedded static page.
package web

import (
	"net/http"
	"time"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/auth"
)

type Options struct {
	Username     string
	PasswordHash string
	SessionTTL   time.Duration
	Sessions     *auth.Sessions
	Limiter      *auth.Limiter
	API          http.Handler // mounted at /api/
	TabSocket    http.Handler // mounted at GET /ws/tabs/{id}
}

type server struct{ o Options }

func New(o Options) http.Handler {
	s := &server{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", s.login)
	mux.Handle("POST /logout", s.requireSession(http.HandlerFunc(s.logout)))
	mux.Handle("/api/", s.requireSession(o.API))
	mux.Handle("GET /ws/tabs/{id}", s.requireSession(o.TabSocket))
	// "/" (all methods) rather than "GET /": "GET /" conflicts with "/api/" in Go 1.22+ routing.
	mux.Handle("/", staticHandler())
	return mux
}

// requireSession runs before any WebSocket upgrade, so a handshake without a cookie gets HTTP 401.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName) // only error: http.ErrNoCookie
		if err != nil || !s.o.Sessions.Valid(c.Value) {
			apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: "session",
				Kind: apierr.KindUnauthorized, Message: "missing or expired session cookie"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 5: Write `host/internal/web/login.go`**

```go
package web

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	const op = "login"
	if retry, ok := s.o.Limiter.Allow(); !ok {
		secs := max(1, int(math.Ceil(retry.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		apierr.Write(w, &apierr.Error{Status: http.StatusTooManyRequests, Op: op, Kind: apierr.KindTooManyAttempts,
			Message: fmt.Sprintf("too many failed logins: retry in %d s", secs)})
		return
	}
	var req loginRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		apierr.Write(w, e)
		return
	}
	if req.Username == "" || req.Password == "" {
		apierr.Write(w, &apierr.Error{Status: http.StatusBadRequest, Op: op, Kind: apierr.KindBadRequest,
			Message: "username, password: expected non-empty strings, got an empty value"})
		return
	}
	// Both checks always run (no early exit on a wrong username).
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.o.Username)) == 1
	passOK, err := auth.VerifyPassword(s.o.PasswordHash, req.Password)
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: op, Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	if !userOK || !passOK {
		s.o.Limiter.Fail()
		apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: op, Kind: apierr.KindUnauthorized,
			Message: "wrong username or password"})
		return
	}
	s.o.Limiter.Succeed()
	token, err := s.o.Sessions.Create()
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: op, Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	http.SetCookie(w, auth.SessionCookie(token, s.o.SessionTTL))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.CookieName)
	if err != nil { // unreachable behind requireSession; kept as a loud failure, not a silent pass
		apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: "logout", Kind: apierr.KindUnauthorized, Message: err.Error()})
		return
	}
	if err := s.o.Sessions.Delete(c.Value); err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "logout", Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	http.SetCookie(w, auth.ClearCookie())
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 6: Write `host/internal/web/static.go`**

```go
package web

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"roosterm/host/internal/apierr"
)

//go:embed all:dist
var dist embed.FS

const indexFile = "index.html"

// staticHandler serves dist/; unknown paths get index.html (SPA routes).
func staticHandler() http.Handler {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("web: embedded dist: %v", err)) // constant directory name: cannot fail at run time
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			apierr.Write(w, &apierr.Error{Status: http.StatusMethodNotAllowed, Op: "static", Kind: apierr.KindBadRequest,
				Message: fmt.Sprintf("method: expected GET or HEAD for %s, got %s", r.URL.Path, r.Method)})
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = indexFile
		}
		info, err := fs.Stat(files, name)
		switch {
		case errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir()):
			name = indexFile
		case err != nil:
			apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "static", Kind: apierr.KindInternal, Message: err.Error()})
			return
		}
		http.ServeFileFS(w, r, files, name)
	})
}
```

- [ ] **Step 7: Run the tests to see them pass**

```bash
cd host
gofmt -w internal/web
go vet ./internal/web/; echo VET_RC=$?
go test ./internal/web/ -count=1 -race -v; echo RC=$?
gofmt -l internal/web
cd .. && grep -n 'time.Now' host/internal/web/*.go; echo GREP_RC=$?
```
Expected: seven `--- PASS` lines (names in the acceptance criteria), `VET_RC=0`, `RC=0`, no gofmt output, `GREP_RC=1`.

- [ ] **Step 8: Commit**

```bash
git add host/internal/web/server.go host/internal/web/login.go host/internal/web/static.go host/internal/web/dist/index.html host/internal/web/web_test.go
git commit -m "feat(host): login, logout, session gate, embedded static page" -- host/internal/web
git status --porcelain host/internal/web
```
Expected: the status command prints nothing.

### Task 7: REST API — tabs and swarms

**Files:**
- Create: `host/internal/api/api.go`, `host/internal/api/tabs.go`, `host/internal/api/swarms.go`
- Test: `host/internal/api/api_test.go`

**Interfaces:**
- Consumes: `hc.Start`, `hc.Supervisor.Client`, `hc.Client.{Run,List,Get,Label,Kill,Remove,Done}`, `hc.RunParams`, `hc.Terminal`, `hc.Size`, `hc.RPCError`, `hc.ErrCoreOffline`, `hc.ErrCoreGone`, `hc.IsTab`, `hc.LabelOwner`, `hc.LabelOwnerTab`, `hc.LabelName`, `hc.StateFinished`, `hc.StateLive`, `hc.KindNotFound` (Task 3); `hctest.Start`, `Fake.{Seed,Get,FailNext,Stop}` (Task 3, tests); `swarm.List` (Task 4); `apierr.{Error,Write,WriteJSON,DecodeJSON}` and kinds (Task 1).
- Produces (`package api`):
  - `type Options struct { Core *hc.Supervisor; Shell string; Home string; SwarmDir string }`
  - `func New(o Options) http.Handler` — handles `GET /api/tabs`, `POST /api/tabs`, `PATCH /api/tabs/{id}`, `DELETE /api/tabs/{id}`, `GET /api/swarms`, and a JSON 404 for any other `/api/` request, on its own `ServeMux` (Task 6 mounts it at `/api/`).
  - `type Tab struct { ID string \`json:"id"\`; Label string \`json:"label"\`; Exited bool \`json:"exited"\`; ExitCode *int \`json:"exitCode"\`; Signal *int \`json:"signal"\`; Reason *string \`json:"reason"\` }`

**Behavior:**
- Exactly the "HTTP/WS API" section. Ops for error bodies: `tab.list`, `tab.create`, `tab.rename`, `tab.close`, `swarm.list`, `api` (unknown endpoint).
- Named constants: `maxLabelLen = 64`, `initialSize = hc.Size{Cols: 80, Rows: 24}`; body limit is `apierr.MaxBody`.
- hc error mapping (unexported `hcError(op, tab string, err error) *apierr.Error`): `errors.Is(err, hc.ErrCoreOffline)` or `errors.Is(err, hc.ErrCoreGone)` → 503 `core_offline`; `errors.As(*hc.RPCError)` with kind `not_found` → 404 with hc kind/hint; any other kind → 502 with hc kind/hint; message `<method>[ <tab>]: <hc message>`; else 500 `internal`.
- Ownership: PATCH/DELETE call `Get` first; `!hc.IsTab` → 404 `not_found`, message `tab <id> not found`.
- List: `List(ctx, {LabelOwner: LabelOwnerTab})`, stable sort by `CreatedMS`, map to `Tab`. Empty → `[]`, never `null`.
- Create: input validated before touching hc; argv as specified in the API section; `Cwd = o.Home`; labels `{LabelOwner: LabelOwnerTab, LabelName: label}`; `NoParent: true`; `Size: initialSize`; 201 with the mapped record.
- Rename: `Label(ctx, id, {LabelName: label})` → 200 mapped record.
- Delete: not finished → `Kill` then `Remove`; finished → `Remove`; 204.
- Swarms: `swarm.List(o.SwarmDir)` → 200 JSON array; error → 500 `internal` with the error text.

**Constraints:** stdlib + internal packages.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/api/ -count=1 -race -v` runs and passes `TestCreateTabDefaultShell`, `TestCreateTabCommandAndLabel`, `TestCreateTabRejectsBadInput`, `TestListTabsOnlyRoostermInOrder`, `TestRenameTab`, `TestDeleteLiveTabKillsThenRemoves`, `TestDeleteFinishedTab`, `TestForeignTerminalIsNotFound`, `TestHCErrorForwarded`, `TestCoreOfflineIs503`, `TestSwarmsEndpoint`.

**Steps:**

- [ ] **Step 1: Write the failing test `host/internal/api/api_test.go`**

```go
package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"roosterm/host/internal/api"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

type env struct {
	t        *testing.T
	fake     *hctest.Fake
	sup      *hc.Supervisor
	h        http.Handler
	swarmDir string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	f := hctest.Start(t)
	sup, err := hc.Start(t.Context(), f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sup.Close(); err != nil {
			t.Errorf("supervisor close: %v", err)
		}
	})
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return &env{t: t, fake: f, sup: sup, swarmDir: dir,
		h: api.New(api.Options{Core: sup, Shell: "/bin/zsh", Home: "/home/me", SwarmDir: dir})}
}

func (e *env) do(method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(e.t.Context(), method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return v
}

type errBody struct {
	Error struct {
		Op, Tab, Message, Kind, Hint string
	} `json:"error"`
}

func tabTerm(id string, created int64, label string) hc.Terminal {
	return hc.Terminal{Address: id, State: hc.StateLive, CreatedMS: created,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: label}}
}

func TestCreateTabDefaultShell(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do("POST", "/api/tabs", `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	tab := decode[api.Tab](t, rec)
	if tab.ID != "t1" || tab.Label != "zsh" || tab.Exited || tab.ExitCode != nil || tab.Reason != nil {
		t.Fatalf("tab: got %+v", tab)
	}
	term, ok := e.fake.Get("t1")
	if !ok || !slices.Equal(term.Argv, []string{"/bin/zsh", "-l"}) || term.Cwd != "/home/me" ||
		term.Labels[hc.LabelOwner] != hc.LabelOwnerTab || term.Labels[hc.LabelName] != "zsh" || term.Cols != 80 || term.Rows != 24 {
		t.Fatalf("hc terminal: got %+v", term)
	}
}

func TestCreateTabCommandAndLabel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do("POST", "/api/tabs", `{"command":["tmux","-L","claude-swarm-4242","a"],"label":"swarm 4242"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	if tab := decode[api.Tab](t, rec); tab.Label != "swarm 4242" {
		t.Fatalf("label: got %q", tab.Label)
	}
	term, _ := e.fake.Get("t1")
	if !slices.Equal(term.Argv, []string{"tmux", "-L", "claude-swarm-4242", "a"}) {
		t.Fatalf("argv: got %v", term.Argv)
	}
	rec = e.do("POST", "/api/tabs", `{"command":["/usr/bin/top"]}`)
	if tab := decode[api.Tab](t, rec); tab.Label != "top" {
		t.Fatalf("default label from argv[0]: expected top, got %q", tab.Label)
	}
}

func TestCreateTabRejectsBadInput(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cases := map[string]int{
		`{"command":[]}`:              http.StatusBadRequest,
		`{"command":["sh",""]}`:       http.StatusBadRequest,
		`{"label":""}`:                http.StatusBadRequest,
		`{"label":"a\u0007"}`:         http.StatusBadRequest,
		`{"label":"` + strings.Repeat("x", 65) + `"}`:    http.StatusBadRequest,
		`{"bogus":1}`:                 http.StatusBadRequest,
		`not json`:                    http.StatusBadRequest,
		`{} {}`:                       http.StatusBadRequest,
		`{"label":"` + strings.Repeat("x", 70000) + `"}`: http.StatusRequestEntityTooLarge,
	}
	for body, want := range cases {
		rec := e.do("POST", "/api/tabs", body)
		if rec.Code != want || decode[errBody](t, rec).Error.Kind != "bad_request" {
			t.Errorf("body %.40q: expected %d bad_request, got %d %s", body, want, rec.Code, rec.Body)
		}
	}
	if _, ok := e.fake.Get("t1"); ok {
		t.Fatal("bad input: expected no terminal created")
	}
}

func TestListTabsOnlyRoostermInOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if rec := e.do("GET", "/api/tabs", ""); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("empty list: expected 200 [], got %d %q", rec.Code, rec.Body)
	}
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, CreatedMS: 1, Labels: map[string]string{}})
	e.fake.Seed(tabTerm("x3", 30, "c"))
	e.fake.Seed(tabTerm("x2", 20, "b"))
	done := tabTerm("x4", 40, "d")
	reason, code := "exited", 0
	done.State, done.Reason, done.ExitCode = hc.StateFinished, &reason, &code
	e.fake.Seed(done)
	tabs := decode[[]api.Tab](t, e.do("GET", "/api/tabs", ""))
	var ids []string
	for _, tab := range tabs {
		ids = append(ids, tab.ID)
	}
	if !slices.Equal(ids, []string{"x2", "x3", "x4"}) {
		t.Fatalf("ids: expected [x2 x3 x4], got %v", ids)
	}
	last := tabs[2]
	if !last.Exited || last.ExitCode == nil || *last.ExitCode != 0 || last.Reason == nil || *last.Reason != "exited" {
		t.Fatalf("finished tab: got %+v", last)
	}
}

func TestRenameTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.do("POST", "/api/tabs", `{}`)
	rec := e.do("PATCH", "/api/tabs/t1", `{"label":"hello world"}`)
	if rec.Code != http.StatusOK || decode[api.Tab](t, rec).Label != "hello world" {
		t.Fatalf("rename: expected 200 hello world, got %d %s", rec.Code, rec.Body)
	}
	if term, _ := e.fake.Get("t1"); term.Labels[hc.LabelName] != "hello world" || term.Labels[hc.LabelOwner] != hc.LabelOwnerTab {
		t.Fatalf("hc labels: got %v", term.Labels)
	}
}

func TestDeleteLiveTabKillsThenRemoves(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.do("POST", "/api/tabs", `{}`)
	if rec := e.do("DELETE", "/api/tabs/t1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete live: expected 204, got %d %s", rec.Code, rec.Body)
	}
	if _, ok := e.fake.Get("t1"); ok {
		t.Fatal("delete live: terminal still in hc")
	}
}

func TestDeleteFinishedTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	done := tabTerm("x1", 1, "a")
	done.State = hc.StateFinished
	e.fake.Seed(done)
	if rec := e.do("DELETE", "/api/tabs/x1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete finished: expected 204, got %d %s", rec.Code, rec.Body)
	}
	if _, ok := e.fake.Get("x1"); ok {
		t.Fatal("delete finished: terminal still in hc")
	}
}

func TestForeignTerminalIsNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, Labels: map[string]string{}})
	for _, req := range [][3]string{{"PATCH", "/api/tabs/x1", `{"label":"a"}`}, {"DELETE", "/api/tabs/x1", ""}, {"DELETE", "/api/tabs/t999", ""}} {
		rec := e.do(req[0], req[1], req[2])
		if rec.Code != http.StatusNotFound || decode[errBody](t, rec).Error.Kind != "not_found" {
			t.Errorf("%s %s: expected 404 not_found, got %d %s", req[0], req[1], rec.Code, rec.Body)
		}
	}
	if _, ok := e.fake.Get("x1"); !ok {
		t.Fatal("foreign terminal was removed")
	}
}

func TestHCErrorForwarded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.FailNext("terminal.run", hc.RPCError{Message: "spawn pending", Kind: "unavailable", Hint: "retry later"})
	rec := e.do("POST", "/api/tabs", `{}`)
	b := decode[errBody](t, rec).Error
	if rec.Code != http.StatusBadGateway || b.Kind != "unavailable" || b.Hint != "retry later" || !strings.Contains(b.Message, "spawn pending") || b.Op != "tab.create" {
		t.Fatalf("hc error: expected 502 unavailable, got %d %s", rec.Code, rec.Body)
	}
}

func TestCoreOfflineIs503(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c, err := e.sup.Client()
	if err != nil {
		t.Fatal(err)
	}
	e.fake.Stop()
	<-c.Done()
	rec := e.do("GET", "/api/tabs", "")
	if rec.Code != http.StatusServiceUnavailable || decode[errBody](t, rec).Error.Kind != "core_offline" {
		t.Fatalf("offline: expected 503 core_offline, got %d %s", rec.Code, rec.Body)
	}
}

func TestSwarmsEndpoint(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, name := range []string{"claude-swarm-7", "default"} {
		ln, err := net.Listen("unix", filepath.Join(e.swarmDir, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
	}
	rec := e.do("GET", "/api/swarms", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "[\"claude-swarm-7\"]\n" {
		t.Fatalf("swarms: expected 200 [\"claude-swarm-7\"], got %d %q", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd host && go test ./internal/api/ -count=1`
Expected: FAIL to build (`undefined: api.New`).

- [ ] **Step 3: Write `host/internal/api/api.go`**

```go
// Package api serves the REST tab and swarm endpoints (plan section "HTTP/WS API").
package api

import (
	"errors"
	"fmt"
	"net/http"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
)

type Options struct {
	Core     *hc.Supervisor
	Shell    string // argv[0] of a default tab, run with "-l"
	Home     string // cwd of every tab
	SwarmDir string // tmux socket directory
}

type Tab struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Exited   bool    `json:"exited"`
	ExitCode *int    `json:"exitCode"`
	Signal   *int    `json:"signal"`
	Reason   *string `json:"reason"`
}

type server struct{ o Options }

func New(o Options) http.Handler {
	s := &server{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tabs", s.listTabs)
	mux.HandleFunc("POST /api/tabs", s.createTab)
	mux.HandleFunc("PATCH /api/tabs/{id}", s.renameTab)
	mux.HandleFunc("DELETE /api/tabs/{id}", s.closeTab)
	mux.HandleFunc("GET /api/swarms", s.listSwarms)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		apierr.Write(w, &apierr.Error{Status: http.StatusNotFound, Op: "api", Kind: apierr.KindNotFound,
			Message: fmt.Sprintf("no endpoint %s %s", r.Method, r.URL.Path)})
	})
	return mux
}

func toTab(t hc.Terminal) Tab {
	return Tab{ID: t.Address, Label: t.Labels[hc.LabelName], Exited: t.State == hc.StateFinished,
		ExitCode: t.ExitCode, Signal: t.Signal, Reason: t.Reason}
}

// hcError maps an hc failure to the API error table.
func hcError(op, tab string, err error) *apierr.Error {
	if errors.Is(err, hc.ErrCoreOffline) || errors.Is(err, hc.ErrCoreGone) {
		return &apierr.Error{Status: http.StatusServiceUnavailable, Op: op, Tab: tab, Kind: apierr.KindCoreOffline, Message: err.Error()}
	}
	var re *hc.RPCError
	if errors.As(err, &re) {
		msg := re.Method
		if tab != "" {
			msg += " " + tab
		}
		msg += ": " + re.Message
		status := http.StatusBadGateway
		if re.Kind == hc.KindNotFound {
			status = http.StatusNotFound
		}
		return &apierr.Error{Status: status, Op: op, Tab: tab, Kind: re.Kind, Hint: re.Hint, Message: msg}
	}
	return &apierr.Error{Status: http.StatusInternalServerError, Op: op, Tab: tab, Kind: apierr.KindInternal, Message: err.Error()}
}
```

- [ ] **Step 4: Write `host/internal/api/tabs.go`**

```go
package api

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"unicode"
	"unicode/utf8"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
)

const maxLabelLen = 64

var initialSize = hc.Size{Cols: 80, Rows: 24}

func badRequest(op, tab, msg string) *apierr.Error {
	return &apierr.Error{Status: http.StatusBadRequest, Op: op, Tab: tab, Kind: apierr.KindBadRequest, Message: msg}
}

func checkLabel(op, tab, label string) *apierr.Error {
	n := utf8.RuneCountInString(label)
	ok := n >= 1 && n <= maxLabelLen
	for _, r := range label {
		if unicode.IsControl(r) {
			ok = false
		}
	}
	if !ok {
		return badRequest(op, tab, fmt.Sprintf("label: expected 1..%d characters without control characters, got %q", maxLabelLen, label))
	}
	return nil
}

func (s *server) client(op, tab string) (*hc.Client, *apierr.Error) {
	c, err := s.o.Core.Client()
	if err != nil {
		return nil, hcError(op, tab, err)
	}
	return c, nil
}

// owned returns the terminal if it exists and carries the roosterm tab label; anything else is 404.
func owned(ctx context.Context, c *hc.Client, op, id string) (hc.Terminal, *apierr.Error) {
	t, err := c.Get(ctx, id)
	if err != nil {
		return hc.Terminal{}, hcError(op, id, err)
	}
	if !hc.IsTab(t) {
		return hc.Terminal{}, &apierr.Error{Status: http.StatusNotFound, Op: op, Tab: id, Kind: apierr.KindNotFound,
			Message: fmt.Sprintf("tab %s not found", id)}
	}
	return t, nil
}

func (s *server) listTabs(w http.ResponseWriter, r *http.Request) {
	const op = "tab.list"
	c, e := s.client(op, "")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	ts, err := c.List(r.Context(), map[string]string{hc.LabelOwner: hc.LabelOwnerTab})
	if err != nil {
		apierr.Write(w, hcError(op, "", err))
		return
	}
	slices.SortStableFunc(ts, func(a, b hc.Terminal) int { return cmp.Compare(a.CreatedMS, b.CreatedMS) })
	out := make([]Tab, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTab(t))
	}
	apierr.WriteJSON(w, http.StatusOK, out)
}

type createRequest struct {
	Command []string `json:"command"`
	Label   *string  `json:"label"`
}

func (s *server) createTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.create"
	var req createRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		apierr.Write(w, e)
		return
	}
	argv := []string{s.o.Shell, "-l"}
	if req.Command != nil {
		argv = req.Command
		if len(argv) == 0 {
			apierr.Write(w, badRequest(op, "", "command: expected a non-empty array of non-empty strings, got []"))
			return
		}
		for i, a := range argv {
			if a == "" {
				apierr.Write(w, badRequest(op, "", fmt.Sprintf("command[%d]: expected a non-empty string, got \"\"", i)))
				return
			}
		}
	}
	label := filepath.Base(argv[0])
	if req.Label != nil {
		label = *req.Label
	}
	if e := checkLabel(op, "", label); e != nil {
		apierr.Write(w, e)
		return
	}
	c, e := s.client(op, "")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	t, err := c.Run(r.Context(), hc.RunParams{Argv: argv, Cwd: s.o.Home, Size: initialSize, NoParent: true,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: label}})
	if err != nil {
		apierr.Write(w, hcError(op, "", err))
		return
	}
	apierr.WriteJSON(w, http.StatusCreated, toTab(t))
}

func (s *server) renameTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.rename"
	id := r.PathValue("id")
	var req struct {
		Label string `json:"label"`
	}
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		e.Tab = id
		apierr.Write(w, e)
		return
	}
	if e := checkLabel(op, id, req.Label); e != nil {
		apierr.Write(w, e)
		return
	}
	c, e := s.client(op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	if _, e := owned(r.Context(), c, op, id); e != nil {
		apierr.Write(w, e)
		return
	}
	t, err := c.Label(r.Context(), id, map[string]string{hc.LabelName: req.Label})
	if err != nil {
		apierr.Write(w, hcError(op, id, err))
		return
	}
	apierr.WriteJSON(w, http.StatusOK, toTab(t))
}

func (s *server) closeTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.close"
	id := r.PathValue("id")
	c, e := s.client(op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	t, e := owned(r.Context(), c, op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	if t.State != hc.StateFinished {
		if _, err := c.Kill(r.Context(), id); err != nil {
			apierr.Write(w, hcError(op, id, err))
			return
		}
	}
	if err := c.Remove(r.Context(), id); err != nil {
		apierr.Write(w, hcError(op, id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Write `host/internal/api/swarms.go`**

```go
package api

import (
	"net/http"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/swarm"
)

func (s *server) listSwarms(w http.ResponseWriter, r *http.Request) {
	names, err := swarm.List(s.o.SwarmDir)
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "swarm.list", Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	apierr.WriteJSON(w, http.StatusOK, names)
}
```

- [ ] **Step 6: Run the tests to see them pass**

```bash
cd host
gofmt -w internal/api
go vet ./internal/api/; echo VET_RC=$?
go test ./internal/api/ -count=1 -race -v; echo RC=$?
gofmt -l internal/api
```
Expected: eleven `--- PASS` lines (names in the acceptance criteria), `VET_RC=0`, `RC=0`, no gofmt output.

- [ ] **Step 7: Commit**

```bash
git add host/internal/api/api.go host/internal/api/tabs.go host/internal/api/swarms.go host/internal/api/api_test.go
git commit -m "feat(host): REST tab and swarm endpoints over hc" -- host/internal/api
git status --porcelain host/internal/api
```
Expected: the status command prints nothing.

### Task 8: WebSocket ↔ hc bridge

**Files:**
- Create: `host/internal/bridge/bridge.go`
- Test: `host/internal/bridge/bridge_test.go`

**Interfaces:**
- Consumes: `hc.Start`, `hc.Supervisor.{Client,Await,Close}`, `hc.Client.{Get,Attach,Run}`, `hc.Stream.{Recv,Credit,Write,Resize,Close}`, `hc.Exit`, `hc.RPCError`, `hc.ErrCoreGone`, `hc.IsTab`, `hc.StateFinished`, `hc.StateLive`, `hc.KindNotFound`, `hc.KindFinished`, `hc.Size`, `hc.Terminal`, `hc.RunParams`, label constants (Task 3); `hctest.{Start,Paint}`, `Fake.{Stop,Restart,Exit,Seed,Get,Input,Resized,FailNext}` (Task 3, tests); `apierr.{Error,Write}`, kinds (Task 1); `github.com/coder/websocket` `Accept`, `Conn.{Read,Write,Ping,Close,CloseNow,SetReadLimit}`, `StatusCode`, `StatusGoingAway`, `MessageBinary`, `MessageText`, `CloseError`, `Dial` (tests).
- Produces (`package bridge`):
  - `func New(core *hc.Supervisor, pingInterval time.Duration) http.Handler` — reads path value `id` (pattern `GET /ws/tabs/{id}` registered by Task 6; tests register the same pattern on their own mux).
  - Close-code constants (type `websocket.StatusCode`): `CloseExited = 4000`, `CloseNotFound = 4004`, `CloseBadMessage = 4400`, `CloseHCError = 4500`.

**Behavior:**
- Before `websocket.Accept`: parse `cols`, `rows` (integers 1..`maxCells = 1000`); invalid → `apierr.Write` 400 `bad_request` op `tab.attach` naming the parameter, expected range and actual value.
- After accept: `conn.SetReadLimit(maxInput)` with `maxInput = 1 << 20`; `defer conn.CloseNow()`. One reader goroutine sends `(type, bytes)` on channel `in` and closes `in` on read error (client gone). One ping goroutine: `time.NewTicker(pingInterval)`; each tick `conn.Ping` with a context bounded by `pingInterval` (fail-fast IO cap); error → cancel the session context.
- Session loop; `size` starts from the query and is updated by every valid resize message, also while offline:
  1. `core.Client()`. Offline → write text `{"type":"core","state":"offline"}` (once per outage), then `core.Await(ctx)` in a goroutine while the loop keeps draining `in` (binary → text `{"type":"error","op":"tab.input","tab":<id>,"kind":"core_offline","message":"input dropped: hc core offline"}`; text → validate and update `size`, or 4400).
  2. `c.Get(ctx, id)`: `*hc.RPCError` kind `not_found`, or `!hc.IsTab(t)` → close 4004 `tab <id> not found`; error wrapping `ErrCoreGone` → step 1; other error → text error (op `tab.attach`, hc kind or `internal`) + close 4500 (reason = error text truncated to 123 bytes at a UTF-8 boundary). `t.State == hc.StateFinished` → close 4000 with `{"reason":…,"exitCode":…,"signal":…}` from the record.
  3. `c.Attach(ctx, id, size)`: kind `finished` → `Get` again and close 4000 as in step 2; otherwise errors as in step 2.
  4. When an offline message was sent in this outage → write `{"type":"core","state":"online"}` now (the first paint follows). Pump goroutine: `p, err := st.Recv(ctx)` → `conn.Write(ctx, websocket.MessageBinary, p)` → `st.Credit(len(p))`; its terminal error goes on a channel. Main select: `in` binary → `st.Write`; `in` text → decode `{"type":"resize","cols","rows"}` (`DisallowUnknownFields`, type must be `resize`, 1..1000) → `st.Resize`; invalid → detach, text error (kind `bad_request`) + close 4400 `bad message: <detail>`; an hc write error (always wraps `ErrCoreGone`) → step 1; `in` closed (client gone) → `st.Close()`, return; pump `*hc.Exit` → close 4000 with its JSON; pump `ErrCoreGone` → step 1; session ctx done while the client is still connected (server shutdown via `BaseContext`, or ping failure) → `st.Close()`, close 1001 `server shutdown`.
- 4000 reason JSON keys are exactly `reason`, `exitCode`, `signal` (in that order).

**Constraints:** `github.com/coder/websocket` only; compression off (default); default Origin check (no `InsecureSkipVerify`, no `OriginPatterns`). `conn.Write` may be called from the pump and the main loop at once (coder/websocket allows concurrent writes; only reads must be single-goroutine).

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/bridge/ -count=1 -race -v` runs and passes `TestBridgeFirstPaintThenEcho`, `TestBridgeResizeReachesHC`, `TestBridgeExitClosesWith4000`, `TestBridgeFinishedTabCloses4000`, `TestBridgeUnknownTabCloses4004`, `TestBridgeForeignTerminalCloses4004`, `TestBridgeBadTextCloses4400`, `TestBridgeBadSizeIs400`, `TestBridgeHCAttachErrorCloses4500`, `TestBridgeCoreOfflineThenOnline`.
- [ ] `cd host && go test ./internal/bridge/ -count=20 -race` passes (flake check; not part of `scripts/test.py`).
- [ ] `grep -nE 'InsecureSkipVerify|OriginPatterns' host/internal/bridge/*.go; echo RC=$?` → `RC=1`.

**Steps:**

- [ ] **Step 1: Write the failing test `host/internal/bridge/bridge_test.go`**

```go
package bridge_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"roosterm/host/internal/bridge"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

// noPing keeps the ticker out of the tests.
const noPing = time.Hour

type env struct {
	t    *testing.T
	fake *hctest.Fake
	sup  *hc.Supervisor
	srv  *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	f := hctest.Start(t)
	sup, err := hc.Start(t.Context(), f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sup.Close(); err != nil {
			t.Errorf("supervisor close: %v", err)
		}
	})
	mux := http.NewServeMux()
	mux.Handle("GET /ws/tabs/{id}", bridge.New(sup, noPing))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{t: t, fake: f, sup: sup, srv: srv}
}

func (e *env) tab() string {
	e.t.Helper()
	c, err := e.sup.Client()
	if err != nil {
		e.t.Fatal(err)
	}
	term, err := c.Run(e.t.Context(), hc.RunParams{Argv: []string{"/bin/sh"}, Size: hc.Size{Cols: 80, Rows: 24}, NoParent: true,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: "sh"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return term.Address
}

func (e *env) dial(id string) *websocket.Conn {
	e.t.Helper()
	u := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/tabs/" + id + "?cols=80&rows=24"
	conn, _, err := websocket.Dial(e.t.Context(), u, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	typ, b, err := conn.Read(t.Context())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return typ, b
}

// closeOf reads until the server closes and returns the close frame.
func closeOf(t *testing.T, conn *websocket.Conn) (websocket.CloseError, [][]byte) {
	t.Helper()
	var texts [][]byte
	for {
		typ, b, err := conn.Read(t.Context())
		if err == nil {
			if typ == websocket.MessageText {
				texts = append(texts, b)
			}
			continue
		}
		var ce websocket.CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("read: expected a close frame, got %v", err)
		}
		return ce, texts
	}
}

func write(t *testing.T, conn *websocket.Conn, typ websocket.MessageType, b string) {
	t.Helper()
	if err := conn.Write(t.Context(), typ, []byte(b)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestBridgeFirstPaintThenEcho(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	if typ, b := read(t, conn); typ != websocket.MessageBinary || !bytes.Equal(b, hctest.Paint(id)) {
		t.Fatalf("first message: expected binary paint, got %v %q", typ, b)
	}
	write(t, conn, websocket.MessageBinary, "echo $$\r")
	if got := <-e.fake.Input(id); string(got) != "echo $$\r" {
		t.Fatalf("hc input: got %q", got)
	}
	if typ, b := read(t, conn); typ != websocket.MessageBinary || string(b) != "echo $$\r" {
		t.Fatalf("echo: got %v %q", typ, b)
	}
}

func TestBridgeResizeReachesHC(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	write(t, conn, websocket.MessageText, `{"type":"resize","cols":100,"rows":30}`)
	if got := <-e.fake.Resized(id); got != (hc.Size{Cols: 100, Rows: 30}) {
		t.Fatalf("hc resize: got %+v", got)
	}
}

func TestBridgeExitClosesWith4000(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	e.fake.Exit(id, 7)
	ce, _ := closeOf(t, conn)
	if ce.Code != bridge.CloseExited || ce.Reason != `{"reason":"exited","exitCode":7,"signal":null}` {
		t.Fatalf("close: expected 4000 with exit JSON, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeFinishedTabCloses4000(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	e.fake.Exit(id, 3)
	ce, _ := closeOf(t, e.dial(id))
	if ce.Code != bridge.CloseExited || ce.Reason != `{"reason":"exited","exitCode":3,"signal":null}` {
		t.Fatalf("close: expected 4000 exitCode 3, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeUnknownTabCloses4004(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ce, _ := closeOf(t, e.dial("t999"))
	if ce.Code != bridge.CloseNotFound || ce.Reason != "tab t999 not found" {
		t.Fatalf("close: expected 4004, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeForeignTerminalCloses4004(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, Labels: map[string]string{}})
	ce, _ := closeOf(t, e.dial("x1"))
	if ce.Code != bridge.CloseNotFound {
		t.Fatalf("close: expected 4004, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeBadTextCloses4400(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	for _, msg := range []string{`{"type":"nope"}`, `{"type":"resize","cols":0,"rows":5}`, `not json`} {
		conn := e.dial(id)
		read(t, conn)
		write(t, conn, websocket.MessageText, msg)
		ce, texts := closeOf(t, conn)
		if ce.Code != bridge.CloseBadMessage || !strings.HasPrefix(ce.Reason, "bad message: ") {
			t.Fatalf("%s: expected 4400, got %d %q", msg, ce.Code, ce.Reason)
		}
		if len(texts) != 1 || !strings.Contains(string(texts[0]), `"kind":"bad_request"`) {
			t.Fatalf("%s: expected one error text before close, got %q", msg, texts)
		}
	}
}

func TestBridgeBadSizeIs400(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, q := range []string{"cols=0&rows=24", "cols=80", "cols=80&rows=1001", "cols=x&rows=24"} {
		req, err := http.NewRequestWithContext(t.Context(), "GET", e.srv.URL+"/ws/tabs/t1?"+q, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Error struct{ Kind string } `json:"error"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || decErr != nil || body.Error.Kind != "bad_request" {
			t.Errorf("%s: expected 400 bad_request, got %d (%v)", q, resp.StatusCode, decErr)
		}
	}
}

func TestBridgeHCAttachErrorCloses4500(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	e.fake.FailNext("terminal.attach", hc.RPCError{Message: "spawn pending", Kind: "unavailable"})
	ce, texts := closeOf(t, e.dial(id))
	if ce.Code != bridge.CloseHCError || !strings.Contains(ce.Reason, "spawn pending") {
		t.Fatalf("close: expected 4500 spawn pending, got %d %q", ce.Code, ce.Reason)
	}
	if len(texts) != 1 || !strings.Contains(string(texts[0]), `"kind":"unavailable"`) {
		t.Fatalf("expected one error text with kind unavailable, got %q", texts)
	}
}

func TestBridgeCoreOfflineThenOnline(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	e.fake.Stop()
	if typ, b := read(t, conn); typ != websocket.MessageText || string(b) != `{"type":"core","state":"offline"}` {
		t.Fatalf("after core stop: expected offline text, got %v %q", typ, b)
	}
	write(t, conn, websocket.MessageBinary, "x")
	if _, b := read(t, conn); !strings.Contains(string(b), `"kind":"core_offline"`) || !strings.Contains(string(b), `"op":"tab.input"`) {
		t.Fatalf("input while offline: expected core_offline error, got %q", b)
	}
	write(t, conn, websocket.MessageText, `{"type":"resize","cols":90,"rows":20}`)
	write(t, conn, websocket.MessageBinary, "y") // its error reply proves the resize before it was handled
	if _, b := read(t, conn); !strings.Contains(string(b), `"kind":"core_offline"`) {
		t.Fatalf("second input while offline: expected core_offline error, got %q", b)
	}
	e.fake.Restart(t)
	if typ, b := read(t, conn); typ != websocket.MessageText || string(b) != `{"type":"core","state":"online"}` {
		t.Fatalf("after restart: expected online text, got %v %q", typ, b)
	}
	if typ, b := read(t, conn); typ != websocket.MessageBinary || !bytes.Equal(b, hctest.Paint(id)) {
		t.Fatalf("after online: expected first paint, got %v %q", typ, b)
	}
	if term, _ := e.fake.Get(id); term.Cols != 90 || term.Rows != 20 {
		t.Fatalf("re-attach size: expected 90x20 from the offline resize, got %dx%d", term.Cols, term.Rows)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd host && go test ./internal/bridge/ -count=1`
Expected: FAIL to build (`undefined: bridge.New`).

- [ ] **Step 3: Write `host/internal/bridge/bridge.go`**

```go
// Package bridge turns one WebSocket into one hc attach stream (plan section "WebSocket").
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
)

const (
	CloseExited     websocket.StatusCode = 4000
	CloseNotFound   websocket.StatusCode = 4004
	CloseBadMessage websocket.StatusCode = 4400
	CloseHCError    websocket.StatusCode = 4500
)

const (
	maxCells       = 1000
	maxInput       = 1 << 20
	maxCloseReason = 123 // coder/websocket: 125-byte close payload minus the 2-byte code
	opAttach       = "tab.attach"
	opInput        = "tab.input"
)

type handler struct {
	core *hc.Supervisor
	ping time.Duration
}

// New serves GET /ws/tabs/{id}?cols=N&rows=N. The caller enforces the session cookie before it.
func New(core *hc.Supervisor, pingInterval time.Duration) http.Handler {
	return &handler{core: core, ping: pingInterval}
}

func dim(r *http.Request, id, name string) (uint16, *apierr.Error) {
	v := r.URL.Query().Get(name)
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxCells {
		return 0, &apierr.Error{Status: http.StatusBadRequest, Op: opAttach, Tab: id, Kind: apierr.KindBadRequest,
			Message: fmt.Sprintf("query %s: expected an integer 1..%d, got %q", name, maxCells, v)}
	}
	return uint16(n), nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cols, e := dim(r, id, "cols")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	rows, e := dim(r, id, "rows")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	conn, err := websocket.Accept(w, r, nil) // default Origin check, compression off
	if err != nil {
		log.Printf("ws accept tab %s: %v", id, err) // Accept already wrote the HTTP error
		return
	}
	defer func() {
		// ponytail: after a completed Close handshake CloseNow is a no-op; its error only says "already closed".
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(maxInput)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s := &session{conn: conn, core: h.core, id: id, size: hc.Size{Cols: cols, Rows: rows}, in: make(chan clientMsg)}
	go s.read(ctx)
	go s.pinger(ctx, cancel, h.ping)
	s.run(ctx)
}

type clientMsg struct {
	typ  websocket.MessageType
	data []byte
}

type session struct {
	conn        *websocket.Conn
	core        *hc.Supervisor
	id          string
	size        hc.Size
	in          chan clientMsg
	offlineSent bool
}

// read forwards client messages; a read error means the client is gone or the conn closed: in is closed.
func (s *session) read(ctx context.Context) {
	defer close(s.in)
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case s.in <- clientMsg{typ, data}:
		case <-ctx.Done():
			return
		}
	}
}

func (s *session) pinger(ctx context.Context, cancel context.CancelFunc, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, pcancel := context.WithTimeout(ctx, every) // fail-fast cap: a peer silent for a whole interval is gone
			err := s.conn.Ping(pctx)
			pcancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("ws ping tab %s: %v", s.id, err)
				cancel()
				return
			}
		}
	}
}

type step int

const (
	stepStop  step = iota // session over, the socket was closed or the client left
	stepRetry             // hc connection ended: back to waiting for a client
	stepPump              // attached: pump the stream
)

func (s *session) run(ctx context.Context) {
	for {
		c, ok := s.online(ctx)
		if !ok {
			return
		}
		st, next := s.open(ctx, c)
		switch next {
		case stepStop:
			return
		case stepRetry:
			continue
		}
		if s.pump(ctx, st) == stepStop {
			return
		}
	}
}

type textMsg struct {
	Type    string `json:"type"`
	State   string `json:"state,omitempty"`
	Op      string `json:"op,omitempty"`
	Tab     string `json:"tab,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *session) text(ctx context.Context, m textMsg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("ws text tab %s: %w", s.id, err)
	}
	return s.conn.Write(ctx, websocket.MessageText, b)
}

func truncate(s string) string {
	if len(s) <= maxCloseReason {
		return s
	}
	n := maxCloseReason
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (s *session) close(code websocket.StatusCode, reason string) {
	if err := s.conn.Close(code, truncate(reason)); err != nil {
		log.Printf("ws close tab %s (%d): %v", s.id, code, err)
	}
}

func (s *session) shutdown() { s.close(websocket.StatusGoingAway, "server shutdown") }

// online returns a client; while hc is offline it tells the client once and keeps draining input.
func (s *session) online(ctx context.Context) (*hc.Client, bool) {
	if c, err := s.core.Client(); err == nil {
		return c, true
	}
	if !s.offlineSent {
		if err := s.text(ctx, textMsg{Type: "core", State: "offline"}); err != nil {
			return nil, false
		}
		s.offlineSent = true
	}
	actx, acancel := context.WithCancel(ctx)
	defer acancel()
	type awaited struct {
		c   *hc.Client
		err error
	}
	ready := make(chan awaited, 1)
	go func() {
		c, err := s.core.Await(actx)
		ready <- awaited{c, err}
	}()
	for {
		select {
		case a := <-ready:
			if a.err != nil {
				s.shutdown()
				return nil, false
			}
			return a.c, true
		case m, ok := <-s.in:
			if !ok {
				return nil, false
			}
			if !s.offlineInput(ctx, m) {
				return nil, false
			}
		case <-ctx.Done():
			s.shutdown()
			return nil, false
		}
	}
}

func (s *session) offlineInput(ctx context.Context, m clientMsg) bool {
	if m.typ == websocket.MessageBinary {
		err := s.text(ctx, textMsg{Type: "error", Op: opInput, Tab: s.id, Kind: apierr.KindCoreOffline,
			Message: "input dropped: hc core offline"})
		return err == nil
	}
	size, err := parseResize(m.data)
	if err != nil {
		s.badMessage(ctx, err)
		return false
	}
	s.size = size
	return true
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func parseResize(b []byte) (hc.Size, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m resizeMsg
	if err := dec.Decode(&m); err != nil {
		return hc.Size{}, fmt.Errorf("expected {\"type\":\"resize\",\"cols\":N,\"rows\":N}, got %v", err)
	}
	if m.Type != "resize" {
		return hc.Size{}, fmt.Errorf("type: expected \"resize\", got %q", m.Type)
	}
	if m.Cols < 1 || m.Cols > maxCells || m.Rows < 1 || m.Rows > maxCells {
		return hc.Size{}, fmt.Errorf("size: expected cols and rows 1..%d, got %dx%d", maxCells, m.Cols, m.Rows)
	}
	return hc.Size{Cols: uint16(m.Cols), Rows: uint16(m.Rows)}, nil
}

func (s *session) badMessage(ctx context.Context, err error) {
	if terr := s.text(ctx, textMsg{Type: "error", Op: opInput, Tab: s.id, Kind: apierr.KindBadRequest, Message: err.Error()}); terr != nil {
		return // client already gone
	}
	s.close(CloseBadMessage, "bad message: "+err.Error())
}

type exitReason struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exitCode"`
	Signal   *int   `json:"signal"`
}

func (s *session) closeExit(e *hc.Exit) {
	b, err := json.Marshal(exitReason{Reason: e.Reason, ExitCode: e.ExitCode, Signal: e.Signal})
	if err != nil {
		log.Printf("ws exit reason tab %s: %v", s.id, err)
	}
	s.close(CloseExited, string(b))
}

func exitOf(t hc.Terminal) *hc.Exit {
	e := &hc.Exit{ExitCode: t.ExitCode, Signal: t.Signal}
	if t.Reason != nil {
		e.Reason = *t.Reason
	}
	return e
}

// hcFail handles an error from Get/Attach.
func (s *session) hcFail(ctx context.Context, err error) step {
	var re *hc.RPCError
	switch {
	case errors.Is(err, hc.ErrCoreGone):
		return stepRetry
	case ctx.Err() != nil:
		s.shutdown()
	case errors.As(err, &re) && re.Kind == hc.KindNotFound:
		s.close(CloseNotFound, fmt.Sprintf("tab %s not found", s.id))
	default:
		kind := apierr.KindInternal
		if re != nil {
			kind = re.Kind
		}
		if terr := s.text(ctx, textMsg{Type: "error", Op: opAttach, Tab: s.id, Kind: kind, Message: err.Error()}); terr == nil {
			s.close(CloseHCError, err.Error())
		}
	}
	return stepStop
}

// open checks the tab and attaches; the stream is non-nil only with stepPump.
func (s *session) open(ctx context.Context, c *hc.Client) (*hc.Stream, step) {
	t, err := c.Get(ctx, s.id)
	if err != nil {
		return nil, s.hcFail(ctx, err)
	}
	if !hc.IsTab(t) {
		s.close(CloseNotFound, fmt.Sprintf("tab %s not found", s.id))
		return nil, stepStop
	}
	if t.State == hc.StateFinished {
		s.closeExit(exitOf(t))
		return nil, stepStop
	}
	st, err := c.Attach(ctx, s.id, s.size)
	var re *hc.RPCError
	if errors.As(err, &re) && re.Kind == hc.KindFinished {
		t, err := c.Get(ctx, s.id)
		if err != nil {
			return nil, s.hcFail(ctx, err)
		}
		s.closeExit(exitOf(t))
		return nil, stepStop
	}
	if err != nil {
		return nil, s.hcFail(ctx, err)
	}
	return st, stepPump
}

func (s *session) detach(st *hc.Stream) {
	if err := st.Close(); err != nil && !errors.Is(err, hc.ErrCoreGone) {
		log.Printf("hc detach tab %s: %v", s.id, err)
	}
}

// forward copies hc output to the WebSocket and returns credit after each message.
func (s *session) forward(ctx context.Context, st *hc.Stream) error {
	for {
		p, err := st.Recv(ctx)
		if err != nil {
			return err
		}
		if err := s.conn.Write(ctx, websocket.MessageBinary, p); err != nil {
			return fmt.Errorf("ws write tab %s: %w", s.id, err)
		}
		if err := st.Credit(len(p)); err != nil {
			return err
		}
	}
}

func (s *session) pump(ctx context.Context, st *hc.Stream) step {
	if s.offlineSent {
		if err := s.text(ctx, textMsg{Type: "core", State: "online"}); err != nil {
			s.detach(st)
			return stepStop
		}
		s.offlineSent = false
	}
	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()
	out := make(chan error, 1)
	go func() { out <- s.forward(pctx, st) }()
	for {
		select {
		case err := <-out:
			var ex *hc.Exit
			switch {
			case errors.As(err, &ex):
				s.closeExit(ex)
				return stepStop
			case errors.Is(err, hc.ErrCoreGone):
				return stepRetry
			case ctx.Err() != nil:
				s.detach(st)
				s.shutdown()
				return stepStop
			default: // WebSocket write failed (client gone) or unexpected stream end
				log.Printf("ws tab %s: %v", s.id, err)
				s.detach(st)
				return stepStop
			}
		case m, ok := <-s.in:
			if !ok {
				s.detach(st)
				return stepStop
			}
			var err error
			if m.typ == websocket.MessageBinary {
				err = st.Write(m.data)
			} else {
				size, perr := parseResize(m.data)
				if perr != nil {
					s.detach(st)
					s.badMessage(ctx, perr)
					return stepStop
				}
				s.size = size
				err = st.Resize(size)
			}
			if err != nil { // every hc write error wraps ErrCoreGone
				return stepRetry
			}
		case <-ctx.Done():
			s.detach(st)
			s.shutdown()
			return stepStop
		}
	}
}
```

- [ ] **Step 4: Run the tests to see them pass, then the flake check**

```bash
cd host
gofmt -w internal/bridge
go vet ./internal/bridge/; echo VET_RC=$?
go test ./internal/bridge/ -count=1 -race -v; echo RC=$?
go test ./internal/bridge/ -count=20 -race; echo FLAKE_RC=$?
gofmt -l internal/bridge
cd .. && grep -nE 'InsecureSkipVerify|OriginPatterns' host/internal/bridge/*.go; echo GREP_RC=$?
```
Expected: ten `--- PASS` lines (names in the acceptance criteria), `VET_RC=0`, `RC=0`, `FLAKE_RC=0`, no gofmt output, `GREP_RC=1`.

- [ ] **Step 5: Commit**

```bash
git add host/internal/bridge/bridge.go host/internal/bridge/bridge_test.go
git commit -m "feat(host): WebSocket to hc attach bridge with core-offline handling" -- host/internal/bridge
git status --porcelain host/internal/bridge
```
Expected: the status command prints nothing.

### Task 9: `cmd/roosterm` — `serve`, `passwd`, module tidy

**Files:**
- Create: `host/cmd/roosterm/main.go`, `host/cmd/roosterm/passwd.go`, `host/cmd/roosterm/serve.go`
- Modify: `host/go.mod`, `host/go.sum` (`go mod tidy` only)

**Interfaces:**
- Consumes: `config.{DefaultPath,Load,Save,SessionsPath,Config,DefaultListen,DefaultPingInterval,DefaultSessionTTL}`, `Config.SocketPath` (Task 1); `auth.{NewSalt,HashPassword,VerifyPassword,OpenSessions,NewLimiter}` (Task 2); `hc.Start`, `Supervisor.Close` (Task 3); `swarm.Dir` (Task 4); `web.New`, `web.Options` (Task 6); `api.New`, `api.Options` (Task 7); `bridge.New` (Task 8); `golang.org/x/term.{IsTerminal,ReadPassword}`.
- Produces: binary `roosterm`: `roosterm passwd [--config PATH]`, `roosterm serve [--config PATH]`. Exit codes: 0 ok, 1 runtime error, 2 usage error.
  - `func run(args []string, stdin *os.File, stdout, stderr io.Writer) int`

**Behavior:**
- `main` = `os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))`. Unknown/missing subcommand → usage on stderr, 2. `--config` via a per-subcommand `flag.FlagSet` (`ContinueOnError`); extra arguments → 2; default `config.DefaultPath()`.
- `passwd`: config exists → `Load` (any error → 1); missing → new `Config{Listen: DefaultListen, PingInterval: DefaultPingInterval, SessionTTL: DefaultSessionTTL, Shell: $SHELL}` (empty `SHELL` → `roosterm passwd: SHELL is empty; run this from your login shell`, 1). Prompts `username: `, `password: `, `repeat password: ` on stderr. Terminal stdin (`term.IsTerminal`) → `term.ReadPassword` for both passwords; else lines (scripted use). Empty username/password or mismatch → error naming the field, 1. `HashPassword(pw, salt)`, `Save`, print `wrote <path>` on stdout.
- `serve`: `Load`; `VerifyPassword(cfg.PasswordHash, "")` error → `roosterm serve: config <path>: password_hash: <err>`, 1. `hc.Start(ctx, cfg.SocketPath())` error → `roosterm serve: <err>` on stderr, 1 (criterion 6: the message contains the socket path and `hc server start`). `OpenSessions(config.SessionsPath(path), cfg.SessionTTL, time.Now)`; `NewLimiter(time.Now)`; `os.UserHomeDir()`; `swarm.Dir(os.Getenv, os.Getuid())`. Handler `web.New(...)` with `api.New(...)` and `bridge.New(sup, cfg.PingInterval)`. `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`; `net.Listen("tcp", cfg.Listen)` (error → 1); `http.Server{Handler, ReadHeaderTimeout: 10 s, BaseContext: ctx}`; print `listening on <addr>, hc <socket>` once listening; on ctx done → `srv.Shutdown` bounded by `shutdownGrace = 5 s`, `sup.Close()`, exit 0. Every error path after `hc.Start` closes the supervisor.
- `time.Now` appears only in `serve.go` (composition root).

**Acceptance criteria:**
- [ ] After the tidy commit: `cd host && go mod tidy && git diff --exit-code go.mod go.sum; echo RC=$?` → `RC=0`; `go.mod` has the five modules as direct requires and only `golang.org/x/sys` as `// indirect`.
- [ ] `python3 scripts/build.py; echo RC=$?` → `RC=0`, `build/roosterm` exists; `python3 scripts/build.py --target linux/arm64; echo RC=$?` → `RC=0`, `file build/roosterm-linux-arm64` says `ELF 64-bit LSB executable, ARM aarch64`.
- [ ] Run-once (scratch, not committed), all in step 6: passwd writes a 0600 config whose `password_hash` starts with `$argon2id$v=19$`; serve with a missing socket → RC 1 and stderr contains the socket path and `hc server start`; serve with an unknown config key → RC 1 naming the key; no arguments → RC 2.
- [ ] `grep -rn 'time.Now' host/internal; echo RC=$?` → `RC=1`.
- [ ] `python3 scripts/test.py; echo RC=$?` → `RC=0`, `skipped: 0`.

**Steps:**

- [ ] **Step 1: Write `host/cmd/roosterm/main.go`**

```go
// Command roosterm serves browser terminals backed by herdr-core.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"roosterm/host/internal/config"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const usage = `usage:
  roosterm passwd [--config PATH]   set the login user and password
  roosterm serve  [--config PATH]   run the HTTP server
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "passwd":
		return passwd(args[1:], stdin, stdout, stderr)
	case "serve":
		return serve(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "roosterm: unknown command %q\n%s", args[0], usage)
	return exitUsage
}

// configPath parses --config for one subcommand; ok false means exit with code.
func configPath(name string, args []string, stderr io.Writer) (path string, code int, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	p := fs.String("config", "", "config file (default $XDG_CONFIG_HOME or ~/.config, then roosterm/config.toml)")
	if err := fs.Parse(args); err != nil {
		return "", exitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "roosterm %s: unexpected arguments %v\n%s", name, fs.Args(), usage)
		return "", exitUsage, false
	}
	if *p != "" {
		return *p, exitOK, true
	}
	d, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "roosterm %s: %v\n", name, err)
		return "", exitError, false
	}
	return d, exitOK, true
}
```

- [ ] **Step 2: Write `host/cmd/roosterm/passwd.go`**

```go
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"golang.org/x/term"

	"roosterm/host/internal/auth"
	"roosterm/host/internal/config"
)

func passwd(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	path, code, ok := configPath("passwd", args, stderr)
	if !ok {
		return code
	}
	if err := setPassword(path, stdin, stderr); err != nil {
		fmt.Fprintf(stderr, "roosterm passwd: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	return exitOK
}

func setPassword(path string, stdin *os.File, prompts io.Writer) error {
	cfg, err := loadOrNew(path)
	if err != nil {
		return err
	}
	p := &prompter{r: bufio.NewReader(stdin), fd: int(stdin.Fd()), tty: term.IsTerminal(int(stdin.Fd())), out: prompts}
	user, err := p.line("username: ")
	if err != nil {
		return err
	}
	pw, err := p.secret("password: ")
	if err != nil {
		return err
	}
	again, err := p.secret("repeat password: ")
	if err != nil {
		return err
	}
	switch {
	case user == "":
		return errors.New("username: expected a non-empty value, got \"\"")
	case pw == "":
		return errors.New("password: expected a non-empty value, got \"\"")
	case pw != again:
		return errors.New("repeat password: expected the same password, got a different one")
	}
	salt, err := auth.NewSalt()
	if err != nil {
		return err
	}
	cfg.Username = user
	cfg.PasswordHash = auth.HashPassword(pw, salt)
	return config.Save(path, cfg)
}

// loadOrNew loads an existing config or starts one with defaults and the current $SHELL.
func loadOrNew(path string) (config.Config, error) {
	_, err := os.Stat(path)
	if err == nil {
		return config.Load(path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return config.Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		return config.Config{}, errors.New("SHELL is empty; run this from your login shell")
	}
	return config.Config{Listen: config.DefaultListen, PingInterval: config.DefaultPingInterval,
		SessionTTL: config.DefaultSessionTTL, Shell: shell}, nil
}

type prompter struct {
	r   *bufio.Reader
	fd  int
	tty bool
	out io.Writer
}

func (p *prompter) line(prompt string) (string, error) {
	fmt.Fprint(p.out, prompt)
	s, err := p.r.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && s != "") {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(prompt, ": "), err)
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// secret reads without echo on a terminal; scripted input (a pipe) is read as lines.
func (p *prompter) secret(prompt string) (string, error) {
	if !p.tty {
		return p.line(prompt)
	}
	fmt.Fprint(p.out, prompt)
	b, err := term.ReadPassword(p.fd)
	fmt.Fprintln(p.out)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(prompt, ": "), err)
	}
	return string(b), nil
}
```

- [ ] **Step 3: Write `host/cmd/roosterm/serve.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"roosterm/host/internal/api"
	"roosterm/host/internal/auth"
	"roosterm/host/internal/bridge"
	"roosterm/host/internal/config"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/swarm"
	"roosterm/host/internal/web"
)

const (
	shutdownGrace     = 5 * time.Second  // fail-fast cap on graceful HTTP shutdown
	readHeaderTimeout = 10 * time.Second // slow-header (slowloris) cap
)

func serve(args []string, stdout, stderr io.Writer) int {
	path, code, ok := configPath("serve", args, stderr)
	if !ok {
		return code
	}
	if err := serveConfig(path, stdout); err != nil {
		fmt.Fprintf(stderr, "roosterm serve: %v\n", err)
		return exitError
	}
	return exitOK
}

// serveConfig is the composition root: the only place that reads the real clock.
func serveConfig(path string, stdout io.Writer) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if _, err := auth.VerifyPassword(cfg.PasswordHash, ""); err != nil {
		return fmt.Errorf("config %s: password_hash: %w", path, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	socket := cfg.SocketPath()
	sup, err := hc.Start(ctx, socket)
	if err != nil {
		return err
	}
	sessions, err := auth.OpenSessions(config.SessionsPath(path), cfg.SessionTTL, time.Now)
	if err != nil {
		return errors.Join(err, sup.Close())
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return errors.Join(fmt.Errorf("home directory: %w", err), sup.Close())
	}
	h := web.New(web.Options{
		Username: cfg.Username, PasswordHash: cfg.PasswordHash, SessionTTL: cfg.SessionTTL,
		Sessions: sessions, Limiter: auth.NewLimiter(time.Now),
		API:       api.New(api.Options{Core: sup, Shell: cfg.Shell, Home: home, SwarmDir: swarm.Dir(os.Getenv, os.Getuid())}),
		TabSocket: bridge.New(sup, cfg.PingInterval),
	})
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return errors.Join(fmt.Errorf("listen %s: %w", cfg.Listen, err), sup.Close())
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	fmt.Fprintf(stdout, "listening on %s, hc %s\n", ln.Addr(), socket)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return errors.Join(fmt.Errorf("http serve: %w", err), sup.Close())
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return errors.Join(srv.Shutdown(sctx), sup.Close())
}
```

- [ ] **Step 4: Format, vet, tidy (after the Task 6, 7, 8 commits are in)**

```bash
cd host
gofmt -w cmd/roosterm
go vet ./...; echo VET_RC=$?
go mod tidy; echo TIDY_RC=$?
cat go.mod
gofmt -l .
```
Expected: `VET_RC=0`, `TIDY_RC=0`; `go.mod` shows one `require (...)` block with `github.com/BurntSushi/toml v1.6.0`, `github.com/coder/websocket v1.8.15`, `github.com/fsnotify/fsnotify v1.10.1`, `golang.org/x/crypto v0.59.0`, `golang.org/x/term v0.48.0`, and `require golang.org/x/sys v0.50.0 // indirect`; no gofmt output. Any version change → STOP and report.

- [ ] **Step 5: Build both targets**

```bash
python3 scripts/build.py; echo BUILD_RC=$?
python3 scripts/build.py --target linux/arm64; echo BUILD_LINUX_RC=$?
file build/roosterm build/roosterm-linux-arm64
```
Expected: `web/: no package.json, embedding host/internal/web/dist as committed` (until the client lands), `BUILD_RC=0`, `BUILD_LINUX_RC=0`, `file` shows `Mach-O 64-bit executable arm64` and `ELF 64-bit LSB executable, ARM aarch64 ... statically linked`.

- [ ] **Step 6: Run-once CLI checks (scratch, not committed)**

```bash
mkdir -p scratch/t9
printf 'me\npw\npw\n' | build/roosterm passwd --config scratch/t9/config.toml; echo PASSWD_RC=$?
stat -f %Lp scratch/t9/config.toml
grep -c '^password_hash = "\$argon2id\$v=19\$m=65536,t=3,p=4\$' scratch/t9/config.toml
grep -v '^password_hash' scratch/t9/config.toml
```
Expected: stderr shows the three prompts, stdout `wrote scratch/t9/config.toml`, `PASSWD_RC=0`, `600`, `1`, and the other keys with the defaults (`listen = "127.0.0.1:8787"`, `hc_socket = ""`, `ping_interval = "25s"`, `session_ttl = "720h0m0s"`, `shell = "<your $SHELL>"`, `username = "me"`).

Then edit `scratch/t9/config.toml` with the Edit tool: set `hc_socket = "/tmp/roosterm-no-such/hc.sock"`.
```bash
build/roosterm serve --config scratch/t9/config.toml; echo SERVE_RC=$?
```
Expected: `roosterm serve: hc not running: dial unix /tmp/roosterm-no-such/hc.sock: connect: no such file or directory; expected a listening herdr-core socket at /tmp/roosterm-no-such/hc.sock (start it with "hc server start")`, `SERVE_RC=1`.

Then add a line `bogus = 1` to the file with the Edit tool.
```bash
build/roosterm serve --config scratch/t9/config.toml; echo UNKNOWN_RC=$?
build/roosterm; echo NOARGS_RC=$?
```
Expected: an error containing `unknown keys [bogus]`, `UNKNOWN_RC=1`; usage text, `NOARGS_RC=2`. Paste every RC and decisive line into the report. Leave `scratch/t9/` for the lead (deleted at sign-off).

- [ ] **Step 7: Full suite and clock check**

```bash
python3 scripts/test.py; echo RC=$?
grep -rn 'time.Now' host/internal; echo GREP_RC=$?
```
Expected: `tests: 47` (7 auth + 9 hc + 3 swarm + 7 web + 11 api + 10 bridge), `skipped: 0`, `wall` ≤ 40 s, `PASS`, `RC=0`; `GREP_RC=1`.

- [ ] **Step 8: Commit**

```bash
git add host/cmd/roosterm/main.go host/cmd/roosterm/passwd.go host/cmd/roosterm/serve.go host/go.mod host/go.sum
git commit -m "feat(host): roosterm serve and passwd commands, tidy module" -- host/cmd/roosterm host/go.mod host/go.sum
git status --porcelain host
```
Expected: the status command prints nothing.

---

## Post-round-2 gate (contract criteria 3–6; a gate member runs it after Tasks 6–9 are signed off)

Tool: `websocat` (named by the contract). Not installed on either host: the gate member reads the latest version from `https://api.github.com/repos/vi/websocat/releases/latest`, installs the macOS-aarch64 and linux-aarch64 release binaries into `~/.local/bin` on each host, records the version, and checks its flags with `websocat --help` before use (expected: `-b` binary mode, `-H 'Name: value'` extra header). The hc core must run with a clean login environment: start it from a normal terminal outside Claude Code (`hc server restart --fresh`) or via the Task 5 unit.

G1 macOS (every command with self-captured `RC=$?`, log in `docs/logs/`):
1. `printf 'me\npw\npw\n' | build/roosterm passwd --config scratch/live/config.toml`; edit `listen = "127.0.0.1:18787"` with the Edit tool.
2. Run roosterm inside an hc terminal so it outlives the agent's foreground limit and has a known PID: `hc run --bg --name roosterm-live <abs path>/build/roosterm serve --config <abs path>/scratch/live/config.toml`; PID from `hc get roosterm-live` (`pid`).
3. Login: `curl -si -X POST -H 'Content-Type: application/json' -d '{"username":"me","password":"pw"}' http://127.0.0.1:18787/login` → `204`, `Set-Cookie: roosterm_session=…; Path=/; Max-Age=2592000; HttpOnly; Secure; SameSite=Strict`. Save the token as `TOK`. Static check: `curl -s http://127.0.0.1:18787/` contains `Roosterm host is running`.
4. No cookie: `curl -si http://127.0.0.1:18787/api/tabs` → `401`; `curl -si -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' 'http://127.0.0.1:18787/ws/tabs/t1?cols=80&rows=24'` → `401`.
5. Create: `curl -si -X POST -H "Cookie: roosterm_session=$TOK" -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:18787/api/tabs` → `201`, save `id` as `ID`.
6. Attach: send `echo PID=$$` + `\r` over websocat binary mode with header `Cookie: roosterm_session=$TOK` to `ws://127.0.0.1:18787/ws/tabs/$ID?cols=80&rows=24`; output contains `PID=<n>`.
7. `kill -9 <roosterm pid>`; start it again as in step 2 under a new name `roosterm-live2`; repeat step 6 (same `TOK`: sessions survive the restart) → the same `PID=<n>`; `ps -p <n>` → alive.
G2 Linux: G1 inside OrbStack `roosterm-spike` with the binary from `python3 scripts/build.py --target linux/arm64` (hc 0.1.21 is installed there; socket `/run/user/501/herdr-core/default.sock`).
G3 swarm: create a test swarm server inside an hc terminal (`hc run --bg --name gate-swarm tmux -L claude-swarm-gate new-session -s s`); `GET /api/swarms` → contains `claude-swarm-gate` and only `claude-swarm-` names; create a tab with `{"command":["tmux","-L","claude-swarm-gate","a"]}`, attach via websocat → output shows the tmux status line `[s] 0:`; then `tmux -L claude-swarm-gate kill-server`. Never touch other `claude-swarm-*` servers.
G4: never `hc server stop` on a host with live programs. Re-run the Task 9 step 6 missing-socket check with the gate's build and record it in the gate log.

---

## Self-review

Run by the plan author on 2026-11-19 after both stages.

**1. Spec coverage** (`docs/superpowers/specs/2026-11-19-roosterm-design.md`, backend parts; `web/` and `app/` are out of contract scope):

| Spec requirement | Task |
|---|---|
| Tabs = hc terminals, list from `terminal.list` only, no own state | 7 (`TestListTabsOnlyRoostermInOrder`), 3 |
| New tab runs `$SHELL -l` | 7 (`TestCreateTabDefaultShell`), 9 (`shell` recorded by `passwd`) |
| Swarm scan lists only `claude-swarm-*`, opens `tmux -L <name> a` | 4, 7 (`TestSwarmsEndpoint`, `TestCreateTabCommandAndLabel`), gate G3 |
| Program end shows "finished (code N)" | 8 (close 4000 with exit code), 7 (`exited`, `exitCode`) |
| Login sets HttpOnly/Secure/SameSite=Strict cookie; every HTTP request and WS upgrade checks it first, 401 before upgrade | 2, 6 (`TestWebSocketUpgradeRejectedWithoutCookie`), gate G1.4 |
| `GET/POST/PATCH/DELETE /api/tabs`, `GET /api/swarms` | 7 |
| WS attach: full screen first, then live output; keys as binary; size as JSON control message | 8 (spec overrides: DATA and RESIZE frames) |
| Several clients per tab, last size wins | hc rule; bridge attaches each socket separately (8) |
| Reconnect = re-attach, hc sends full screen; no timer retry; WS ping interval from config | 8 (`TestBridgeCoreOfflineThenOnline`, pinger), 1 (`ping_interval`) |
| Errors shown, never degraded silently; message has op, tab id, expected and actual | 1 (`apierr`), every task's error strings |
| hc socket missing at start → non-zero exit with expected path | 3, 9 step 6, gate G4 |
| hc core restart → file watch, reconnect, tabs show "core offline" | 3 (supervisor), 8 (`core` messages) |
| Program end → custom close code with exit code | 8 (4000) |
| hc error forwarded as-is | 7 (`TestHCErrorForwarded`, 502), 8 (4500) |
| Too many failed logins → 429 with retry time | 2, 6 (`TestLoginRateLimited`) |
| `roosterm passwd`, argon2id hash in config | 2, 9 |
| Session file 0600, survives restart, expires | 2 (`TestSessionsFileMode`, `TestSessionsSurviveReopen`, `TestSessionExpiry`), gate G1.7 |
| Injectable clock for the rate limit | 2 (`Clock`), 9 (`time.Now` only in `serve.go`) |
| Commands `passwd`, `serve`; config fields | 1, 9 |
| `deploy/` launchd + systemd, hc autostart from spike item 4 | 5 |
| Node only at build time | 4 (`build.py` runs npm only when `web/package.json` exists) |
| `scripts/test.py` calls `go test`, ≤ 40 s, parallel, ≤ 1 s per test, no network, no real clock | 4 |
| Kept tests limited to auth, bridge, swarm scan | Global Constraints; config is run-once (Task 1 step 6) |

Gaps: none in contract scope. Out of contract scope and not planned: `web/` UI, `app/`.

**2. Placeholder scan:** the lead's `grep -nE` for the three placeholder patterns (to-be-decided, to-do, triple question mark) over this file → no match (recorded in the author's report with RC). Every code step contains the full file; no "similar to Task N"; every acceptance item is a command, a test name, or a grep.

**3. Type and name consistency** (Interfaces blocks vs code, checked by compiling all code in this plan in one scratch module and running every named test with `-race`, 20 repetitions):
- `apierr.DecodeJSON`, `apierr.WriteJSON`, `apierr.MaxBody` — produced by Task 1, used by Tasks 6, 7.
- `auth.Cost`, `auth.HashPasswordCost` — produced by Task 2, used by Task 6 tests; `auth.MaxFailures`, `auth.Lockout` — Task 6 tests.
- `hc.Frame`, `hc.WriteFrame`, `hc.ReadFrame`, `hc.ParseResize`, `hc.Kind*` frame constants — Task 3, used by `hctest`.
- `hc.KindNotFound`, `hc.KindFinished` — Task 3, used by Tasks 7, 8.
- `bridge.Close*` are `websocket.StatusCode` — Task 8 tests compare against `websocket.CloseError.Code`.
- Path value name `id` — Task 6 route, Task 7 routes, Task 8 handler and tests.
- Labels `roosterm` / `tab` / `roosterm.name` — `hc` constants only; no literal copies outside `hc` and `hctest` callers' tests.
