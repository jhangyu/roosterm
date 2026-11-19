# Contract: folder picker and folder-named tabs (2026-11-20)

End state: New session lets the user pick a start folder under the host home with the 4b drill-down picker (remembered last folder, opens there), offers Shell / Claude Code / Swarm, and a new tab is labelled `<folder> · <Claude Code summary>` until the user renames it.

Design spec: docs/mockups/4b-folder-drill.html (final revision by designer-drill). User decisions: one-step open — the current (remembered) folder is the start folder and "Open … in <folder>" opens directly, navigating only to start elsewhere; Claude Code session type runs `claude` in the chosen folder and uses the picker; phone session type is one 48 px segmented row; duplicate folder-only labels are acceptable; a renamed tab no longer gets the summary; unlistable folders stay selectable with the error shown.

## API (frozen)
- `GET /api/dirs?path=<rel>` — `rel` is relative to the host home, `""` = home. 200 `{"path": "<rel>", "entries": [{"name": "...", "hidden": bool}]}` sorted by name, directories only (a symlink to a directory counts if it resolves inside home). Errors via apierr: 400 for absolute paths, `..`, NUL; 403 when the resolved path escapes home or listing is denied (permission denied keeps kind distinct from escape); 404 when missing or not a directory.
- `POST /api/tabs` gains optional `"cwd": "<rel>"` (same validation; default `""`). Default label = basename of cwd, or `~` for home. Claude Code type is the client sending `"command": ["claude"]`.
- `Tab` JSON gains `"cwd": "<rel>"`, `"title": "<hc terminal title>"`, `"renamed": bool`. `PATCH` with `label` sets `renamed` (terminal label `roosterm.renamed=1`).
- Last-used folder is remembered per device in the browser (localStorage), not on the server.

## In scope
1. Host: dirs endpoint, cwd on create, Tab fields above, tests.
2. Web: picker per 4b, wired into NewSessionDialog with the three session types, remembered folder.
3. Web: tab label shows `<folder> · <title>` when not renamed (title from the listing; the attached terminal's live title may refresh it sooner), folder alone while the title is empty.

## Out of scope
Flutter app changes; creating folders; server-side remembered folder.

## Acceptance
1. Go tests: dirs listing (sorted, dirs only, hidden flag), rejects `..`/absolute (400), symlink escaping home (403), missing (404); create with cwd sets hc cwd and default label; rename sets `renamed`. `python3 scripts/test.py` green.
2. `./node_modules/.bin/tsc -b` RC=0 in web/.
3. Live check on the deployed service: picker opens in the remembered folder after a reload; a Claude Code tab opened in `~/project/roosterm` shows `roosterm` then `roosterm · <summary>` once Claude Code sets its title; rename replaces it.
4. `scripts/build.py` RC=0 on the combined tree.

Round budget: 3.
