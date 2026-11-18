# Roosterm Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use podium:team-spawn (recommended) or podium:team-fable to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `roosterm` (Go, `host/`) authenticates one user and bridges HTTP/WebSocket tabs to herdr-core (`hc`) terminals so programs keep their PID across client disconnects, roosterm restarts and hc core restarts.

**Architecture:** One Go binary. `internal/hc` speaks hc's framed JSON-RPC over the Unix socket (one shared connection, many attach channels) and a supervisor re-dials when the socket file re-appears. `internal/web` serves login, REST tabs/swarms and the embedded static page; `internal/bridge` turns one WebSocket into one hc attach stream. Tabs are hc terminals carrying the label `roosterm=tab`; roosterm stores no tab state.

**Tech Stack:** Go 1.26.4 stdlib (`net/http` 1.22+ patterns, `crypto/rand`, `embed`), `github.com/coder/websocket` v1.8.15, `github.com/fsnotify/fsnotify` v1.10.1, `github.com/BurntSushi/toml` v1.6.0, `golang.org/x/crypto` v0.59.0 (argon2), `golang.org/x/term` v0.48.0 (all versions read from proxy.golang.org `@latest` on 2026-11-19; APIs checked with `go doc` at those versions and context7). Python 3 stdlib for `scripts/`.

