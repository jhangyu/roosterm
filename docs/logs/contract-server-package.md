# Contract: server release package, interactive installer, fixed APK signing key

## End state
A tag-triggered GitHub Actions release publishes, next to the per-ABI APKs, a complete server package per supported host platform; a user downloads it and runs one interactive Python installer that installs and configures the whole server side (binaries, hc, password, config, service). Release APKs are signed with one fixed key so app updates install in place.

## In scope
1. CI scripts (`scripts/ci.py` platform table) gain server package build(s); the workflow builds and uploads them in the same release. CI still never runs tests (guard stays and covers new commands).
2. Server package content: roosterm binary (with embedded web UI), the interactive installer, and whatever else install needs (service templates, README excerpt). Package naming consistent with the APK naming scheme.
3. Interactive Python installer (stdlib only): checks prerequisites, installs roosterm (and hc, as far as an install source is known), sets the login password, writes config with prompted values and sane defaults, installs and starts the service (launchd on macOS, systemd on Linux), prints the URL and next steps. Re-runnable (upgrade / reconfigure) without losing existing config or sessions unless the user chooses so. Non-interactive flags allowed if cheap, not required.
4. Fixed APK signing key: release builds sign with a fixed keystore supplied through GitHub Actions secrets; local `prepush` build still works without the secret (explicit, documented behavior, no silent fallback in CI: CI fails if the secret is missing).
5. README (both languages): install section switches to "download package → run installer"; manual steps move to an advanced/manual section.

## Out of scope
- Windows host. Changing host server behavior or the web UI. App features.
- Publishing to package managers (Homebrew, apt).

## Acceptance criteria
1. Guard RC=0 on the tree; red case: a test command added to the new server build path makes the guard fail.
2. Local build of every server package target produces the package files with the agreed names; package contents listed and checked (binary runs `--help` or version for the host's own platform).
3. Installer verified end to end in an isolated environment (temp HOME / scratch prefix, never the user's real ~/.local/bin, real config, real service, or real passwords): fresh install, re-run keeps config, service files rendered correctly (service not actually loaded unless isolated). Evidence log with commands and self-captured RC.
4. Signing: an APK built with a test keystore through the same code path verifies with `apksigner verify --print-certs` showing that keystore's certificate; CI path fails loudly when the secret is missing.
5. Workflow passes actionlint; release step uploads APKs + server packages exactly.
6. Fresh reviewer: no blockers left. Live cloud release only after user approval.
7. Commits with explicit pathspecs; no stray files; build outputs only in gitignored build dirs.

## User decisions
- Server targets: macOS and Linux only (darwin-arm64, darwin-amd64, linux-amd64, linux-arm64).
- hc: installer runs herdr's official script (https://releases.herdr.dev/core/install.sh) for 0.1.21 when missing; keeps an installed hc >= 0.1.21.
- Signing key: the lead generates it on the user's Mac (`~/.config/roosterm/release-key.jks`, password file beside it, user-only permissions) and sets the GitHub secrets; the user backs it up.
- Package name `roosterm-server-<os>-<arch>.tar.gz`; release title "roosterm X.Y.Z", tag stays `app-vX.Y.Z`.
- Linux service is live-tested once in the OrbStack VM `roosterm-spike` (isolated).

## Round budget
3 rounds.

## Parking lot
(empty)
