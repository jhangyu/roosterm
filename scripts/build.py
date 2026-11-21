#!/usr/bin/env python3
"""Build roosterm into build/: the web client, then the Go binary.

python3 scripts/build.py                        -> build/roosterm (host platform)
python3 scripts/build.py --target linux/arm64   -> build/roosterm-linux-arm64
python3 scripts/build.py --package darwin/arm64 linux/amd64
    -> build/roosterm-server-<os>-<arch>.tar.gz: the binary plus everything under deploy/
"""
import argparse
import gzip
import os
import pathlib
import shutil
import subprocess
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
HOST = ROOT / "host"
WEB = ROOT / "web"
BUILD = ROOT / "build"
DEPLOY = ROOT / "deploy"
EXECUTABLE_MODE = 0o755
FILE_MODE = 0o644
EXECUTABLES = {"roosterm", "install.py"}  # tarball entries that get EXECUTABLE_MODE


def step(cmd: list[str], cwd: pathlib.Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(cmd), flush=True)
    rc = subprocess.run(cmd, cwd=cwd, env=env).returncode
    if rc != 0:
        print(f"FAIL: {' '.join(cmd)} (cwd {cwd}) exited {rc}")
        sys.exit(rc)


def parse_target(ap: argparse.ArgumentParser, flag: str, value: str) -> tuple[str, str]:
    parts = value.split("/")
    if len(parts) != 2 or not all(parts):
        ap.error(f"{flag}: expected GOOS/GOARCH like linux/arm64, got {value!r}")
    return parts[0], parts[1]


def write_package(goos: str, goarch: str, binary: pathlib.Path) -> None:
    """Tar `binary` + deploy/ under roosterm-server-<os>-<arch>/ with fixed modes, owner and mtime."""
    top = f"roosterm-server-{goos}-{goarch}"
    mtime = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))
    files = {"roosterm": binary}
    for path in sorted(DEPLOY.rglob("*")):
        if path.is_file():
            files[path.relative_to(DEPLOY).as_posix()] = path
    tarball = BUILD / f"{top}.tar.gz"
    tarball.unlink(missing_ok=True)
    # GzipFile(mtime=mtime): the gzip header otherwise carries the current time.
    with tarball.open("wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=mtime) as gz, \
            tarfile.open(fileobj=gz, mode="w") as tar:
        for rel, path in files.items():
            info = tar.gettarinfo(str(path), arcname=f"{top}/{rel}")
            info.mode = EXECUTABLE_MODE if rel in EXECUTABLES else FILE_MODE
            info.uid = info.gid = 0
            info.uname = info.gname = ""
            info.mtime = mtime
            with path.open("rb") as f:
                tar.addfile(info, f)
    print(f"wrote {tarball.relative_to(ROOT)}")


def main() -> None:
    ap = argparse.ArgumentParser()
    group = ap.add_mutually_exclusive_group()
    group.add_argument("--target", help="GOOS/GOARCH, e.g. linux/arm64")
    group.add_argument("--package", nargs="+", metavar="GOOS/GOARCH",
                       help="server tarball per target, e.g. darwin/arm64 linux/amd64")
    args = ap.parse_args()
    packages = [parse_target(ap, "--package", t) for t in args.package or []]
    if packages and not DEPLOY.is_dir():
        ap.error(f"--package: expected directory {DEPLOY}, not found")
    step(["npm", "ci", "--prefix", str(WEB)], ROOT)
    step(["npm", "run", "build", "--prefix", str(WEB)], ROOT)
    # vite emptyOutDir deletes the tracked placeholder; go:embed all:dist needs it on a fresh clone.
    (HOST / "internal" / "web" / "dist" / ".gitkeep").touch()
    BUILD.mkdir(exist_ok=True)
    env = dict(os.environ, CGO_ENABLED="0")
    if packages:
        for goos, goarch in packages:
            staging = BUILD / f"roosterm-server-{goos}-{goarch}"
            shutil.rmtree(staging, ignore_errors=True)
            staging.mkdir()
            target_env = dict(env, GOOS=goos, GOARCH=goarch)
            step(["go", "build", "-trimpath", "-o", str(staging / "roosterm"), "./cmd/roosterm"], HOST, target_env)
            write_package(goos, goarch, staging / "roosterm")
        return
    name = "roosterm"
    if args.target:
        goos, goarch = parse_target(ap, "--target", args.target)
        env["GOOS"], env["GOARCH"] = goos, goarch
        name = f"roosterm-{goos}-{goarch}"
    out = BUILD / name
    step(["go", "build", "-trimpath", "-o", str(out), "./cmd/roosterm"], HOST, env)
    print(f"wrote {out.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
