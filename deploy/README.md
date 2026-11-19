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
