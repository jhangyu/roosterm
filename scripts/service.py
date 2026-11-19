#!/usr/bin/env python3
"""Manage the roosterm launchd job (macOS).

python3 scripts/service.py install     render the plist into ~/Library/LaunchAgents and load it (idempotent)
python3 scripts/service.py uninstall   unload it and remove the plist
python3 scripts/service.py start       load/start roosterm (hc keeps running)
python3 scripts/service.py stop        unload roosterm until the next install/start/login (hc keeps running)
python3 scripts/service.py restart     kill and start roosterm again
python3 scripts/service.py status      loaded / pid / last exit / listen port

Never touches the hc core: `hc server stop` would end every program in every tab.
"""
import argparse
import os
import pathlib
import re
import socket
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
LABEL = "dev.roosterm.serve"
TEMPLATE = ROOT / "deploy" / "launchd" / f"{LABEL}.plist"
HOME = pathlib.Path.home()
PLIST = HOME / "Library" / "LaunchAgents" / f"{LABEL}.plist"
LOG = HOME / "Library" / "Logs" / "roosterm.log"
BINARIES = [HOME / ".local" / "bin" / "hc", HOME / ".local" / "bin" / "roosterm"]
DOMAIN = f"gui/{os.getuid()}"
TARGET = f"{DOMAIN}/{LABEL}"
CONNECT_TIMEOUT_S = 2.0  # fail-fast cap on the port probe


def die(msg: str) -> None:
    print(f"FAIL: {msg}")
    sys.exit(1)


def launchctl(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["launchctl", *args], capture_output=True, text=True)


def run_ok(*args: str) -> str:
    r = launchctl(*args)
    if r.returncode != 0:
        die(f"launchctl {' '.join(args)}: expected exit 0, got {r.returncode}: {(r.stderr or r.stdout).strip()}")
    return r.stdout


def print_job() -> str | None:
    """`launchctl print` text, or None when the job is not loaded."""
    r = launchctl("print", TARGET)
    if r.returncode == 0:
        return r.stdout
    # 113 = "Could not find service"; any other failure is a broken launchctl, not "not loaded".
    if r.returncode == 113:
        return None
    die(f"launchctl print {TARGET}: expected exit 0 or 113 (not loaded), got {r.returncode}: {r.stderr.strip()}")


def field(text: str, name: str) -> str | None:
    m = re.search(rf"^\t{name} = (.+)$", text, re.M)
    return m.group(1) if m else None


def pid_of(text: str) -> int | None:
    v = field(text, "pid")
    return int(v) if v else None


def listen_addrs(pid: int) -> list[tuple[str, int]]:
    r = subprocess.run(["lsof", "-nP", "-a", "-p", str(pid), "-iTCP", "-sTCP:LISTEN", "-Fn"],
                       capture_output=True, text=True)
    if r.returncode not in (0, 1):  # lsof exits 1 when nothing matches
        die(f"lsof -p {pid}: expected exit 0 or 1, got {r.returncode}: {r.stderr.strip()}")
    out = []
    for line in r.stdout.splitlines():
        if line.startswith("n"):
            host, _, port = line[1:].rpartition(":")
            out.append(("127.0.0.1" if host in ("*", "") else host.strip("[]"), int(port)))
    return out


def answers(host: str, port: int) -> bool:
    try:
        with socket.create_connection((host, port), timeout=CONNECT_TIMEOUT_S):
            return True
    except OSError:
        return False


def install() -> None:
    for b in BINARIES:
        if not b.is_file():
            die(f"expected {b} to exist, got nothing (install hc and roosterm into ~/.local/bin first)")
    if print_job() is not None:
        run_ok("bootout", TARGET)
    PLIST.parent.mkdir(parents=True, exist_ok=True)
    LOG.parent.mkdir(parents=True, exist_ok=True)
    PLIST.write_text(TEMPLATE.read_text().replace("/Users/USERNAME", str(HOME)))
    run_ok("bootstrap", DOMAIN, str(PLIST))
    print(f"installed {PLIST} and loaded {TARGET}; run `status` to confirm the port answers")


def uninstall() -> None:
    if print_job() is not None:
        run_ok("bootout", TARGET)
    PLIST.unlink(missing_ok=True)
    print(f"unloaded {TARGET} and removed {PLIST}")


def require_installed() -> None:
    if not PLIST.is_file():
        die(f"expected {PLIST} to exist, got nothing (run `install` first)")


def start() -> None:
    require_installed()
    text = print_job()
    if text is None:
        run_ok("bootstrap", DOMAIN, str(PLIST))  # RunAtLoad starts it
    elif pid_of(text) is None:
        run_ok("kickstart", TARGET)
    else:
        print(f"already running, pid {pid_of(text)}")
        return
    print("started; run `status` to confirm the port answers")


def stop() -> None:
    if print_job() is None:
        print(f"{TARGET} is not loaded; nothing to stop")
        return
    # bootout, not kill: KeepAlive{SuccessfulExit=false} respawns a job that exits nonzero.
    run_ok("bootout", TARGET)
    print(f"stopped {TARGET} (hc untouched); it loads again at next login or `start`")


def restart() -> None:
    require_installed()
    if print_job() is None:
        run_ok("bootstrap", DOMAIN, str(PLIST))
    else:
        run_ok("kickstart", "-k", TARGET)
    print("restarted; run `status` to confirm the port answers")


def status() -> None:
    text = print_job()
    if text is None:
        print(f"{TARGET}: not loaded (plist {'present' if PLIST.is_file() else 'absent'})")
        sys.exit(3)
    pid = pid_of(text)
    print(f"{TARGET}: loaded, state={field(text, 'state')}, pid={pid}, last exit code={field(text, 'last exit code')}, log={LOG}")
    if pid is None:
        print("port: not running")
        sys.exit(3)
    addrs = listen_addrs(pid)
    if not addrs:
        print(f"port: pid {pid} has no TCP listener yet")
        sys.exit(3)
    ok = [a for a in addrs if answers(*a)]
    for host, port in addrs:
        print(f"port: {host}:{port} {'answers' if (host, port) in ok else 'does NOT answer'}")
    if not ok:
        sys.exit(3)


COMMANDS = {"install": install, "uninstall": uninstall, "start": start, "stop": stop,
            "restart": restart, "status": status}


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("command", choices=COMMANDS)
    args = ap.parse_args()
    if sys.platform != "darwin":
        die(f"expected macOS (launchd), got {sys.platform}; Linux uses deploy/systemd, see deploy/README.md")
    COMMANDS[args.command]()


if __name__ == "__main__":
    main()
