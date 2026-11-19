# Design brief: folder picker for New session (2026-11-20)

## User request
When creating a new session, the user picks which folder under the host's home directory the session starts in.
- The next time the dialog opens, it remembers the last chosen path and opens with the tree/list already expanded to that path.
- After creation, the tab label defaults to `<folder name> · <Claude Code session summary>` (the summary updates as Claude Code reports it; before it exists, show the folder name alone).

## Constraints
- Visual language: Fluent 2 (components, typography, iconography, colours, elevation) with Material Design 3 corner-radius scale (extra-small 4, small 8, medium 12, large 16, extra-large 28 px). State which M3 radius token each surface uses.
- Must fit the existing app: chosen look is docs/mockups/3-calm-surfaces.html; current dialog is web/src/workspace/NewSessionDialog.tsx (session type radio: shell / Claude / swarm attach; hidden sessions list with "Show"). The picker extends this dialog; it does not replace the hidden-sessions list.
- Works on desktop (pointer) and phone (touch, ~390 px wide, soft keyboard may be open). Touch targets >= 44 px.
- Icons from the Fluent icon pack the project already uses (web/src/components/icons.tsx; `python3 ~/ui-kits/find-icon.py <keyword>`). No hand-drawn icons.
- Root is the host home directory (`~`); the user cannot leave it. Hidden (dot) folders: decide whether shown, toggle or not, and justify.
- States to show: loading a folder, empty folder, permission error on a folder, very long paths / names, deep nesting (6+ levels), many children (200+).
- The terminal itself is dark grey (#1e1e1e); app chrome stays light.

## Deliverable per designer
One self-contained HTML mockup in docs/mockups/ (static, no build), showing desktop and phone frames side by side, plus the resulting tab strip with a `<folder> · <summary>` label (including truncation of a long summary). Top of the file: a short comment block with the design rationale and the M3 radius mapping.
