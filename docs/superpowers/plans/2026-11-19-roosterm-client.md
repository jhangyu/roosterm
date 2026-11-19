# Roosterm Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use podium:team-spawn (recommended) or podium:team-fable to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Roosterm web UI (`web/`, embedded into the Go host) and the Flutter Android shell (`app/`) in one final round, after the user picks a mockup, so a user can log in, open tabs, and run programs from a browser or the app.

**Architecture:** A single-page React app talks to the host over same-origin REST (`/api/*`, `/login`, `/logout`) and one WebSocket per open tab (`/ws/tabs/{id}?cols&rows`). Only the active tab mounts an xterm.js terminal and a socket; switching tabs re-attaches, and hc sends the full screen. All visual values live in one token file; the Flutter app is a WebView around a locally stored host list.

**Tech Stack:** React 19, Vite 8, TypeScript 7, Tailwind CSS 4, shadcn/ui (Radix base), lucide icons, `@xterm/xterm` 6, `@xterm/addon-fit` 0.11; Flutter 3.44 with `webview_flutter` 4.14 and `shared_preferences` 2.5.

**Decisions (hard-to-reverse only):**
- *Socket per active tab only* vs socket per open tab. Drivers: spec says reconnect = re-attach and hc sends the full frame, so a kept-alive background socket buys nothing; one socket keeps reconnect logic single-path. Drawback: switching tabs costs one re-attach round trip; background-tab exit is learned from the tab list, not live.
- *No router, no data-fetching library, no state library.* Drivers: two screens (login, workspace), five endpoints. Drawback: if deep links per tab are needed later, add a hash parameter, not a router.
- *Icons: lucide via `lucide-react`.* Drivers: shadcn components already import `lucide-react` (`components.json` `"iconLibrary": "lucide"`), so any other pack would mix two packs in one UI. The same Lucide set is the `~/ui-kits/icons/lucide` pack (lucide-static 1.73.0); every icon name used below was checked to exist there with `find-icon.py` / `test -f ~/ui-kits/icons/lucide/default/<name>.svg`.
- *Flutter accepts only `https://` host URLs.* Driver: the session cookie is `Secure` (spec 資料流 1); on `http://` the WebView drops it and login loops silently. Drawback: LAN testing needs the reverse proxy or a TLS dev setup.

Plan gate: `/Users/jhangyu/.claude/plugins/cache/jhangyu/podium/1.1.1/protocols/review.md` §L0 Plan gate.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-11-19-roosterm-design.md`. Backend plan and API owner: `docs/superpowers/plans/2026-11-19-roosterm-backend.md` §"HTTP/WS API" (line 50, commit `86de08a`).
- 分頁內程式被終止而沒有結束代碼時（`exitCode` 為 null），顯示「已結束（signal N）」；兩者皆無時顯示「已結束（<reason>）」。
- 元件一律使用 shadcn/ui 現成元件，不手刻。圖示整個介面只用同一套圖示包的同一風格 (lucide, stroke style).
- 特殊按鍵列（Esc、Tab、Shift+Tab、Ctrl+C、Ctrl+D、方向鍵）放在網頁內，手機瀏覽器與 App 共用同一實作。No native key bar in Flutter.
- 重新連線: on WebSocket close show「已斷線」banner with a reconnect button; auto-reconnect only on `online` and `visibilitychange` (visible). No timer retry, no backoff loop.
- 分頁內程式結束時，分頁顯示「已結束（code N）」，由使用者關閉。
- 所有錯誤明確顯示，不默默降級。錯誤訊息包含操作名稱、受影響的分頁 id、期望值與實際值。Server error text is shown verbatim.
- Node is build-time only; the host runtime never needs Node. Build output is embedded with `go:embed`.
- Test policy: no UI, browser, or screenshot tests are committed. `web/` and `app/` add nothing to `scripts/test.py`. Verification is `npm run build` / `flutter analyze` / `flutter build apk` exit codes plus one live run per package.
- No `sleep`, polling, `setInterval`, or `setTimeout`-based retry anywhere in `web/` or `app/` (grep-checked in acceptance). The keepalive ping is the server's job (WebSocket ping frames; browsers answer pongs automatically).
- Visual values (colors, fonts, radius, terminal palette) appear only in `web/src/styles/tokens.css`. Components use Tailwind semantic classes (`bg-background`, `text-muted-foreground`, `rounded-md` …), never raw color literals (grep-checked).
- Unreleased project: no backward compatibility, no dead code, no commented-out code.
- Shared git tree: no stash (including `stash list`), reset, `checkout --`, clean, `git add -A`. Commit with `git add <own paths>` then `git commit -m "..." -- <own paths>`; no `;` inside `-m`.

## Verified dependency versions

Looked up 2026-11-19 with `npm view <pkg> version` and the pub.dev API (`https://pub.dev/api/packages/<pkg>` → `latest`). APIs named in this plan were checked against these versions' shipped typings/README (xterm `typings/xterm.d.ts`, addon-fit `typings/addon-fit.d.ts`, webview_flutter and shared_preferences README from the pub archive) or context7 (`/shadcn-ui/ui/shadcn_4.21.0`, `/vitejs/vite`).

| Package | Version | Notes |
|---|---|---|
| create-vite | 9.3.4 | `template-react-ts` pins `typescript ~7.0.2`, `vite ^8.3.6`, `@vitejs/plugin-react ^6.1.2`, script `"build": "tsc -b && vite build"` |
| react, react-dom | 19.3.1 | |
| @types/react, @types/react-dom | 19.3.0 | |
| vite | 8.3.8 | engines node `^20.19.0 \|\| >=22.12.0` (local: v25.9.0) |
| @vitejs/plugin-react | 6.3.0 | peer `vite ^8.0.0`; other peers optional |
| typescript | 7.0.2 | `tsc` bin present |
| tailwindcss, @tailwindcss/vite | 4.3.5 | plugin peer `vite ^5.2.0 \|\| ^6 \|\| ^7 \|\| ^8` |
| shadcn (CLI) | 4.23.0 | `init -b radix`, `add <components>` |
| radix-ui | 1.8.1 | installed by shadcn for the Radix base |
| class-variance-authority | 0.7.1 | installed by shadcn |
| clsx | 2.1.1 | installed by shadcn |
| tailwind-merge | 3.7.0 | installed by shadcn |
| tw-animate-css | 1.4.0 | installed by shadcn |
| lucide-react | 1.75.0 | peer react `^19.0.0` ok |
| @types/node | 26.6.6 | for `path` in `vite.config.ts` |
| @xterm/xterm | 6.0.0 | CSS at `@xterm/xterm/css/xterm.css` |
| @xterm/addon-fit | 0.11.0 | no peer deps declared |
| webview_flutter | 4.14.1 | sdk `^3.10.0`, flutter `>=3.38.0`; pulls webview_flutter_android 4.14.1 (flutter `>=3.44.0`) |
| shared_preferences | 2.5.6 | sdk `^3.11.0`, flutter `>=3.41.0`; use `SharedPreferencesAsync` |
| Flutter SDK (local) | 3.44.6 | satisfies all floors above |

Shadcn-installed versions are whatever `shadcn@4.23.0 add` resolves; the implementer records the resulting `package.json` versions in the commit, not from this table.

## HTTP/WS API used by the client

Owner: `arch-backend-opus`. Status: **RECONCILED** with `docs/superpowers/plans/2026-11-19-roosterm-backend.md` §"HTTP/WS API" (line 50) at commit `86de08a`. That section is authoritative; this table lists only what the client uses. All wire details are encoded in `web/src/api/*` only.

| Use | Request | Response the client relies on |
|---|---|---|
| Log in | `POST /login` `{"username": string, "password": string}` | `204` + `Set-Cookie: roosterm_session=…; Path=/; Max-Age=<session_ttl s>; HttpOnly; Secure; SameSite=Strict` (Max-Age ⇒ Android WebView keeps it across restarts); `401` kind `unauthorized`; `429` kind `too_many_attempts` + header `Retry-After: <integer seconds ≥1>` |
| Log out | `POST /logout` | `204`, cookie cleared (`Max-Age=0`) |
| Session check | `GET /api/tabs` | `401` ⇒ show login (no whoami endpoint) |
| List tabs | `GET /api/tabs` | `200` `Tab[]` ordered by creation; wire `Tab = {"id": string, "label": string, "exited": boolean, "exitCode": number\|null, "signal": number\|null, "reason": string\|null, "hidden": boolean, "cwd": string, "title": string, "program": string, "renamed": boolean}`; `reason` ∈ `exited`, `killed`, `server_stop`, `host_shutdown`, `holder_lost`, `stopped_by_restart`, `null` while live |
| New tab | `POST /api/tabs` `{"command"?: string[], "label"?: string}`; `command` omitted ⇒ `[config.shell, "-l"]`; `label` omitted ⇒ basename of argv[0]; label 1–64 chars, no control chars | `201` `Tab` |
| Swarm tab | `POST /api/tabs` `{"command": ["tmux","-L",<name>,"a"], "label": <name>}` | `201` `Tab` |
| Rename / hide | `PATCH /api/tabs/{id}` `{"label"?: string, "hidden"?: boolean}` (at least one) | `200` `Tab` |
| Folders | `GET /api/dirs?path=<rel to home>` | `200` `{"path", "entries": [{"name", "hidden"}]}` |
| Close | `DELETE /api/tabs/{id}` (live ⇒ kill then remove) | `204` |
| Swarms | `GET /api/swarms` | `200` sorted `string[]`, e.g. `"claude-swarm-4242"` |
| Errors | any non-2xx JSON response | `{"error": {"op": string, "tab"?: string, "message": string, "kind": string, "hint"?: string}}`; kinds: `bad_request` (400/405/413), `unauthorized` (401), `not_found` (404, also unknown `/api/` path), `too_many_attempts` (429), `core_offline` (503), hc kinds (502), `internal` (500). A non-JSON body (e.g. a proxy page) is shown raw |
| Attach | `WS /ws/tabs/{id}?cols=N&rows=N` (both required, 1..1000; cookie checked before upgrade ⇒ a 401/400 is seen by the browser as close `1006`) | server→client binary = output; the first binary message after each (re)attach is the full screen (starts with `\x18\x1b[!p`), written as-is, no client reset |
| Input / resize | client→server binary = keystroke bytes (max 1 MiB); text `{"type":"resize","cols":N,"rows":N}` | — |
| Core state | server→client text `{"type":"core","state":"offline"}` / `{"type":"core","state":"online"}`; socket stays open; after `online` a full-screen binary follows | ⇒ banner「主機核心離線」while offline, cleared on online |
| Server error message | server→client text `{"type":"error","op","tab","kind","message"}` — sent per dropped input while offline, and before every 4400/4500 close | ⇒ shown in the banner |
| Close codes | `4000` reason JSON `{"reason": string, "exitCode": number\|null, "signal": number\|null}` (also immediately when the tab is already finished); `4004` `tab <id> not found`; `4400` `bad message: <detail>`; `4500` hc message (≤123 bytes); `1001` `server shutdown`; anything else (e.g. `1006`) = connection lost | ⇒ exited / tab gone / protocol error / attach error / host stopping / disconnected |
| Keepalive | server WebSocket pings every `ping_interval`; browser answers | client sends no app-level ping |
| Dev server | `roosterm serve` default listen `127.0.0.1:8787` | Vite dev proxy target |

## File map

```
web/
├─ package.json, package-lock.json, tsconfig*.json, index.html, components.json   WP1
├─ vite.config.ts                       WP1  outDir + dev proxy
├─ src/main.tsx                         WP1  mounts <App/>, imports globals.css + xterm.css
├─ src/styles/tokens.css                WP1  THE token file (only raw visual values)
├─ src/styles/globals.css               WP1  Tailwind + @theme inline mapping (never edited per mockup)
├─ src/lib/utils.ts                     WP1  shadcn `cn()`
├─ src/components/ui/*.tsx              WP1  shadcn components (generated, not hand-edited)
├─ src/api/types.ts                     WP2  Tab, ApiError
├─ src/api/http.ts                      WP2  REST calls
├─ src/api/tab-socket.ts                WP2  TabSocket (one WS per attach)
├─ src/lib/keys.ts                      WP2  special-key sequences
├─ src/lib/resume.ts                    WP2  online/visibilitychange subscription
├─ src/lib/terminal-theme.ts            WP2  reads --term-* tokens into an xterm ITheme
├─ src/terminal/TerminalPane.tsx        WP3  xterm + fit + socket lifecycle + states
├─ src/terminal/KeyBar.tsx              WP3
├─ src/terminal/pane-state.ts           WP3  PaneState union
├─ src/terminal/StatusBanner.tsx        WP3  disconnected / core offline / exited / errors
├─ public/.gitkeep                      WP1  copied by Vite into dist/ so the embed dir stays non-empty and unchanged in git
├─ src/App.tsx                          WP4  login gate + workspace state
├─ src/screens/LoginScreen.tsx          WP4
├─ src/workspace/TabBar.tsx             WP4  tabs, rename, close, log out
├─ src/workspace/NewTabDialog.tsx       WP4  shell / swarm picker
└─ src/workspace/ErrorAlert.tsx         WP4  shows ApiError verbatim
app/                                    WP5  Flutter project (whole directory)
host/internal/web/dist/index.html       WP1  delete the backend placeholder (git rm); replaced by .gitkeep + build output
host/internal/web/dist/.gitkeep         WP1  committed, empty
.gitignore                              WP1  web/node_modules/, host/internal/web/dist/* except .gitkeep
```

