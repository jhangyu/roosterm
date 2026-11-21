#!/usr/bin/env python3
"""roosterm CI. The workflow and .githooks/pre-push call only this script. It never runs tests
(scripts/ci_guard.py fails CI if any test could run).

  ci.py guard                          fail if CI could run a test
  ci.py build [platform ...]           compile and collect named release files (default: all platforms)
  ci.py prepush                        guard + build, the pre-push hook
  ci.py bump {major,minor,patch} [--dry-run]
                                       bump app/pubspec.yaml, commit it, tag app-vX.Y.Z (does not push)
  ci.py release [platform ...] [--dry-run]
                                       on CI: publish the tag's GitHub Release with the collected files
"""
import argparse
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from typing import NamedTuple

sys.dont_write_bytecode = True   # importing ci_guard must not leave scripts/__pycache__ (not gitignored)
import ci_guard

ROOT = Path(__file__).resolve().parent.parent
VERSION_FILE = ROOT / "app/pubspec.yaml"
VERSION_LINE = re.compile(r"^version: (\d+)\.(\d+)\.(\d+)\+(\d+)$", re.MULTILINE)
TAG_PREFIX = "app-v"   # .github/workflows/android.yml `tags: ["app-v*"]` must match
DIST = ROOT / "app/build/dist"   # inside the gitignored app/build/


class Platform(NamedTuple):
    workdir: str        # repo-relative cwd of the build command
    tools: tuple        # executables that must be on PATH
    command: tuple      # argv literal; keep it a literal so ci_guard can scan it
    archs: tuple        # one release file per arch
    output: str         # repo-relative build output, {arch} placeholder
    name_source: str    # repo-relative file holding the package name
    name_key: str       # regex with one group: the package name
    release_name: str   # final file name, {package} and {arch} placeholders


# One entry per platform. Adding a platform = one entry here + its steps in the workflow.
PLATFORMS = {
    "android": Platform(
        workdir="app",
        tools=("flutter",),
        command=("flutter", "build", "apk", "--release", "--split-per-abi"),
        archs=("arm64-v8a", "armeabi-v7a", "x86_64"),
        output="app/build/app/outputs/flutter-apk/app-{arch}-release.apk",
        name_source="app/android/app/build.gradle.kts",
        name_key=r'^\s*applicationId\s*=\s*"([^"]+)"',
        release_name="{package}-{arch}.apk",
    ),
}


def fail(message):
    raise SystemExit(f"ci: {message}")


def run(argv, cwd=ROOT, capture=False):
    result = subprocess.run(argv, cwd=cwd, text=True, capture_output=capture)
    if result.returncode != 0:
        detail = f": {result.stderr.strip()}" if capture else ""
        fail(f"{' '.join(argv)} in {cwd} exited {result.returncode}, expected 0{detail}")
    return result.stdout if capture else None


def package_name(platform):
    source = ROOT / platform.name_source
    found = re.findall(platform.name_key, source.read_text(), re.MULTILINE)
    if len(found) != 1:
        fail(f"expected exactly one match of {platform.name_key!r} in {platform.name_source}, got {found}")
    return found[0]


def release_files(name):
    """The exact files `build` produces and `release` uploads for this platform."""
    platform = PLATFORMS[name]
    package = package_name(platform)
    return [DIST / platform.release_name.format(package=package, arch=a) for a in platform.archs]


def build(name):
    platform = PLATFORMS[name]
    missing = [t for t in platform.tools if shutil.which(t) is None]
    if missing:
        fail(f"{name}: expected {missing} on PATH, not found (PATH={os.environ.get('PATH')!r})")
    outputs = [ROOT / platform.output.format(arch=a) for a in platform.archs]
    targets = release_files(name)
    for stale in outputs + targets:   # a stale file must not pass the checks below
        stale.unlink(missing_ok=True)
    run(platform.command, cwd=ROOT / platform.workdir)
    absent = [str(p.relative_to(ROOT)) for p in outputs if not p.is_file()]
    if absent:
        fail(f"{name}: expected build outputs {absent} after a successful build, not found")
    DIST.mkdir(parents=True, exist_ok=True)
    for src, dst in zip(outputs, targets):
        src.replace(dst)
        print(f"ci: {name} -> {dst.relative_to(ROOT)} ({dst.stat().st_size} bytes)")


