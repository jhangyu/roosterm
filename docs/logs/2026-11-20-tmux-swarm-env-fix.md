# tmux swarm env leak fix and swarm socket move (2026-11-20)

Follows `2026-11-20-tmux-swarm-env-handover.md`. Nothing is committed yet.

## Root cause
- An `hc` command run from the Bash tool of Claude session ff67dc11 (pid 11959, inside tmux pane %15 of swarm 28482) first started the herdr-core core and holder. hc keeps the start env across `hc server restart`, so both still hand `TMUX`, `TMUX_PANE` and the whole `CLAUDE*` set of that session to every new program.
- Decisive check: `hc exec /bin/sh -c 'echo PPID=$PPID; env'` → `PPID=54059` with `TMUX=/private/tmp/tmux-501/claude-swarm-28482,91353,0`, `CLAUDE_PID=11959`. `ps eww` of the core shows a clean env, because it shows only the env present at exec.
- Claude Code reads `TMUX` once at startup. If it is set, Claude uses `tmux -S <$TMUX socket>` and `TMUX_PANE` without checking them. The server was gone, so spawning failed with `Could not determine current tmux pane/window`.
- The launchd job's `hc server start` does nothing when a core already runs, so it never replaced the dirty core.

## Socket mechanism
- Outside tmux, Claude Code runs `tmux -L claude-swarm-<pid>`. tmux puts the socket in `${TMUX_TMPDIR:-/tmp}/tmux-<uid>/`. The Claude binary has no `TMUX_TMPDIR` reference, so the env var is the only knob.
- If `TMUX_TMPDIR` names a missing directory, tmux 3.6b silently falls back to `/private/tmp`. The chosen dir is `$HOME/.local/state`, which already exists. Sockets go to `~/.local/state/tmux-501/`, which is about 55 bytes, under the 104-byte `sun_path` limit.

## Done (uncommitted)
- roosterm: `host/internal/api/scrub.go` `ScrubArgv` wraps every tab it creates, shell and swarm attach alike, in `/bin/sh -c 'unset TMUX TMUX_PANE $(CLAUDE* names); exec "$@"'`. `HC_*`, `TERM*` and `TMUX_TMPDIR` are kept. `program()` skips the wrapper (`api.go`). Swarm attach uses `tmux -S <dir>/<name> a` (`swarm.go`). `docs/hc-bridge.md` §7 is updated. Deployed to `~/.local/bin/roosterm` and the service restarted.
- podium (source `~/project/podium`, plus installed copies in plugins cache 1.1.1 and marketplaces): `team_shutdown.py:30` SOCKET_DIR follows `TMUX_TMPDIR`; `protocols/shutdown.md` wording updated.
- `~/.claude/hooks/session-relay`: strips exact `TMUX`/`TMUX_PANE` instead of the `TMUX` prefix, so `TMUX_TMPDIR` survives.
- Backups: `~/.claude/backups/*-20261120`.

## Evidence (code state `git diff HEAD -- host docs | shasum` = 0b697b0e5a2d)
- `(cd host && go test ./...)` RC=0. New tests cover the scrub wrapper and the `-S` attach argv.
- `grep -acF 'unset TMUX TMUX_PANE' ~/.local/bin/roosterm` RC=0, so the deployed binary has the change.
- Live new tab through roosterm, running `env`: no forbidden names; `HC_TERMINAL_ID` non-empty; program names reported as `env` and `zsh`.
- podium: `TMUX_TMPDIR=/some/dir` gives `/some/dir/tmux-501`; unset gives `/tmp/tmux-501`. Checked in all 3 trees, RC=0.
- session-relay `launch()` with stubbed collaborators: TMUX, TMUX_PANE and CLAUDE_FOO stripped; TMUX_TMPDIR kept. RC=0.
- No herdr-core, tab or tmux server was killed (pgrep before and after match).

## Remaining steps (user)
1. Wait until the live swarms 15556, 58189, 63400 and the hand-recreated 28482 have ended. Otherwise their shutdown would look in the new directory while their sockets stay in /tmp.
2. Add to `~/.zshenv`: `export TMUX_TMPDIR="$HOME/.local/state"`.
3. In a new Terminal.app window (not in Herdr, tmux or Claude), run `hc server restart --fresh`. hc docs say running programs keep running.
4. Restart roosterm: `python3 scripts/service.py restart`.
5. Verify:
   - `hc exec /bin/sh -c env | grep -cE '^(TMUX=|TMUX_PANE=|CLAUDE)'` → `0`, and `TMUX_TMPDIR` is present.
   - Spawn one teammate from a new roosterm tab → `~/.local/state/tmux-501/claude-swarm-<pid>` appears.
   - If the first check fails, only a holder replacement (`hc server stop` + `start`) fixes it. That ends every tab.
6. Delete only the `claude-swarm-*` sockets in `/private/tmp/tmux-501` that refuse a connection.
7. Undo the handover workarounds once ceyx is done: restore `~/.claude/hooks/global-bash-safety` from `~/.claude/backups/global-bash-safety.bak-20261120`, and stop the hand-made server on `claude-swarm-28482`.
8. Reopen old tabs and Claude sessions; they still carry the dirty env.
9. Commit and push `~/project/podium`, or the next plugin update overwrites the installed copies. That repo also has an unrelated uncommitted change in `skills/test-driven-development/SKILL.md`.

## Parking lot
- `~/.claude/settings.json` maps the haiku tier to `claude-haiku-5-5`; `model-dispatch.md` §0 says the haiku tier runs Sonnet 5.5.
- Claude messaging sockets in `/tmp/cc-socks/` also live in /tmp.
- A Claude launched outside zsh (GUI or IDE) does not read `.zshenv`. The fallback would be `launchctl setenv`.
- Optional, declined: a global-bash-safety deny for `hc server start` from Claude.