**Decisions (hard-to-reverse only):**
- *Keystrokes go as DATA on the attach channel, not `terminal.send`.* Alternatives: `terminal.send` (spec text), `terminal.keys`. Driver: spike — `terminal.send` pastes text and presses Enter (`docs/hc-bridge.md` §4); raw bytes are the only path that supports arbitrary TUI input. Drawback: none known; this corrects the spec.
- *WebSocket stays open while the hc core is offline; roosterm re-attaches server-side.* Alternatives: close with a code and let the client reconnect. Driver: spec forbids timer retries in the client and the client cannot know when hc returns; the server sees the socket file event. Drawback: the bridge holds one goroutine per open tab during an outage.
- *Tab ownership by label `roosterm=tab`; title in label `roosterm.name`.* Alternatives: list every hc terminal; hc `name`. Driver: hc terminals started by agents (`hc run`) would flood the tab bar; hc `name` allows only `[a-z0-9_.-]{1,32}`. Drawback: terminals made outside roosterm are invisible to it.
- *Login rate limit is global (single user), not per IP.* Alternatives: per-IP with trusted `X-Forwarded-For`. Driver: single account behind the user's reverse proxy; every request has the proxy's IP. Drawback: an attacker can keep the owner locked out for the lockout window.
- *Config gains `shell` (not in the spec's field list).* Driver: under launchd `SHELL` is unset, so "`$SHELL -l`" has no value at serve time; `roosterm passwd` records `$SHELL` once from the user's interactive shell.

Plan gate: `/Users/jhangyu/.claude/plugins/cache/jhangyu/podium/1.1.1/protocols/review.md` §L0 Plan gate.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-11-19-roosterm-design.md`. Contract: `docs/logs/contract-backend.md` (frozen). hc wire facts: `docs/hc-bridge.md` (authoritative over the spec where they differ).
- herdr-core pinned at 0.1.21, protocol generation 1. hello must report `generation == 1`, `framed == true`, features ⊇ {`attach`}.
- Go module path `roosterm/host`, directory `host/`, `go 1.26.4`. No other third-party modules than the Tech Stack list.
- macOS and Linux, one code path: no `runtime.GOOS` switches, no build tags.
- No sleep, no polling loops, no timer-based retries in product or test code. Wait on channels, fsnotify events, or context cancellation. Time as business data (session expiry, lockout, ping interval) uses an injected clock or a config value.
- No silent fallback: missing required data → error. Every error message names the operation, the affected object (tab id, path, method) and expected vs actual.
- No swallowed errors (no `_ = err` on fallible calls except `Close` on an already-failed path, which must carry a `ponytail:` or explanatory comment).
- No dead code, no commented-out code, no TODOs, no constant feature flags. Magic values are named constants with semantic names.
- Tests: entry `python3 scripts/test.py`; whole suite ≤ 40 s, each test ≤ 1 s, parallel (`t.Parallel()` in every test), hermetic (no network, no real clock in assertions, no real hc). Fake hc over a Unix socket (`internal/hc/hctest`). Socket paths in tests come from `os.MkdirTemp("", "rt")` (macOS limits Unix socket paths to 104 bytes; `t.TempDir()` is too long). Test files must not contain `time.Sleep` or the word `sleep`.
- Kept tests only cover: auth (argon2id verify, cookie attributes, session expiry, rate limit, WS upgrade rejected without cookie), bridge (WS↔JSON-RPC translation, exit code propagation, hc offline behavior), swarm scan. Config/CLI/static get a run-once check, not a kept test, except where named below.
- Shared-tree git: no stash (including `stash list`), reset, `checkout --`, clean, `git add -A`. Commit with `git add <own paths>` then `git commit -m "<msg>" -- <own paths>`; no `;` in `-m`.
- Build output only in `build/` (gitignored, Task 4).

---

## HTTP/WS API (contract owned by this plan; the client plan consumes it)

All paths are same-origin. JSON bodies are UTF-8, `Content-Type: application/json`. Max request body 64 KiB (413 above).

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
| `unauthorized` | 401 | missing/unknown/expired cookie; wrong credentials on `/login` |
| `not_found` | 404 | tab id unknown to hc, or exists but lacks label `roosterm=tab` |
| `too_many_attempts` | 429 | login locked; header `Retry-After: <seconds>` (integer, ≥1) |
| `core_offline` | 503 | hc socket not connected right now |
| any other hc kind (`finished`, `unavailable`, `invalid_params`, …) | 502 | hc returned a JSON-RPC error; message/kind/hint verbatim |
| `internal` | 500 | roosterm bug or I/O failure (session file write etc.) |

### Endpoints

| Method + path | Auth | Request | Success |
|---|---|---|---|
| `POST /login` | none | `{"username":string,"password":string}` | `204`, `Set-Cookie` |
| `POST /logout` | cookie | none | `204`, clearing `Set-Cookie`; session removed from file |
| `GET /api/tabs` | cookie | none | `200` `Tab[]`, ordered by `created_ms` ascending |
| `POST /api/tabs` | cookie | `{"command"?: string[], "label"?: string}` | `201` `Tab` |
| `PATCH /api/tabs/{id}` | cookie | `{"label": string}` | `200` `Tab` |
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

Login rate limit: after `login_max_failures = 5` consecutive failures, every `/login` returns 429 for `login_lockout = 60 s` (constants in `internal/auth`); a success resets the count. Requests during lockout do not extend it and are not checked against the password.

### WebSocket `/ws/tabs/{id}?cols=N&rows=N`

Pre-upgrade (plain HTTP responses with the error body): 401 without valid cookie; 400 if `cols`/`rows` missing or not in 1..1000. Origin check: coder/websocket default (Origin host must equal request Host).

After upgrade:

| Direction | Frame | Content |
|---|---|---|
| client → server | binary | keystroke/paste bytes, forwarded verbatim as hc DATA (max message 1 MiB) |
| client → server | text | `{"type":"resize","cols":N,"rows":N}` (1..1000) → hc RESIZE |
| server → client | binary | terminal output. The first binary message after (re)attach is the full screen (starts with `\x18\x1b[!p`); write it as-is, no client reset needed |
| server → client | text | `{"type":"core","state":"offline"}` — hc core gone; tab stays open, input is dropped |
| server → client | text | `{"type":"core","state":"online"}` — re-attached; a full-screen binary message follows |
| server → client | text | `{"type":"error","op":"tab.input","tab":"t7","kind":"core_offline","message":"…"}` — one per client message dropped while offline, and before every 4400/4500 close |
| server → client | ping | every `ping_interval` (config); browsers answer automatically |

Close codes (server → client):

| code | reason text | meaning |
|---|---|---|
| 4000 | JSON `{"reason":"exited","exitCode":7,"signal":null}` | program ended (also sent immediately when the tab is already finished at connect) |
| 4004 | `tab t7 not found` | id unknown or not a roosterm tab |
| 4400 | `bad message: <detail>` | invalid text frame (bad JSON, unknown type, size out of range) |
| 4500 | hc message, truncated to 123 bytes | hc returned an error on attach/input |
| 1001 | `server shutdown` | roosterm is stopping |

The client reconnects only on its own `online`/`visibilitychange` signals (spec). While a socket is open, hc restarts are handled by the server (`core` messages).

Multiple sockets on one tab are allowed; the last one that attached/typed/resized sets the size (hc rule).

### Config file `~/.config/roosterm/config.toml` (`--config <path>` overrides)
```toml
listen = "127.0.0.1:8787"
hc_socket = ""          # empty: hc's own rule, docs/hc-bridge.md §1
ping_interval = "25s"
session_ttl = "720h"
shell = "/bin/zsh"
username = "me"
password_hash = "$argon2id$v=19$m=65536,t=3,p=4$<salt b64>$<key b64>"
```
Sessions file: `sessions.json` in the same directory as the config file, mode 0600.

---

## Work packages

| Round | Task | Package | Model | Owned files | Depends on |
|---|---|---|---|---|---|
| 1 | 1 | Module scaffold, config, API error type | sonnet | `host/go.mod`, `host/go.sum`, `host/internal/config/*`, `host/internal/apierr/*` | — |
| 1 | 2 | Auth: argon2id, sessions file, rate limit, cookie | sonnet | `host/internal/auth/*` | Task 1 step 1 (go.mod) |
| 1 | 3 | hc client, attach streams, supervisor with socket watch, fake hc | opus | `host/internal/hc/*`, `host/internal/hc/hctest/*` | Task 1 step 1 |
| 1 | 4 | Swarm scan, `scripts/test.py`, `scripts/build.py`, `.gitignore` | sonnet | `host/internal/swarm/*`, `scripts/test.py`, `scripts/build.py`, `.gitignore` | Task 1 step 1 |
| 1 | 5 | Deploy units | sonnet | `deploy/**` | — |
| 2 | 6 | Web shell: login/logout, session middleware, static embed | sonnet | `host/internal/web/*`, `host/internal/web/dist/index.html` | 1, 2 |
| 2 | 7 | REST API: tabs, swarms | sonnet | `host/internal/api/*` | 1, 3, 4 |
| 2 | 8 | WebSocket ↔ hc bridge | opus | `host/internal/bridge/*` | 1, 3 |
| 2 | 9 | `cmd/roosterm` (`serve`, `passwd`), module tidy | sonnet | `host/cmd/roosterm/*`; `host/go.mod`, `host/go.sum` (tidy only) | 1, 2, 3; step 4 waits for the Task 6, 7, 8 commits |

Round 1 start rule: Task 1 commits `host/go.mod` + `host/go.sum` first (step 1, minutes); Tasks 2–4 begin coding at once and run `go test` after that commit lands. Task 9 is the only intra-round ordering in round 2: its steps 1–3 depend on round 1 only; step 4 (serve wiring) starts when Tasks 6, 7, 8 have each committed their exported constructor.

No two tasks in a round share a file. `host/go.mod`/`go.sum`: Task 1 in round 1, Task 9 in round 2. Every other task that finds it needs a module change STOPs and reports (all modules are pinned in Task 1).

### Contract acceptance mapping

| Contract criterion | Satisfied by |
|---|---|
| 1 spike items 1–3 | Done: `docs/logs/2026-11-19-spike-hc.md` (commit d64498d) |
| 2 `python3 scripts/test.py` exit 0, ≤40 s, 0 skipped, no sleep | Task 4 (runner + checks); tests from Tasks 1, 2, 3, 4, 6, 7, 8 |
| 3 live proof macOS | Tasks 3, 6, 7, 8, 9; executed in "Post-round-2 gate" G1 |
| 4 live proof Linux | Same + Task 4 `build.py --target linux/arm64`; gate G2 |
| 5 `/api/swarms` lists only `claude-swarm-*`; swarm tab attaches | Task 4 (scan, `TestListOnlyClaudeSwarmSockets`), Task 7 (`TestSwarmsEndpoint`), gate G3 |
| 6 non-zero exit with socket path when hc is down | Task 3 (`TestDialMissingSocketNamesPath`), Task 9 (step 3 run-once check), gate G4 |
| 7 launchd plist + systemd user unit start `roosterm serve` | Task 5 |

---

## Round 1

### Task 1: Module scaffold, config, API error type

**Files:**
- Create: `host/go.mod`, `host/go.sum`
- Create: `host/internal/config/config.go`, `host/internal/config/config_test.go`
- Create: `host/internal/apierr/apierr.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `package config`
    - `type Config struct { Listen string; HCSocket string; PingInterval time.Duration; SessionTTL time.Duration; Shell string; Username string; PasswordHash string }` — `HCSocket` is the raw value (may be `""`).
    - `const DefaultListen = "127.0.0.1:8787"`, `const DefaultPingInterval = 25 * time.Second`, `const DefaultSessionTTL = 720 * time.Hour`
    - `func DefaultPath() (string, error)` — `$XDG_CONFIG_HOME/roosterm/config.toml`, else `$HOME/.config/roosterm/config.toml`; error if `HOME` is empty and `XDG_CONFIG_HOME` unset.
    - `func Load(path string) (Config, error)`
    - `func Save(path string, c Config) error`
    - `func SessionsPath(configPath string) string` — `filepath.Join(filepath.Dir(configPath), "sessions.json")`
    - `func (c Config) SocketPath() string` — `c.HCSocket` if non-empty, else `DefaultSocket(os.Getenv, os.Getuid(), dirExists)`.
    - `func DefaultSocket(getenv func(string) string, uid int, dirExists func(string) bool) string`
  - `package apierr`
    - `type Error struct { Status int \`json:"-"\`; Op string \`json:"op"\`; Tab string \`json:"tab,omitempty"\`; Message string \`json:"message"\`; Kind string \`json:"kind"\`; Hint string \`json:"hint,omitempty"\` }`
    - `func (e *Error) Error() string` — `"<op> <tab>: <message> (kind <kind>)"`, tab part omitted when empty.
    - `func Write(w http.ResponseWriter, e *Error)` — sets `Content-Type: application/json`, writes `e.Status`, body `{"error":<e>}`.
    - Kind constants: `KindBadRequest = "bad_request"`, `KindUnauthorized = "unauthorized"`, `KindNotFound = "not_found"`, `KindTooManyAttempts = "too_many_attempts"`, `KindCoreOffline = "core_offline"`, `KindInternal = "internal"`.

**Behavior:**
- `Load`: decode with `toml.DecodeFile` into a struct of string fields; `md.Undecoded()` non-empty → error listing the unknown keys. Required (non-empty): `listen`, `ping_interval`, `session_ttl`, `shell`, `username`, `password_hash`. `hc_socket` may be empty. Durations via `time.ParseDuration`, must be > 0. `shell` must be absolute and `os.Stat` must show a regular file with any execute bit. Each error: `config <path>: <key>: expected <what>, got <actual>`.
- `Save`: `os.MkdirAll(dir, 0o700)`, write `<path>.tmp` with mode 0600 via `toml.NewEncoder`, then `os.Rename`. Durations written with `Duration.String()`.
- `DefaultSocket` (hc rule, `docs/hc-bridge.md` §1): `XDG_RUNTIME_DIR` set → `$XDG_RUNTIME_DIR/herdr-core/default.sock`; else `/run/user/<uid>` exists → `/run/user/<uid>/herdr-core/default.sock`; else `<XDG_STATE_HOME or $HOME/.local/state>/herdr-core/default/default.sock`.

**Constraints:**
- Only `github.com/BurntSushi/toml` beyond stdlib in `config`; `apierr` stdlib only.
- go.mod pins exactly the Tech Stack versions.

**Acceptance criteria:**
- [ ] `cd host && go list -m all` lists `github.com/coder/websocket v1.8.15`, `github.com/fsnotify/fsnotify v1.10.1`, `github.com/BurntSushi/toml v1.6.0`, `golang.org/x/crypto v0.59.0`, `golang.org/x/term v0.48.0`.
- [ ] `cd host && go test ./internal/config/ -run 'TestLoadRejectsUnknownKey|TestLoadRoundTrip|TestDefaultSocketRule' -count=1` passes.
- [ ] `cd host && go vet ./internal/config/ ./internal/apierr/` exits 0.

### Task 2: Auth

**Files:**
- Create: `host/internal/auth/password.go`, `session.go`, `ratelimit.go`, `cookie.go`
- Test: `host/internal/auth/auth_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces (`package auth`):
  - `type Clock func() time.Time`
  - `func NewSalt() ([]byte, error)` — 16 bytes, `crypto/rand`.
  - `func HashPassword(password string, salt []byte) string` — PHC `$argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>` (raw std base64, no padding).
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
- argon2id params as named constants: `argonTime = 3`, `argonMemoryKiB = 64 * 1024`, `argonThreads = 4`, `argonKeyLen = 32`, `saltLen = 16`. `VerifyPassword` parses params from the PHC string (does not assume the constants) and compares with `subtle.ConstantTimeCompare`. Version other than 19 → error.
- Sessions file JSON `{"sessions":[{"hash":"<sha256 hex>","expires_ms":<unix ms>}]}`. `OpenSessions`: missing file → empty store; existing file with mode bits beyond 0600 → error `sessions <path>: expected mode 0600, got <mode>`; broken JSON → error (never reset). Expired entries are dropped from memory on open. `Create`: 32 random bytes, base64url raw → token; store sha256; purge expired; write file atomically (tmp 0600 + rename). `Valid`: entry exists and `now() < expires`. `Delete`: remove entry (unknown token → no error), persist.
- Limiter: counts consecutive failures; on the `MaxFailures`th failure records `lockedUntil = now + Lockout`. `Allow`: if `now < lockedUntil` → `(lockedUntil-now, false)`; once the lockout passes, failures reset to 0. `Succeed` resets.
- Cookies: `Path "/"`, `HttpOnly`, `Secure`, `SameSite http.SameSiteStrictMode`, `MaxAge int(ttl.Seconds())`; clear has `MaxAge -1` (net/http writes `Max-Age=0`) and empty value.

**Constraints:**
- `golang.org/x/crypto/argon2` only; no bcrypt/scrypt. Clock injected everywhere; no `time.Now` outside the `Clock` passed by callers.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/auth/ -count=1 -v` passes and runs `TestVerifyPassword`, `TestVerifyPasswordMalformed`, `TestSessionCookieAttributes`, `TestSessionExpiry`, `TestSessionsSurviveReopen`, `TestSessionsFileMode`, `TestLimiterLockout`.
- [ ] `grep -n 'time.Now' host/internal/auth/*.go` → no matches.

### Task 3: hc client, attach streams, supervisor, fake hc

**Files:**
- Create: `host/internal/hc/proto.go` (frame codec), `types.go`, `client.go`, `stream.go`, `supervisor.go`
- Create: `host/internal/hc/hctest/fake.go`
- Test: `host/internal/hc/client_test.go`, `host/internal/hc/supervisor_test.go` (package `hc_test`)

**Interfaces:**
- Consumes: wire format in `docs/hc-bridge.md` §2–§6.
- Produces (`package hc`):
  - `type Size struct { Cols uint16 \`json:"cols"\`; Rows uint16 \`json:"rows"\` }`
  - `type Terminal struct { Address string \`json:"address"\`; Argv []string \`json:"argv"\`; Labels map[string]string \`json:"labels"\`; State string \`json:"state"\`; Reason *string \`json:"reason"\`; ExitCode *int \`json:"exit_code"\`; Signal *int \`json:"signal"\`; PID *int \`json:"pid"\`; Cwd string \`json:"cwd"\`; CreatedMS int64 \`json:"created_ms"\`; Cols uint16 \`json:"cols"\`; Rows uint16 \`json:"rows"\` }`
  - `const StateLive = "live"`, `const StateFinished = "finished"`
  - `const LabelOwner = "roosterm"`, `const LabelOwnerTab = "tab"`, `const LabelName = "roosterm.name"`; `func IsTab(t Terminal) bool` — `t.Labels[LabelOwner] == LabelOwnerTab` (tab ownership, shared by Tasks 7 and 8)
  - `type RunParams struct { Argv []string \`json:"argv"\`; Cwd string \`json:"cwd"\`; Size Size \`json:"size"\`; Labels map[string]string \`json:"labels"\`; NoParent bool \`json:"no_parent"\` }`
  - `type RPCError struct { Method string; Code int; Message string; Kind string; Hint string }` with `Error() string` = `"hc <method>: <message> (kind <kind>)"`
  - `type Exit struct { Reason string \`json:"reason"\`; ExitCode *int \`json:"exit_code"\`; Signal *int \`json:"signal"\` }` with `Error() string` = `"program ended: reason <r>, exit code <n|none>, signal <n|none>"`
  - `var ErrCoreGone = errors.New("hc core connection closed")`, `var ErrCoreOffline = errors.New("hc core offline")`, `var ErrDetached = errors.New("hc stream detached")`
  - `func Dial(ctx context.Context, socketPath string) (*Client, error)`
  - `func (c *Client) Call(ctx context.Context, method string, params, result any) error`
  - `func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error)`
  - `func (c *Client) List(ctx context.Context, labels map[string]string) ([]Terminal, error)` — sends `all: true`
  - `func (c *Client) Get(ctx context.Context, id string) (Terminal, error)`
  - `func (c *Client) Label(ctx context.Context, id string, labels map[string]string) (Terminal, error)`
  - `func (c *Client) Kill(ctx context.Context, id string) (Terminal, error)`
  - `func (c *Client) Remove(ctx context.Context, id string) error`
  - `func (c *Client) Attach(ctx context.Context, id string, size Size) (*Stream, error)`
  - `func (c *Client) Done() <-chan struct{}` — closed when the Unix connection ends.
  - `func (c *Client) Close() error`
  - `func (s *Stream) Recv(ctx context.Context) ([]byte, error)` — next DATA payload; at stream end returns `*Exit` (core CLOSE with reason), `ErrDetached` (after our `Close`) or `ErrCoreGone` (connection ended).
  - `func (s *Stream) Credit(n int) error`
  - `func (s *Stream) Write(p []byte) error` — DATA
  - `func (s *Stream) Resize(size Size) error` — RESIZE
  - `func (s *Stream) Close() error` — detach (CLOSE, empty)
  - `func Start(ctx context.Context, socketPath string) (*Supervisor, error)`
  - `func (s *Supervisor) Client() (*Client, error)` — current client or `ErrCoreOffline`
  - `func (s *Supervisor) Await(ctx context.Context) (*Client, error)` — blocks until online or ctx done
  - `func (s *Supervisor) Close() error`
- Produces (`package hctest`, test helper imported by Tasks 7 and 8):
  - `type Fake struct { Path string }` (plus unexported state)
  - `func Start(t testing.TB) *Fake` — socket at `<os.MkdirTemp("", "rt")>/hc.sock`, cleanup via `t.Cleanup`.
  - `func (f *Fake) Stop()` — close listener and every connection, remove the socket file (models crash/stop).
  - `func (f *Fake) Restart(t testing.TB)` — listen again on `f.Path`.
  - `func (f *Fake) Seed(term hc.Terminal)` — add a terminal record as-is.
  - `func (f *Fake) Get(id string) (hc.Terminal, bool)`
  - `func (f *Fake) Exit(id string, code int)` — mark finished (`reason "exited"`), send CLOSE with reason JSON to every attach stream of `id`.
  - `func (f *Fake) Input(id string) <-chan []byte` — DATA payloads received from viewers (buffered 64).
  - `func (f *Fake) Resized(id string) <-chan hc.Size` — RESIZE frames received (buffered 64).
  - `func (f *Fake) FailNext(method string, e hc.RPCError)` — the next call of `method` returns this error.
  - `func Paint(id string) []byte` — first-paint bytes the fake sends: `"\x18\x1b[!p" + "screen:" + id`.

**Behavior:**
- One Unix connection per `Client`. `Dial`: `net.Dialer.DialContext("unix", path)`; on failure return `fmt.Errorf("hc not running: %w; expected a listening herdr-core socket at %s (start it with \"hc server start\")", err, path)` — the wrapped `*net.OpError` already includes `dial unix <path>`. Then send plain hello `{"framed":true,"generation":1}`, read one `\n` line, check `generation == 1`, `framed == true`, `features` contains `attach`; mismatch → error `hc hello at <path>: expected <field> <want>, got <got>`.
- A reader goroutine owns the read side: channel 0 DATA → JSON-RPC reply routed by `id` to the waiting `Call`; other channels → that stream's queue (mutex + slice + notify channel; unbounded is safe because hc stops at the 256 KiB credit window — mark with `ponytail:`). CLOSE from core: payload non-empty → parse `Exit`, deliver as stream end; empty → reply to our detach; `Recv` then returns `ErrDetached`. Unknown channel → ignore frame (stream already closed by us). Read error/EOF → close `Done()`, fail all pending calls with `ErrCoreGone`, end all streams with `ErrCoreGone`.
- Writes serialized by one mutex (frames must not interleave).
- `Call` result decoding: JSON-RPC `error` → `*RPCError{Method, Code, Message, Kind: data.kind, Hint: data.hint}`. ctx done → return `ctx.Err()` wrapped with method name; the reply, if it arrives later, is dropped.
- `Attach`: call `terminal.attach {terminal, cols, rows}`; register the returned channel **before** the reply is processed by the reader so first-paint frames are not lost (the reader registers the stream when it routes the attach reply: `Call` for attach passes a hook). Simplest correct approach: hold a `pendingAttach` map keyed by request id; the reader, on routing an attach reply with a `result.channel`, creates the stream queue under the same lock before reading the next frame.
- Credit: bridge calls `Credit(len(p))` after writing each payload to the WebSocket.
- Supervisor: `Start` creates an fsnotify watcher on `filepath.Dir(socketPath)` **before** dialing, then dials once; dial failure → close watcher, return the `Dial` error (criterion 6). Online: holds `*Client`; a goroutine waits on `client.Done()` → state offline. While offline, each fsnotify event whose `filepath.Clean(event.Name) == filepath.Clean(socketPath)` (any op) triggers one `Dial`; success → online and wake all `Await`ers; failure → stay offline (log one line `hc reconnect: <err>` to stderr via `log.Printf`). Events while online are ignored (macOS repeats spurious `CREATE`). Watcher error channel → log and keep watching. ponytail: one dial per event; a dial landing between the new core's bind and listen waits for the next event on that name.
- Await: wait on a `chan struct{}` that is closed on each transition to online and replaced on each transition to offline.
- Fake: speaks the real format: plain hello line reply `{"jsonrpc":"2.0","id":<id>,"result":{"generation":1,"framed":true,"features":["framed","attach","events"],"version":"0.1.21","core":{"id":"fake","name":"fake"},"methods":[]}}`, then frames. Methods: `terminal.run` (assign `t1`, `t2`, …, `pid` 1000+n, state live, store labels/argv/size), `terminal.list` (filter by all given labels; `all` honored), `terminal.get`, `terminal.label` (merge, `""` deletes), `terminal.kill` (finished, reason `killed`, signal 1, CLOSE `{"exit_code":null,"reason":"killed","signal":1}` to streams), `terminal.rm` (finished only; live → error kind `invalid_params`), `terminal.attach` (unknown → `-32000` kind `not_found` message `no terminal "<id>"`; finished → kind `finished` message `<id> has finished`; else reply `{"channel":n,"automatic_input":true,"resize_notifications":false,"terminal":{…}}` then DATA `Paint(id)`). Stream DATA from viewer → copied to `Input(id)` and echoed back as DATA (so bridge tests observe output). RESIZE → `Resized(id)` and record size. CLOSE from viewer → empty CLOSE reply.

**Constraints:**
- Only stdlib + fsnotify in `hc`; `hctest` stdlib + `hc`.
- No goroutine leaks: `Client.Close` stops the reader; `Supervisor.Close` closes the watcher and client.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/hc/... -count=1 -race -v` passes and runs `TestFrameRoundTrip`, `TestDialMissingSocketNamesPath`, `TestDialRejectsWrongGeneration`, `TestCallReturnsRPCError`, `TestAttachFirstPaintThenEcho`, `TestStreamExitDeliversCode`, `TestStreamEndsWhenCoreGone`, `TestSupervisorOfflineThenReconnect`, `TestSupervisorIgnoresOtherFiles`.
- [ ] `TestDialMissingSocketNamesPath` asserts the error string contains the socket path and `hc server start`.

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
- `test.py` (stdlib only), from repo root:
  1. Forbidden-word scan: every `*_test.go` and every file under `host/internal/hc/hctest/` matched against regex `(?i)\bsleep\b|time\.Sleep` → any hit: print `file:line` and exit 1.
  2. `gofmt -l host` must print nothing; `go vet ./...` (cwd `host`) must exit 0.
  3. `go test ./... -json -count=1 -race` (cwd `host`, default package parallelism), subprocess timeout `SUITE_BUDGET_S = 40`. Parse events: per-test `Elapsed` from `pass`/`fail` actions; count `skip` actions on tests.
  4. Fail (exit 1) if: go test RC ≠ 0, wall time > `SUITE_BUDGET_S`, any test `Elapsed` > `TEST_BUDGET_S = 1.0`, skipped > 0, zero tests ran. Always print: total tests, skipped count, wall seconds, and the 5 slowest tests.
- `build.py`: if `web/package.json` exists → `npm ci --prefix web` then `npm run build --prefix web` (the client plan sets Vite `outDir` to `host/internal/web/dist`); else print `web/: no package.json, embedding host/internal/web/dist as committed` . Then `go build -trimpath -o build/<name> ./cmd/roosterm` (cwd `host`, `CGO_ENABLED=0`, `GOOS`/`GOARCH` from `--target`). Non-zero RC of any step → print the command and exit with that RC.

**Constraints:**
- Python stdlib only, no platform branches. Named constants for budgets.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/swarm/ -count=1 -v` runs `TestListOnlyClaudeSwarmSockets`, `TestListMissingDir`, `TestDir` and passes.
- [ ] `python3 scripts/test.py; echo RC=$?` → `RC=0`, output contains `skipped: 0`.
- [ ] Injected violation: copy any `_test.go` to `scratch/t4/` backup, append `// sleep` to it, `python3 scripts/test.py` → RC 1 naming that file; restore from backup.
- [ ] `grep -n '^build/$' .gitignore` → 1 match.

### Task 5: Deploy units

**Files:**
- Create: `deploy/launchd/dev.roosterm.serve.plist`
- Create: `deploy/systemd/herdr-core.service`, `deploy/systemd/roosterm.service`
- Create: `deploy/README.md`

**Interfaces:**
- Consumes: `roosterm serve` (Task 9), `hc server start` (idempotent, spike log "Autostart").
- Produces: files a user copies to `~/Library/LaunchAgents/` or `~/.config/systemd/user/`.

**Behavior:**
- launchd: `Label dev.roosterm.serve`; `ProgramArguments` = `/bin/zsh`, `-lc`, `hc server start && exec "$HOME/.local/bin/roosterm" serve` (a login shell so the hc core inherits the login PATH; README says to replace `/bin/zsh` with the user's login shell); `RunAtLoad true`; `KeepAlive` `{SuccessfulExit false}` (restart roosterm after a crash, not after a clean exit); `StandardOutPath`/`StandardErrorPath` `~/Library/Logs/roosterm.log` written as an absolute path placeholder the README tells the user to edit (launchd does not expand `~`): use `/Users/USERNAME/Library/Logs/roosterm.log` and a README `sed` line.
- systemd `herdr-core.service`: `Type=oneshot`, `RemainAfterExit=yes`, `ExecStart=%h/.local/bin/hc server start`, `[Install] WantedBy=default.target`. No `ExecStop` (hc server stop would end every program; README states this).
- systemd `roosterm.service`: `Requires=herdr-core.service`, `After=herdr-core.service`, `ExecStart=%h/.local/bin/roosterm serve`, `Restart=on-failure`, `[Install] WantedBy=default.target`.
- README: install commands for both, `loginctl enable-linger $USER` note for boot without login, warning "never `hc server stop` on a live host", log locations.

**Acceptance criteria:**
- [ ] `plutil -lint deploy/launchd/dev.roosterm.serve.plist` → `OK`.
- [ ] In OrbStack `roosterm-spike`: `systemd-analyze --user verify deploy/systemd/herdr-core.service deploy/systemd/roosterm.service; echo RC=$?` → `RC=0` (binary-missing warnings for `roosterm` are acceptable only if the message is exactly about the ExecStart path; record the output).
- [ ] `grep -c 'roosterm serve' deploy/launchd/dev.roosterm.serve.plist deploy/systemd/roosterm.service` → ≥1 each.

---

## Round 2

### Task 6: Web shell — login, logout, session middleware, static embed

**Files:**
- Create: `host/internal/web/server.go`, `host/internal/web/login.go`, `host/internal/web/static.go`
- Create: `host/internal/web/dist/index.html` (placeholder page)
- Test: `host/internal/web/web_test.go`

**Interfaces:**
- Consumes: `auth.Sessions`, `auth.Limiter`, `auth.VerifyPassword`, `auth.SessionCookie`, `auth.ClearCookie`, `auth.CookieName` (Task 2); `apierr.Error`, `apierr.Write`, kind constants (Task 1).
- Produces (`package web`):
  - `type Options struct { Username string; PasswordHash string; SessionTTL time.Duration; Sessions *auth.Sessions; Limiter *auth.Limiter; API http.Handler; TabSocket http.Handler }`
  - `func New(o Options) http.Handler`

**Behavior:**
- Routes on one `http.ServeMux`: `POST /login` (public), `POST /logout` (session), `/api/` → `o.API` (session), `GET /ws/tabs/{id}` → `o.TabSocket` (session; the mux sets the `id` path value on the request, which the middleware passes through unchanged), `GET /` → static (public).
- Session middleware (unexported `requireSession`): read cookie `auth.CookieName`; missing or `!Sessions.Valid` → `apierr.Write` 401 kind `unauthorized`, op `session`, message `missing or expired session cookie`. It runs before any upgrade, so a WebSocket handshake without a cookie gets HTTP 401 and the handler is never called.
- `POST /login`: body ≤ 64 KiB (`http.MaxBytesReader`; overflow → 413 kind `bad_request`), `json.Decoder.DisallowUnknownFields`, both fields non-empty (400). `Limiter.Allow` false → 429, header `Retry-After` = `ceil(retryAfter.Seconds())` (≥1), kind `too_many_attempts`, message names the seconds. Credentials: `subtle.ConstantTimeCompare` on username **and** `VerifyPassword` (always run both, no early exit); mismatch → `Limiter.Fail`, 401 kind `unauthorized`, message `wrong username or password`. `VerifyPassword` error → 500 kind `internal`. Success → `Limiter.Succeed`, `Sessions.Create` (error → 500), `http.SetCookie(w, auth.SessionCookie(token, o.SessionTTL))`, 204.
- `POST /logout`: `Sessions.Delete(token)` (error → 500), `http.SetCookie(w, auth.ClearCookie())`, 204.
- Static: `//go:embed all:dist`; `fs.Sub(dist, "dist")`; request path cleaned; existing file → `http.ServeFileFS`; otherwise `index.html` (SPA routes). `dist/index.html` placeholder: `<!doctype html><title>Roosterm</title><p>Roosterm host is running. The web client is not built yet.</p>`.

**Constraints:** stdlib + internal packages only.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/web/ -count=1 -race -v` runs and passes `TestLoginSetsCookieAttributes`, `TestLoginWrongPassword`, `TestLoginRateLimited`, `TestAPIRequiresSession`, `TestWebSocketUpgradeRejectedWithoutCookie`, `TestExpiredSessionRejected`, `TestLogoutClearsSession`.
- [ ] `TestWebSocketUpgradeRejectedWithoutCookie` sends `Connection: Upgrade`, `Upgrade: websocket`, `Sec-WebSocket-Version: 13`, `Sec-WebSocket-Key` without cookie → status 401 and the `TabSocket` handler records zero calls.

### Task 7: REST API — tabs and swarms

**Files:**
- Create: `host/internal/api/api.go`, `host/internal/api/tabs.go`, `host/internal/api/swarms.go`
- Test: `host/internal/api/api_test.go`

**Interfaces:**
- Consumes: `hc.Start`, `hc.Supervisor.Client`, `hc.Client.{Run,List,Get,Label,Kill,Remove}`, `hc.RunParams`, `hc.Terminal`, `hc.Size`, `hc.RPCError`, `hc.ErrCoreOffline`, `hc.ErrCoreGone`, `hc.IsTab`, `hc.LabelOwner`, `hc.LabelOwnerTab`, `hc.LabelName`, `hc.StateFinished`, `hc.StateLive` (Task 3); `hctest.Start`, `Fake.{Seed,Get,FailNext,Stop}` (Task 3, tests); `swarm.List` (Task 4); `apierr` (Task 1).
- Produces (`package api`):
  - `type Options struct { Core *hc.Supervisor; Shell string; Home string; SwarmDir string }`
  - `func New(o Options) http.Handler` — handles `GET /api/tabs`, `POST /api/tabs`, `PATCH /api/tabs/{id}`, `DELETE /api/tabs/{id}`, `GET /api/swarms` on its own `ServeMux` (Task 6 mounts it at `/api/`).
  - `type Tab struct { ID string \`json:"id"\`; Label string \`json:"label"\`; Exited bool \`json:"exited"\`; ExitCode *int \`json:"exitCode"\`; Signal *int \`json:"signal"\`; Reason *string \`json:"reason"\` }`

**Behavior:**
- Exactly the "HTTP/WS API" section. Ops for error bodies: `tab.list`, `tab.create`, `tab.rename`, `tab.close`, `swarm.list`.
- Named constants: `maxLabelLen = 64`, `initialSize = hc.Size{Cols: 80, Rows: 24}`, `maxBody = 64 << 10`.
- hc error mapping (unexported `hcError(op, tab string, err error) *apierr.Error`): `errors.Is(err, hc.ErrCoreOffline)` or `errors.Is(err, hc.ErrCoreGone)` → 503 `core_offline`; `errors.As(*hc.RPCError)` with kind `not_found` → 404 with hc message/hint; any other kind → 502 with hc message/kind/hint; else 500 `internal`.
- Ownership: PATCH/DELETE call `Get` first; `!hc.IsTab` → 404 `not_found`, message `tab <id> not found`.
- List: `List(ctx, {LabelOwner: LabelOwnerTab})`, stable sort by `CreatedMS`, map to `Tab`. Empty → `[]`, never `null`.
- Create: argv as specified in the API section; `Cwd = o.Home`; labels `{LabelOwner: LabelOwnerTab, LabelName: label}`; `NoParent: true`; `Size: initialSize`; 201 with the mapped record.
- Rename: `Label(ctx, id, {LabelName: label})` → 200 mapped record.
- Delete: live → `Kill` then `Remove`; finished → `Remove`; 204.
- Swarms: `swarm.List(o.SwarmDir)` → 200 JSON array; error → 500 `internal` with the error text.

**Constraints:** stdlib + internal packages.

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/api/ -count=1 -race -v` runs and passes `TestCreateTabDefaultShell`, `TestCreateTabCommandAndLabel`, `TestCreateTabRejectsBadInput`, `TestListTabsOnlyRoostermInOrder`, `TestRenameTab`, `TestDeleteLiveTabKillsThenRemoves`, `TestDeleteFinishedTab`, `TestForeignTerminalIsNotFound`, `TestHCErrorForwarded`, `TestCoreOfflineIs503`, `TestSwarmsEndpoint`.

### Task 8: WebSocket ↔ hc bridge

**Files:**
- Create: `host/internal/bridge/bridge.go`
- Test: `host/internal/bridge/bridge_test.go`

**Interfaces:**
- Consumes: `hc.Start`, `hc.Supervisor.{Client,Await}`, `hc.Client.{Get,Attach}`, `hc.Stream.{Recv,Credit,Write,Resize,Close}`, `hc.Exit`, `hc.RPCError`, `hc.ErrCoreOffline`, `hc.ErrCoreGone`, `hc.IsTab`, `hc.StateFinished`, `hc.Size` (Task 3); `hctest` (Task 3, tests); `apierr` (Task 1).
- Produces (`package bridge`):
  - `func New(core *hc.Supervisor, pingInterval time.Duration) http.Handler` — reads path value `id` (pattern `GET /ws/tabs/{id}` registered by Task 6; tests register the same pattern on their own mux).
  - Close-code constants: `CloseExited = 4000`, `CloseNotFound = 4004`, `CloseBadMessage = 4400`, `CloseHCError = 4500`.

**Behavior:**
- Before `websocket.Accept`: parse `cols`, `rows` (integers 1..`maxCells = 1000`); invalid → `apierr.Write` 400 `bad_request` naming the parameter, expected range and actual value.
- After accept: `conn.SetReadLimit(maxInput)` with `maxInput = 1 << 20`. One reader goroutine sends `(type, bytes)` on channel `in` and closes `in` on read error. One ping goroutine: `time.NewTicker(pingInterval)`; each tick `conn.Ping` with a context bounded by `pingInterval` (fail-fast IO cap); error → cancel the session context.
- Session loop; `size` starts from the query and is updated by every valid resize message, also while offline:
  1. `c, err := core.Client()`. `ErrCoreOffline` → write text `{"type":"core","state":"offline"}` (once per outage), then `core.Await(ctx)` in a goroutine while the loop keeps draining `in` (binary → text `{"type":"error","op":"tab.input","tab":<id>,"kind":"core_offline","message":"input dropped: hc core offline"}`; text → validate, update `size`, or 4400). When online and an offline message was sent → write `{"type":"core","state":"online"}`.
  2. `t, err := c.Get(ctx, id)`: `*hc.RPCError` kind `not_found`, or `!hc.IsTab(t)` → close 4004 `tab <id> not found`; `ErrCoreGone` → step 1; other error → text error + close 4500 (reason = error text truncated to 123 bytes). `t.State == hc.StateFinished` → close 4000 with `{"reason":…,"exitCode":…,"signal":…}` from the record.
  3. `st, err := c.Attach(ctx, id, size)`: kind `finished` → `Get` again and close 4000 as in step 2; kind `not_found` → 4004; `ErrCoreGone` → step 1; other → 4500.
  4. Pump goroutine: `p, err := st.Recv(ctx)` → `conn.Write(ctx, websocket.MessageBinary, p)` → `st.Credit(len(p))`; its terminal error goes on a channel. Main select: `in` binary → `st.Write`; `in` text → decode `{"type":"resize","cols","rows"}` (`DisallowUnknownFields`, type must be `resize`, 1..1000) → `st.Resize`; invalid → text error + close 4400 `bad message: <detail>`; `in` closed (client gone) → `st.Close()`, return; pump `*hc.Exit` → close 4000 with its JSON; pump `ErrCoreGone` → step 1; session ctx done while the client is still connected (server shutdown via `BaseContext`) → `st.Close()`, close 1001 `server shutdown`.
- 4000 reason JSON keys are exactly `reason`, `exitCode`, `signal`.

**Constraints:** `github.com/coder/websocket` only; compression off; default Origin check (no `InsecureSkipVerify`, no `OriginPatterns`).

**Acceptance criteria:**
- [ ] `cd host && go test ./internal/bridge/ -count=1 -race -v` runs and passes `TestBridgeFirstPaintThenEcho`, `TestBridgeResizeReachesHC`, `TestBridgeExitClosesWith4000`, `TestBridgeFinishedTabCloses4000`, `TestBridgeUnknownTabCloses4004`, `TestBridgeForeignTerminalCloses4004`, `TestBridgeBadTextCloses4400`, `TestBridgeBadSizeIs400`, `TestBridgeHCAttachErrorCloses4500`, `TestBridgeCoreOfflineThenOnline`.

### Task 9: `cmd/roosterm` — `serve`, `passwd`, module tidy

**Files:**
- Create: `host/cmd/roosterm/main.go`, `host/cmd/roosterm/passwd.go`, `host/cmd/roosterm/serve.go`
- Modify: `host/go.mod`, `host/go.sum` (`go mod tidy` only)

**Interfaces:**
- Consumes: `config.{DefaultPath,Load,Save,SessionsPath,Config,DefaultListen,DefaultPingInterval,DefaultSessionTTL}`, `Config.SocketPath` (Task 1); `auth.{NewSalt,HashPassword,VerifyPassword,OpenSessions,NewLimiter}` (Task 2); `hc.Start` (Task 3); `swarm.Dir` (Task 4); `web.New`, `web.Options` (Task 6); `api.New`, `api.Options` (Task 7); `bridge.New` (Task 8).
- Produces: binary `roosterm`: `roosterm passwd [--config PATH]`, `roosterm serve [--config PATH]`. Exit codes: 0 ok, 1 runtime error, 2 usage error.
  - `func run(args []string, stdin *os.File, stdout, stderr io.Writer) int`

**Behavior:**
- `main` = `os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))`. Unknown/missing subcommand → usage on stderr, 2. `--config` via a per-subcommand `flag.FlagSet` (`ContinueOnError`); default `config.DefaultPath()`.
- `passwd`: config exists → `Load` (errors other than `fs.ErrNotExist` → 1); else new `Config{Listen: DefaultListen, PingInterval: DefaultPingInterval, SessionTTL: DefaultSessionTTL, Shell: os.Getenv("SHELL")}` (empty `SHELL` → `passwd: SHELL is empty; run this from your login shell`, 1). Prompts `username: `, `password: `, `repeat password: ` on stderr. Terminal stdin (`term.IsTerminal(int(stdin.Fd()))`) → `term.ReadPassword`; else read lines (scripted use). Empty username/password or mismatch → error, 1. `HashPassword(pw, salt)`, `Save`, print `wrote <path>`.
- `serve`: `Load`; `VerifyPassword(cfg.PasswordHash, "")` error → `roosterm serve: config <path>: password_hash: <err>`, 1. `hc.Start(ctx, cfg.SocketPath())` error → `roosterm serve: <err>` on stderr, 1 (criterion 6: the message contains the socket path). `OpenSessions(config.SessionsPath(path), cfg.SessionTTL, time.Now)`; `NewLimiter(time.Now)`; `home, err := os.UserHomeDir()`; `swarmDir := swarm.Dir(os.Getenv, os.Getuid())`. Handler `web.New(web.Options{Username, PasswordHash, SessionTTL, Sessions, Limiter, API: api.New(api.Options{Core: sup, Shell: cfg.Shell, Home: home, SwarmDir: swarmDir}), TabSocket: bridge.New(sup, cfg.PingInterval)})`. `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`; `http.Server{Addr: cfg.Listen, Handler: h, BaseContext: func(net.Listener) context.Context { return ctx }}`; listen error → 1; on ctx done → `srv.Shutdown` with a context bounded by `shutdownGrace = 5 * time.Second` (fail-fast cap), `sup.Close()`, exit 0. Print `listening on <addr>, hc <socket>` once listening.
- `time.Now` appears only here (composition root).

**Acceptance criteria:**
- [ ] After the tidy commit: `cd host && go mod tidy && git diff --exit-code go.mod go.sum; echo RC=$?` → `RC=0`.
- [ ] `python3 scripts/build.py; echo RC=$?` → `RC=0`, `build/roosterm` exists.
- [ ] Run-once (scratch, not committed): config with `hc_socket = "/tmp/roosterm-no-such/hc.sock"` → `build/roosterm serve --config <it>; echo RC=$?` → `RC=1`, stderr contains `/tmp/roosterm-no-such/hc.sock` and `hc server start`.
- [ ] Run-once: `printf 'me\npw\npw\n' | build/roosterm passwd --config scratch/t9/config.toml; echo RC=$?` → `RC=0`; `stat -f %Lp` (macOS) → `600`; `password_hash` starts with `$argon2id$v=19$`.
- [ ] `python3 scripts/test.py; echo RC=$?` → `RC=0`.

---

## Post-round-2 gate (contract criteria 3–6; a gate member runs it after Tasks 6–9 are signed off)

Tool: `websocat` (named by the contract). Not installed on either host: the gate member reads the latest version from `https://api.github.com/repos/vi/websocat/releases/latest`, installs the macOS-aarch64 and linux-aarch64 release binaries into `~/.local/bin` on each host, records the version, and checks its flags with `websocat --help` before use. The hc core must run with a clean login environment: start it from a normal terminal outside Claude Code (`hc server restart --fresh`) or via the Task 5 unit.

G1 macOS (every command with self-captured `RC=$?`, log in `docs/logs/`):
1. `printf 'me\npw\npw\n' | build/roosterm passwd --config scratch/live/config.toml`; edit `listen = "127.0.0.1:18787"`.
2. Run roosterm inside an hc terminal so it outlives the agent's foreground limit and has a known PID: `hc run --bg --name roosterm-live <abs path>/build/roosterm serve --config <abs path>/scratch/live/config.toml`; PID from `hc get roosterm-live` (`pid`).
3. Login: `curl -si -X POST -H 'Content-Type: application/json' -d '{"username":"me","password":"pw"}' http://127.0.0.1:18787/login` → `204`, `Set-Cookie: roosterm_session=…; Max-Age=…; HttpOnly; Secure; SameSite=Strict`. Save the token as `TOK`.
4. No cookie: `curl -si http://127.0.0.1:18787/api/tabs` → `401`; `curl -si -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' 'http://127.0.0.1:18787/ws/tabs/t1?cols=80&rows=24'` → `401`.
5. Create: `curl -si -X POST -H "Cookie: roosterm_session=$TOK" -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:18787/api/tabs` → `201`, save `id` as `ID`.
6. Attach: send `echo PID=$$` + newline over websocat binary mode with header `Cookie: roosterm_session=$TOK` to `ws://127.0.0.1:18787/ws/tabs/$ID?cols=80&rows=24`; output contains `PID=<n>`.
7. `kill -9 <roosterm pid>`; `hc rm roosterm-live` is not needed — start it again as in step 2 under a new name `roosterm-live2`; repeat step 6 → the same `PID=<n>`; `ps -p <n>` → alive.
G2 Linux: G1 inside OrbStack `roosterm-spike` with the binary from `python3 scripts/build.py --target linux/arm64` (hc 0.1.21 is installed there; socket `/run/user/501/herdr-core/default.sock`).
G3 swarm: create a test swarm server inside an hc terminal (`hc run --bg --name gate-swarm tmux -L claude-swarm-gate new-session -s s`); `GET /api/swarms` → contains `claude-swarm-gate` and only `claude-swarm-` names; create a tab with `{"command":["tmux","-L","claude-swarm-gate","a"]}`, attach via websocat → output shows the tmux status line `[s] 0:`; then `tmux -L claude-swarm-gate kill-server`. Never touch other `claude-swarm-*` servers.
G4: never `hc server stop` on a host with live programs. Re-run the Task 9 criterion-6 run-once with the gate's build and record it in the gate log.
