# Deploy roosterm

roosterm needs a running herdr-core (`hc`). Both units below run `hc server start` (idempotent) before `roosterm serve`. Install `hc` and `roosterm` into `~/.local/bin` first, then run `roosterm passwd` once from your login shell.

**Never run `hc server stop` on a live host: it ends every program in every tab.** Neither unit stops hc.

## macOS (launchd)

```sh
python3 scripts/service.py install     # render the plist into ~/Library/LaunchAgents and load it (idempotent)
python3 scripts/service.py status      # loaded, pid, last exit code, whether the listen port answers
python3 scripts/service.py restart     # also: start, stop, uninstall
```

`roosterm serve` takes its listen address from `~/.config/roosterm/config.toml`. The plist starts a login shell (`/bin/zsh -lc`) so hc inherits your login environment. If your login shell is not zsh, replace `/bin/zsh` in `deploy/launchd/dev.roosterm.serve.plist` and run `install` again. launchd restarts roosterm after a crash, not after a clean exit. Log: `~/Library/Logs/roosterm.log`.

`stop` unloads the job (hc keeps running); it loads again at the next login or on `start`. Only one roosterm may own the listen port: stop any hand-started `roosterm serve` before `install` or `start`.

## Linux (systemd user units)

```sh
mkdir -p ~/.config/systemd/user
cp deploy/systemd/herdr-core.service deploy/systemd/roosterm.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now herdr-core.service roosterm.service
loginctl enable-linger "$USER"   # start at boot without a login session
```

Log: `journalctl --user -u roosterm.service`. Stop roosterm (hc keeps running): `systemctl --user stop roosterm.service`. Stopping `herdr-core.service` does not stop hc (the unit has no `ExecStop`).
