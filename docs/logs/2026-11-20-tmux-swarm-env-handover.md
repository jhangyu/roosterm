# Handover — sessions started through roosterm/herdr cannot create a tmux team swarm (2026-11-20)

Written by the ceyx lead session f05efab5 (Claude Code, teammateMode "tmux"). Task for the next session: find and fix the root cause so a Claude session launched through roosterm/herdr can spawn tmux teammates without manual repair.

## Symptom
- In session f05efab5 (started from roosterm), every teammate spawn (Agent tool with name + team_name, teammateMode "tmux" in ~/.claude/settings.json:370) failed with `Could not determine current tmux pane/window`.
- The session's environment had `TMUX=/private/tmp/tmux-501/claude-swarm-28482,91353,0` and `TMUX_PANE=%15`, but no tmux server existed on that socket (`error connecting to /private/tmp/tmux-501/claude-swarm-28482`). Pid 28482 was an earlier lead session; that swarm was gone.

## Evidence (collected 2026-11-20 ~18:00 +08)
- Process chain of the session: `claude` (13510) <- `/bin/zsh -l` (13467) <- `herdr-core hold --socket ~/.local/state/herdr-core/default/default.holder.c39cdc78.sock` (13640) <- launchd. The session was NOT inside tmux at all.
- The herdr-core holder (13640, started Thu 2026-11-19 08:37:07) itself carries in its environment: `TMUX=/private/tmp/tmux-501/claude-swarm-28482,91353,0`, `TMUX_PANE=%15`, plus `CLAUDECODE=1`, `CLAUDE_CODE_CHILD_SESSION=1`, `CLAUDE_CODE_MESSAGING_SOCKET=/tmp/cc-socks/11959.sock`, `CLAUDE_CODE_MESSAGING_TOKEN=...`, `CLAUDE_CODE_SESSION_ATTENDED=0`, `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1`, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` (command: `ps eww -o command= -p 13640 | tr ' ' '\n' | grep -E '^TMUX|^CLAUDE'`).
- Leading hypothesis (unverified): the herdr-core holder was launched from inside a Claude Code process that ran in tmux pane %15 of swarm 28482 (a teammate or a Bash tool call), so it captured that process's TMUX/TMUX_PANE and CLAUDE_CODE_* variables, and every shell it spawns passes them on. Claude Code then believes it is inside tmux pane %15 of a dead server. The inherited CLAUDE_CODE_MESSAGING_* / CHILD_SESSION variables may also mis-wire messaging of launched sessions — check.

## Workarounds applied in this session (reverse or keep as the fix decides)
1. ~/.claude/hooks/global-bash-safety: main-session exemption so `tmux -S .../claude-swarm-*` / `tmux -L claude-swarm-*` may create detached sessions/windows (backup: ~/.claude/backups/global-bash-safety.bak-20261120; existing test_global_bash_safety.py passes).
2. Recreated a tmux server on the dead socket with 16 windows so pane %15 exists: /Users/jhangyu/project/ceyx/scratch/maliopt/hookcheck/swarm_tmux.py (session `swarm-lead`; members spawn as splits of window 15). swarm_select.py made window 15 active.
3. roosterm: swarm attach now targets the window holding the most claude panes (uncommitted: host/internal/swarm/swarm.go, swarm_test.go, host/internal/api/tabs.go, api_test.go; tests 54 pass; server restarted via scripts/service.py restart). This fix is independent of the env problem and stays useful.

## Suggested checks for the next session
- How herdr-core is started (launchd plist? started by roosterm host? by a tool call?) and whether it scrubs the environment of the shells it spawns (host/internal/hc/*, host/cmd/roosterm/main.go, docs/hc-bridge.md, docs/research/herder.md).
- Fix candidates: start herdr-core with a clean environment (unset TMUX, TMUX_PANE and CLAUDE_CODE_*/CLAUDECODE), or have the spawned shell unset them; then restart herdr-core (this kills sessions it holds — ask the user before restarting).
- Verify live: launch a fresh session through roosterm, check `env | grep -E '^TMUX|^CLAUDE'` is clean, spawn one teammate, confirm it appears in a new claude-swarm-<pid> socket.
- Do not touch swarm sockets claude-swarm-28482 (ceyx members may be running) or claude-swarm-58189 (another session) without asking.
