# 2026-11-21 Android CI

Contract: `docs/logs/contract-android-ci.md`. Model: relume `.github/workflows/android.yml` + `scripts/ci_guard.py` + `.githooks/pre-push`.

## Result
- `scripts/ci.py` — one CLI: `guard`, `build <platform>`, `prepush`, `bump`, `release <platform>`. One `PLATFORMS` table, only `android` registered. Build output renamed to `app/build/dist/{applicationId}-{arch}.apk` in shared code (local and cloud names identical).
- `scripts/ci_guard.py` — fails if any test command can run in CI: workflow `run:` lines, the hook, and argv literals in the scripts they call; strips wrappers (`exec`, `env`, `VAR=val`, `nohup`, `time`, `command`, `sh -c`). Cannot start a process itself (self-checked).
- `.github/workflows/android.yml` — tag `app-v*` only; guard → Java 17 → Flutter 3.44.6 → `ci.py build android` → `ci.py release android` (contents: write, checkout without persisted credentials).
- `.githooks/pre-push` → `ci.py prepush`; enabled with `git config core.hooksPath .githooks`.
- Gradle: arm64-only limit lifted only when `split-per-abi` is set (user decision); plain local builds stay arm64.

## Evidence (HEAD a30ac8f + uncommitted CI files)
- Guard red/green: 18 injected test commands → RC=1 each, naming file:line; clean copy and real tree → RC=0.
- actionlint RC=0.
- bump/release on a scratch clone with a `gh` shim: bump 1.0.0+1 → 1.0.1+2, tag `app-v1.0.1`; dirty tracked file / existing tag → RC=1; release argv = exactly the 3 APKs; unset GITHUB_SHA / tag-version mismatch / missing APK / missing applicationId → RC=1.
- `.githooks/pre-push` (after review fixes), RC=0, ~30 s:
  ```
  ci: guard passed
  ci: android -> app/build/dist/us.jhangy.roosterm-arm64-v8a.apk (18256921 bytes)
  ci: android -> app/build/dist/us.jhangy.roosterm-armeabi-v7a.apk (15620393 bytes)
  ci: android -> app/build/dist/us.jhangy.roosterm-x86_64.apk (19714737 bytes)
  RC=0
  ```
  Each APK contains only its own `lib/<abi>/`.
- Review (opus): mergeable; 2 should-fix + 4 nits all applied.

## Not yet proven
- Live cloud run (needs a pushed `app-v*` tag; user approval pending).
