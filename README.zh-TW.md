# roosterm

*[English version / 英文版本](README.md)*

roosterm 是自行架設的網頁終端機：透過 HTTPS 連到你自己的主機，在瀏覽器或 Android App 中使用完整的終端機（登入 shell、Claude Code，或任何程式）。關閉瀏覽器或重啟 roosterm，都不會中斷正在執行的程式。
<!-- evidence: docs/design.md:4 goal statement; host/cmd/roosterm/main.go:19-22 the two subcommands passwd and serve; app/README.md:3 Android WebView shell -->

roosterm 只是終端機外殼：它不解析 Claude Code，也沒有聊天介面、推播通知、多帳號或 Windows 主機支援。
<!-- evidence: docs/design.md:6-9 non-goals -->

### 主要功能

- **程式不隨用戶端或伺服器結束。** 所有 PTY 都由 herdr-core（`hc`）持有，而非 roosterm。關閉瀏覽器、重啟或崩潰 roosterm、甚至 hc core 被殺掉，程式與 PID 都不變；只有 `hc server stop` 才會結束它們。
  <!-- evidence: docs/design.md:24-25; host/cmd/roosterm/serve.go:57 roosterm only dials hc through hc.Start -->
- **roosterm 不保存分頁狀態。** 分頁就是帶有標籤 `roosterm=tab` 的 hc 終端機，hc 的終端機清單是唯一的分頁儲存處。在 roosterm 之外建立的終端機，roosterm 看不到。
  <!-- evidence: docs/design.md:19; docs/api.md:9 -->
- **三種新工作階段。** Shell、Claude Code，或接上執行中的 Claude Code swarm（`claude-swarm-*` tmux socket）。
  <!-- evidence: web/src/workspace/NewSessionDialog.tsx:25 type Kind = "shell" | "claude" | "swarm"; docs/api.md:53 GET /api/swarms; web/src/workspace/tabs.ts:22 CLAUDE_PROGRAM -->
- **適合手機的按鍵列。** Esc、Tab、Shift-Tab 與方向鍵會送出對應的終端機序列，並支援 application-cursor 模式。
  <!-- evidence: web/src/lib/keys.ts:1-18; web/src/terminal/KeyBar.tsx -->
- **不靠計時器的重新連線。** 重新連線就是重新 attach，hc 會重繪整個畫面。網頁用戶端在瀏覽器恢復連線或頁面回到前景時重連；hc 的 socket 重新出現時，伺服器端會自動重新連上。
  <!-- evidence: web/src/lib/resume.ts:1-10; docs/design.md:27-31; host/internal/hc/supervisor.go:11,34 fsnotify watcher -->
- **單一獨立執行檔。** 網頁用戶端內嵌在 Go 執行檔中；執行時不需要 Docker、資料庫或 Node。
  <!-- evidence: host/internal/web/static.go:15 //go:embed all:dist; docs/design.md:22 -->
- **資料夾選擇器與分頁管理。** 可瀏覽家目錄下的子資料夾，在該處開新分頁；分頁可改名，也可從分頁列隱藏（程式仍持續執行）。
  <!-- evidence: docs/api.md:52 GET /api/dirs, :50 PATCH /api/tabs/{id} (label, hidden); web/src/workspace/FolderPicker.tsx -->

### 目錄

