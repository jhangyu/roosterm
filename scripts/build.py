#!/usr/bin/env python3
"""Build roosterm into build/: the web client (when web/package.json exists), then the Go binary.

python3 scripts/build.py                        -> build/roosterm (host platform)
python3 scripts/build.py --target linux/arm64   -> build/roosterm-linux-arm64
"""
import argparse
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
HOST = ROOT / "host"
WEB = ROOT / "web"
BUILD = ROOT / "build"


def step(cmd: list[str], cwd: pathlib.Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(cmd), flush=True)
    rc = subprocess.run(cmd, cwd=cwd, env=env).returncode
    if rc != 0:
        print(f"FAIL: {' '.join(cmd)} (cwd {cwd}) exited {rc}")
        sys.exit(rc)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--target", help="GOOS/GOARCH, e.g. linux/arm64")
    args = ap.parse_args()
    env = dict(os.environ, CGO_ENABLED="0")
    name = "roosterm"
    if args.target:
        parts = args.target.split("/")
        if len(parts) != 2 or not all(parts):
            ap.error(f"--target: expected GOOS/GOARCH like linux/arm64, got {args.target!r}")
        env["GOOS"], env["GOARCH"] = parts
        name = f"roosterm-{parts[0]}-{parts[1]}"
    if (WEB / "package.json").exists():
        step(["npm", "ci", "--prefix", str(WEB)], ROOT)
        step(["npm", "run", "build", "--prefix", str(WEB)], ROOT)
    else:
        print("web/: no package.json, embedding host/internal/web/dist as committed")
    BUILD.mkdir(exist_ok=True)
    out = BUILD / name
    step(["go", "build", "-trimpath", "-o", str(out), "./cmd/roosterm"], HOST, env)
    print(f"wrote {out.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
