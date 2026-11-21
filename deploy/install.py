#!/usr/bin/env python3
"""Install, upgrade or reconfigure the roosterm server for this user (macOS or Linux).

python3 install.py              install or upgrade: roosterm binary, hc, login, config, service
python3 install.py status       service state and whether the listen port answers
python3 install.py start|stop|restart
python3 install.py uninstall    remove the service only (keeps hc, roosterm, config and sessions)

Re-running is the upgrade path: the binary is replaced, config and sessions are kept unless you
choose to change them. Never stops the hc core: `hc server stop` would end every program in every tab.
Needs Python 3.9+ and nothing outside the standard library.
"""
from __future__ import annotations

import argparse
import getpass
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import NoReturn
from xml.sax.saxutils import escape

HERE = Path(__file__).resolve().parent
HOME = Path.home()
BIN_DIR = HOME / ".local" / "bin"
ROOSTERM = BIN_DIR / "roosterm"
HC = BIN_DIR / "hc"
HC_CORE = BIN_DIR / "herdr-core"
CONFIG_HOME = HOME / ".config"  # the services run without XDG_CONFIG_HOME; preflight refuses a different one
CONFIG = CONFIG_HOME / "roosterm" / "config.toml"

HC_MIN_VERSION = (0, 1, 21)  # installs exactly this when hc is missing or older; keeps anything newer
HC_INSTALL_URL = "https://releases.herdr.dev/core/install.sh"
DEFAULT_LISTEN = "127.0.0.1:8787"  # host/internal/config/config.go DefaultListen
USAGE_EXIT = 2  # roosterm with no arguments prints usage and exits 2 (host/cmd/roosterm/main.go)
CONNECT_TIMEOUT_S = 2.0  # fail-fast cap on the port probe
LOOPBACK = ("127.0.0.1", "localhost", "::1")

LABEL = "dev.roosterm.serve"
PLIST_TEMPLATE = HERE / "launchd" / f"{LABEL}.plist"
PLIST = HOME / "Library" / "LaunchAgents" / f"{LABEL}.plist"
MAC_LOG = HOME / "Library" / "Logs" / "roosterm.log"
UNIT_TEMPLATES = [HERE / "systemd" / "herdr-core.service", HERE / "systemd" / "roosterm.service"]
UNIT_DIR = CONFIG_HOME / "systemd" / "user"
UNIT = "roosterm.service"
MAC = sys.platform == "darwin"


def die(msg: str) -> NoReturn:
    print(f"FAIL: {msg}", file=sys.stderr)
    sys.exit(1)


def run(argv: list, **kw) -> subprocess.CompletedProcess:
    return subprocess.run([str(a) for a in argv], capture_output=True, text=True, **kw)


def run_ok(argv: list, **kw) -> str:
    r = run(argv, **kw)
    if r.returncode != 0:
        die(f"{' '.join(str(a) for a in argv)}: expected exit 0, got {r.returncode}: {(r.stderr or r.stdout).strip()}")
    return r.stdout


# ---- prompts: getpass only on a terminal, plain lines otherwise (scripted input never hangs on /dev/tty)

def read_line(prompt: str, secret: bool = False) -> str:
    if secret and sys.stdin.isatty():
        return getpass.getpass(prompt)
    print(prompt, end="", flush=True)
    line = sys.stdin.readline()
    if not line:
        die(f"{prompt.strip()} expected an answer, got end of input")
    return line.rstrip("\r\n")


def ask(prompt: str, default: str = "") -> str:
    answer = read_line(f"{prompt} [{default}]: " if default else f"{prompt}: ").strip()
    return answer or default


def yes(prompt: str, default: bool) -> bool:
    answer = read_line(f"{prompt} [{'Y/n' if default else 'y/N'}]: ").strip().lower()
    if answer == "":
        return default
    if answer in ("y", "yes", "n", "no"):
        return answer.startswith("y")
    die(f"{prompt}: expected y or n, got {answer!r}")


def ask_shell(default: str) -> str:
    shell = ask("Login shell for terminals", default)
    p = Path(shell)
    # mirrors host/internal/config/config.go checkShell
    if not p.is_absolute() or not p.is_file() or not os.access(p, os.X_OK):
        die(f"shell: expected an absolute path to an executable file, got {shell!r}")
    return shell


