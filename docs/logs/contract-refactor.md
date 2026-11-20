# Contract: refactor campaign (2026-11-20)

Frozen by user approval 2026-11-20. Base HEAD 4867f96. Only the user may change this contract.


End state: every tier-A item and every user decision below is implemented as a behaviour-preserving refactor (except the swarm request shape and the folder-memory notice, which the user decided), and both end-of-round gates pass.

In scope (item ids from roi-host.md / roi-client.md / decisions.md):
- Host: H1, H2, H3, H9, H10, H11 (tier A + accepted B), H4, H6, H7, H12 (accepted B).
- Web: C1, C2, C3, C4, C5, C7 (A), C6, C8, C9, C11 (accepted B).
- App: C12, C13 (A).
- User decisions: D1 folder-memory visible notice, D2a delete non-Android webview branch, D2b delete rejected mockups, D2c keep dark-mode styles (no action), D3 docs/api.md extraction + delete plans, D4 explicit swarm field (replaces H8).

Out of scope: tier C (H13-H16, C15-C17); rejected B (section 5); the setState-after-dispose bug in app/lib/host_list_page.dart:23-33; config Load/Save test coverage gap (pf-host-edge-10); any rewrite of docs/logs/*.

Acceptance criteria (all run from repo root unless stated; each must hold on main after round 2):
1. `python3 scripts/test.py; echo RC=$?` → RC=0, output line `skipped: 0`.
2. `(cd web && npx tsc -b && npx oxlint src); echo RC=$?` → RC=0.
3. `(cd app && flutter analyze); echo RC=$?` → RC=0 and output contains `No issues found!`.
4. `python3 scripts/build.py; echo RC=$?` → RC=0 (run by the gate runner; may exceed 30 s).
5. `test -f docs/api.md && test ! -e docs/superpowers/plans && echo OK` → `OK`.
6. `grep -rln "superpowers/plans\|backend plan\|client plan" --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=scratch --exclude-dir=dist --exclude-dir=logs .` → empty.
7. `grep -rn "1-dense\|2-thumb\|4a-folder\|icon/drafts" --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=scratch --exclude-dir=logs .` → empty; `ls docs/mockups/3-calm-surfaces.html docs/mockups/4b-folder-drill.html` → both exist.
8. `grep -rn "IsAttachArgv\|\"-L\"" host web/src` → empty; `grep -c '"swarm"' docs/api.md` ≥ 1.
9. `grep -rnE 'func badRequest|func dirError|func \(e \*Error\) Error|MaxBody|DefaultSocket|dirExists|HashPasswordCost|firstPID' host --include='*.go'` → empty.
10. `grep -rnE 'Status: +http\.Status(BadRequest|NotFound|Unauthorized|InternalServerError)' host/internal --include='*.go' | grep -v _test.go | grep -v internal/apierr/apierr.go` → empty.
11. `grep -rn '".tmp"' host/internal --include='*.go'` → only host/internal/atomicfile/atomicfile.go.
12. `grep -rlw -E 'ScrollArea|CardHeader|AlertDialogMedia|AlertTitle|DialogHeader|ChevronDownIcon|readPhoneMetrics|showCloseButton' web/src` → empty; `test ! -e web/public/.gitkeep && echo OK` → `OK`.
13. `grep -rn "console.warn" web/src` → empty; `grep -rn "status === [0-9]" web/src` → empty.
14. `grep -c "CLAUDE_PROGRAM = " web/src/workspace/*.ts*` summed → 1; `grep -rn "getComputedStyle" web/src` → exactly 1 hit (lib/terminal-theme.ts).
15. `grep -rc "text-body leading-(--lh-body) font-semibold" web/src` summed → 1.
16. `grep -c "is AndroidWebViewController" app/lib/host_page.dart` → 0; `grep -c "A new Flutter project" app/README.md app/pubspec.yaml` → 0 each; `test ! -e app/assets/icons/chevron_right.svg && echo OK` → `OK`.
17. `grep -c "dark:" web/src/components/ui/button.tsx` ≥ 1 and `grep -c "@custom-variant dark" web/src/styles/globals.css` = 1 (D2c: dark styles kept).
18. `git status --porcelain` → empty after each round's commits (no stray files outside scratch/).

Round budget: 3 rounds (2 planned + 1 repair round). A third round only fixes failures of these criteria; no new items.