`scripts/build.py` is not touched: the backend version already runs `npm ci --prefix web` and `npm run build --prefix web` when `web/package.json` exists (backend plan line 2919).

## Token file design

`web/src/styles/tokens.css` is the only file that changes when the user picks a mockup. It declares raw values on `:root` (and `.dark` only if the mockup has both themes); nothing else in `web/` contains a color, font name, or radius literal.

```css
/* web/src/styles/tokens.css — mockup swap point. Values below are shadcn neutral-dark defaults. */
:root {
  /* shadcn semantic colors (names fixed by shadcn components) */
  --background: oklch(0.145 0 0);
  --foreground: oklch(0.985 0 0);
  --card: oklch(0.205 0 0);
  --card-foreground: oklch(0.985 0 0);
  --popover: oklch(0.205 0 0);
  --popover-foreground: oklch(0.985 0 0);
  --primary: oklch(0.922 0 0);
  --primary-foreground: oklch(0.205 0 0);
  --secondary: oklch(0.269 0 0);
  --secondary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.269 0 0);
  --muted-foreground: oklch(0.708 0 0);
  --accent: oklch(0.269 0 0);
  --accent-foreground: oklch(0.985 0 0);
  --destructive: oklch(0.704 0.191 22.216);
  --border: oklch(1 0 0 / 10%);
  --input: oklch(1 0 0 / 15%);
  --ring: oklch(0.556 0 0);
  /* app-specific semantic colors */
  --warning: oklch(0.795 0.184 86.047);
  --warning-foreground: oklch(0.205 0 0);

  /* shape and type */
  --radius: 0.625rem;                 /* shadcn derives sm..4xl from this */
  --app-font-sans: ui-sans-serif, system-ui, sans-serif;
  --app-font-mono: ui-monospace, "SF Mono", Menlo, monospace;
  --terminal-font-size: 14;           /* unitless px, read by terminal-theme.ts */
  --terminal-line-height: 1.2;        /* unitless, read by terminal-theme.ts */

  /* terminal palette: #rrggbb or #rrggbbaa only (xterm parses hex; other formats must be opaque) */
  --term-background: #0a0a0a;
  --term-foreground: #e5e5e5;
  --term-cursor: #e5e5e5;
  --term-selection: #ffffff40;
  --term-black: #000000;   --term-bright-black: #666666;
  --term-red: #cd3131;     --term-bright-red: #f14c4c;
  --term-green: #0dbc79;   --term-bright-green: #23d18b;
  --term-yellow: #e5e510;  --term-bright-yellow: #f5f543;
  --term-blue: #2472c8;    --term-bright-blue: #3b8eea;
  --term-magenta: #bc3fbc; --term-bright-magenta: #d670d6;
  --term-cyan: #11a8cd;    --term-bright-cyan: #29b8db;
  --term-white: #e5e5e5;   --term-bright-white: #ffffff;
}
```

- `globals.css` (WP1) holds `@import "tailwindcss"; @import "tw-animate-css"; @import "shadcn/tailwind.css"; @import "./tokens.css";` and the shadcn `@theme inline` block mapping `--color-*` → the vars above, plus `--color-warning`, `--color-warning-foreground`, `--font-sans: var(--app-font-sans)`, `--font-mono: var(--app-font-mono)`, and the radius scale (`--radius-sm: calc(var(--radius) * 0.6)` … `--radius-4xl: calc(var(--radius) * 2.6)`, verified against shadcn 4.21 theming docs). WP1 moves the `:root`/`.dark` color blocks that `shadcn init` writes into `tokens.css` and deletes them from `globals.css`.
- Mockup swap procedure (post-round, one file): replace values in `tokens.css`; if the mockup uses a web font, add one `<link>` in `index.html`. Acceptance for a swap: `git diff --stat` touches only `web/src/styles/tokens.css` (and optionally `web/index.html`).
- xterm cannot read CSS variables, so `terminal-theme.ts` (WP2) reads them once via `getComputedStyle(document.documentElement)` when a terminal is created.

## Build integration

- Vite output directory: **`host/internal/web/dist/`** = the backend's `//go:embed all:dist` in `host/internal/web/static.go` (backend plan Task 6). Set in `web/vite.config.ts` as `build.outDir: path.resolve(__dirname, "../host/internal/web/dist")` with `build.emptyOutDir: true` (required: the dir is outside the Vite root, and Vite does not empty it by default — verified in Vite `resolveEmptyOutDir`).
- `scripts/build.py` (backend-owned, unchanged by this round): if `web/package.json` exists ⇒ `npm ci --prefix web`, then `npm run build --prefix web` (runs `tsc -b && vite build`), then `go build -trimpath -o build/<name> ./cmd/roosterm` in `host/`; any non-zero RC stops the build with that RC.
- Git: built assets are never committed. The backend placeholder `host/internal/web/dist/index.html` would be overwritten by every build (dirty tree), so WP1 replaces it: `git rm` the placeholder, commit an empty `host/internal/web/dist/.gitkeep`, add `web/public/.gitkeep` (Vite copies `public/` into `outDir`, so the emptied dir gets the same empty file back), and ignore the rest with `.gitignore` lines `host/internal/web/dist/*` and `!host/internal/web/dist/.gitkeep`. `all:dist` embeds dotfiles, so `go build`/`go test` still compile without Node; backend tests do not read the placeholder (checked: no static-route assertion in backend Task 6 tests). Mechanical check: after `python3 scripts/build.py`, `git status --porcelain host/internal/web/dist` prints nothing.
- Dev loop: `npm run dev --prefix web` with `server.proxy` forwarding `/api`, `/login`, `/logout` to `http://127.0.0.1:8787` (backend default `listen`) and `/ws` with `ws: true`. Same-origin in production, so the client never builds absolute URLs. The session cookie is `Secure`; browsers accept it on `http://localhost`/`127.0.0.1`, so open the dev server as `http://localhost:5173`.

---

## Round structure (one final round)

Preconditions to start the round: (a) the user picked a mockup, (b) backend `host/` is implemented per `86de08a` and serves the embed dir, `/login`, `/logout`, `/api`, `/ws`.

| Package | Owner files | Model | Depends on | Starts |
|---|---|---|---|---|
| WP1 web scaffold + tokens + build | `web/` config files, `web/public/.gitkeep`, `web/src/main.tsx`, `web/src/styles/*`, `web/src/lib/utils.ts`, `web/src/components/ui/*`, `host/internal/web/dist/{index.html (delete), .gitkeep}`, `.gitignore` web lines | sonnet | — | first; mechanical, ~30 min |
| WP2 API + terminal libs | `web/src/api/*`, `web/src/lib/keys.ts`, `web/src/lib/resume.ts`, `web/src/lib/terminal-theme.ts` | sonnet | WP1 committed (needs `package.json`) | after WP1 |
| WP3 terminal pane | `web/src/terminal/*` | sonnet | WP2 interfaces (frozen below); WP1 shadcn components | after WP1, parallel with WP2/WP4 |
| WP4 shell screens | `web/src/App.tsx`, `web/src/screens/*`, `web/src/workspace/*` | sonnet | WP2, WP3 interfaces (frozen below) | after WP1, parallel with WP2/WP3 |
| WP5 Flutter app | `app/**` | sonnet | none on web code; live check needs a host with the web UI | immediately, parallel with WP1 |

Serial evidence: WP2–WP4 need WP1's `package.json`, `tsconfig`, and generated `components/ui/*` to typecheck, so WP1 lands first. WP2–WP4 then run in parallel against the frozen interfaces; each typechecks with `npm run build --prefix web`, which needs all imported modules present, so WP3/WP4 may create a throwaway stub only in their own working copy and must not commit files outside their owned paths. The live web acceptance (L1–L6 below) runs once, after WP2–WP4 are all committed, by a fresh verifier (not an implementer).

### Round-level live acceptance (fresh verifier, after WP1–WP4 commit)

Run against a real `roosterm serve` + hc (backend contract criterion 3 setup), browser = desktop Chrome, then the same on a phone browser or the WP5 app. Record each as command/action, observed result, HEAD hash.

- L1 `python3 scripts/build.py` RC=0, then `ls host/internal/web/dist/index.html` exists, `git status --porcelain host/internal/web/dist` prints nothing, and the built binary serves the app at `/`.
- L2 Wrong password ⇒ error text shows `unauthorized` and the server message; 5 more wrong attempts ⇒ 「嘗試次數過多，請於 N 秒後再試」; correct password (after lockout) ⇒ workspace with tab list. 登出 ⇒ login screen; reload ⇒ still login screen (cookie cleared).
- L3 New tab ⇒ shell prompt; type `echo $$` ⇒ PID printed; `cd /tmp && pwd` ⇒ `/tmp`; run `claude` ⇒ UI renders (spec 需求 4).
- L4 Key bar: in `cat -v`, Esc/Tab/Shift+Tab/arrows print `^[`, tab, `^[[Z`, `^[[A`…; Ctrl+C interrupts `sleep 100` (typed in the remote shell); Ctrl+D on empty prompt exits the shell ⇒ pane shows「已結束（code 0）」 and a close button that removes the tab.
- L5 Disconnect: kill `roosterm` (SIGKILL) ⇒ banner「已斷線」appears; restart it, press 重新連線 ⇒ same tab re-attached, `echo $$` prints the same PID as L3. Toggle the machine's network off/on ⇒ reconnect happens without pressing the button.
- L6 Swarm: with a `tmux -L claude-swarm-<n>` server running, New tab dialog lists exactly the `claude-swarm-*` sockets; picking one opens a tab labelled with the socket name, attached to it.
- L6b Core restart: with a tab open, restart the hc core (backend live-proof procedure) ⇒ banner「主機核心離線」appears with the socket still open and the key bar disabled, then the banner clears by itself when the core is back and the screen is redrawn; `echo $$` prints the same PID.
- L6c Killed tab: in a second browser window, close a running tab via its X ⇒ the first window's pane shows「已結束（signal N）」/ the reason (close 4000) or「分頁不存在」(close 4004) — never「協定錯誤」.
- L6d Unknown tab: open `/ws/tabs/nope?cols=80&rows=24` from the browser console with `new WebSocket(...)` ⇒ close code 4004 (confirms the code the pane maps to「分頁不存在」).
- L7 Grep gates: `grep -rnE "setInterval|setTimeout|sleep\(" web/src app/lib` ⇒ no matches; `grep -rnE "#[0-9a-fA-F]{3,8}\b|oklch\(|rgb\(" web/src --include=*.tsx --include=*.ts` ⇒ no matches; `grep -rn "from \"lucide-react\"" web/src | grep -v components/ui` lists only lucide imports (no other icon package in `package.json`).

---

### Task WP1: Web scaffold, tokens, build integration

**Files:**
- Create: `web/package.json`, `web/package-lock.json`, `web/tsconfig.json`, `web/tsconfig.app.json`, `web/tsconfig.node.json`, `web/index.html`, `web/vite.config.ts`, `web/components.json`, `web/src/main.tsx`, `web/src/styles/tokens.css`, `web/src/styles/globals.css`, `web/src/lib/utils.ts`, `web/src/components/ui/{button,input,label,card,dialog,alert-dialog,alert,badge,scroll-area}.tsx`
- Create: `web/public/.gitkeep` (empty), `host/internal/web/dist/.gitkeep` (empty)
- Delete: `host/internal/web/dist/index.html` (backend placeholder, `git rm`)
- Modify: `.gitignore` (add `web/node_modules/`, `host/internal/web/dist/*`, `!host/internal/web/dist/.gitkeep`). `scripts/build.py` is not modified (already runs the web build).
- Delete from scaffold: `web/src/App.css`, `web/src/index.css`, `web/src/assets/`, `web/public/vite.svg`, template `README.md`, `oxlint` config and dependency (no lint gate requested; dead config)

