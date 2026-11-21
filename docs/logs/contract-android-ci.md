# Contract: Android app CI scripts

## End state
roosterm has an Android-only, compile-only CI for the Flutter app in `app/`, modeled on the CI of `/Users/jhangyu/project/relume`, with all CI logic in refactored Python scripts that can gain other platforms later without rewriting.

## In scope
1. A GitHub Actions workflow that builds the Android app only (no other platform, no tests).
2. A Python CI guard script that fails the CI when any test would run in CI (test invocations in the workflow or in the CI scripts, test tasks in the build command, etc.). The workflow runs the guard before the build.
3. A Python CI build script (the only build entry the workflow calls) that compiles the Android app. Platform-specific parts sit behind one small per-platform table/registry so a new platform is one new entry, not a new code path. Only Android is registered.
4. Local pre-push equivalent of the CI (guard + build), if relume has one, following relume's pattern.
5. CI script `prepush` argument: runs guard + local Android build; must pass locally. Wired to `.githooks/pre-push`.
6. Version bump command in the CI scripts: bumps the app version, and pushing the result to GitHub triggers the Actions workflow (relume-style tag trigger). The cloud runner builds and then automatically publishes a GitHub Release with the APKs.
7. Release APKs are split per ABI; each file is named `{package name}-{arch}.apk` (package name = Android applicationId; arch = arm64-v8a / armeabi-v7a / x86_64).
8. Docs: one short section in the project README or CLAUDE.md-level docs describing how CI is triggered and run locally.

## Out of scope
- Any test in CI. Any non-Android platform implementation (only the extension point).
- Changing app source code (exception, user decision: `app/android/build.gradle.kts` and `app/android/app/build.gradle.kts` may change so the arm64-only limit is lifted only for the CI split-per-ABI build; normal local builds stay arm64-only), the Rust/Go host, `scripts/build.py`, `scripts/service.py`, `scripts/test.py` behavior.
- Release signing secrets setup on GitHub (document required secrets only, if any).
- Pushing to remote.

## Acceptance criteria
1. Every CI script is Python 3 stdlib only; the workflow YAML contains no inline shell logic beyond calling Python scripts (setup actions such as checkout / setup-java / flutter setup are allowed).
2. Guard: running it on the repo exits 0. Injecting a test invocation (e.g. `flutter test` in the workflow, or a `test` task in the build command) into a scratch copy makes it exit non-zero with a message naming the file and offending text. Red-then-green evidence recorded.
3. Build: the CI build script, run locally, produces the Android APK (release or the relume-equivalent variant) with RC=0; the build output goes only into the existing gitignored `app/build/`.
4. The build script contains no test task and builds only Android; adding a platform requires adding one registry entry (reviewer confirms by reading).
5. Workflow YAML parses (python yaml-free check or `actionlint` if installed) and references only scripts that exist.
6. One opus reviewer reviews the diff; all blockers fixed.
7. `prepush` run locally exits 0 and produces the per-ABI APKs with the required names (rename happens in the shared build code, so cloud and local produce identical names).
8. Bump command, run on a scratch copy / dry-run, updates the version and creates the tag the workflow listens on; the workflow's release step uploads exactly the per-ABI APKs. Live cloud run is done only after the user approves pushing.
9. Commit(s) on main with explicit pathspecs; no stray files; no new build directories outside `app/build/`.

## Round budget
3 rounds.

## Parking lot
- Guard does not inspect `uses:` action steps or Gradle `dependsOn(test)` in build files.
- A second platform's release job would collide on `gh release create` for the same tag.