def split_listen(listen: str) -> tuple:
    host, sep, port = listen.rpartition(":")
    if not sep or not port.isdigit() or not 1 <= int(port) <= 65535:
        die(f"listen: expected host:port with port 1-65535 (like {DEFAULT_LISTEN}), got {listen!r}")
    return host.strip("[]"), int(port)


def ask_listen(default: str) -> str:
    listen = ask("Listen address (host:port)", default)
    host, _ = split_listen(listen)
    if host not in LOOPBACK:
        print(f"warning: {listen} is reachable from other machines over plain HTTP; put an HTTPS reverse proxy in front")
    return listen


# ---- config.toml: written by roosterm's TOML encoder as one `key = "value"` line per key

KEY_LINE = r'^{key} = "(?:[^"\\]|\\.)*"$'


def config_get(text: str, key: str) -> str:
    found = re.findall(KEY_LINE.format(key=re.escape(key)), text, re.M)
    if len(found) != 1:
        die(f"{CONFIG}: expected exactly one `{key} = \"...\"` line, got {len(found)}")
    return json.loads(found[0].split(" = ", 1)[1])


def config_set(values: dict) -> None:
    text = CONFIG.read_text()
    for key, value in values.items():
        config_get(text, key)  # exactly one line, or fail
        line = f"{key} = {json.dumps(value)}"  # ASCII-escaped basic string: valid TOML
        text = re.sub(KEY_LINE.format(key=re.escape(key)), lambda _m: line, text, flags=re.M)
    fd, tmp = tempfile.mkstemp(dir=CONFIG.parent, prefix=".config.toml.")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(text)
        os.chmod(tmp, 0o600)
        os.replace(tmp, CONFIG)
    except BaseException:
        os.unlink(tmp)
        raise


def set_password(shell: str) -> None:
    print("Login for the browser and the app.")
    user = read_line("username: ").strip()
    pw = read_line("password: ", secret=True)
    again = read_line("repeat password: ", secret=True)
    # roosterm passwd reads the three answers as lines from a pipe; the password never goes on argv.
    r = run([ROOSTERM, "passwd", "--config", CONFIG], input=f"{user}\n{pw}\n{again}\n",
            env=dict(os.environ, SHELL=shell))
    if r.returncode != 0:
        die(f"roosterm passwd: expected exit 0, got {r.returncode}: {r.stderr.strip()}")


# ---- install steps

def preflight(binary: Path, load: bool) -> None:
    if sys.version_info < (3, 9):
        die(f"expected Python 3.9 or newer, got {sys.version.split()[0]}")
    if sys.platform not in ("darwin", "linux"):
        die(f"expected macOS or Linux, got {sys.platform}")
    if os.getuid() == 0:
        die("expected a normal user, got root: roosterm installs into your home directory and runs as you")
    if not binary.is_file():
        die(f"expected the roosterm binary at {binary}, got nothing (pass --binary PATH)")
    templates = [PLIST_TEMPLATE] if MAC else UNIT_TEMPLATES
    for t in templates:
        if not t.is_file():
            die(f"expected the service template {t}, got nothing (run install.py from the unpacked package)")
    if load:
        if MAC:
            r = run(["launchctl", "print", f"gui/{os.getuid()}"])
            if r.returncode != 0:
                die(f"launchctl print gui/{os.getuid()}: expected exit 0, got {r.returncode}: run the installer from a logged-in desktop session")
        else:
            r = run(["systemctl", "--user", "show-environment"])
            if r.returncode != 0:
                die(f"systemctl --user show-environment: expected exit 0, got {r.returncode}: {r.stderr.strip()} (log in as this user, not via su/sudo)")
    if str(BIN_DIR) not in os.environ.get("PATH", "").split(os.pathsep):
        print(f"warning: {BIN_DIR} is not on your PATH; the service does not need it, your shell does to run `roosterm`/`hc`")
    xdg = os.environ.get("XDG_CONFIG_HOME")
    if xdg and Path(xdg).resolve() != CONFIG_HOME.resolve():
        die(f"XDG_CONFIG_HOME: expected unset or {CONFIG_HOME}, got {xdg}: the services read {CONFIG} "
            "and the systemd user manager looks for units under ~/.config; unset it for the install")


