# hc bridge spec (herdr-core 0.1.21, protocol generation 1)

Source of truth for `host/internal/hc`. Every item below was observed on a real hc 0.1.21 (spike log `docs/logs/2026-11-19-spike-hc.md`) unless marked *schema-only* (from `schema/hc.openrpc.json` / `schema/README.md` at herdr-core commit c751780).

## 1. Endpoint

| OS | Default socket |
|---|---|
| macOS | `$HOME/.local/state/herdr-core/default/default.sock` |
| Linux | `/run/user/<uid>/herdr-core/default.sock` (if `$XDG_RUNTIME_DIR` set: `$XDG_RUNTIME_DIR/herdr-core/default.sock`) |

Both rows come from one OS-independent rule (hc `docs/security.md` "State and endpoints"): `$XDG_RUNTIME_DIR/herdr-core/default.sock` if `XDG_RUNTIME_DIR` is set; else `/run/user/<uid>/herdr-core/default.sock` if `/run/user/<uid>` exists; else `$XDG_STATE_HOME/herdr-core/default/default.sock` (`XDG_STATE_HOME` defaults to `~/.local/state`). Roosterm applies this rule only when `hc_socket` in `config.toml` is empty; a set value wins. `HC_SOCKET` is not read by roosterm.

Dial results:
| errno | Meaning |
|---|---|
| success | core up |
| `ENOENT` | no core (clean `hc server stop` removes the file) |
| `ECONNREFUSED` | stale file after a core crash; core not (yet) listening |

## 2. Connection modes

One message per line in **plain mode**: JSON-RPC 2.0, `\n`-terminated. The first call on every roosterm connection is:

```
→ {"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true,"generation":1}}\n
← {"id":0,"jsonrpc":"2.0","result":{"core":{"id":"…","name":"…"},"features":["framed","attach","events","status","shell","resize_notifications","automatic_input","restart_environment"],"framed":true,"generation":1,"methods":[…],"version":"0.1.21",…}}\n
```
The hello reply is still a plain line. Everything after it is **framed**.

Required checks on the hello reply (fail loudly with expected/actual): `generation == 1`, `framed == true`, `features` contains `attach` and `events`.

## 3. Frames

`[channel u32 LE][kind u8][len u32 LE][payload]`, payload ≤ 16 MiB.

| kind | name | direction used by roosterm | payload |
|---|---|---|---|
| 0 | DATA | both | channel 0: one JSON-RPC message (no trailing newline). Other channels: stream bytes |
| 1 | CREDIT | roosterm → core | u32 LE: bytes consumed on that channel |
| 2 | CLOSE | both | roosterm → core: empty (detach). core → roosterm: JSON reason, or empty as the reply to a detach |
| 3 | RESIZE | roosterm → core | u16 LE cols, u16 LE rows |
| 4 | FOCUS | not used | |
| 5 | FACTS | not used | |
| 6 | AUTOMATIC_INPUT | not used (roosterm forwards all client bytes as DATA) | |

Channel 0 multiplexes concurrent calls; match replies by `id`.

## 4. Methods used

Errors are `{"code":-32000,"message":…,"data":{"kind":…,"hint":…}}`. Observed kinds: `not_found` (`no terminal "t999"`, hint "see `hc ls --all`"), `finished` (`t5 has finished`, hint "read what it left: hc read t5"). *schema-only*: `unavailable` (lost/keep recovery, spawn pending), `invalid_params`.

### terminal.run → new tab
```
params: {"argv":["/bin/zsh","-l"],"cwd":"/Users/me","size":{"cols":120,"rows":40},
         "labels":{"roosterm":"tab","roosterm.name":"shell"},"no_parent":true}
result: Terminal record (below); "address":"t7", "pid":12345, "state":"live"
```
- roosterm wraps `argv` with `ScrubArgv` (`host/internal/api/scrub.go`, see §7): `/bin/sh -c '<unset vars>; exec "$@"' sh <argv…>`, so the example's argv is the inner command.
- `argv` empty and `line` absent → the machine's `$SHELL` without `-l`. Roosterm always sends an explicit `argv`.
- `line` (string) runs via `sh -c`; roosterm uses `argv` only.
- `cwd` default is the **core's** cwd → roosterm always sends the folder the client chose under home (home when none; `resolveDir`, `host/internal/api/dirs.go`).
- `size` default 200×120 until a viewer attaches.
- `no_parent: true` always (roosterm is not inside an hc terminal; avoids parent notes).
- `name` must match `[a-z0-9_.-]{1,32}` and be unique among live terminals; roosterm does not use `name` (tab titles go in a label).

### Terminal record (fields roosterm reads)
```
{"address":"t1","argv":["/bin/zsh","-l"],"cols":100,"rows":30,"created_ms":1795045332019,
 "cwd":"…","labels":{…},"pid":37057,"state":"live"|"finished","reason":null|"exited"|"killed"|"server_stop"|…,
 "exit_code":null|int,"signal":null|int,"finished_ms":null|int,"title":"","foreground":{…}|absent}
```
`pid` stays set after finish (observed `pid: 37554` on a finished claude).

### terminal.list → GET tabs
```
params: {"all":true,"labels":{"roosterm":"tab"}}
result: {"terminals":[Terminal…],"hidden_finished":0,"machines":[]}
```
`all:true` is required: without it finished terminals are omitted (`hidden_finished` counts them). Finished records are kept 24 h, then expire.