**Interfaces:**
- Produces: import alias `@/` → `web/src/`; `cn(...classes)` from `@/lib/utils`; shadcn components at `@/components/ui/<name>`; Tailwind semantic classes `bg-background text-foreground bg-card text-muted-foreground bg-destructive text-destructive bg-warning text-warning-foreground border-border rounded-{sm,md,lg,xl} font-sans font-mono`; CSS vars `--term-*`, `--app-font-mono`, `--terminal-font-size`, `--terminal-line-height` on `:root`; placeholder `web/src/App.tsx` is NOT created here (WP4 owns it) — `main.tsx` imports `./App` and WP1 verifies the build with a local uncommitted stub.

**Behavior:**
- `index.html`: `<html lang="zh-Hant" class="dark">`, `<meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content">` so the mobile keyboard resizes the layout and the key bar stays visible; `<title>Roosterm</title>`.
- `main.tsx` imports `@xterm/xterm/css/xterm.css` then `./styles/globals.css`, renders `<App/>` into `#root` in `StrictMode`.
- `body` and `#root` fill `100dvh`, no page scroll (`html, body, #root { height: 100dvh; overflow: hidden }` in `globals.css` `@layer base`).

**Constraints:**
- Scaffold with `create-vite@9.3.4` `react-ts` and `shadcn@4.23.0 init -b radix`; no other UI library, no router, no state library.
- Runtime deps after this task, exactly: react, react-dom, @xterm/xterm, @xterm/addon-fit, lucide-react, plus what `shadcn` adds (radix-ui, class-variance-authority, clsx, tailwind-merge, tw-animate-css, shadcn). Dev deps: typescript, vite, @vitejs/plugin-react, tailwindcss, @tailwindcss/vite, @types/react, @types/react-dom, @types/node.

**Acceptance criteria:**
- [ ] With a local stub `web/src/App.tsx` (`export default function App() { return null }`, not committed): `npm ci --prefix web && npm run build --prefix web` RC=0 and `host/internal/web/dist/index.html` exists.
- [ ] `grep -c -- "--term-" web/src/styles/tokens.css` ≥ 20; `grep -nE "oklch\(|#[0-9a-fA-F]{6}" web/src/styles/globals.css` ⇒ no matches.
- [ ] `grep -n "emptyOutDir: true" web/vite.config.ts` and `grep -n "ws: true" web/vite.config.ts` both match.
- [ ] After the stub build, `git status --porcelain host/internal/web/dist` prints nothing (`.gitkeep` restored by Vite's `public/` copy, build output ignored).
- [ ] `cd host && go build ./... && go vet ./internal/web` RC=0 with only `.gitkeep` checked out in `dist/` (embed pattern still matches).
- [ ] `git diff HEAD~1 --stat -- scripts/build.py` is empty; `git show --stat HEAD` lists only WP1 owned paths.

**Steps:**

- [ ] **Step 1: Scaffold Vite React TS**

```bash
cd /Users/jhangyu/project/roosterm
npm create vite@9.3.4 web -- --template react-ts --no-interactive
rm -rf web/src/App.css web/src/index.css web/src/assets web/public/vite.svg web/README.md web/.oxlintrc.json
npm uninstall --prefix web oxlint
npm install --prefix web @xterm/xterm@6.0.0 @xterm/addon-fit@0.11.0 lucide-react@1.75.0
npm install --prefix web -D tailwindcss@4.3.5 @tailwindcss/vite@4.3.5 @types/node@26.6.6
```

Remove the `"lint"` script from `web/package.json`. If `--no-interactive` is rejected by create-vite 9.3.4, run without it and answer the prompts with the same template; record which form worked in the commit message body.

- [ ] **Step 2: Write `web/vite.config.ts`**

```ts
import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// Must equal the Go host's `//go:embed all:dist` directory (host/internal/web/static.go).
const EMBED_DIR = path.resolve(__dirname, "../host/internal/web/dist")
// Default `listen` of `roosterm serve` (backend config); the dev server proxies to it.
const HOST_DEV_URL = "http://127.0.0.1:8787"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  build: { outDir: EMBED_DIR, emptyOutDir: true },
  server: {
    proxy: {
      "/api": HOST_DEV_URL,
      "/login": HOST_DEV_URL,
      "/logout": HOST_DEV_URL,
      "/ws": { target: HOST_DEV_URL, ws: true },
    },
  },
})
```

Add to `web/tsconfig.json` and `web/tsconfig.app.json` `compilerOptions`: `"baseUrl": "."` (only if TS 7 still requires it for `paths`; drop it if `tsc -b` reports it as removed) and `"paths": { "@/*": ["./src/*"] }`.

- [ ] **Step 3: Init shadcn and add components**

```bash
cd web
npx shadcn@4.23.0 init -b radix
npx shadcn@4.23.0 add -y button input label card dialog alert-dialog alert badge scroll-area
```

Check `components.json` has `"iconLibrary": "lucide"`, `"tailwind": {"css": "src/styles/globals.css", "cssVariables": true}`. If init wrote the CSS elsewhere, move it to `src/styles/globals.css` and update `components.json`.

- [ ] **Step 4: Split tokens out of globals.css**

Create `web/src/styles/tokens.css` with exactly the block in "Token file design" above. Edit `globals.css`: delete the `:root { … }` and `.dark { … }` value blocks that init generated, add `@import "./tokens.css";` after the existing imports, and add inside `@theme inline`:

```css
  --color-warning: var(--warning);
  --color-warning-foreground: var(--warning-foreground);
  --font-sans: var(--app-font-sans);
  --font-mono: var(--app-font-mono);
```

and inside `@layer base`:

```css
  html, body, #root { height: 100dvh; overflow: hidden; }
```

- [ ] **Step 5: `index.html` and `main.tsx`**

`index.html` head/body per Behavior. `web/src/main.tsx`:

```tsx
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import "@xterm/xterm/css/xterm.css"
import "./styles/globals.css"
import App from "./App"