def install_binary(binary: Path) -> None:
    BIN_DIR.mkdir(parents=True, exist_ok=True)
    new = ROOSTERM.with_name("roosterm.new")
    shutil.copyfile(binary, new)
    new.chmod(0o755)
    if MAC:
        # macOS quarantines a copy of a quarantined file (browser download -> tar -> this copy) and kills
        # the ad-hoc signed copy on exec. Running install.py is the user's consent to run this binary.
        if "com.apple.quarantine" in run_ok(["xattr", new]).split():
            run_ok(["xattr", "-d", "com.apple.quarantine", new])
    try:
        r = run([new])
    except OSError as e:
        new.unlink()
        die(f"{binary}: expected a roosterm binary for this machine, got {e} (wrong package for this OS/CPU?)")
    if r.returncode != USAGE_EXIT or "usage:" not in r.stderr:
        new.unlink()
        die(f"{binary}: expected exit {USAGE_EXIT} with usage on stderr, got exit {r.returncode}: {(r.stderr or r.stdout).strip()[:200]}")
    os.replace(new, ROOSTERM)
    print(f"installed {ROOSTERM}")


def hc_version():
    """Installed hc version tuple, or None when ~/.local/bin/hc is missing."""
    if not HC.exists():
        return None
    r = run([HC, "--version"])
    m = re.match(r"hc (\d+)\.(\d+)\.(\d+)", r.stdout.strip())
    if r.returncode != 0 or not m:
        die(f"{HC} --version: expected `hc X.Y.Z` and exit 0, got exit {r.returncode}: {(r.stdout or r.stderr).strip()}")
    return tuple(int(x) for x in m.groups())


def dotted(v: tuple) -> str:
    return ".".join(str(x) for x in v)


def install_hc(installer) -> None:
    have = hc_version()
    if have is not None and have >= HC_MIN_VERSION:
        print(f"hc {dotted(have)} already installed (needs >= {dotted(HC_MIN_VERSION)}), kept")
        return
    env = dict(os.environ, HC_VERSION=dotted(HC_MIN_VERSION), HC_INSTALL_DIR=str(BIN_DIR))
    print(f"installing hc {dotted(HC_MIN_VERSION)} with herdr's installer ({installer or HC_INSTALL_URL})")
    with tempfile.TemporaryDirectory() as tmp:
        if installer is None:
            for tool in ("curl", "tar"):
                if shutil.which(tool) is None:
                    die(f"installing hc: expected `{tool}` on PATH, got nothing; install it and run again")
            installer = Path(tmp) / "install.sh"
            run_ok(["curl", "-fsSL", "-o", installer, HC_INSTALL_URL])
        r = subprocess.run(["sh", str(installer)], env=env, stdin=subprocess.DEVNULL)
    if r.returncode != 0:
        die(f"hc installer {installer}: expected exit 0, got {r.returncode} (output above)")
    # herdr's installer skips the hc link when another hc is on PATH; the service needs ~/.local/bin/hc.
    if not HC.exists() and not HC.is_symlink() and HC_CORE.exists():
        HC.symlink_to("herdr-core")
    have = hc_version()
    if have is None or have < HC_MIN_VERSION:
        die(f"{HC}: expected hc >= {dotted(HC_MIN_VERSION)} after the hc installer, got {dotted(have) if have else 'nothing'}")


def configure() -> str:
    """Create or update config.toml; returns the login shell for the service."""
    if not CONFIG.is_file():
        print(f"new config {CONFIG}")
        shell = ask_shell(os.environ.get("SHELL", ""))
        listen = ask_listen(DEFAULT_LISTEN)
        set_password(shell)  # roosterm passwd writes defaults, this shell and the login
        if listen != DEFAULT_LISTEN:
            config_set({"listen": listen})
        return shell
    text = CONFIG.read_text()
    cur = {k: config_get(text, k) for k in ("listen", "shell", "username")}
    print(f"existing config {CONFIG}: listen={cur['listen']} shell={cur['shell']} username={cur['username']}")
    changes = {}
    if not yes("Keep listen address and shell?", True):
        listen, shell = ask_listen(cur["listen"]), ask_shell(cur["shell"])
        changes = {k: v for k, v in (("listen", listen), ("shell", shell)) if v != cur[k]}
        if changes:
            config_set(changes)
    if yes("Change username/password? (every device is logged out after the restart)", False):
        set_password(changes.get("shell", cur["shell"]))
    return changes.get("shell", cur["shell"])


