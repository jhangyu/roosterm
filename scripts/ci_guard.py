#!/usr/bin/env python3
"""Fail when CI could run a test, or when a workflow step runs anything but a Python CI script. CI only compiles.

Scanned: every `run:` value in .github/workflows/*.y*ml, every line of .githooks/pre-push, and every argv
literal (list/tuple of str constants) in each scripts/*.py those two reference, and transitively in every
scripts/*.py named by a string constant of a scanned script (ci.py -> build.py).
This file is excluded from the argv scan, because its rule tables would match. That is safe because the
guard cannot start a process: if this file imports subprocess or calls os.system/popen/exec*/spawn*, the guard fails.

Usage: ci_guard.py [repo_root]   (default: this repository; pass a scratch copy to prove the guard goes red)
Stdlib only: PyYAML is not guaranteed on the runner, so `run:` values are extracted line-based.
"""
import ast
import re
import shlex
import sys
from pathlib import Path, PurePosixPath

WORKFLOWS = ".github/workflows"
HOOK = ".githooks/pre-push"
SELF = "scripts/ci_guard.py"
RUN_KEY = re.compile(r"^(\s*)(-\s+)?run:\s*(.*)$")
NAME_KEY = re.compile(r"^\s*(?:-\s+)?name:\s*(.*)$")
BLOCK_SCALAR = re.compile(r"^[|>][+-]?\d*$")
SEGMENT_SPLIT = re.compile(r"&&|\|\||;|\||\$\(|\)|`")
PYTHON_STEP = re.compile(r"^python3 (scripts/[\w-]+\.py)(?: [\w.-]+)*$")
SCRIPT_REF = re.compile(r"\bscripts/[\w-]+\.py\b")
TEST_SCRIPT = re.compile(r"(^|/)(test-[^/]*\.sh|[^/]*_test\.(sh|py)|test_[^/]*\.py|test\.py)$")
TEST_RUNNERS = {"pytest", "py.test", "ctest", "unittest"}
GRADLE = {"gradlew", "gradle"}
GRADLE_TEST_WORD = re.compile(r"test|^check$", re.IGNORECASE)  # test, check, connectedAndroidTest, testReleaseUnitTest
PKG_MANAGERS = {"npm", "yarn", "pnpm"}
TEST_SUBCOMMANDS = {"go": {"test"}, "cargo": {"test"}, "flutter": {"test", "drive"}, "dart": {"test"}}
OUTPUT_ONLY = {"echo", "printf"}
WRAPPERS = {"exec", "env", "command", "nohup", "time"}  # run the next word as the command
ASSIGNMENT = re.compile(r"^[A-Za-z_]\w*=")                 # VAR=val prefix
SHELLS = {"sh", "bash", "zsh", "dash"}                     # `sh -c "<cmd>"` is scanned recursively
PROCESS_CALL = re.compile(r"^(system|popen|exec\w*|spawn\w*)$")  # os.* functions that start a process

NO_TESTS = "CI runs no tests"
PYTHON_ONLY = "CI workflow steps run only 'python3 scripts/<name>.py [args]'"
ARGV_ONLY = "CI scripts start processes from argv lists only"
SELF_PURE = "ci_guard must not start processes"
MISSING = "CI references a missing script"


def extract_runs(lines):
    """Yield (step_name, line_no, command_text) for every run: value."""
    name = None
    i = 0
    while i < len(lines):
        line = lines[i]
        if re.match(r"^\s*-\s", line):
            name = None
        m = NAME_KEY.match(line)
        if m:
            name = m.group(1).strip().strip("\"'")
        m = RUN_KEY.match(line)
        if not m:
            i += 1
            continue
        key_indent = len(m.group(1)) + (len(m.group(2)) if m.group(2) else 0)
        value = m.group(3).strip()
        label = name or f"line {i + 1}"
        if BLOCK_SCALAR.match(value):
            body = []
            j = i + 1
            while j < len(lines) and (not lines[j].strip() or len(lines[j]) - len(lines[j].lstrip()) > key_indent):
                body.append(lines[j].strip())
                j += 1
            yield label, i + 1, "\n".join(body)
            i = j
            continue
        yield label, i + 1, value.strip("\"'")
        i += 1


def offending(command_text):
    """Return the command segments that run tests."""
    joined = re.sub(r"\\\n\s*", " ", command_text)
    found = []
    for raw in joined.splitlines():
        if raw.lstrip().startswith("#"):
            continue
        for segment in SEGMENT_SPLIT.split(raw):
            segment = segment.strip()
            if not segment:
                continue
            try:
                tokens = shlex.split(segment)
            except ValueError:  # unbalanced quotes after splitting on $( ... ); fall back to whitespace tokens
                tokens = [t.strip("\"'") for t in segment.split()]
            if tokens and runs_tests(tokens):
                found.append(segment)
    return found


def strip_wrappers(tokens):
    """Drop leading VAR=val assignments and wrapper words (with their -flags) so the real command is tokens[0]."""
    i = 0
    while i < len(tokens):
        if PurePosixPath(tokens[i]).name in WRAPPERS:
            i += 1
            while i < len(tokens) and tokens[i].startswith("-"):
                i += 1
        elif ASSIGNMENT.match(tokens[i]):
            i += 1
        else:
            break
    return tokens[i:]


