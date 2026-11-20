# Contract: refactor campaign 2 (2026-11-21)

Frozen by user approval 2026-11-21. Base HEAD 1f840e8. Only the user may change this contract.
Plan and designs: scratch/refactor2/plan.md and the four architect files it names (scratch, deleted at signoff).

End state: every item of the two rounds in the plan, with the user decisions below, is implemented as a behaviour-preserving refactor or the stated bug fix, and both end-of-round gates pass on main.

User decisions:
- Lost-connection banner shows the clock time of the loss ("stopped responding at HH:MM:SS"); no live counter.
- No committed web test: the folder-highlight fix is proved once in scratch; scripts/test.py stays Go-only; web/test/ is not created.
- Docs: delete docs/research/; replace the original design spec with docs/design.md (English, ≤45 lines) and delete docs/superpowers/ and the spec-deviation table in docs/api.md.
- Delete the stray logs in build/ (keep build/roosterm and build/roosterm-linux-arm64) and delete host/build/.
- Keep the hc startup requirement for the "events" feature unchanged (no code or doc change).
- Add tests for config loading and `roosterm passwd` (hash cost as a parameter, cheap in tests).
- Android release stays debug-signed (sideload only): replace the two template TODOs in build.gradle.kts with one sentence. No "reset host list" button.
- Keep the class-variance-authority package.

In scope:
- Round 1: R1-SWARM, R1-TERM, R1-STATE, R1-UI-KIT, R1-PICKER (plan.md "Round 1").
- Round 2: R2-RECIPES, R2-AUTH, R2-CORE (BUG-4, B1, D5), R2-APP (BUG-3, B5, A15, signing comment), R2-DOCS (A5, D2, D3, D4).

Out of scope: tier C items of both ranked lists; the StatusFooter lost badge reusing the shared badge recipe; real Android release signing; dropping the hc "events" requirement; any web test runner; any rewrite of earlier docs/logs files. The old parking-lot item "Android host list setState after dispose" is closed without code (the page is never popped).

Acceptance criteria (run from repo root; each must hold on main after round 2; 1-4 also after round 1):
1. `python3 scripts/test.py; echo RC=$?` → RC=0, output has `skipped: 0` and no "over 1.0 s" line.
2. `(cd web && npx tsc -b && npx oxlint src); echo RC=$?` → RC=0.
3. `(cd app && flutter analyze)` → output contains `No issues found!`.
4. `python3 scripts/build.py; echo RC=$?` → RC=0, then `git status --porcelain` → empty and `git ls-files host/internal/web/dist` → `host/internal/web/dist/.gitkeep`.
5. `grep -rnE 'Setenv|exec\.Command\(|net\.Dial\(' host/internal/swarm/` → empty; `grep -rn 'MemberTarget' host` → empty.
6. `grep -c '{"swarm":""}' host/internal/api/api_test.go` → 1 (that test passes in criterion 1).
7. `grep -n 'filepath.Rel' host/internal/api/dirs.go` → empty.
8. `grep -rn 'ClaudeProgram' host` → empty; `grep -c 'swarm.Prefix' host/internal/api/tabs.go` → 1.
9. `grep -rnE 'Limiter\.Fail|func \(l \*Limiter\) Fail|\.Allow\(\)' host --include='*.go'` → empty; TestLimiterLockout covers parallel attempts that have not completed and passes in criterion 1.
10. `grep -c 'json.NewEncoder' host/internal/apierr/apierr.go` → 1.
11. `grep -c 'func (c \*Client) forget(id uint64) bool' host/internal/hc/client.go` → 1.
12. `grep -c 'Getenv("HOME")' host/internal/config/config.go` → 1; `grep -c 'UserHomeDir' host/cmd/roosterm/serve.go` → 1.
13. `grep -n contract scripts/*.py` → empty.
14. `grep -rnE 'ctrl-|SPECIAL_KEYS|\[color:var\(|\[var\(--|inset_0_0_0_1px|disconnectedAt|MS_PER_SECOND' web/src` → empty; `grep -c 'metrics()' web/src/terminal/TerminalPane.tsx` → 2.
15. `grep -rn 'withTitle\|readonly tabId\|ApiError(op, 0\|brand-140\|grey-90\|--radius:' web/src` → empty.
16. `test ! -e web/src/components/ui/badge.tsx && test ! -e web/src/components/ui/alert.tsx && echo OK` → OK; `grep -rnE 'data-size|data-\[size|card-footer|alert-dialog-media|field-label|group-data-\[disabled|use client' web/src` → empty; `grep -c 'dark:' web/src/components/ui/button.tsx` ≥ 1; `grep -c '@custom-variant dark' web/src/styles/globals.css` = 1.
17. `grep -c 'toLowerCase' web/src/workspace/FolderPicker.tsx` → 0; `grep -cE 'export function (matchRange|folderView)' web/src/workspace/folders.ts` → 2; `grep -cE 'tone=|tone ===|\[46, 62' web/src/workspace/FolderPicker.tsx` → 0; `test ! -e web/test && echo OK` → OK.
18. `grep -rn 'maxLength={64}' web/src` → empty; `grep -rn 'recent: \[\], showHidden: false' web/src | wc -l` → 1.
19. `test ! -e web/src/workspace/classes.ts && test -f web/src/components/classes.ts && echo OK` → OK; `grep -rnE 'const (ACTION|BANNER|TONES)\b' web/src/terminal web/src/workspace` → empty.
20. `grep -c 'on Exception catch' app/lib/host_list_page.dart` → 1; `grep -c 'class _Notice' app/lib/host_list_page.dart` → 0; `test -f app/lib/notice.dart`; `grep -cE 'useMaterial3|Roboto' app/lib/tokens.dart` → 0; `grep -c TODO app/android/app/build.gradle.kts` → 0.
21. `grep -c 'always sends `\$HOME`' docs/hc-bridge.md` → 0; `grep -c 'ScrubArgv' docs/hc-bridge.md` ≥ 1; `grep -c '"events"' host/internal/hc/client.go` → 1 (requirement kept).
22. `test ! -e docs/research && test ! -e docs/superpowers && test -f docs/design.md && echo OK` → OK; `grep -rn 'superpowers\|design spec' docs/api.md docs/hc-bridge.md docs/design.md` → empty; `wc -l < docs/design.md` ≤ 45.
23. `ls build` → exactly `roosterm` and `roosterm-linux-arm64`; `test ! -e host/build && echo OK` → OK.
24. `cd host && go test -count=1 -v ./internal/config ./cmd/roosterm` → RC=0 and output has `--- PASS` lines for the new config and passwd tests; criterion 1 output shows `tests:` greater than 56 (amended by user 2026-11-21: test.py never lists test names).
25. `git status --porcelain` → empty after each round's commits.

Round budget: 3 rounds (2 planned + 1 repair). The repair round only fixes failures of these criteria; no new items.