def write_service(shell: str) -> None:
    if MAC:
        PLIST.parent.mkdir(parents=True, exist_ok=True)
        MAC_LOG.parent.mkdir(parents=True, exist_ok=True)
        text = PLIST_TEMPLATE.read_text()
        for placeholder in ("/Users/USERNAME", "<string>/bin/zsh</string>"):
            if placeholder not in text:
                die(f"{PLIST_TEMPLATE}: expected placeholder {placeholder!r}, got none")
        text = text.replace("/Users/USERNAME", escape(str(HOME))).replace("<string>/bin/zsh</string>", f"<string>{escape(shell)}</string>")
        PLIST.write_text(text)
        print(f"wrote {PLIST}")
    else:
        UNIT_DIR.mkdir(parents=True, exist_ok=True)
        for t in UNIT_TEMPLATES:
            shutil.copyfile(t, UNIT_DIR / t.name)  # %h needs no rendering
        print(f"wrote {', '.join(str(UNIT_DIR / t.name) for t in UNIT_TEMPLATES)}")


def load_service() -> None:
    if MAC:
        if mac_job() is not None:
            run_ok(["launchctl", "bootout", MAC_TARGET])  # the plist may have changed: reload it
        run_ok(["launchctl", "bootstrap", MAC_DOMAIN, PLIST])
    else:
        systemctl("daemon-reload")
        systemctl("enable", "herdr-core.service", UNIT)
        # one start per run: restart starts a stopped unit (and herdr-core via Requires=) and
        # picks up a new binary or password on upgrade
        systemctl("restart", UNIT)
        user = getpass.getuser()
        linger = run(["loginctl", "show-user", user, "-p", "Linger", "--value"])
        if linger.returncode != 0:
            print(f"warning: loginctl show-user {user}: exit {linger.returncode}: {linger.stderr.strip()}; "
                  f"to start roosterm at boot run `loginctl enable-linger {user}`")
        elif linger.stdout.strip() == "no" and yes("Start roosterm at boot without a login (loginctl enable-linger)?", True):
            r = run(["loginctl", "enable-linger", user])
            if r.returncode != 0:  # polkit may refuse; the service still runs while you are logged in
                print(f"warning: loginctl enable-linger {user}: exit {r.returncode}: {r.stderr.strip()}; run it with sudo")
    print("service loaded")


def setup(args) -> None:
    binary = Path(args.binary).resolve()
    installer = Path(args.hc_installer).resolve() if args.hc_installer else None
    preflight(binary, load=not args.no_load)
    install_binary(binary)
    install_hc(installer)
    shell = configure()
    write_service(shell)
    if args.no_load:
        print("--no-load: service files written, not loaded")
    else:
        load_service()
    listen = config_get(CONFIG.read_text(), "listen")
    host, port = split_listen(listen)
    local = "127.0.0.1" if host in ("", "0.0.0.0", "::") else host
    print(f"""
roosterm is installed.
  open:     http://{f'[{local}]' if ':' in local else local}:{port}
  remote:   put an HTTPS reverse proxy in front before exposing it beyond this machine
  app:      add this host in the roosterm Android app with that URL and your username
  log:      {MAC_LOG if MAC else 'journalctl --user -u roosterm.service'}
  check:    python3 {Path(__file__).resolve()} status
  control:  python3 {Path(__file__).resolve()} start|stop|restart|uninstall
  upgrade:  run this installer from a newer package; config and sessions are kept""")


# ---- service control: launchd (macOS)

MAC_DOMAIN = f"gui/{os.getuid()}"
MAC_TARGET = f"{MAC_DOMAIN}/{LABEL}"


def mac_job():
    """`launchctl print` text, or None when the job is not loaded."""
    r = run(["launchctl", "print", MAC_TARGET])
    if r.returncode == 0:
        return r.stdout
    # 113 = "Could not find service"; any other failure is a broken launchctl, not "not loaded".
    if r.returncode == 113:
        return None
    die(f"launchctl print {MAC_TARGET}: expected exit 0 or 113 (not loaded), got {r.returncode}: {r.stderr.strip()}")


def field(text: str, name: str):
    m = re.search(rf"^\t{name} = (.+)$", text, re.M)
    return m.group(1) if m else None


def require_service_file() -> None:
    path = PLIST if MAC else UNIT_DIR / UNIT
    if not path.is_file():
        die(f"expected {path} to exist, got nothing (run `python3 install.py` first)")


def systemctl(*args: str) -> str:
    return run_ok(["systemctl", "--user", *args])


