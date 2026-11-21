# Contract: README rewrite (host-first)

## End state
README.md and README.zh-TW.md lead with configuring and starting the host server, read well for a lightly technical user, point to the prebuilt Android APKs on GitHub Releases, and move app build instructions to a later section.

## In scope
1. Rewrite direction plan (opus) in `scratch/readme-rewrite/plan.md`.
2. README.md rewrite (English).
3. README.zh-TW.md rewrite (Traditional Chinese), same structure and facts as the English one.

## Out of scope
- Code, scripts, docs other than the two READMEs. Release notes. New screenshots/images.

## Acceptance criteria
1. Both READMEs: the first sections after the intro cover install/download → configure → start the host server → connect clients (web and the prebuilt APK from Releases, linking https://github.com/jhangyu/roosterm/releases). App build-from-source and CI/dev material come after usage.
2. Every command, path, flag, config key and default value in the new READMEs exists in the code or scripts (verified against the repo, not copied from the old README unchecked). Nothing from the old README that is still true and useful is dropped without the plan saying why.
3. The two READMEs have the same section structure (same headings count/order) and the same facts; language links between them still work.
4. Security notes stay (may move, may be reworded, may not be weakened).
5. A fresh reviewer (not a writer) confirms 1–4.
6. One commit with explicit pathspecs; no stray files.

## Round budget
2 rounds.

## Parking lot
- herdr-core installer URL: the README gives only the `install.sh` command; the user must supply the source URL.
- Hidden evidence comments differ slightly in wording between the two files.
- CI signs each APK with a fresh runner debug key; in-place app updates likely fail (unverified).
- `http://localhost:8787` quick-check tip is not browser-tested.

## Result
Two rounds. Review round 1: 1 blocker, 6 should-fix; round 2 re-check: all fixed, criteria 1–4 pass (17 `##` / 5 `###` in both, identical code blocks except diagram labels, all anchors resolve).
