# 2026-11-21 Server package, interactive installer, fixed APK signing key

Contract: `docs/logs/contract-server-package.md`.

## Result
- `scripts/ci.py`: `server` platform entry; `build android server` / `release android server`; release title `roosterm X.Y.Z`; `android_signing_env` decodes the keystore secret in CI (fails loudly when missing), accepts a local `ANDROID_KEYSTORE_FILE`, else debug-signs with a notice.
- `scripts/build.py --package GOOS/GOARCH...` → `build/roosterm-server-<os>-<arch>.tar.gz` (binary + `deploy/`, fixed modes, owner and mtime, reproducible gzip). Targets: darwin/linux × arm64/amd64, CGO off, cross-built on ubuntu.
- `scripts/ci_guard.py`: transitive scan of scripts named by scanned scripts (ci.py → build.py).
- `deploy/install.py` (stdlib, Python 3.9): prerequisite checks, binary to `~/.local/bin` (macOS quarantine flag cleared on the installed copy), hc via `https://releases.herdr.dev/core/install.sh` 0.1.21 when missing, `roosterm passwd`, config prompts, launchd / systemd --user service, re-run keeps config and sessions, `status|start|stop|restart|uninstall`. Replaces `scripts/service.py`.
- `deploy/systemd/herdr-core.service`: `KillMode=process`, so stopping it never kills terminal programs.
- Gradle release signing from `ANDROID_KEYSTORE_FILE` + password/alias env; workflow passes the 4 secrets only to the Android build step.
- Release key: `~/.config/roosterm/release-key.jks` (+ `release-key.password`, both 0600), cert SHA-256 `f87d42716ed7211099a85263a5a0d20f8479e03929395856088317e9024017fd`; 4 GitHub secrets set.

## Evidence (HEAD c5993be + uncommitted change)
- Guard: real tree RC=0; `go test ./...` injected into build.py → RC=1 at `scripts/build.py:102`; the c5993be guard → RC=0 on the same injection (gap was real).
- `ci.py build server`: RC=0, 4 tarballs, 6 entries each; `file`: Mach-O arm64 / x86_64, ELF x86-64 / aarch64 static; darwin-arm64 binary no args RC=2 `usage:`; codesign `adhoc,linker-signed`. Two host builds give identical shasum.
- Signed APKs with a throwaway test keystore: apksigner signer SHA-256 = keystore cert SHA-256 on all 3; no-env build → `CN=Android Debug`; CI with missing secrets → RC=1 naming them.
- actionlint RC=0. Release dry-run: exactly 3 APKs + 4 tarballs, title `roosterm 0.0.0`.
- Installer, isolated (temp HOME, PATH shims, `/usr/bin/python3` 3.9.6): fresh, re-run keep (config and sessions.json byte-identical), listen change, quarantine, XDG refusal — all pass; real Mac binary, config and launchd job unchanged. VM `roosterm-spike`: isolated runs pass; live systemd install active, HTTP 200; hc core and a terminal program survive uninstall and re-install; stand-in unit shows KillMode=process keeps the child alive (default kills it).
- Review (opus): 1 blocker (uninstall could kill hc core), 2 should-fix (XDG config path, secrets visible to npm build), 3 nits — all fixed and re-checked: mergeable.

## Live release
- `app-v0.0.1` (merge commit 7a3417c, tag commit 7a647fa): GitHub Actions run 44870926585 succeeded in 11m9s; release "roosterm 0.0.1" has the 3 APKs + 4 server tarballs; downloaded `us.jhangy.roosterm-arm64-v8a.apk` → apksigner signer SHA-256 `f87d4271…17fd` (the fixed key).

## Not yet proven
- `loginctl enable-linger` prompt path (VM already had linger on).