def answers(host: str, port: int) -> bool:
    try:
        with socket.create_connection((host, port), timeout=CONNECT_TIMEOUT_S):
            return True
    except OSError:
        return False


def cmd_start() -> None:
    require_service_file()
    if MAC:
        text = mac_job()
        if text is None:
            run_ok(["launchctl", "bootstrap", MAC_DOMAIN, PLIST])  # RunAtLoad starts it
        elif field(text, "pid") is None:
            run_ok(["launchctl", "kickstart", MAC_TARGET])
        else:
            print(f"already running, pid {field(text, 'pid')}")
            return
    else:
        systemctl("start", UNIT)
    print("started; run `status` to confirm the port answers")


def cmd_stop() -> None:
    if MAC:
        if mac_job() is None:
            print(f"{MAC_TARGET} is not loaded; nothing to stop")
            return
        # bootout, not kill: KeepAlive{SuccessfulExit=false} respawns a job that exits nonzero.
        run_ok(["launchctl", "bootout", MAC_TARGET])
        print(f"stopped {MAC_TARGET} (hc untouched); it loads again at next login or `start`")
    else:
        systemctl("stop", UNIT)
        print(f"stopped {UNIT} (hc untouched)")


def cmd_restart() -> None:
    require_service_file()
    if MAC:
        if mac_job() is None:
            run_ok(["launchctl", "bootstrap", MAC_DOMAIN, PLIST])
        else:
            run_ok(["launchctl", "kickstart", "-k", MAC_TARGET])
    else:
        systemctl("restart", UNIT)
    print("restarted; run `status` to confirm the port answers")


def cmd_uninstall() -> None:
    if MAC:
        if mac_job() is not None:
            run_ok(["launchctl", "bootout", MAC_TARGET])
        PLIST.unlink(missing_ok=True)
        print(f"unloaded {MAC_TARGET} and removed {PLIST}")
    else:
        units = [UNIT_DIR / t.name for t in UNIT_TEMPLATES]
        if any(u.is_file() for u in units):
            # disable --now stops roosterm; herdr-core.service has KillMode=process, so its stop leaves hc running.
            systemctl("disable", "--now", UNIT, "herdr-core.service")
        for u in units:
            u.unlink(missing_ok=True)
        systemctl("daemon-reload")
        print(f"disabled {UNIT} and herdr-core.service and removed their unit files")
    print("kept: hc, roosterm, config and sessions")


def cmd_status() -> None:
    if MAC:
        text = mac_job()
        if text is None:
            print(f"{MAC_TARGET}: not loaded (plist {'present' if PLIST.is_file() else 'absent'})")
            sys.exit(3)
        print(f"{MAC_TARGET}: loaded, state={field(text, 'state')}, pid={field(text, 'pid')}, "
              f"last exit code={field(text, 'last exit code')}, log={MAC_LOG}")
        running = field(text, "pid") is not None
    else:
        r = run(["systemctl", "--user", "is-active", UNIT])  # exit 3 = inactive, not an error
        state = r.stdout.strip()
        print(f"{UNIT}: {state or r.stderr.strip()}")
        running = state == "active"
    if not running:
        print("port: not running")
        sys.exit(3)
    host, port = split_listen(config_get(CONFIG.read_text(), "listen"))
    probe = "127.0.0.1" if host in ("", "0.0.0.0", "::") else host
    ok = answers(probe, port)
    print(f"port: {probe}:{port} {'answers' if ok else 'does NOT answer'}")
    if not ok:
        sys.exit(3)


COMMANDS = {"status": cmd_status, "start": cmd_start, "stop": cmd_stop, "restart": cmd_restart,
            "uninstall": cmd_uninstall}


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("command", nargs="?", choices=list(COMMANDS), help="service control; omit to install or upgrade")
    ap.add_argument("--binary", default=str(HERE / "roosterm"), help="roosterm binary to install (default: next to install.py)")
    ap.add_argument("--hc-installer", help=f"run this local hc install script instead of downloading {HC_INSTALL_URL}")
    ap.add_argument("--no-load", action="store_true", help="write the service files but do not load or start them")
    args = ap.parse_args()
    if sys.platform not in ("darwin", "linux"):
        die(f"expected macOS or Linux, got {sys.platform}")
    if args.command:
        COMMANDS[args.command]()
    else:
        setup(args)


if __name__ == "__main__":
    main()