- [架構](#架構)
- [需求](#需求)
- [建置](#建置)
- [首次設定](#首次設定)
- [設定檔](#設定檔)
- [以服務方式執行](#以服務方式執行)
- [使用用戶端](#使用用戶端)
- [安全性說明](#安全性說明)
- [API](#api)
- [開發與測試](#開發與測試)
- [專案結構](#專案結構)

### 架構

```
browser / Flutter WebView ──HTTPS (你的反向代理)──▶ roosterm (單一 Go 執行檔) ──Unix socket, JSON-RPC──▶ herdr-core (hc)
```

- `host/` 是 Go 伺服器：登入、分頁的 REST、每個分頁一條 WebSocket（`/ws/tabs/{id}`），以及內嵌的網頁檔案。它不持有任何程序，也不保存分頁狀態。
- `web/` 是 React + Vite + xterm.js 用戶端；按鍵列與重新連線邏輯在這裡，並與 App 共用。
- `app/` 是 Flutter WebView 外殼加主機清單；WebView 負責保存 session cookie。
- hc（固定為 0.1.21）持有所有 PTY，並執行你的 shell、`claude`，或 swarm 分頁用的 `tmux a`。
<!-- evidence: docs/design.md:11-22; docs/hc-bridge.md:1 hc 0.1.21; web/package.json:15-16 react and @xterm/xterm dependencies -->

roosterm 本身不處理 TLS：它以純 HTTP 監聽（預設 `127.0.0.1:8787`），HTTPS 需由反向代理負責。
<!-- evidence: host/internal/config/config.go:17 DefaultListen; host/cmd/roosterm/serve.go:71 net.Listen("tcp") with no TLS; docs/api.md:16 "the reverse proxy terminates TLS" -->

### 需求

- 主機上要有執行中的 herdr-core（`hc` 0.1.21）。啟動時若連不到 hc socket，roosterm 會以非零狀態結束。roosterm 不負責安裝 hc。
  <!-- evidence: docs/design.md:18,35; deploy/README.md:3 -->
- macOS 或 Linux 主機（Windows 主機不在目標內）。提供 launchd（macOS）與 systemd（Linux）的服務腳本。
  <!-- evidence: docs/design.md:7; scripts/service.py:2,176-177; deploy/systemd/roosterm.service -->
- 建置需要：Go 1.26.11（`host/go.mod`）、Node 與 npm（網頁用戶端）、Python 3（腳本）。
  <!-- evidence: host/go.mod:3; scripts/build.py:39-45 npm ci, npm run build, go build -->
- 建置 Android App 需要 Flutter SDK（Dart `^3.12.2`）。
  <!-- evidence: app/pubspec.yaml:8; app/README.md:5-8 -->

### 建置

```sh
python3 scripts/build.py                        # 產生 build/roosterm（本機平台）
python3 scripts/build.py --target linux/arm64   # 產生 build/roosterm-linux-arm64
```

腳本會先對網頁用戶端執行 `npm ci` 與 `npm run build`，再以 `CGO_ENABLED=0` 執行 `go build -trimpath`。`--target` 接受 `GOOS/GOARCH`。
<!-- evidence: scripts/build.py:3-5,29,31,37-45 -->

Android App（debug 簽章，供側載）：

```sh
cd app
flutter analyze
flutter build apk
```
<!-- evidence: app/README.md:5-8; docs/logs/2026-11-21-refactor2.md "Android release stays debug-signed" -->

發版與 CI：`python3 scripts/ci.py bump patch`（或 `minor`/`major`，或明確指定版本如 `bump 0.0.0+1`）會更新 `app/pubspec.yaml`、提交並建立標籤 `app-vX.Y.Z`；已追蹤檔案有未提交變更（未追蹤檔案不計）或標籤已存在時會拒絕執行。接著 `git push origin HEAD app-vX.Y.Z` 觸發 `.github/workflows/android.yml`：先跑測試防護（`ci.py guard`；CI 永不跑測試），再建置各 ABI 的 release APK（`ci.py build android`），最後發佈 GitHub Release，附 `us.jhangy.roosterm-{arm64-v8a,armeabi-v7a,x86_64}.apk`。發佈使用內建的 `GITHUB_TOKEN`，不需設定 secrets。本機執行 `git config core.hooksPath .githooks` 後，每次 push 都會跑 `python3 scripts/ci.py prepush`（防護＋同一套建置，輸出在 `app/build/dist/`）。一般的 `flutter build apk` 仍只含 arm64；只有分 ABI 建置才包含三種 ABI。
<!-- evidence: .github/workflows/android.yml; scripts/ci.py PLATFORMS, bump, release; scripts/ci_guard.py; .githooks/pre-push; app/android/build.gradle.kts split-per-abi -->

### 首次設定

1. 將 `hc` 與 `roosterm` 安裝到 `~/.local/bin`（服務設定檔預期在這個路徑）。
2. 在你的登入 shell 執行 `roosterm passwd`。它會詢問使用者名稱、密碼與確認密碼，以 argon2id 雜湊密碼，並寫入 `~/.config/roosterm/config.toml`（權限 0600），內容為預設值加上目前的 `$SHELL`。
3. 執行 `roosterm serve`，或安裝下方的服務。

<!-- evidence: deploy/README.md:3; host/cmd/roosterm/passwd.go:38-64 prompts, hash, config.Save; host/cmd/roosterm/passwd.go:76-81 defaults and $SHELL; host/internal/config/config.go:119-120 Save writes mode 0600 -->

兩個子命令都接受 `--config PATH`；預設為 `$XDG_CONFIG_HOME/roosterm/config.toml`，否則 `~/.config/roosterm/config.toml`。對既有設定檔再次執行 `passwd` 時，只會更新使用者名稱與雜湊。
<!-- evidence: host/cmd/roosterm/main.go:47; host/internal/config/config.go:50-57; host/cmd/roosterm/passwd.go:69-71,62-63 -->

### 設定檔

`config.toml` 的欄位（除 `hc_socket` 外皆必填；載入時會拒絕未知欄位）：

| 欄位 | `passwd` 寫入的預設值 | 說明 |
|---|---|---|
| `listen` | `127.0.0.1:8787` | HTTP 監聽位址 |
| `hc_socket` | 空 | hc socket 路徑；留空則使用 hc 自己的預設位置規則 |
| `ping_interval` | `25s` | WebSocket ping 間隔，避免代理切斷閒置連線 |
| `session_ttl` | `720h0m0s` | session cookie 與 session 的有效期限 |
| `shell` | 你的 `$SHELL` | 可執行檔的絕對路徑；新的 shell 分頁以 `shell -l` 啟動 |
| `username` | 由 `passwd` 設定 | 唯一的登入使用者 |
| `password_hash` | 由 `passwd` 設定 | argon2id 雜湊 |

<!-- evidence: host/internal/config/config.go:17-19 defaults, :39-45 toml keys, :71-72 unknown keys rejected, :105-108 absolute executable shell; docs/api.md:123-133 example file; docs/api.md:73 command omitted => [config.shell, "-l"] -->

session 儲存在設定檔旁的 `sessions.json`（權限 0600）。
<!-- evidence: host/internal/config/config.go:61 SessionsPath; host/internal/auth/session.go:23,55 -->

### 以服務方式執行

兩種服務都不會停止 hc：**切勿在正在使用的主機上執行 `hc server stop`，它會結束每個分頁中的所有程式。**
<!-- evidence: deploy/README.md:5; scripts/service.py:11 -->

macOS（launchd），先把 `hc` 與 `roosterm` 放到 `~/.local/bin`：

```sh
python3 scripts/service.py install    # 產生 plist 到 ~/Library/LaunchAgents 並載入
python3 scripts/service.py status     # 是否載入、pid、上次結束碼、連接埠是否有回應
python3 scripts/service.py restart    # 另有 start、stop、uninstall
```

日誌位於 `~/Library/Logs/roosterm.log`。plist 會啟動登入 shell（`/bin/zsh -lc`）；若你的 shell 不是 zsh，請修改 `deploy/launchd/dev.roosterm.serve.plist` 後重新執行 `install`。
<!-- evidence: scripts/service.py:4-9,22,26,27,92-94; deploy/README.md:9-17 -->

Linux（systemd 使用者單元；`herdr-core.service` 執行 `hc server start`，`roosterm.service` 依賴它）：

```sh
mkdir -p ~/.config/systemd/user
cp deploy/systemd/herdr-core.service deploy/systemd/roosterm.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now herdr-core.service roosterm.service
loginctl enable-linger "$USER"   # 開機即啟動，不需登入工作階段
```

日誌：`journalctl --user -u roosterm.service`。
<!-- evidence: deploy/README.md:21-30; deploy/systemd/herdr-core.service:6-7; deploy/systemd/roosterm.service:2-8 -->

### 使用用戶端

**網頁用戶端。** 開啟反向代理的 HTTPS 網址並登入。側邊欄列出你的工作階段；新增工作階段的對話框可選 Shell、Claude 或 Swarm，並提供資料夾選擇器。工作階段可改名或隱藏，已結束的工作階段可重新啟動或關閉。關閉分頁會終止仍在執行的程式。
<!-- evidence: web/src/screens/LoginScreen.tsx; web/src/workspace/Sessions.tsx; web/src/workspace/NewSessionDialog.tsx:25-31; docs/api.md:82-83 DELETE kills live then removes; web/src/terminal/TerminalPane.tsx -->

**Android App。** 新增主機（名稱加上 `https://` 網址；因為 session cookie 帶有 `Secure`，純 `http` 會被拒絕），再於 WebView 中開啟。App 只載入所設定主機同源的頁面；指向其他網站的連結會被擋下，並顯示該網址的提示。僅支援 Android。
<!-- evidence: app/lib/hosts.dart:10-20,25-32 sameOrigin; app/lib/host_page.dart:26-27 WebViewController; app/lib/host_page.dart:29-34 onNavigationRequest; app/README.md:3 -->

### 安全性說明

- 單一帳號，argon2id 密碼雜湊存於 `config.toml`。登入限流：失敗次數分兩個桶計算，帶有 `roosterm_device` cookie 的瀏覽器一桶，其餘一桶；單一桶連續失敗 5 次會鎖定該桶的 `/login` 60 秒（HTTP 429 並附 `Retry-After`），登入成功會重置計數。此限制不是依 IP。
  <!-- evidence: host/internal/auth/ratelimit.go:8-10,13; host/internal/web/login.go:29-32 bucket choice; host/cmd/roosterm/serve.go:67 two limiters; docs/api.md:89 -->
- 已知裝置 cookie `roosterm_device`：登入成功時設定，`Path=/login`、400 天、HttpOnly、Secure、SameSite=Strict。它只用來選擇限流桶，不授予任何存取權。
  <!-- evidence: host/internal/web/login.go:54; host/internal/auth/cookie.go:14-16,28-30 -->
- Session cookie `roosterm_session`：HttpOnly、Secure、SameSite=Strict；伺服器只保存 token 的 SHA-256。sessions 檔案權限若不是 0600，伺服器會拒絕載入。
  <!-- evidence: host/internal/auth/cookie.go:10-12,33-35; docs/api.md:12,13; host/internal/auth/session.go:57-58 -->
- 更改密碼會登出所有裝置，但要等伺服器重啟後才生效：`sessions.json` 記錄密碼雜湊的摘要，伺服器以不同雜湊啟動時會丟棄所有 session。執行 `roosterm passwd` 後，舊 session 在重啟 `roosterm serve` 之前仍然有效。要登出所有裝置，再執行一次 `roosterm passwd` 並重啟。升級到此版本後的第一次啟動會讓所有人登出一次。
  <!-- evidence: host/internal/auth/session.go:32,47-49,68; host/cmd/roosterm/serve.go:61; host/cmd/roosterm/passwd.go:28 -->
- 跨來源防護：瀏覽器標示為來自其他來源的 POST、PATCH、DELETE 請求（含 `/login`、`/logout`）會以 403、kind `cross_origin` 拒絕。JSON 請求本文必須帶 `Content-Type: application/json`，否則回 415。不經瀏覽器的 API 用戶端必須送出此標頭。
  <!-- evidence: host/internal/web/server.go:38,41-52 crossOriginGuard; host/internal/apierr/apierr.go:20,76-81 -->
- 對外暴露的內容：分頁的 REST 端點、每個分頁一條 WebSocket，以及僅限家目錄下的資料夾列表（解析後落在家目錄之外的路徑，含透過 symlink 者，會以 403 拒絕）。能登入的人就擁有 roosterm 使用者身分的完整 shell，因此請放在 HTTPS 之後並使用強密碼。
  <!-- evidence: docs/api.md:33,86 outside_home -->
- 分頁中啟動的程式會取得清理過的環境：繼承自 hc core 的 `TMUX`、`TMUX_PANE` 與 `CLAUDE*` 變數會被 unset。
  <!-- evidence: host/internal/api/scrub.go:5-9 -->

### API

HTTP 與 WebSocket 的契約（端點、cookie、錯誤格式、關閉碼）見 [docs/api.md](docs/api.md)。hc 通訊協定筆記見 [docs/hc-bridge.md](docs/hc-bridge.md)，設計摘要見 [docs/design.md](docs/design.md)。
<!-- evidence: docs/api.md:1-3; docs/hc-bridge.md:1-3; docs/design.md:1 -->

### 開發與測試

```sh
python3 scripts/test.py                          # gofmt、go vet、go test -race，含時間預算
cd web && npx tsc -b && npx oxlint src           # 網頁型別檢查與 lint
cd app && flutter analyze                        # Android 外殼靜態分析
```

`scripts/test.py` 在下列情況會失敗：測試程式碼出現 sleep 類字樣、任一測試超過 1 秒、整體超過 40 秒，或有測試被跳過。網頁用戶端沒有提交任何測試。
<!-- evidence: scripts/test.py:14-20,28-37,54-63,93-106; web/package.json:10 lint script; docs/logs/2026-11-21-refactor2.md "Evidence" section; docs/logs/2026-11-21-refactor2.md "no committed web test" -->

### 專案結構

```
host/cmd/roosterm/   main、serve、passwd
host/internal/       api、apierr、atomicfile、auth、bridge、config、hc、swarm、web
web/src/             api、components、lib、screens、terminal、workspace、styles
app/lib/             Flutter 主機清單與 WebView 頁面
deploy/              systemd 單元、launchd plist、deploy/README.md
docs/                design.md、api.md、hc-bridge.md、logs/
scripts/             build.py、service.py、test.py
```
<!-- evidence: host/cmd/roosterm/ listing; host/internal/ listing; web/src listing; app/lib listing; deploy/ listing -->