def guard():
    violations = ci_guard.check(ROOT)
    if violations:
        print("\n".join(violations), file=sys.stderr)
        fail(f"guard found {len(violations)} violation(s), expected 0")
    print("ci: guard passed")


def read_version():
    found = VERSION_LINE.findall(VERSION_FILE.read_text())
    if len(found) != 1:
        fail(f"expected one 'version: X.Y.Z+N' line in {VERSION_FILE.relative_to(ROOT)}, got {found}")
    return tuple(int(n) for n in found[0])


def bump(part, dry_run):
    major, minor, patch, code = read_version()
    major, minor, patch = {
        "major": (major + 1, 0, 0), "minor": (major, minor + 1, 0), "patch": (major, minor, patch + 1),
    }[part]
    version = f"{major}.{minor}.{patch}"
    tag = TAG_PREFIX + version
    dirty = run(["git", "status", "--porcelain", "--untracked-files=no"], capture=True)
    if dirty:
        fail(f"expected a clean tracked tree before bump, got:\n{dirty}")
    if run(["git", "tag", "--list", tag], capture=True).strip():
        fail(f"tag {tag} already exists, expected a new version")
    line = f"version: {version}+{code + 1}"
    print(f"ci: {VERSION_FILE.relative_to(ROOT)} -> {line}; tag {tag}")
    if dry_run:
        return
    text = VERSION_FILE.read_text()
    VERSION_FILE.write_text(VERSION_LINE.sub(line, text, count=1))
    run(["git", "commit", "-m", f"app: release {version}", "--", str(VERSION_FILE.relative_to(ROOT))])
    run(["git", "tag", "-a", tag, "-m", f"roosterm app {version}"])
    print(f"ci: to publish, run: git push origin HEAD {tag}")


def release(names, dry_run):
    tag = os.environ.get("GITHUB_REF_NAME", "")
    major, minor, patch, _ = read_version()
    expected = f"{TAG_PREFIX}{major}.{minor}.{patch}"
    if tag != expected:
        fail(f"GITHUB_REF_NAME={tag!r}, expected {expected!r} (from {VERSION_FILE.relative_to(ROOT)})")
    sha = os.environ.get("GITHUB_SHA", "")
    if not sha:
        fail(f"GITHUB_SHA={sha!r}, expected the tagged commit sha (set by GitHub Actions)")
    files = [f for name in names for f in release_files(name)]
    absent = [str(f.relative_to(ROOT)) for f in files if not f.is_file()]
    if absent:
        fail(f"expected release files {absent}, not found; run `ci.py build` first")
    argv = ["gh", "release", "create", tag, *map(str, files), "--verify-tag",
            "--title", f"roosterm app {major}.{minor}.{patch}",
            "--notes", sha]
    print("ci: " + " ".join(argv))
    if not dry_run:
        run(argv)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("guard")
    sub.add_parser("prepush")
    for cmd in ("build", "release"):
        p = sub.add_parser(cmd)
        p.add_argument("platforms", nargs="*", choices=sorted(PLATFORMS))
        if cmd == "release":
            p.add_argument("--dry-run", action="store_true")
    p = sub.add_parser("bump")
    p.add_argument("part", choices=("major", "minor", "patch"))
    p.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    names = getattr(args, "platforms", None) or list(PLATFORMS)
    if args.cmd == "guard":
        guard()
    elif args.cmd == "prepush":
        guard()
        for name in names:
            build(name)
    elif args.cmd == "build":
        for name in names:
            build(name)
    elif args.cmd == "bump":
        bump(args.part, args.dry_run)
    else:
        release(names, args.dry_run)


if __name__ == "__main__":
    main()
