#!/usr/bin/env python3
"""Roosterm test entry: forbidden-word scan, gofmt, go vet, go test with time budgets.

Run from anywhere: python3 scripts/test.py. Exit 0 only if every check passes.
"""
import json
import pathlib
import re
import subprocess
import sys
import time
from typing import NoReturn

ROOT = pathlib.Path(__file__).resolve().parent.parent
HOST = ROOT / "host"
SUITE_BUDGET_S = 40.0  # whole script wall time (contract criterion 2)
TEST_BUDGET_S = 1.0  # any single test
SLOWEST_SHOWN = 5
GO_TEST_TIMEOUT = "30s"  # go's own hang guard: dumps goroutines before the budget kill
FORBIDDEN = re.compile(r"(?i)\bsleep\b|time\.Sleep")


def fail(msg: str) -> NoReturn:
    print(f"FAIL: {msg}")
    sys.exit(1)


def scan_forbidden() -> None:
    files = sorted(HOST.rglob("*_test.go")) + sorted((HOST / "internal" / "hc" / "hctest").rglob("*.go"))
    hits = []
    for f in files:
        for n, line in enumerate(f.read_text(encoding="utf-8").splitlines(), 1):
            if FORBIDDEN.search(line):
                hits.append(f"{f.relative_to(ROOT)}:{n}: {line.strip()}")
    if hits:
        fail("sleep-like text in test code:\n" + "\n".join(hits))
    print(f"forbidden-word scan: {len(files)} files clean")


def run(cmd: list[str], timeout: float) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(cmd, cwd=HOST, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        fail(f"{' '.join(cmd)}: exceeded the {SUITE_BUDGET_S:.0f} s suite budget")


def remaining(start: float) -> float:
    left = SUITE_BUDGET_S - (time.monotonic() - start)
    if left <= 0:
        fail(f"suite budget {SUITE_BUDGET_S:.0f} s used up before the next step")
    return left


def main() -> None:
    start = time.monotonic()
    scan_forbidden()
    fmt = run(["gofmt", "-l", "."], remaining(start))
    if fmt.returncode != 0 or fmt.stdout.strip():
        fail(f"gofmt -l (rc {fmt.returncode}) lists unformatted files:\n{fmt.stdout}{fmt.stderr}")
    vet = run(["go", "vet", "./..."], remaining(start))
    if vet.returncode != 0:
        fail(f"go vet rc {vet.returncode}:\n{vet.stdout}{vet.stderr}")
    test = run(["go", "test", "./...", "-json", "-count=1", "-race", "-timeout", GO_TEST_TIMEOUT], remaining(start))
    elapsed: dict[str, float] = {}
    failed, skipped, output = [], [], []
    for line in test.stdout.splitlines():
        try:
            ev = json.loads(line)
        except json.JSONDecodeError:
            output.append(line)
            continue
        if ev.get("Action") == "output":
            output.append(ev.get("Output", "").rstrip("\n"))
        name = ev.get("Test")
        if not name:
            continue
        key = f"{ev['Package']}.{name}"
        action = ev.get("Action")
        if action in ("pass", "fail"):
            elapsed[key] = ev.get("Elapsed", 0.0)
        if action == "fail":
            failed.append(key)
        if action == "skip":
            skipped.append(key)
    wall = time.monotonic() - start
    slowest = sorted(elapsed.items(), key=lambda kv: kv[1], reverse=True)[:SLOWEST_SHOWN]
    print(f"tests: {len(elapsed)}")
    print(f"skipped: {len(skipped)}")
    print(f"wall: {wall:.1f} s (budget {SUITE_BUDGET_S:.0f} s)")
    print("slowest:")
    for key, secs in slowest:
        print(f"  {secs:.2f} s  {key}")
    problems = []
    if test.returncode != 0:
        problems.append(f"go test rc {test.returncode}; failed: {failed}\n" + "\n".join(output[-60:]) + test.stderr)
    if wall > SUITE_BUDGET_S:
        problems.append(f"wall {wall:.1f} s > {SUITE_BUDGET_S:.0f} s")
    over = [f"{k} {s:.2f} s" for k, s in elapsed.items() if s > TEST_BUDGET_S]
    if over:
        problems.append(f"tests over {TEST_BUDGET_S} s: {over}")
    if skipped:
        problems.append(f"skipped tests: {skipped}")
    if not elapsed:
        problems.append("zero tests ran")
    if problems:
        fail("\n".join(problems))
    print("PASS")


if __name__ == "__main__":
    main()