def runs_tests(tokens):
    tokens = strip_wrappers(tokens)
    if not tokens:
        return False
    base = [PurePosixPath(t).name for t in tokens]
    if base[0] in SHELLS and len(tokens) > 2 and tokens[1] == "-c":
        return bool(offending(tokens[2]))
    if base[0] in OUTPUT_ONLY:
        return False
    if any(TEST_SCRIPT.search(t) for t in tokens):
        return True
    if any(b in TEST_RUNNERS for b in base):
        return True
    if any(b in GRADLE for b in base[:2]):
        return any(GRADLE_TEST_WORD.search(t) for t in tokens[1:] if not t.startswith("-"))
    if base[0] in PKG_MANAGERS:
        return "test" in tokens[1:3]
    if base[0] in TEST_SUBCOMMANDS:
        sub = next((t for t in tokens[1:] if not t.startswith("-")), None)
        return sub in TEST_SUBCOMMANDS[base[0]]
    return False


def argv_violations(tree):
    """Yield (line_no, text) for every list/tuple literal whose str constants form a test command."""
    for node in ast.walk(tree):
        if isinstance(node, (ast.List, ast.Tuple)):
            strs = [e.value for e in node.elts if isinstance(e, ast.Constant) and isinstance(e.value, str)]
            if strs and runs_tests(strs):
                yield node.lineno, " ".join(strs)


def os_process_calls(tree):
    """Yield (line_no, text) for os.<process call> attributes and `from os import <process call>`."""
    for node in ast.walk(tree):
        if (isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name) and node.value.id == "os"
                and PROCESS_CALL.match(node.attr)):
            yield node.lineno, f"os.{node.attr}"
        elif isinstance(node, ast.ImportFrom) and node.module == "os":
            for alias in node.names:
                if PROCESS_CALL.match(alias.name):
                    yield node.lineno, f"from os import {alias.name}"


def process_violations(tree):
    for node in ast.walk(tree):
        if isinstance(node, ast.keyword) and node.arg == "shell" and not (
                isinstance(node.value, ast.Constant) and node.value.value is False):
            yield node.value.lineno, f"shell={ast.unparse(node.value)}"
    yield from os_process_calls(tree)


def self_violations(tree):
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name.split(".")[0] == "subprocess":
                    yield node.lineno, f"import {alias.name}"
        elif isinstance(node, ast.ImportFrom) and (node.module or "").split(".")[0] == "subprocess":
            yield node.lineno, f"from {node.module} import ..."
    yield from os_process_calls(tree)


def parse(root, rel, violations):
    try:
        return ast.parse((root / rel).read_text(encoding="utf-8"), filename=rel)
    except SyntaxError as e:
        violations.append(f"CI scripts must parse; {rel}:{e.lineno}: {e.msg}")
        return None


def check(root):
    """Return one message per violation in the repository at `root` (empty list = clean)."""
    root = Path(root)
    violations = []
    referenced = {}  # script -> "file:line" of its first reference

    workflow_dir = root / WORKFLOWS
    workflows = sorted(p for p in workflow_dir.glob("*") if p.suffix in (".yml", ".yaml")) if workflow_dir.is_dir() else []
    if not workflows:
        violations.append(f"expected workflow files in {WORKFLOWS}, found none")
    for path in workflows:
        rel = f"{WORKFLOWS}/{path.name}"
        for step, line_no, text in extract_runs(path.read_text(encoding="utf-8").splitlines()):
            for cmd in offending(text):
                violations.append(f"{NO_TESTS}; {rel}:{line_no} step '{step}' runs: {cmd}")
            if not PYTHON_STEP.match(text):
                violations.append(f"{PYTHON_ONLY}; {rel}:{line_no} runs: {text}")
            for script in SCRIPT_REF.findall(text):
                referenced.setdefault(script, f"{rel}:{line_no}")

    hook = root / HOOK
    if not hook.is_file():
        violations.append(f"expected the pre-push hook {HOOK}, not found")
    else:
        for line_no, line in enumerate(hook.read_text(encoding="utf-8").splitlines(), 1):
            if line.lstrip().startswith("#"):
                continue
            for cmd in offending(line):
                violations.append(f"{NO_TESTS}; {HOOK}:{line_no} runs: {cmd}")
            for script in SCRIPT_REF.findall(line):
                referenced.setdefault(script, f"{HOOK}:{line_no}")

    pending = sorted(referenced.items())
    seen = set()
    while pending:  # transitive: a scanned script's string constants may name further scripts
        script, where = pending.pop(0)
        if script == SELF or script in seen:
            continue
        seen.add(script)
        if not (root / script).is_file():
            violations.append(f"{MISSING}; {where}: {script}")
            continue
        tree = parse(root, script, violations)
        if tree is None:
            continue
        for line_no, text in argv_violations(tree):
            violations.append(f"{NO_TESTS}; {script}:{line_no} argv: {text}")
        for line_no, text in process_violations(tree):
            violations.append(f"{ARGV_ONLY}; {script}:{line_no}: {text}")
        for node in ast.walk(tree):
            if isinstance(node, ast.Constant) and isinstance(node.value, str):
                for ref in SCRIPT_REF.findall(node.value):
                    pending.append((ref, f"{script}:{node.lineno}"))

    if not (root / SELF).is_file():
        violations.append(f"expected the guard itself at {SELF}, not found")
    else:
        tree = parse(root, SELF, violations)
        if tree is not None:
            for line_no, text in self_violations(tree):
                violations.append(f"{SELF_PURE}; {SELF}:{line_no}: {text}")

    return violations


def main():
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parent.parent
    violations = check(root)
    if violations:
        print("\n".join(violations), file=sys.stderr)
        sys.exit(1)
    print("ci_guard: workflows, hook and CI scripts run no test commands")


if __name__ == "__main__":
    main()