const root = document.getElementById("root")
if (!root) throw new Error("main: expected element #root in index.html, found none")
createRoot(root).render(<StrictMode><App /></StrictMode>)
```

- [ ] **Step 6: Embed dir and ignore rules**

```bash
git rm host/internal/web/dist/index.html
touch host/internal/web/dist/.gitkeep web/public/.gitkeep
```

Append to `.gitignore` with the Edit tool:

```
web/node_modules/
host/internal/web/dist/*
!host/internal/web/dist/.gitkeep
```

Do not edit `scripts/build.py`; confirm it already has the web step: `grep -n "npm" scripts/build.py` shows `ci --prefix web` and `run build --prefix web`.

- [ ] **Step 7: Verify with a local stub, then remove it**

```bash
cd host && go build ./... > ../scratch/wp1-go.log 2>&1; echo "RC=$?" >> ../scratch/wp1-go.log; cd ..
printf 'export default function App() { return null }\n' > web/src/App.tsx
npm ci --prefix web > scratch/wp1-build.log 2>&1; echo "RC=$?" >> scratch/wp1-build.log
npm run build --prefix web >> scratch/wp1-build.log 2>&1; echo "RC=$?" >> scratch/wp1-build.log
ls host/internal/web/dist/index.html host/internal/web/dist/.gitkeep
git status --porcelain host/internal/web/dist
rm web/src/App.tsx
```

Expected: `wp1-go.log` ends `RC=0` (Go embed compiles with only `.gitkeep`); both `RC=0` lines in `wp1-build.log`; both files listed; `git status` prints nothing. If `.gitkeep` is missing after the build (Vite did not copy the dotfile), report BLOCKED with the `ls -la host/internal/web/dist` output instead of changing `emptyOutDir`.

- [ ] **Step 8: Commit**

```bash
git add web/package.json web/package-lock.json web/tsconfig.json web/tsconfig.app.json web/tsconfig.node.json web/index.html web/vite.config.ts web/components.json web/public/.gitkeep web/src/main.tsx web/src/styles web/src/lib/utils.ts web/src/components/ui host/internal/web/dist/.gitkeep .gitignore
git commit -m "feat(web): scaffold Vite React shadcn app with token file" -- web host/internal/web/dist .gitignore
```

`git rm` in Step 6 already staged the placeholder deletion; the pathspec `host/internal/web/dist` includes it.

---

### Task WP2: API client, tab socket, key sequences, resume, terminal look

**Files:**
- Create: `web/src/api/types.ts`, `web/src/api/http.ts`, `web/src/api/tab-socket.ts`, `web/src/lib/keys.ts`, `web/src/lib/resume.ts`, `web/src/lib/terminal-theme.ts`

**Interfaces:**
- Consumes: WP1 alias `@/`, CSS vars `--term-*`, `--app-font-mono`, `--terminal-font-size`, `--terminal-line-height`.
- Produces (frozen; WP3/WP4 code against these exact names):
  - `type TabEnd = { reason: string; exitCode: number | null; signal: number | null }`
  - `type Tab = { id: string; label: string; end: TabEnd | null }` — `end === null` = running.
  - `parseTabEnd(v: unknown): TabEnd | undefined` — `undefined` = shape mismatch (caller reports it).
  - `class ApiError extends Error { op: string; status: number; kind: string | null; detail: string; tabId: string | undefined; retryAfter: string | null }` — `status` 0 = network failure; `kind` = server error kind, `null` when the body was not the JSON error shape.
  - `login(username: string, password: string): Promise<void>`, `logout(): Promise<void>`
  - `listTabs(): Promise<Tab[]>`, `createTab(request: { command?: string[]; label?: string }): Promise<Tab>`, `renameTab(id: string, label: string): Promise<Tab>`, `closeTab(id: string): Promise<void>`, `listSwarms(): Promise<string[]>`
  - `type SocketEnd = { kind: "exited"; end: TabEnd } | { kind: "not-found"; reason: string } | { kind: "server-error"; code: number; detail: string } | { kind: "server-shutdown" } | { kind: "disconnected"; code: number; reason: string } | { kind: "protocol-error"; message: string }`
  - `type CoreState = "offline" | "online"`
  - `type TabSocketHandlers = { onOpen: () => void; onOutput: (data: Uint8Array) => void; onCore: (state: CoreState) => void; onServerError: (message: string) => void; onEnd: (end: SocketEnd) => void }`
  - `type TabSocket = { sendInput: (data: string) => void; sendResize: (cols: number, rows: number) => void; close: () => void }`
  - `openTabSocket(tabId: string, size: { cols: number; rows: number }, handlers: TabSocketHandlers): TabSocket`
  - `SPECIAL_KEYS: readonly ["esc","tab","shift-tab","ctrl-c","ctrl-d","up","down","left","right"]`, `type SpecialKey`, `keySequence(key: SpecialKey, applicationCursor: boolean): string`
  - `onResume(callback: () => void): () => void` (returns unsubscribe)
  - `type TerminalLook = { theme: ITheme; fontFamily: string; fontSize: number; lineHeight: number }`, `readTerminalLook(): TerminalLook`

**Behavior:**
- All wire parsing happens here (parse, don't validate). A response that does not match the expected shape throws `ApiError` whose `detail` names the expected shape and quotes the actual JSON. Inner code trusts `Tab`.
- Non-2xx ⇒ `ApiError`: body parsed as `{"error":{op,tab?,message,kind,hint?}}` ⇒ `kind` = `error.kind`, `detail` = `"<server op>: <message>"` plus `" (hint: <hint>)"` when present, `tabId` = `error.tab` or the client's tab id; a body that is not that shape ⇒ `kind: null`, `detail` = raw text (proxy pages stay visible). `Retry-After` header copied verbatim. `fetch` rejection ⇒ `ApiError(op, 0, null, "network error: <message>")`.
- Wire `Tab` ⇒ `exited: false` ⇒ `end: null`; `exited: true` ⇒ `end` from `{reason, exitCode, signal}` (all three required; `exitCode`/`signal` integer or `null`). Same parser for the 4000 close reason.
- `openTabSocket`: URL `ws(s)://<host>/ws/tabs/<id>?cols=<c>&rows=<r>` (scheme from `location.protocol`; the caller passes the fitted size, 1..1000 per the API); `binaryType = "arraybuffer"`; binary ⇒ `onOutput` (the first one after attach and after `core online` is the full screen; written as-is).
- Text frames: `{"type":"core","state":"offline"|"online"}` ⇒ `onCore`; `{"type":"error",op,tab,kind,message}` ⇒ `onServerError("<op> <tab>: <kind>: <message>")` and remembered as the detail for a following 4400/4500 close; anything else (bad JSON, unknown type) ⇒ `protocol-error`, socket closed.
- Close codes: `4000` + valid JSON reason ⇒ `exited`; `4000` + invalid reason ⇒ `protocol-error`; `4004` ⇒ `not-found`; `4400`/`4500` ⇒ `server-error` (detail = last error frame text, else close reason); `1001` ⇒ `server-shutdown`; any other code (incl. `1006` from a pre-upgrade 401/400) ⇒ `disconnected`. A client-initiated `close()` never calls `onEnd`. `sendInput`/`sendResize` on a socket that is not OPEN throw (the caller gates on state; dropped keystrokes must not be silent).
- Key sequences: Esc `\x1b`, Tab `\t`, Shift+Tab `\x1b[Z`, Ctrl+C `\x03`, Ctrl+D `\x04`; arrows `\x1b[A/B/C/D`, or `\x1bOA/B/C/D` when the terminal is in application-cursor mode (xterm `term.modes.applicationCursorKeysMode`, verified in xterm 6 typings line 1911).
- `onResume` fires on `window` `online` and on `document` `visibilitychange` when `visibilityState === "visible"`. No timers.
- `readTerminalLook` throws if any token is missing or a number token is not finite (message names the variable and `tokens.css`).

**Constraints:**
- No new dependencies. No `setTimeout`/`setInterval`.
- `tsconfig.app.json` has `erasableSyntaxOnly: true` (create-vite 9.3.4 template): no constructor parameter properties, no enums.
- Close codes, text-frame types, and the resize message shape are the only wire details outside `http.ts`/`types.ts`; they are named constants at the top of `tab-socket.ts`.

**Acceptance criteria:**
- [ ] `npm run build --prefix web` RC=0 with WP2 files present (plus local uncommitted stubs for missing WP3/WP4 modules if those are not committed yet).
- [ ] One-off scratch check (not committed): `node scratch/wp2/keys-check.ts` prints `keys ok` (Node 25 strips types natively).
- [ ] `grep -rnE "setTimeout|setInterval" web/src/api web/src/lib` ⇒ no matches.
- [ ] `git show --stat HEAD` lists only the six WP2 files.

**Steps:**

- [ ] **Step 1: `web/src/api/types.ts`**

```ts
export type TabEnd = { reason: string; exitCode: number | null; signal: number | null }
export type Tab = { id: string; label: string; end: TabEnd | null }

const isIntOrNull = (v: unknown): v is number | null => v === null || Number.isInteger(v)

// Shared by GET/POST/PATCH tab bodies and the WebSocket 4000 close reason.
export function parseTabEnd(v: unknown): TabEnd | undefined {
  if (typeof v !== "object" || v === null) return undefined
  const { reason, exitCode, signal } = v as Record<string, unknown>
  if (typeof reason !== "string" || !isIntOrNull(exitCode) || !isIntOrNull(signal)) return undefined
  return { reason, exitCode, signal }
}

export class ApiError extends Error {
  readonly op: string
  readonly status: number
  readonly kind: string | null
  readonly detail: string
  readonly tabId: string | undefined
  readonly retryAfter: string | null

  constructor(op: string, status: number, kind: string | null, detail: string, tabId?: string, retryAfter: string | null = null) {
    super(`${op}${tabId === undefined ? "" : ` (tab ${tabId})`}: HTTP ${status}${kind === null ? "" : ` ${kind}`}: ${detail}`)
    this.op = op
    this.status = status
    this.kind = kind
    this.detail = detail
    this.tabId = tabId
    this.retryAfter = retryAfter
  }
}
```

- [ ] **Step 2: `web/src/api/http.ts`**

```ts
import { ApiError, parseTabEnd, type Tab } from "./types"

// Backend error body: {"error":{op,tab?,message,kind,hint?}}. Anything else (e.g. a proxy page) is shown raw.
function errorFromBody(op: string, status: number, text: string, tabId: string | undefined, retryAfter: string | null): ApiError {
  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    return new ApiError(op, status, null, text, tabId, retryAfter)
  }
  const e = typeof body === "object" && body !== null ? (body as Record<string, unknown>).error : undefined
  if (typeof e === "object" && e !== null) {
    const { op: serverOp, tab, message, kind, hint } = e as Record<string, unknown>
    if (typeof serverOp === "string" && typeof message === "string" && typeof kind === "string") {
      const detail = `${serverOp}: ${message}${typeof hint === "string" ? ` (hint: ${hint})` : ""}`
      return new ApiError(op, status, kind, detail, typeof tab === "string" ? tab : tabId, retryAfter)
    }
  }
  return new ApiError(op, status, null, text, tabId, retryAfter)
}

async function call(op: string, method: string, path: string, body?: unknown, tabId?: string): Promise<Response> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: "same-origin",
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    throw new ApiError(op, 0, null, `network error: ${e instanceof Error ? e.message : String(e)}`, tabId)
  }
  if (!res.ok) throw errorFromBody(op, res.status, await res.text(), tabId, res.headers.get("Retry-After"))
  return res
}

const TAB_SHAPE =
  "{id: string, label: string, exited: boolean, exitCode: int|null, signal: int|null, reason: string (when exited)}"

function parseTab(op: string, status: number, v: unknown): Tab {
  if (typeof v === "object" && v !== null) {
    const { id, label, exited } = v as Record<string, unknown>
    if (typeof id === "string" && typeof label === "string" && typeof exited === "boolean") {
      if (!exited) return { id, label, end: null }
      const end = parseTabEnd(v)
      if (end !== undefined) return { id, label, end }
    }
  }
  throw new ApiError(op, status, null, `expected ${TAB_SHAPE}, got ${JSON.stringify(v)}`)
}

async function json(op: string, res: Response): Promise<unknown> {
  const text = await res.text()
  try {
    return JSON.parse(text)
  } catch {
    throw new ApiError(op, res.status, null, `expected JSON body, got ${JSON.stringify(text.slice(0, 200))}`)
  }
}

const tabPath = (id: string) => `/api/tabs/${encodeURIComponent(id)}`

export async function login(username: string, password: string): Promise<void> {
  await call("login", "POST", "/login", { username, password })
}

export async function logout(): Promise<void> {
  await call("logout", "POST", "/logout")
}

export async function listTabs(): Promise<Tab[]> {
  const op = "list tabs"
  const res = await call(op, "GET", "/api/tabs")
  const v = await json(op, res)
  if (!Array.isArray(v)) throw new ApiError(op, res.status, null, `expected array of ${TAB_SHAPE}, got ${JSON.stringify(v)}`)
  return v.map((t) => parseTab(op, res.status, t))
}

export async function createTab(request: { command?: string[]; label?: string }): Promise<Tab> {
  const op = "create tab"
  const res = await call(op, "POST", "/api/tabs", request)
  return parseTab(op, res.status, await json(op, res))
}

export async function renameTab(id: string, label: string): Promise<Tab> {
  const op = "rename tab"
  const res = await call(op, "PATCH", tabPath(id), { label }, id)
  return parseTab(op, res.status, await json(op, res))
}

export async function closeTab(id: string): Promise<void> {
  await call("close tab", "DELETE", tabPath(id), undefined, id)
}

export async function listSwarms(): Promise<string[]> {
  const op = "list swarms"
  const res = await call(op, "GET", "/api/swarms")
  const v = await json(op, res)
  if (!Array.isArray(v) || !v.every((s) => typeof s === "string")) {
    throw new ApiError(op, res.status, null, `expected string[], got ${JSON.stringify(v)}`)
  }
  return v
}
```

`JSON.stringify(request)` drops `undefined` fields, so `createTab({})` sends `{}` (backend defaults: `[shell, "-l"]`, label = basename).

- [ ] **Step 3: `web/src/api/tab-socket.ts`**

```ts
import { parseTabEnd, type TabEnd } from "./types"

// Wire constants owned by the backend plan (HTTP/WS API section, commit 86de08a).
const CLOSE_EXITED = 4000
const CLOSE_NOT_FOUND = 4004
const CLOSE_BAD_MESSAGE = 4400
const CLOSE_HC_ERROR = 4500
const CLOSE_GOING_AWAY = 1001
const resizeMessage = (cols: number, rows: number) => JSON.stringify({ type: "resize", cols, rows })

export type CoreState = "offline" | "online"

export type SocketEnd =
  | { kind: "exited"; end: TabEnd }
  | { kind: "not-found"; reason: string }
  | { kind: "server-error"; code: number; detail: string }
  | { kind: "server-shutdown" }
  | { kind: "disconnected"; code: number; reason: string }
  | { kind: "protocol-error"; message: string }

export type TabSocketHandlers = {
  onOpen: () => void
  onOutput: (data: Uint8Array) => void
  onCore: (state: CoreState) => void
  onServerError: (message: string) => void
  onEnd: (end: SocketEnd) => void
}

export type TabSocket = {
  sendInput: (data: string) => void
  sendResize: (cols: number, rows: number) => void
  close: () => void
}

const encoder = new TextEncoder()

type Control = { kind: "core"; state: CoreState } | { kind: "error"; message: string } | { kind: "invalid"; message: string }

function parseControl(tabId: string, text: string): Control {
  const invalid: Control = { kind: "invalid", message: `tab ${tabId}: unexpected text frame ${JSON.stringify(text.slice(0, 200))}` }
  let v: unknown
  try {
    v = JSON.parse(text)
  } catch {
    return invalid
  }
  if (typeof v !== "object" || v === null) return invalid
  const m = v as Record<string, unknown>
  if (m.type === "core" && (m.state === "offline" || m.state === "online")) return { kind: "core", state: m.state }
  if (m.type === "error" && typeof m.message === "string") {
    return { kind: "error", message: `${String(m.op)} ${String(m.tab)}: ${String(m.kind)}: ${m.message}` }
  }
  return invalid
}

function classifyClose(tabId: string, code: number, reason: string, lastError: string | null): SocketEnd {
  switch (code) {
    case CLOSE_EXITED: {
      let parsed: unknown
      try {
        parsed = JSON.parse(reason)
      } catch {
        parsed = undefined
      }
      const end = parseTabEnd(parsed)
      if (end === undefined) {
        return {
          kind: "protocol-error",
          message: `tab ${tabId}: close ${CLOSE_EXITED} expected reason {"reason","exitCode","signal"}, got ${JSON.stringify(reason)}`,
        }
      }
      return { kind: "exited", end }
    }
    case CLOSE_NOT_FOUND:
      return { kind: "not-found", reason }
    case CLOSE_BAD_MESSAGE:
    case CLOSE_HC_ERROR:
      return { kind: "server-error", code, detail: lastError ?? reason }
    case CLOSE_GOING_AWAY:
      return { kind: "server-shutdown" }
    default:
      return { kind: "disconnected", code, reason }
  }
}

export function openTabSocket(tabId: string, size: { cols: number; rows: number }, handlers: TabSocketHandlers): TabSocket {
  const scheme = location.protocol === "https:" ? "wss:" : "ws:"
  const query = new URLSearchParams({ cols: String(size.cols), rows: String(size.rows) })
  const ws = new WebSocket(`${scheme}//${location.host}/ws/tabs/${encodeURIComponent(tabId)}?${query}`)
  ws.binaryType = "arraybuffer"
  let ended = false
  let lastError: string | null = null
  const end = (e: SocketEnd) => {
    if (ended) return
    ended = true
    handlers.onEnd(e)
  }

  ws.onopen = () => handlers.onOpen()
  ws.onmessage = (ev) => {
    if (ev.data instanceof ArrayBuffer) {
      handlers.onOutput(new Uint8Array(ev.data))
      return
    }
    const c = parseControl(tabId, String(ev.data))
    switch (c.kind) {
      case "core":
        handlers.onCore(c.state)
        return
      case "error":
        lastError = c.message
        handlers.onServerError(c.message)
        return
      case "invalid":
        end({ kind: "protocol-error", message: c.message })
        ws.close()
        return
    }
  }
  ws.onclose = (ev) => end(classifyClose(tabId, ev.code, ev.reason, lastError))

  const requireOpen = (what: string) => {
    if (ws.readyState !== WebSocket.OPEN) {
      throw new Error(`tab ${tabId}: ${what} expected socket state OPEN (1), got ${ws.readyState}`)
    }
  }
  return {
    sendInput: (data) => {
      requireOpen("send input")
      ws.send(encoder.encode(data))
    },
    sendResize: (cols, rows) => {
      requireOpen("send resize")
      ws.send(resizeMessage(cols, rows))
    },
    close: () => {
      ended = true // client-initiated: no onEnd
      ws.close()
    },
  }
}
```

- [ ] **Step 4: `web/src/lib/keys.ts`**

```ts
export const SPECIAL_KEYS = ["esc", "tab", "shift-tab", "ctrl-c", "ctrl-d", "up", "down", "left", "right"] as const
export type SpecialKey = (typeof SPECIAL_KEYS)[number]

const ESC = "\x1b"

export function keySequence(key: SpecialKey, applicationCursor: boolean): string {
  const arrow = applicationCursor ? `${ESC}O` : `${ESC}[`
  switch (key) {
    case "esc": return ESC
    case "tab": return "\t"
    case "shift-tab": return `${ESC}[Z`
    case "ctrl-c": return "\x03"
    case "ctrl-d": return "\x04"
    case "up": return `${arrow}A`
    case "down": return `${arrow}B`
    case "right": return `${arrow}C`
    case "left": return `${arrow}D`
  }
}
```

- [ ] **Step 5: `web/src/lib/resume.ts`**

```ts
// Reconnect triggers per spec: network back online, or page back in the foreground. No timers.
export function onResume(callback: () => void): () => void {
  const onVisibility = () => {
    if (document.visibilityState === "visible") callback()
  }
  window.addEventListener("online", callback)
  document.addEventListener("visibilitychange", onVisibility)
  return () => {
    window.removeEventListener("online", callback)
    document.removeEventListener("visibilitychange", onVisibility)
  }
}
```

- [ ] **Step 6: `web/src/lib/terminal-theme.ts`**

```ts
import type { ITheme } from "@xterm/xterm"

const THEME_VARS = {
  background: "--term-background",
  foreground: "--term-foreground",
  cursor: "--term-cursor",
  selectionBackground: "--term-selection",
  black: "--term-black", brightBlack: "--term-bright-black",
  red: "--term-red", brightRed: "--term-bright-red",
  green: "--term-green", brightGreen: "--term-bright-green",
  yellow: "--term-yellow", brightYellow: "--term-bright-yellow",
  blue: "--term-blue", brightBlue: "--term-bright-blue",
  magenta: "--term-magenta", brightMagenta: "--term-bright-magenta",
  cyan: "--term-cyan", brightCyan: "--term-bright-cyan",
  white: "--term-white", brightWhite: "--term-bright-white",
} as const satisfies Partial<Record<keyof ITheme, string>>

export type TerminalLook = { theme: ITheme; fontFamily: string; fontSize: number; lineHeight: number }

export function readTerminalLook(): TerminalLook {
  const css = getComputedStyle(document.documentElement)
  const read = (name: string) => {
    const v = css.getPropertyValue(name).trim()
    if (v === "") throw new Error(`terminal look: expected CSS variable ${name} in web/src/styles/tokens.css, found none`)
    return v
  }
  const num = (name: string) => {
    const v = read(name)
    const n = Number(v)
    if (!Number.isFinite(n)) throw new Error(`terminal look: expected ${name} to be a unitless number, got ${JSON.stringify(v)}`)
    return n
  }
  const theme: ITheme = {}
  for (const [key, name] of Object.entries(THEME_VARS)) theme[key as keyof typeof THEME_VARS] = read(name)
  return {
    theme,
    fontFamily: read("--app-font-mono"),
    fontSize: num("--terminal-font-size"),
    lineHeight: num("--terminal-line-height"),
  }
}
```

- [ ] **Step 7: One-off key check (scratch, not committed)**

`scratch/wp2/keys-check.ts`:

```ts
import { keySequence } from "../../web/src/lib/keys.ts"
const cases: [Parameters<typeof keySequence>, string][] = [
  [["esc", false], "\x1b"], [["tab", false], "\t"], [["shift-tab", true], "\x1b[Z"],
  [["ctrl-c", false], "\x03"], [["ctrl-d", false], "\x04"],
  [["up", false], "\x1b[A"], [["up", true], "\x1bOA"], [["left", true], "\x1bOD"], [["right", false], "\x1b[C"],
]
for (const [args, want] of cases) {
  const got = keySequence(...args)
  if (got !== want) throw new Error(`keySequence(${args.join(",")}): expected ${JSON.stringify(want)}, got ${JSON.stringify(got)}`)
}
console.log("keys ok")
```

Run `node scratch/wp2/keys-check.ts` ⇒ `keys ok`. Then change `"\x1b[Z"` in `keys.ts` to `"\x1b[z"`, rerun ⇒ throws (red seen), restore from a `cp` backup taken before the edit, rerun ⇒ `keys ok`.

- [ ] **Step 8: Build and commit**

```bash
npm run build --prefix web
git add web/src/api web/src/lib/keys.ts web/src/lib/resume.ts web/src/lib/terminal-theme.ts
git commit -m "feat(web): API client, tab socket, key sequences, terminal look" -- web/src/api web/src/lib/keys.ts web/src/lib/resume.ts web/src/lib/terminal-theme.ts
```

---

### Task WP3: Terminal pane, key bar, status banner

**Files:**
- Create: `web/src/terminal/pane-state.ts`, `web/src/terminal/TerminalPane.tsx`, `web/src/terminal/KeyBar.tsx`, `web/src/terminal/StatusBanner.tsx`

**Interfaces:**
- Consumes: WP2 `Tab`, `TabEnd`, `openTabSocket`, `TabSocket`, `SocketEnd`, `CoreState`, `SPECIAL_KEYS`, `SpecialKey`, `keySequence`, `onResume`, `readTerminalLook`; WP1 `@/components/ui/{button,alert}`; lucide `ArrowUp ArrowDown ArrowLeft ArrowRight WifiOff ServerOff RefreshCw X CircleAlert`.
- Produces (frozen; WP4 uses):
  - `type TerminalPaneProps = { tab: Tab; onExited: (tabId: string, end: TabEnd) => void; onClose: (tabId: string) => void; onSocketLost: () => void }`
  - `export function TerminalPane(props: TerminalPaneProps): JSX.Element` — parent MUST render it with `key={tab.id}`; one instance = one tab.

**Behavior:**
- Mount: create `Terminal` with `readTerminalLook()`, load `FitAddon`, `open()` into the host div, `fit()`, observe the host div with `ResizeObserver` ⇒ `fit()`. If `tab.end !== null` show the ended state and never connect. Otherwise connect.
- Connect: state `connecting`, `openTabSocket(tab.id, {cols: term.cols, rows: term.rows}, …)` (size goes in the URL). No `term.reset()`: the server's full-screen message starts with a terminal reset sequence. `onOpen` ⇒ `live`, focus. `onOutput` ⇒ `term.write`.
- `onCore("offline")` ⇒ `core-offline` (socket stays open, input disabled); `onCore("online")` ⇒ `live` (the full screen follows by itself). `onServerError(msg)` while `core-offline` ⇒ shown as the banner notice; at other times the socket keeps it as the detail of the 4400/4500 close that follows.
- `onEnd(end)` ⇒ state `ended`; then `exited` ⇒ `onExited(tab.id, end.end)`; `disconnected`, `not-found`, `server-shutdown` ⇒ `onSocketLost()` (App re-lists tabs: a 401 sends the user to login, a removed tab disappears).
- Input: `term.onData` ⇒ `socket.sendInput` only in state `live`; in any other state `term.options.disableStdin = true` and key-bar buttons are disabled, so no keystroke is dropped silently. `term.onResize` ⇒ `sendResize` only when live (a new attach sends its size in the URL).
- Key bar: `term.input(keySequence(key, term.modes.applicationCursorKeysMode), true)` — routes through `onData`, so the bar and the keyboard share one path. Buttons call `preventDefault()` on `pointerdown` so focus (and the phone keyboard) stays on the terminal.
- Resume: `onResume` ⇒ reconnect when `canReconnect(state)` (ended, and not `exited`/`not-found`). Banner button「重新連線」does the same. No timer.
- Banner texts (by state / end kind): connecting「連線中…」; core offline「主機核心離線」+ notice; exited `endLabel(end)` =「已結束（code N）」, else「已結束（signal N）」, else「已結束（<reason>）」+ button「關閉分頁」⇒ `onClose(tab.id)`; not-found「分頁不存在」+ reason (the re-list removes it); 4400「訊息格式錯誤」/ 4500「主機核心錯誤」+ detail + 重新連線; server-shutdown「主機服務停止」+ 重新連線; disconnected「已斷線」+ `close <code> <reason>` + 重新連線; protocol error「協定錯誤」+ message + 重新連線.
- Unmount: close socket (client-initiated, no `onEnd`), dispose listeners and terminal.

**Constraints:**
- Mount effect runs once per tab; socket callbacks and the resume listener read the latest state and props through refs (`stateRef`, `propsRef`), so parent re-renders never re-attach.
- Scrollback is a named constant `SCROLLBACK_LINES = 5000`.
- Colors only via Tailwind semantic classes or `var(--term-background)`.

**Acceptance criteria:**
- [ ] `npm run build --prefix web` RC=0.
- [ ] `grep -nE "setTimeout|setInterval" web/src/terminal/*` ⇒ no matches.
- [ ] Round live checks L3, L4, L5, L6b, L6c pass (run by the round verifier).
- [ ] `git show --stat HEAD` lists only the four WP3 files.

**Steps:**

- [ ] **Step 1: `web/src/terminal/pane-state.ts`**

```ts
import type { TabEnd } from "@/api/types"
import type { SocketEnd } from "@/api/tab-socket"

export type PaneState =
  | { kind: "connecting" }
  | { kind: "live" }
  | { kind: "core-offline"; notice: string | null }
  | { kind: "ended"; end: SocketEnd }

// An exited or removed tab has nothing to re-attach to; every other end can be retried.
export const canReconnect = (s: PaneState) =>
  s.kind === "ended" && s.end.kind !== "exited" && s.end.kind !== "not-found"

export function endLabel(end: TabEnd): string {
  if (end.exitCode !== null) return `已結束（code ${end.exitCode}）`
  if (end.signal !== null) return `已結束（signal ${end.signal}）`
  return `已結束（${end.reason}）`
}
```

- [ ] **Step 2: `web/src/terminal/KeyBar.tsx`**

```tsx
import type { ReactNode } from "react"
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp } from "lucide-react"
import { Button } from "@/components/ui/button"
import { SPECIAL_KEYS, type SpecialKey } from "@/lib/keys"

const LABELS: Record<SpecialKey, { text: ReactNode; aria: string }> = {
  esc: { text: "Esc", aria: "Escape" },
  tab: { text: "Tab", aria: "Tab" },
  "shift-tab": { text: "⇧Tab", aria: "Shift+Tab" },
  "ctrl-c": { text: "^C", aria: "Ctrl+C" },
  "ctrl-d": { text: "^D", aria: "Ctrl+D" },
  up: { text: <ArrowUp />, aria: "上" },
  down: { text: <ArrowDown />, aria: "下" },
  left: { text: <ArrowLeft />, aria: "左" },
  right: { text: <ArrowRight />, aria: "右" },
}

export function KeyBar({ disabled, onKey }: { disabled: boolean; onKey: (key: SpecialKey) => void }) {
  return (
    <div className="flex shrink-0 gap-1 overflow-x-auto border-t border-border bg-card p-1">
      {SPECIAL_KEYS.map((k) => (
        <Button
          key={k}
          size="sm"
          variant="secondary"
          className="font-mono"
          disabled={disabled}
          aria-label={LABELS[k].aria}
          onPointerDown={(e) => e.preventDefault()}
          onClick={() => onKey(k)}
        >
          {LABELS[k].text}
        </Button>
      ))}
    </div>
  )
}
```

- [ ] **Step 3: `web/src/terminal/StatusBanner.tsx`**

```tsx
import type { ReactNode } from "react"
import { CircleAlert, RefreshCw, ServerOff, WifiOff, X } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { endLabel, type PaneState } from "./pane-state"

type Props = { state: PaneState; onReconnect: () => void; onClose: () => void }

const BAR = "shrink-0 rounded-none border-x-0"
const WARN = `${BAR} bg-warning text-warning-foreground`

function Banner(props: { tone: "warn" | "error" | "plain"; icon?: ReactNode; title: string; detail?: string; action?: ReactNode }) {
  const cls = props.tone === "warn" ? WARN : BAR
  return (
    <Alert variant={props.tone === "error" ? "destructive" : "default"} className={cls}>
      {props.icon}
      <AlertTitle>{props.title}</AlertTitle>
      <AlertDescription className="flex items-center justify-between gap-2">
        <span className="break-all">{props.detail}</span>
        {props.action}
      </AlertDescription>
    </Alert>
  )
}

export function StatusBanner({ state, onReconnect, onClose }: Props) {
  const reconnect = (
    <Button size="sm" variant="outline" onClick={onReconnect}>
      <RefreshCw /> 重新連線
    </Button>
  )
  switch (state.kind) {
    case "live":
      return null
    case "connecting":
      return <p className="shrink-0 px-3 py-1 text-sm text-muted-foreground">連線中…</p>
    case "core-offline":
      return <Banner tone="warn" icon={<ServerOff />} title="主機核心離線" detail={state.notice ?? undefined} />
  }
  const end = state.end
  switch (end.kind) {
    case "exited":
      return (
        <Banner
          tone="plain"
          title={endLabel(end.end)}
          action={
            <Button size="sm" variant="outline" onClick={onClose}>
              <X /> 關閉分頁
            </Button>
          }
        />
      )
    case "not-found":
      return <Banner tone="error" icon={<CircleAlert />} title="分頁不存在" detail={end.reason} />
    case "server-error":
      return (
        <Banner
          tone="error"
          icon={<CircleAlert />}
          title={end.code === 4400 ? "訊息格式錯誤" : "主機核心錯誤"}
          detail={end.detail}
          action={reconnect}
        />
      )
    case "server-shutdown":
      return <Banner tone="warn" icon={<ServerOff />} title="主機服務停止" action={reconnect} />
    case "disconnected":
      return (
        <Banner tone="warn" icon={<WifiOff />} title="已斷線" detail={`close ${end.code}${end.reason ? ` ${end.reason}` : ""}`} action={reconnect} />
      )
    case "protocol-error":
      return <Banner tone="error" icon={<CircleAlert />} title="協定錯誤" detail={end.message} action={reconnect} />
  }
}
```

All lucide names used (`server-off`, `circle-alert`, `wifi-off`, `refresh-cw`, `x`, `arrow-*`) exist in `~/ui-kits/icons/lucide/default/` and as `lucide-react@1.75.0` exports (checked 2026-11-19).

- [ ] **Step 4: `web/src/terminal/TerminalPane.tsx`**

```tsx
import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { Terminal } from "@xterm/xterm"
import { FitAddon } from "@xterm/addon-fit"
import type { Tab, TabEnd } from "@/api/types"
import { openTabSocket, type CoreState, type SocketEnd, type TabSocket } from "@/api/tab-socket"
import { keySequence, type SpecialKey } from "@/lib/keys"
import { onResume } from "@/lib/resume"
import { readTerminalLook } from "@/lib/terminal-theme"
import { KeyBar } from "./KeyBar"
import { StatusBanner } from "./StatusBanner"
import { canReconnect, type PaneState } from "./pane-state"

const SCROLLBACK_LINES = 5000

export type TerminalPaneProps = {
  tab: Tab
  onExited: (tabId: string, end: TabEnd) => void
  onClose: (tabId: string) => void
  onSocketLost: () => void
}

export function TerminalPane({ tab, onExited, onClose, onSocketLost }: TerminalPaneProps) {
  const hostRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const socketRef = useRef<TabSocket | null>(null)
  const initial: PaneState =
    tab.end === null ? { kind: "connecting" } : { kind: "ended", end: { kind: "exited", end: tab.end } }
  const [state, setState] = useState<PaneState>(initial)
  // Socket callbacks and the resume listener outlive renders; they read the latest state/props via refs.
  const stateRef = useRef<PaneState>(initial)
  const propsRef = useRef({ onExited, onSocketLost })
  useLayoutEffect(() => {
    propsRef.current = { onExited, onSocketLost }
  })

  const setPane = (s: PaneState) => {
    stateRef.current = s
    setState(s)
    const term = termRef.current
    if (term !== null) term.options.disableStdin = s.kind !== "live"
  }

  const handleEnd = (end: SocketEnd) => {
    socketRef.current = null
    setPane({ kind: "ended", end })
    switch (end.kind) {
      case "exited":
        propsRef.current.onExited(tab.id, end.end)
        return
      case "disconnected":
      case "not-found":
      case "server-shutdown":
        propsRef.current.onSocketLost()
        return
      case "server-error":
      case "protocol-error":
        return
    }
  }

  const handleCore = (core: CoreState) => {
    setPane(core === "offline" ? { kind: "core-offline", notice: null } : { kind: "live" })
  }

  // Shown only while the core is offline; otherwise the socket attaches it to the 4400/4500 close that follows.
  const handleServerError = (message: string) => {
    if (stateRef.current.kind === "core-offline") setPane({ kind: "core-offline", notice: message })
  }

  const connect = () => {
    const term = termRef.current
    if (term === null) throw new Error(`tab ${tab.id}: connect expected a created terminal, got none`)
    socketRef.current?.close()
    setPane({ kind: "connecting" })
    socketRef.current = openTabSocket(tab.id, { cols: term.cols, rows: term.rows }, {
      onOpen: () => {
        setPane({ kind: "live" })
        term.focus()
      },
      onOutput: (data) => term.write(data),
      onCore: handleCore,
      onServerError: handleServerError,
      onEnd: handleEnd,
    })
  }

  // Mount once per tab (parent uses key={tab.id}). connect/handleEnd read only refs and tab.id,
  // so the first-render closures stay correct for the component's lifetime.
  useEffect(() => {
    const host = hostRef.current
    if (host === null) throw new Error(`tab ${tab.id}: expected terminal host element, got none`)
    const term = new Terminal({ ...readTerminalLook(), cursorBlink: true, scrollback: SCROLLBACK_LINES })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()
    termRef.current = term
    term.options.disableStdin = stateRef.current.kind !== "live"
    const data = term.onData((d) => {
      const socket = socketRef.current
      if (socket === null) throw new Error(`tab ${tab.id}: input expected a live socket, got none`)
      socket.sendInput(d)
    })
    // Not live ⇒ skip; the next attach carries the current size in its URL, so nothing is lost.
    const resize = term.onResize(({ cols, rows }) => {
      if (stateRef.current.kind === "live") socketRef.current?.sendResize(cols, rows)
    })
    const observer = new ResizeObserver(() => fit.fit())
    observer.observe(host)
    if (stateRef.current.kind === "connecting") connect()
    const stopResume = onResume(() => {
      if (canReconnect(stateRef.current)) connect()
    })
    return () => {
      stopResume()
      observer.disconnect()
      data.dispose()
      resize.dispose()
      socketRef.current?.close()
      socketRef.current = null
      term.dispose()
      termRef.current = null
    }
  }, [])

  const sendKey = (key: SpecialKey) => {
    const term = termRef.current
    if (term === null) throw new Error(`tab ${tab.id}: key ${key} expected a terminal, got none`)
    term.input(keySequence(key, term.modes.applicationCursorKeysMode), true)
  }

  return (
    <div className="flex h-full flex-col">
      <StatusBanner state={state} onReconnect={connect} onClose={() => onClose(tab.id)} />
      <div ref={hostRef} className="min-h-0 flex-1 bg-[var(--term-background)] p-1" />
      <KeyBar disabled={state.kind !== "live"} onKey={sendKey} />
    </div>
  )
}
```

StrictMode (dev) mounts, cleans up, and mounts again: the first socket is closed client-side (no `onEnd`) and the second mount attaches again — expected, harmless.

- [ ] **Step 5: Build and commit**

```bash
npm run build --prefix web
git add web/src/terminal
git commit -m "feat(web): terminal pane with key bar and reconnect banner" -- web/src/terminal
```

---

### Task WP4: App shell — login, tab bar, new-tab and swarm dialog, errors

**Files:**
- Create: `web/src/App.tsx`, `web/src/screens/LoginScreen.tsx`, `web/src/workspace/TabBar.tsx`, `web/src/workspace/NewTabDialog.tsx`, `web/src/workspace/ErrorAlert.tsx`

**Interfaces:**
- Consumes: WP2 `Tab`, `TabEnd`, `ApiError`, `login`, `logout`, `listTabs`, `createTab`, `renameTab`, `closeTab`, `listSwarms`, `onResume`; WP3 `TerminalPane`, `TerminalPaneProps`; WP1 `@/components/ui/{button,input,label,card,dialog,alert-dialog,alert,badge,scroll-area}`; lucide `Plus Pencil X Terminal Users CircleAlert LogOut` (`log-out.svg` exists in the lucide pack; `LogOut` is a `lucide-react@1.75.0` export).
- Produces: `export default function App()` (imported by WP1 `main.tsx`). Internal component props:
  - `LoginScreen: { onSignedIn: () => void }`
  - `TabBar: { tabs: Tab[]; activeId: string | null; onSelect: (id: string) => void; onRename: (id: string, label: string) => void; onClose: (id: string) => void; onNew: () => void; onLogout: () => void }`
  - `NewTabDialog: { open: boolean; onOpenChange: (open: boolean) => void; onCreate: (request: { command?: string[]; label?: string }) => void; onUnauthorized: () => void }`
  - `ErrorAlert: { error: ApiError; onDismiss: () => void }`

**Behavior:**
- Start: `listTabs()`; success ⇒ workspace; `ApiError` 401 ⇒ login screen; other error ⇒ error alert with「重試」.
- Any `ApiError` 401 from any action ⇒ login screen (session expired). Other `ApiError` ⇒ `ErrorAlert` shows `error.message` (op, tab id, status, server text) verbatim until dismissed or the next successful action. Non-`ApiError` exceptions are bugs and propagate.
- `onResume` ⇒ `listTabs()` again (picks up tabs created or closed by another client, and detects an expired session). `TerminalPane.onSocketLost` does the same.
- Active tab: kept if still listed, else first tab, else none (empty state with「開新分頁」).
- Tab bar: one button per tab with label; exited tabs show a badge「已結束」. The active tab shows a rename button (Pencil ⇒ dialog with an input, submit ⇒ `renameTab`) and a close button (X). Closing a running tab asks first (AlertDialog「關閉分頁會結束其中的程式」); closing an exited tab does not ask. A trailing Plus button opens the new-tab dialog; after it a LogOut button「登出」⇒ `logout()` then the login screen. Rename replaces the tab with the `Tab` the server returns.
- New-tab dialog: on open, `listSwarms()`. Shows「新的 shell」(Terminal icon ⇒ `onCreate({})`, server picks shell and label), then one button per swarm name (Users icon ⇒ `onCreate({ command: ["tmux", "-L", name, "a"], label: name })`). Empty list ⇒「沒有 claude-swarm-* 工作階段」. Error ⇒ inline error text; 401 ⇒ `onUnauthorized()`.
- Login: card with 帳號 / 密碼 inputs (`autoComplete="username"` / `"current-password"`), submit ⇒ `login()` then `onSignedIn()`. 401 ⇒「帳號或密碼錯誤」+ server text; 429 ⇒「嘗試次數過多，請於 {retryAfter} 秒後再試」+ server text (retryAfter absent ⇒ show the server text only, no invented number); other ⇒ `error.message`. Submit disabled while the request is in flight.

**Constraints:**
- `TerminalPane` rendered with `key={active.id}`.
- No router, no global store, no fetch outside `@/api/http`.
- Copy in zh-TW as written above.

**Acceptance criteria:**
- [ ] `npm run build --prefix web` RC=0 with WP1–WP4 committed.
- [ ] `grep -n "key={active.id}" web/src/App.tsx` matches.
- [ ] `grep -rn "fetch(" web/src --include=*.tsx` ⇒ no matches.
- [ ] Round live checks L2, L3, L6 pass (round verifier).
- [ ] `git show --stat HEAD` lists only the five WP4 files.

**Steps:**

- [ ] **Step 1: `web/src/workspace/ErrorAlert.tsx`**

```tsx
import { CircleAlert, X } from "lucide-react"
import type { ApiError } from "@/api/types"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

export function ErrorAlert({ error, onDismiss }: { error: ApiError; onDismiss: () => void }) {
  return (
    <Alert variant="destructive" className="shrink-0 rounded-none border-x-0">
      <CircleAlert />
      <AlertTitle>{error.op} 失敗</AlertTitle>
      <AlertDescription className="flex items-start justify-between gap-2">
        <span className="break-all font-mono text-xs">{error.message}</span>
        <Button size="icon" variant="ghost" aria-label="關閉錯誤訊息" onClick={onDismiss}>
          <X />
        </Button>
      </AlertDescription>
    </Alert>
  )
}
```

- [ ] **Step 2: `web/src/screens/LoginScreen.tsx`**

```tsx
import { useState, type FormEvent } from "react"
import { login } from "@/api/http"
import { ApiError } from "@/api/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

function loginMessage(e: ApiError): string {
  if (e.status === 401) return `帳號或密碼錯誤（${e.detail}）`
  if (e.status === 429) {
    return e.retryAfter === null ? e.detail : `嘗試次數過多，請於 ${e.retryAfter} 秒後再試（${e.detail}）`
  }
  return e.message
}

export function LoginScreen({ onSignedIn }: { onSignedIn: () => void }) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async (ev: FormEvent) => {
    ev.preventDefault()
    setBusy(true)
    try {
      await login(username, password)
      setError(null)
      onSignedIn()
    } catch (e) {
      if (!(e instanceof ApiError)) throw e
      setError(loginMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid h-full place-items-center bg-background p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Roosterm</CardTitle>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="username">帳號</Label>
              <Input id="username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">密碼</Label>
              <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </div>
            {error !== null && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" disabled={busy}>登入</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
```

- [ ] **Step 3: `web/src/workspace/NewTabDialog.tsx`**

```tsx
import { useEffect, useState } from "react"
import { Terminal, Users } from "lucide-react"
import { listSwarms } from "@/api/http"
import { ApiError } from "@/api/types"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (request: { command?: string[]; label?: string }) => void
  onUnauthorized: () => void
}

type Swarms = { kind: "loading" } | { kind: "ready"; names: string[] } | { kind: "error"; message: string }

// Swarm tab request (backend plan, HTTP/WS API: POST /api/tabs); the socket name doubles as the tab label.
const swarmTab = (name: string) => ({ command: ["tmux", "-L", name, "a"], label: name })

export function NewTabDialog({ open, onOpenChange, onCreate, onUnauthorized }: Props) {
  const [swarms, setSwarms] = useState<Swarms>({ kind: "loading" })

  useEffect(() => {
    if (!open) return
    let current = true
    setSwarms({ kind: "loading" })
    listSwarms().then(
      (names) => current && setSwarms({ kind: "ready", names }),
      (e: unknown) => {
        if (!(e instanceof ApiError)) throw e
        if (e.status === 401) onUnauthorized()
        else if (current) setSwarms({ kind: "error", message: e.message })
      },
    )
    return () => {
      current = false
    }
  }, [open, onUnauthorized])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>開新分頁</DialogTitle>
        </DialogHeader>
        <div className="grid gap-2">
          <Button variant="secondary" className="justify-start" onClick={() => onCreate({})}>
            <Terminal /> 新的 shell
          </Button>
          {swarms.kind === "loading" && <p className="text-sm text-muted-foreground">讀取 swarm 清單…</p>}
          {swarms.kind === "error" && <p className="text-sm text-destructive break-all">{swarms.message}</p>}
          {swarms.kind === "ready" && swarms.names.length === 0 && (
            <p className="text-sm text-muted-foreground">沒有 claude-swarm-* 工作階段</p>
          )}
          {swarms.kind === "ready" &&
            swarms.names.map((name) => (
              <Button key={name} variant="outline" className="justify-start font-mono" onClick={() => onCreate(swarmTab(name))}>
                <Users /> {name}
              </Button>
            ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
```

- [ ] **Step 4: `web/src/workspace/TabBar.tsx`**

```tsx
import { useState, type FormEvent } from "react"
import { LogOut, Pencil, Plus, X } from "lucide-react"
import type { Tab } from "@/api/types"
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area"

type Props = {
  tabs: Tab[]
  activeId: string | null
  onSelect: (id: string) => void
  onRename: (id: string, label: string) => void
  onClose: (id: string) => void
  onNew: () => void
  onLogout: () => void
}

export function TabBar({ tabs, activeId, onSelect, onRename, onClose, onNew, onLogout }: Props) {
  const [renaming, setRenaming] = useState<Tab | null>(null)
  const [draft, setDraft] = useState("")
  const [confirming, setConfirming] = useState<Tab | null>(null)

  const requestClose = (tab: Tab) => (tab.end === null ? setConfirming(tab) : onClose(tab.id))
  const submitRename = (ev: FormEvent) => {
    ev.preventDefault()
    if (renaming === null) return
    onRename(renaming.id, draft.trim())
    setRenaming(null)
  }

  return (
    <div className="flex shrink-0 items-center gap-1 border-b border-border bg-card px-1">
      <ScrollArea className="min-w-0 flex-1">
        <div className="flex items-center gap-1 py-1">
          {tabs.map((tab) => {
            const active = tab.id === activeId
            return (
              <div key={tab.id} className="flex shrink-0 items-center">
                <Button size="sm" variant={active ? "secondary" : "ghost"} onClick={() => onSelect(tab.id)}>
                  {tab.label}
                  {tab.end !== null && <Badge variant="outline">已結束</Badge>}
                </Button>
                {active && (
                  <>
                    <Button size="icon" variant="ghost" aria-label="重新命名分頁" onClick={() => { setDraft(tab.label); setRenaming(tab) }}>
                      <Pencil />
                    </Button>
                    <Button size="icon" variant="ghost" aria-label="關閉分頁" onClick={() => requestClose(tab)}>
                      <X />
                    </Button>
                  </>
                )}
              </div>
            )
          })}
        </div>
        <ScrollBar orientation="horizontal" />
      </ScrollArea>
      <Button size="icon" variant="ghost" aria-label="開新分頁" onClick={onNew}>
        <Plus />
      </Button>
      <Button size="icon" variant="ghost" aria-label="登出" onClick={onLogout}>
        <LogOut />
      </Button>

      <Dialog open={renaming !== null} onOpenChange={(o) => !o && setRenaming(null)}>
        <DialogContent>
          <form onSubmit={submitRename} className="grid gap-4">
            <DialogHeader>
              <DialogTitle>重新命名分頁</DialogTitle>
            </DialogHeader>
            <Input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} required />
            <DialogFooter>
              <Button type="submit" disabled={draft.trim() === ""}>儲存</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={confirming !== null} onOpenChange={(o) => !o && setConfirming(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>關閉「{confirming?.label}」？</AlertDialogTitle>
            <AlertDialogDescription>關閉分頁會結束其中的程式。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={() => confirming !== null && onClose(confirming.id)}>關閉</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
```

If the generated `scroll-area.tsx` does not export `ScrollBar`, drop the `<ScrollBar>` line (horizontal scrolling then relies on the viewport's native overflow).

- [ ] **Step 5: `web/src/App.tsx`**

```tsx
import { useCallback, useEffect, useState } from "react"
import { Plus } from "lucide-react"
import { closeTab, createTab, listTabs, logout, renameTab } from "@/api/http"
import { ApiError, type Tab, type TabEnd } from "@/api/types"
import { Button } from "@/components/ui/button"
import { onResume } from "@/lib/resume"
import { LoginScreen } from "@/screens/LoginScreen"
import { TerminalPane } from "@/terminal/TerminalPane"
import { ErrorAlert } from "@/workspace/ErrorAlert"
import { NewTabDialog } from "@/workspace/NewTabDialog"
import { TabBar } from "@/workspace/TabBar"

type Auth = "checking" | "anonymous" | "signed-in"

export default function App() {
  const [auth, setAuth] = useState<Auth>("checking")
  const [tabs, setTabs] = useState<Tab[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  const [error, setError] = useState<ApiError | null>(null)
  const [newTabOpen, setNewTabOpen] = useState(false)

  const signOut = useCallback(() => setAuth("anonymous"), [])

  const run = useCallback(async (action: () => Promise<void>) => {
    try {
      await action()
      setError(null)
    } catch (e) {
      if (!(e instanceof ApiError)) throw e
      if (e.status === 401) signOut()
      else setError(e)
    }
  }, [signOut])

  const refresh = useCallback(
    () =>
      run(async () => {
        const list = await listTabs()
        setTabs(list)
        setAuth("signed-in")
        setActiveId((cur) => (cur !== null && list.some((t) => t.id === cur) ? cur : (list[0]?.id ?? null)))
      }),
    [run],
  )

  useEffect(() => {
    void refresh()
  }, [refresh])
  useEffect(() => onResume(() => void refresh()), [refresh])

  const create = (request: { command?: string[]; label?: string }) =>
    void run(async () => {
      const tab = await createTab(request)
      setTabs((ts) => [...ts, tab])
      setActiveId(tab.id)
      setNewTabOpen(false)
    })

  const rename = (id: string, label: string) =>
    void run(async () => {
      const updated = await renameTab(id, label)
      setTabs((ts) => ts.map((t) => (t.id === id ? updated : t)))
    })

  const signOutOfHost = () =>
    void run(async () => {
      await logout()
      signOut()
    })

  const close = (id: string) =>
    void run(async () => {
      await closeTab(id)
      const rest = tabs.filter((t) => t.id !== id)
      setTabs(rest)
      if (activeId === id) setActiveId(rest[0]?.id ?? null)
    })

  const markExited = useCallback((id: string, end: TabEnd) => {
    setTabs((ts) => ts.map((t) => (t.id === id ? { ...t, end } : t)))
  }, [])

  if (auth === "anonymous") return <LoginScreen onSignedIn={() => void refresh()} />

  if (auth === "checking") {
    return (
      <div className="grid h-full place-items-center gap-2 bg-background p-4 text-muted-foreground">
        {error === null ? <p>載入中…</p> : (
          <div className="grid gap-2">
            <ErrorAlert error={error} onDismiss={() => setError(null)} />
            <Button onClick={() => void refresh()}>重試</Button>
          </div>
        )}
      </div>
    )
  }

  const active = tabs.find((t) => t.id === activeId) ?? null
  return (
    <div className="flex h-full flex-col bg-background text-foreground">
      <TabBar tabs={tabs} activeId={activeId} onSelect={setActiveId} onRename={rename} onClose={close} onNew={() => setNewTabOpen(true)} onLogout={signOutOfHost} />
      {error !== null && <ErrorAlert error={error} onDismiss={() => setError(null)} />}
      <main className="min-h-0 flex-1">
        {active !== null ? (
          <TerminalPane key={active.id} tab={active} onExited={markExited} onClose={close} onSocketLost={() => void refresh()} />
        ) : (
          <div className="grid h-full place-items-center">
            <Button onClick={() => setNewTabOpen(true)}>
              <Plus /> 開新分頁
            </Button>
          </div>
        )}
      </main>
      <NewTabDialog open={newTabOpen} onOpenChange={setNewTabOpen} onCreate={create} onUnauthorized={signOut} />
    </div>
  )
}
```

- [ ] **Step 6: Build, live smoke, commit**

```bash
npm run build --prefix web
```

Live smoke (implementer, before commit): `npm run dev --prefix web` against a running `roosterm serve`; log in, open one tab, type `echo ok` ⇒ `ok`. Then:

```bash
git add web/src/App.tsx web/src/screens web/src/workspace
git commit -m "feat(web): login, tab bar, new tab and swarm dialog" -- web/src/App.tsx web/src/screens web/src/workspace
```

---

### Task WP5: Flutter Android shell

**Files:**
- Create (via `flutter create`, then trimmed): `app/pubspec.yaml`, `app/pubspec.lock`, `app/analysis_options.yaml`, `app/android/**`, `app/.gitignore`
- Create: `app/lib/main.dart`, `app/lib/hosts.dart`, `app/lib/host_list_page.dart`, `app/lib/host_page.dart`
- Modify: `app/android/app/src/main/AndroidManifest.xml` (INTERNET permission)
- Delete from template: `app/test/widget_test.dart` (UI test; test policy forbids), any non-Android platform dirs

**Interfaces:**
- Consumes: a deployed host URL serving the WP1–WP4 web UI (only for the live check).
- Produces (internal):
  - `class Host { final String name; final Uri url; static Host parse(String name, String url); Map<String, String> toJson(); }`
  - `class HostStore { Future<List<Host>> load(); Future<void> save(List<Host> hosts); }`
  - `class HostListPage extends StatefulWidget`, `class HostPage extends StatefulWidget { final Host host; }`

**Behavior:**
- Host list (home): `ListView` of saved hosts (name, URL). Tap ⇒ push `HostPage`. Each row has text buttons「編輯」「刪除」(delete asks for confirmation). `FloatingActionButton.extended(label: Text('新增主機'))` ⇒ add dialog. Empty list ⇒「尚未新增主機」.
- Add/edit dialog: two `TextField`s (名稱, 網址). Save runs `Host.parse`; a `FormatException` message is shown under the fields and the dialog stays open. Only `https://` with a non-empty host is accepted (Secure cookie, see Decisions).
- Storage: `SharedPreferencesAsync`, key `hosts`, value = JSON list of `{name, url}`. Missing key ⇒ empty list (first launch). Corrupt JSON or a stored entry that fails `Host.parse` ⇒ exception shown on the list page as an error text (not swallowed, not reset).
- Host page: full-screen `WebViewWidget` inside `SafeArea`, no app bar (maximizes terminal rows). `JavaScriptMode.unrestricted`. Android back: if `canGoBack()` ⇒ `goBack()`, else pop to the host list (`PopScope(canPop: false, onPopInvokedWithResult: …)`). Main-frame load error (`WebResourceError.isForMainFrame`, `null` treated as main frame) ⇒ error view with `description`, `errorCode`, URL and a「重新載入」button (`loadRequest(host.url)`). No automatic retry.
- Cookies: the WebView's own cookie store (Android `CookieManager`) keeps the session cookie across app restarts; the app never reads or writes cookies. This relies on the host's `Max-Age=<session_ttl>` attribute (confirmed in the backend API section); a cookie without it would be a session cookie and be lost on app restart.
- Keyboard: `Scaffold` keeps `resizeToAvoidBottomInset: true` (default), so the web page's key bar stays above the IME. No native key bar.
- Theme: `ThemeData(brightness: Brightness.dark, useMaterial3: true)`; the host list is not part of the mockup token swap. No icons (text buttons only), so no icon pack is involved in the app.

**Constraints:**
- Dependencies exactly: `webview_flutter: ^4.14.1`, `shared_preferences: ^2.5.6`. Nothing else beyond the SDK.
- Android only: `flutter create --platforms android`.
- Application id `us.jhangy.roosterm` (`--org us.jhangy`) (confirmed by the lead).
- No timers, no polling, no `Future.delayed`.

**Acceptance criteria:**
- [ ] `cd app && flutter analyze` ⇒ `No issues found!`, RC=0.
- [ ] `cd app && flutter build apk --debug` RC=0, `app/build/app/outputs/flutter-apk/app-debug.apk` exists.
- [ ] `grep -n "android.permission.INTERNET" app/android/app/src/main/AndroidManifest.xml` matches.
- [ ] `grep -rnE "Future.delayed|Timer\(|sleep" app/lib` ⇒ no matches; `ls app/test` ⇒ absent.
- [ ] Live on the attached Android device (`adb devices` showed `10AFCF269C007FB` on 2026-11-19): `flutter run -d <device>`; adding `http://x` is rejected with the https message; add the real host ⇒ web login ⇒ open tab ⇒ `echo ok` prints `ok`; key bar Ctrl+C works; `adb shell am force-stop us.jhangy.roosterm`, relaunch, open the host ⇒ workspace shows without the login screen (cookie persisted); back button in a fresh page returns to the host list.
- [ ] `git show --stat HEAD` lists only `app/` paths.

**Steps:**

- [ ] **Step 1: Create the project**

```bash
cd /Users/jhangyu/project/roosterm
flutter create --org us.jhangy --project-name roosterm --platforms android app
rm -rf app/test
cd app && flutter pub add webview_flutter:^4.14.1 shared_preferences:^2.5.6
```

Check `app/pubspec.yaml` shows both constraints. Add `<uses-permission android:name="android.permission.INTERNET"/>` inside `<manifest>` of `app/android/app/src/main/AndroidManifest.xml` if the template lacks it. Set `android:label="Roosterm"` on `<application>`.

- [ ] **Step 2: `app/lib/hosts.dart`**

```dart
import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

class Host {
  const Host(this.name, this.url);

  final String name;
  final Uri url;

  /// Boundary parse. https only: the session cookie is Secure, an http host would loop on login.
  static Host parse(String name, String url) {
    final n = name.trim();
    if (n.isEmpty) {
      throw const FormatException('名稱：期望非空字串，實際為空');
    }
    final u = Uri.tryParse(url.trim());
    if (u == null || u.scheme != 'https' || u.host.isEmpty) {
      throw FormatException('網址：期望 https://<主機>[:埠][/路徑]，實際為 "$url"');
    }
    return Host(n, u);
  }

  Map<String, String> toJson() => {'name': name, 'url': url.toString()};
}

class HostStore {
  static const _key = 'hosts';
  final SharedPreferencesAsync _prefs = SharedPreferencesAsync();

  Future<List<Host>> load() async {
    final raw = await _prefs.getString(_key);
    if (raw == null) return []; // first launch: nothing saved yet
    final entries = jsonDecode(raw) as List<dynamic>;
    return [
      for (final e in entries)
        Host.parse((e as Map<String, dynamic>)['name'] as String, e['url'] as String),
    ];
  }

  Future<void> save(List<Host> hosts) =>
      _prefs.setString(_key, jsonEncode([for (final h in hosts) h.toJson()]));
}
```

- [ ] **Step 3: `app/lib/main.dart`**

```dart
import 'package:flutter/material.dart';

import 'host_list_page.dart';

void main() => runApp(const RoostermApp());

class RoostermApp extends StatelessWidget {
  const RoostermApp({super.key});

  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'Roosterm',
        theme: ThemeData(brightness: Brightness.dark, useMaterial3: true),
        home: const HostListPage(),
      );
}
```

- [ ] **Step 4: `app/lib/host_list_page.dart`**

```dart
import 'package:flutter/material.dart';

import 'host_page.dart';
import 'hosts.dart';

class HostListPage extends StatefulWidget {
  const HostListPage({super.key});

  @override
  State<HostListPage> createState() => _HostListPageState();
}

class _HostListPageState extends State<HostListPage> {
  final HostStore _store = HostStore();
  List<Host>? _hosts;
  Object? _loadError;

  @override
  void initState() {
    super.initState();
    _store.load().then(
          (hosts) => setState(() => _hosts = hosts),
          onError: (Object e) => setState(() => _loadError = e),
        );
  }

  Future<void> _replace(List<Host> hosts) async {
    await _store.save(hosts);
    setState(() => _hosts = hosts);
  }

  Future<void> _edit({Host? existing, int? index}) async {
    final host = await showDialog<Host>(
      context: context,
      builder: (_) => _HostDialog(existing: existing),
    );
    if (host == null) return; // dialog cancelled
    final hosts = [...?_hosts];
    if (index == null) {
      hosts.add(host);
    } else {
      hosts[index] = host;
    }
    await _replace(hosts);
  }

  Future<void> _delete(int index) async {
    final host = _hosts![index];
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('刪除「${host.name}」？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('刪除')),
        ],
      ),
    );
    if (ok == true) await _replace([..._hosts!]..removeAt(index));
  }

  @override
  Widget build(BuildContext context) {
    final Widget body;
    if (_loadError != null) {
      body = Center(child: Text('讀取主機清單失敗：$_loadError'));
    } else if (_hosts == null) {
      body = const Center(child: Text('讀取中…'));
    } else if (_hosts!.isEmpty) {
      body = const Center(child: Text('尚未新增主機'));
    } else {
      body = ListView.builder(
        itemCount: _hosts!.length,
        itemBuilder: (context, i) {
          final h = _hosts![i];
          return ListTile(
            title: Text(h.name),
            subtitle: Text(h.url.toString()),
            onTap: () => Navigator.push(
              context,
              MaterialPageRoute<void>(builder: (_) => HostPage(host: h)),
            ),
            trailing: Wrap(children: [
              TextButton(onPressed: () => _edit(existing: h, index: i), child: const Text('編輯')),
              TextButton(onPressed: () => _delete(i), child: const Text('刪除')),
            ]),
          );
        },
      );
    }
    return Scaffold(
      appBar: AppBar(title: const Text('Roosterm')),
      body: body,
      floatingActionButton: _hosts == null
          ? null
          : FloatingActionButton.extended(onPressed: () => _edit(), label: const Text('新增主機')),
    );
  }
}

class _HostDialog extends StatefulWidget {
  const _HostDialog({this.existing});

  final Host? existing;

  @override
  State<_HostDialog> createState() => _HostDialogState();
}

class _HostDialogState extends State<_HostDialog> {
  late final TextEditingController _name = TextEditingController(text: widget.existing?.name);
  late final TextEditingController _url = TextEditingController(text: widget.existing?.url.toString());
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    super.dispose();
  }

  void _save() {
    try {
      Navigator.pop(context, Host.parse(_name.text, _url.text));
    } on FormatException catch (e) {
      setState(() => _error = e.message);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
        title: Text(widget.existing == null ? '新增主機' : '編輯主機'),
        content: Column(mainAxisSize: MainAxisSize.min, children: [
          TextField(controller: _name, decoration: const InputDecoration(labelText: '名稱')),
          TextField(
            controller: _url,
            keyboardType: TextInputType.url,
            decoration: InputDecoration(labelText: '網址', hintText: 'https://', errorText: _error),
          ),
        ]),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')),
          TextButton(onPressed: _save, child: const Text('儲存')),
        ],
      );
}
```

- [ ] **Step 5: `app/lib/host_page.dart`**

```dart
import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';

import 'hosts.dart';

class HostPage extends StatefulWidget {
  const HostPage({super.key, required this.host});

  final Host host;

  @override
  State<HostPage> createState() => _HostPageState();
}

class _HostPageState extends State<HostPage> {
  late final WebViewController _controller;
  WebResourceError? _error;

  @override
  void initState() {
    super.initState();
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(NavigationDelegate(
        onWebResourceError: (e) {
          // null = platform did not say; treat as main frame so the failure is visible.
          if (e.isForMainFrame ?? true) setState(() => _error = e);
        },
      ))
      ..loadRequest(widget.host.url);
  }

  void _reload() {
    setState(() => _error = null);
    _controller.loadRequest(widget.host.url);
  }

  @override
  Widget build(BuildContext context) {
    final error = _error;
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, _) async {
        if (didPop) return;
        if (await _controller.canGoBack()) {
          await _controller.goBack();
        } else if (context.mounted) {
          Navigator.of(context).pop();
        }
      },
      child: Scaffold(
        body: SafeArea(
          child: error == null
              ? WebViewWidget(controller: _controller)
              : Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Column(mainAxisSize: MainAxisSize.min, children: [
                      Text('無法載入 ${widget.host.url}'),
                      Text('${error.errorCode}: ${error.description}'),
                      const SizedBox(height: 16),
                      FilledButton(onPressed: _reload, child: const Text('重新載入')),
                    ]),
                  ),
                ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 6: Analyze, build, live run, commit**

```bash
cd /Users/jhangyu/project/roosterm/app
flutter analyze > ../scratch/wp5-analyze.log 2>&1; echo "RC=$?" >> ../scratch/wp5-analyze.log
flutter build apk --debug > ../scratch/wp5-build.log 2>&1; echo "RC=$?" >> ../scratch/wp5-build.log
```

Both logs end with `RC=0`; the analyze log contains `No issues found!`. `flutter build apk` can exceed the 40 s foreground cap on a cold Gradle cache: if it times out, report BLOCKED to the lead for a gate-runner instead of backgrounding it. Live run per the acceptance list (needs a host URL from the lead). Then:

```bash
cd /Users/jhangyu/project/roosterm
git add app
git commit -m "feat(app): Flutter WebView shell with host list" -- app
```

`app/.gitignore` from the template excludes `build/` and `.dart_tool/`; confirm `git status --porcelain app` shows no build output before committing.

---

## Self-review

1. **Spec coverage:** login and logout (WP4), tab bar + rename + close (WP4), new tab + swarm picker (WP4), terminal view with full screen on attach (WP3), special-key bar in the web page shared by browser and app (WP3), disconnected banner + reconnect on `online`/`visibilitychange`, no timer (WP2 `onResume`, WP3), ended state「已結束（code N）」(signal/reason when no code) closed by the user (WP3), hc core offline/online banner on an open socket (WP2 `onCore`, WP3), close codes 4004/4400/4500/1001 mapped to visible messages (WP2, WP3), errors shown verbatim with op/tab/status/kind/hint (WP2 `ApiError`, WP4 `ErrorAlert`), 429 retry time (WP4), go:embed output dir + clean tree after build (WP1), single token file (WP1), Flutter WebView + local host list + WebView cookie persistence (cookie has `Max-Age`), no native key bar (WP5), no UI tests (all). Multi-client "size = last change" is backend behaviour; the client sends its size on every attach (URL) and resize.
2. **Placeholder scan:** no TBD/TODO and no *(assumed)* marker remain; every wire value is taken from the backend plan at `86de08a`.
3. **Type consistency:** `Tab.end: TabEnd | null` used in the WP2 parser, WP3 initial state, WP4 tab bar and `markExited`; `parseTabEnd` shared by REST and the 4000 close; `SocketEnd` kinds are exactly the ones `StatusBanner` and `handleEnd` switch on; `TerminalPaneProps.onExited(tabId, end: TabEnd)` matches the WP4 call site; `createTab(request)` matches `NewTabDialog.onCreate(request)`; `renameTab` returns `Tab` and WP4 replaces the tab with it; `onResume` returns an unsubscribe used directly as effect cleanup in WP3/WP4.

## Reconciliation record

Reconciled against `docs/superpowers/plans/2026-11-19-roosterm-backend.md` §"HTTP/WS API" (line 50) at `86de08a`. Every former *(assumed)* row is replaced by the backend value. Changes that affected package code, not only names: 4000 reason is JSON with nullable `exitCode` (`TabEnd`, `endLabel`); core offline/online are text messages on an open socket, no 4001 (`onCore`, `core-offline` pane state); WS URL carries `?cols&rows` (size moved from first resize message to URL); error bodies are JSON (`ApiError.kind`); `POST /logout` (WP4 button); dev proxy port `8787`; `scripts/build.py` already runs the web build (WP1 no longer edits it); close codes 4004/4400/4500/1001 mapped in WP2/WP3; backend placeholder `dist/index.html` replaced by `.gitkeep` so builds leave the tree clean. Android application id stays `us.jhangy.roosterm` (lead decision).

