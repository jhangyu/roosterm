# Contract: security campaign (2026-11-21)

Base HEAD df3a695. Only the user may change this contract.
Working files (scan reports, designs, one-time exploit checks) live in the campaign scratch directory and are deleted at signoff; the result and evidence go to docs/logs/2026-11-21-security.md.

End state: every confirmed vulnerability of severity Medium or higher found by the scan is fixed on main with a check that proves the exploit no longer works, and the end gate passes on main.

Process (user request):
- Scan: 4 parallel scan teams, one per area (auth/session/config; REST API and hc/swarm command paths; WebSocket bridge and HTTP server; clients, deploy and dependencies).
- Design: per confirmed vulnerability, 2 architects; output one agreed fix design.
- Fix: per vulnerability, 1-3 implementers with file ownership boundaries.

Confirmed findings (designs agreed by both architects): V1 same-site CSRF on state-changing endpoints (High); V2 global login lockout DoS (Medium); V3 password change does not revoke sessions (Medium); V4 Android WebView navigation not restricted to the host (Medium); V5 Go 1.26.4 Range-header CPU DoS, GO-2026-6609 (Medium).

User decisions (2026-11-21):
- V5: the user upgraded the machine Go with Homebrew (now go1.27.4); host/go.mod moves to `go 1.26.11`.
- V4: links to other sites are blocked with a notice showing the real address; no url_launcher.
- V4: the user's reverse proxy does not send logins to another domain.
- V2: the known-device cookie lives 400 days.

In scope: vulnerabilities in host/, web/src/, app/lib/, app/android/ config, deploy/, scripts/, and dependency manifests, with a concrete attack scenario, severity Medium/High/Critical after lead deduplication.

Out of scope: Low/informational findings and hardening without a concrete attack (parking-lot, reported to the user); hc (herdr-core) itself; the reverse proxy; real Android release signing; the README's documented single-account design and "logged-in user gets a full shell" property.

Red lines: no probing of the running service (~/.local/bin/roosterm, port 8787) or the user's live hc; no `hc` command that creates or kills terminals; no change to the user's config.toml, password, or sessions.json; no deploy/restart of the service.

Acceptance criteria (run from repo root on main after the last fix):
1. `python3 scripts/test.py; echo RC=$?` → RC=0, output has `skipped: 0` and no "over 1.0 s" line.
2. `(cd web && npx tsc -b && npx oxlint src); echo RC=$?` → RC=0.
3. `(cd app && flutter analyze)` → contains `No issues found!` (only if app/ changed).
4. `python3 scripts/build.py; echo RC=$?` → RC=0, then `git status --porcelain` → empty.
5. Every in-scope finding has a design file and a fix commit; each fix has an exploit check that was seen red before the fix and green after (committed Go test when the fix is in host/, else a one-time scratch check recorded in the result log).
6. A fresh security reviewer that did not design or implement the fix confirms, per finding, that the attack scenario is blocked and no caller regressed.

Round budget: 3 (round = design + fix + review for the current finding set).
