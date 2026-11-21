# Install the roosterm server

Unpack the package for your machine and run the installer as your normal user (not root). macOS's built-in `/usr/bin/python3` (3.9) is enough; nothing else is needed besides `curl` and `tar`.

```sh
tar xzf roosterm-server-<os>-<arch>.tar.gz
python3 roosterm-server-<os>-<arch>/install.py
```

The installer:

1. checks the machine (macOS or Linux, not root, a logged-in session for launchd/systemd, `XDG_CONFIG_HOME` unset or `~/.config`: the services always read `~/.config/roosterm/config.toml`);
2. installs `roosterm` into `~/.local/bin` (the copy runs once to prove it fits this OS/CPU before it replaces an older one);
3. installs herdr-core (`hc`) 0.1.21 with herdr's official script `https://releases.herdr.dev/core/install.sh` when `~/.local/bin/hc` is missing or older; a newer hc is kept;
4. asks for the login shell, listen address (default `127.0.0.1:8787`), username and password, and writes `~/.config/roosterm/config.toml` (mode 0600) through `roosterm passwd`;
5. installs and starts the service: launchd job `dev.roosterm.serve` on macOS, systemd user units `herdr-core.service` + `roosterm.service` on Linux (offers `loginctl enable-linger` so it starts at boot);
6. prints the URL and next steps.

**Run it again to upgrade or reconfigure.** The binary is replaced; the existing config and `sessions.json` are kept unless you choose to change the listen address, shell or password (a new password logs every device out after the restart).

The default listen address is loopback only. To reach roosterm from other machines, put an HTTPS reverse proxy in front: roosterm itself speaks plain HTTP.

## Service control

```sh
python3 install.py status      # service state and whether the listen port answers
python3 install.py restart     # also: start, stop
python3 install.py uninstall   # removes only the service; keeps hc, roosterm, config and sessions
```

Both services run `hc server start` (idempotent) before `roosterm serve`. **Never run `hc server stop` on a live host: it ends every program in every tab.** Neither the installer nor the services stop hc.

- macOS: the job starts your login shell (`<shell> -lc`) so hc inherits your login environment. launchd restarts roosterm after a crash, not after a clean exit. `stop` unloads the job; it loads again at the next login or on `start`. Log: `~/Library/Logs/roosterm.log`.
- Linux: log `journalctl --user -u roosterm.service`. Stopping `herdr-core.service` does not stop hc (the unit has no `ExecStop` and uses `KillMode=process`).

Only one roosterm may own the listen port: stop any hand-started `roosterm serve` first.

## Options

- `--binary PATH`: install this roosterm binary instead of the one next to `install.py` (for example `python3 deploy/install.py --binary build/roosterm` from a source checkout).
- `--hc-installer PATH`: run a local copy of hc's install script instead of downloading it.
- `--no-load`: write the service files but do not load or start them.