### terminal.label → rename
```
params: {"terminal":"t5","labels":{"roosterm.name":"hello world"}}
result: Terminal record with merged labels
```
Merge semantics: given keys are set; a key with value `""` is removed. Keys must be non-empty and contain no `=`; values may contain spaces.

### terminal.kill → close a live tab
```
params: {"terminal":"t6"}
result: Terminal record, "state":"finished","reason":"killed","signal":1 (SIGHUP)
```
Killing a finished terminal: *not tested*; roosterm checks `state` first.

### terminal.rm → forget a finished tab
```
params: {"terminal":"t5"}  →  result: {"removed":"t5"}
```
Roosterm's `DELETE /api/tabs/{id}`: `terminal.get`; if `live` → `terminal.kill`; then `terminal.rm`.

### terminal.get
```
params: {"terminal":"t5"}  →  Terminal record
```

### terminal.attach → WebSocket stream
```
params: {"terminal":"t1","cols":100,"rows":30,"history":5000}
result: {"channel":2,"automatic_input":true,"resize_notifications":false,"terminal":{Terminal…}}
```
- Attaching makes this viewer active and sets the terminal size to `cols×rows` (observed `size` event).
- On a finished terminal: error kind `finished`.
- Several attaches on one connection get distinct channels; the last active viewer sets the size.

Stream on the channel (core → roosterm), in order:
1. **First paint**: one DATA frame starting with `\x18\x1b[!p` (CAN + soft reset) followed by mode resets, up to `history` lines of main-screen scrollback (default 0: screen only; observed: a Claude Code tab's whole transcript with `history:5000`, only the last screen without it), and the full current screen. Roosterm asks for 5000 (the web terminal's scrollback) and prefixes the paint with `\x1b[3J` so a reconnecting browser terminal does not stack the replayed scrollback twice. After a core restart it still contains the output written before the crash.
2. Live DATA frames: raw program output.
3. CLOSE with JSON reason when the program ends or is killed:
   `{"exit_code":7,"finished_ms":1795045494302,"reason":"exited","signal":null}` / `{"exit_code":null,…,"reason":"killed","signal":1}`.
4. If the core process dies: the Unix connection reaches EOF (no CLOSE).

Roosterm → core on the channel:
- Keyboard bytes: DATA, verbatim from the client (`exit 7\r` was typed and executed).
- Resize: RESIZE `<HH` cols, rows (observed terminal record changing to 132×40).
- CREDIT: after writing each DATA payload to the WebSocket, send CREDIT with its length. Initial credit is 256 KiB; a viewer > 4 MiB behind gets a fresh repaint (*schema-only*).
- Detach: CLOSE with empty payload; the core answers with an empty CLOSE.

### terminal.send — NOT used for keystrokes
`{"terminal":"t5","text":"echo SENT-VIA-RPC"}` pastes the text and presses Enter (`result: {"started":null,"status":null,"turn":0}`). It is for agents, not for a keyboard. Client keyboard binary frames go as DATA on the attach channel instead.

### events.subscribe — not needed by the bridge
Framed reply `{"channel":1,"seq":15,"terminals":[…]}`; then each notification is one DATA frame on that channel containing a JSON-RPC notification (`{"jsonrpc":"2.0","method":"event","params":{"seq":…,"type":"size","terminal":"t5","data":{"cols":90,"rows":20}}}`; types seen: created, labels, size, foreground, finished, removed). Exit is already delivered by the attach CLOSE frame, so roosterm does not subscribe.

## 5. Lifetimes

| Event | Effect on programs | What roosterm sees |
|---|---|---|
| roosterm exits / WebSocket closes | none (viewer gone) | — |
| core SIGKILL | none; holder restarts the core and re-creates the socket file | EOF on every hc connection; socket file event; dial succeeds again |
| `hc server restart` | none | same as above (*not separately tested*) |
| `hc server stop` | **all non-kept programs end** (`reason: server_stop`), holder exits | EOF; socket file removed |
| program exits | its terminal finishes | CLOSE with reason on each attach channel |

## 6. Reconnect via socket file watch

Watch the socket's parent directory (fsnotify). While offline, every event whose `Name` equals the socket path triggers one dial attempt; on success → online, re-attach all open WebSockets (each gets a fresh first paint). Observed events:
- Linux (inotify): `REMOVE default.sock` at crash, `CREATE default.sock` when the new core binds.
- macOS (kqueue): `CREATE default.sock` when the new core binds, plus spurious repeated `CREATE` for existing socket files on any directory change — harmless because a dial while online is not attempted and a failed dial just stays offline.

ponytail: one dial per event; if the dial lands between the new core's bind and listen, roosterm stays offline until the next event on that name (the window is the core's bind→listen gap). Upgrade path: also dial on events for `core.json` in the same state dir (macOS only, where state and socket share a directory).

## 7. Environment

The core passes its own start environment to every program. Start it from launchd/systemd (see `deploy/`), never from an agent shell: both the core and the terminal holder can carry a dirty start env (a core started inside Claude Code leaked `TMUX`, `TMUX_PANE` and `CLAUDE*` into `claude`). roosterm therefore wraps every tab's argv in `/bin/sh -c 'unset TMUX TMUX_PANE CLAUDE*…; exec "$@"'` (`api.ScrubArgv`), keeping `HC_*`, `TERM*` and `TMUX_TMPDIR`. `hc server restart --fresh` from a clean shell repairs a dirty core.
