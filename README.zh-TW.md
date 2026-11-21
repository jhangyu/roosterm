# roosterm

*[English version / 英文版本](README.md)*

roosterm 是自行架設的網頁終端機：透過 HTTPS 連到你自己的主機，在瀏覽器或 Android App 中使用完整的終端機（登入 shell、Claude Code，或任何程式）。關閉瀏覽器或重啟 roosterm，都不會中斷正在執行的程式。
<!-- evidence: docs/design.md:4; host/cmd/roosterm/main.go:19-22; app/README.md:3 -->

伺服器跑在你自己的 Mac 或 Linux 機器上。你可以用任何瀏覽器連線，也可以用 Android App，App 可直接從 [GitHub Releases](https://github.com/jhangyu/roosterm/releases) 下載現成的版本。
<!-- evidence: docs/design.md:7 (no Windows host); gh release view app-v0.0.0 (three APK assets); .github/workflows/android.yml -->

roosterm 只是終端機外殼：它不解析 Claude Code，也沒有聊天介面、推播通知、多帳號或 Windows 主機支援。
<!-- evidence: docs/design.md:6-9 -->

## 主要功能

本節說明你能用 roosterm 做什麼。

- **程式不隨用戶端或伺服器結束。** 所有終端機都由 herdr-core（`hc`）持有，而非 roosterm。關閉瀏覽器、重啟或崩潰 roosterm，甚至 hc core 被殺掉，程式與 PID 都不變；只有 `hc server stop` 才會結束它們。
  <!-- evidence: docs/design.md:24-25 -->
- **roosterm 不保存分頁狀態。** 分頁就是 hc 裡帶有標籤 `roosterm=tab` 的終端機，hc 的終端機清單是唯一的分頁儲存處。在 roosterm 之外建立的終端機，roosterm 看不到。
  <!-- evidence: docs/design.md:19; docs/api.md:9 -->
- **三種新工作階段。** Shell、Claude Code，或接上執行中的 Claude Code swarm（`claude-swarm-*` tmux socket）。
  <!-- evidence: web/src/workspace/NewSessionDialog.tsx:25,31; docs/api.md:53 -->
- **適合手機的按鍵列。** Esc、Tab、Shift-Tab 與方向鍵會送出對應的終端機序列，並支援 application-cursor 模式。
  <!-- evidence: web/src/lib/keys.ts:1-18 -->
- **不靠計時器的重新連線。** 重新連線就是重新 attach，hc 會重繪整個畫面。網頁用戶端在瀏覽器恢復連線或頁面回到前景時重連；hc 的 socket 重新出現時，伺服器會自動重連。
  <!-- evidence: docs/design.md:27-31; host/internal/hc/supervisor.go:26-29 -->
- **單一獨立執行檔。** 網頁用戶端內嵌在 Go 執行檔中；執行時不需要 Docker、資料庫或 Node。
  <!-- evidence: host/internal/web/static.go:15; docs/design.md:22 -->
- **資料夾選擇器與分頁管理。** 可瀏覽家目錄下的子資料夾，在該處開新分頁；分頁可改名，也可從分頁列隱藏（程式仍持續執行）。
  <!-- evidence: docs/api.md:86, :79-80 -->

## 目錄

- [運作方式](#運作方式)
- [開始之前](#開始之前)
- [安裝](#安裝)
- [建立登入帳號](#建立登入帳號)
- [設定檔](#設定檔)
- [啟動伺服器](#啟動伺服器)
- [設定 HTTPS](#設定-https)
- [在瀏覽器使用](#在瀏覽器使用)
- [Android App](#android-app)
- [安全性說明](#安全性說明)
- [自行建置 Android App](#自行建置-android-app)
- [發布與 CI](#發布與-ci)
- [開發與測試](#開發與測試)
- [API 與設計文件](#api-與設計文件)
- [專案結構](#專案結構)

## 運作方式

本節說明三個元件，以及各自負責什麼。

```
browser / Flutter WebView ──HTTPS (你的反向代理)──▶ roosterm (單一 Go 執行檔) ──Unix socket, JSON-RPC──▶ herdr-core (hc)
```

- herdr-core（`hc`，版本 0.1.21）是讓終端機持續執行的程式，它持有每個終端機與裡面的程式。若主機上沒有它，roosterm 安裝程式會幫你安裝。
  <!-- evidence: docs/hc-bridge.md:1; deploy/README.md:3 -->
- roosterm 是同一台機器上的一支程式：它提供登入頁與終端機畫面，並透過本機 socket 與 `hc` 溝通。它不保存任何終端機狀態。
  <!-- evidence: docs/design.md:15; host/cmd/roosterm/serve.go:56-57 -->
- roosterm 只講純 HTTP（預設 `127.0.0.1:8787`）。你要在它前面放一個提供 HTTPS 的反向代理（在 roosterm 前面加上 HTTPS 的網頁伺服器）。
  <!-- evidence: host/internal/config/config.go:17; host/cmd/roosterm/serve.go:71; docs/api.md:16 -->

## 開始之前

本節列出你需要準備的東西。

- 主機：macOS 或 Linux（不支援 Windows 主機）。
  <!-- evidence: docs/design.md:7; deploy/install.py:166-167; deploy/systemd/ -->
- Python 3.9 或更新版本，用來執行安裝程式（macOS 內建的 `python3` 就夠），以及 `curl` 與 `tar`，安裝程式用它們下載 `hc`。
  <!-- evidence: deploy/install.py:164-165,239-242; docs/logs/contract-server-package.md:9 -->
- `hc` 0.1.21（由安裝程式設定）。啟動時若連不到 `hc` 的 socket，roosterm 會報錯並結束。
  <!-- evidence: host/internal/hc/supervisor.go:26-32; docs/design.md:35 -->
- 只有從原始碼建置 roosterm 時才需要：`git`、Go 1.26.11、Node.js 與 npm、Python 3。
  <!-- evidence: host/go.mod:3; scripts/build.py:77-82,98 -->
- 一個有 HTTPS 的網域或位址，由你自己架設的反向代理提供（任何支援 WebSocket 的代理都可以）。
  <!-- evidence: docs/api.md:16; host/internal/web/server.go:35 (GET /ws/tabs/{id}) -->
- 選用：一支 Android 手機，用來裝 App。
- 一份適合你主機的預先建置伺服器套件，從 [GitHub Releases](https://github.com/jhangyu/roosterm/releases) 下載（見〈安裝〉）。
  <!-- evidence: scripts/ci.py:94-105; .github/workflows/android.yml:34-45 -->

## 安裝

本節把 `hc` 與 `roosterm` 裝到主機上，並以服務執行。

### 用伺服器套件安裝

本步驟下載一個套件，並執行其中的互動式安裝程式。

1. 開啟 [GitHub Releases](https://github.com/jhangyu/roosterm/releases)，下載適合你主機的套件：

   | 檔案 | 主機 |
   |---|---|
   | `roosterm-server-darwin-arm64.tar.gz` | Apple silicon 的 Mac |
   | `roosterm-server-darwin-amd64.tar.gz` | Intel Mac |
   | `roosterm-server-linux-amd64.tar.gz` | Linux x86_64 |
   | `roosterm-server-linux-arm64.tar.gz` | Linux ARM64 |
   <!-- evidence: scripts/ci.py:94-105; docs/logs/contract-server-package.md:27-30 -->
2. 解壓縮，並在你平常的登入 shell 執行安裝程式（請把檔名換成你下載的檔案）：

   ```sh
   tar xzf roosterm-server-darwin-arm64.tar.gz
   python3 roosterm-server-darwin-arm64/install.py
   ```
3. 依提示回答。安裝程式會：
   - 把 `roosterm` 複製到 `~/.local/bin`，並確認它能在這台機器上執行（在 macOS 上，還會清除下載時附加在檔案上的 quarantine 旗標，因為你選擇執行這個套件）；
   - 當 `hc` 不存在或版本較舊時，用 herdr 官方安裝腳本安裝 `hc` 0.1.21；已安裝的 `hc` 若是 0.1.21 或更新則保留；
   - 詢問登入 shell、監聽位址（預設 `127.0.0.1:8787`）、使用者名稱與密碼，並寫入 `~/.config/roosterm/config.toml`；
   - 安裝並啟動服務（macOS 是 launchd 工作，Linux 是 systemd 使用者單元；在 Linux 上還會詢問是否執行 `loginctl enable-linger`，讓 roosterm 開機就啟動），最後印出要開啟的位址。
   <!-- evidence: deploy/install.py:199-203,321-344; scripts/ci.py:94-105 -->

- 要升級或重新設定，再執行一次 `python3 install.py`（在解壓縮後的資料夾中）。已有設定檔時，它會顯示目前的監聽位址、shell 與使用者名稱，然後詢問 `Keep listen address and shell? [Y/n]` 與 `Change username/password? [y/N]`。登入狀態會保留，執行中的程式也不會被停止。
  <!-- evidence: deploy/install.py:256-277,194-212; docs/logs/contract-server-package.md:9 -->
- 在同一個資料夾控制服務：

  ```sh
  python3 install.py status      # service state and whether the port answers
  python3 install.py restart     # also: start, stop, uninstall
  ```

  `uninstall` 只移除服務檔；`hc` 會繼續執行，設定檔與登入狀態也都保留。
  <!-- evidence: deploy/install.py:387-444,471-480 -->
- 當 `XDG_CONFIG_HOME` 設成 `~/.config` 以外的值時，安裝程式會拒絕執行，因為服務讀取的是 `~/.config/roosterm`；安裝時請先取消設定。
  <!-- evidence: deploy/install.py:188-192 -->
- HTTPS 仍需你自己處理：見[設定 HTTPS](#設定-https)。

### 手動安裝（進階）

本步驟以手動方式完成安裝程式做的事，並從原始碼建置 roosterm。下面的〈建立登入帳號〉、〈設定檔〉與〈啟動伺服器〉詳細說明同樣的結果。

1. 用 herdr-core 官方安裝腳本（`https://releases.herdr.dev/core/install.sh`）把 `hc` 0.1.21 裝到 `~/.local/bin`。安裝腳本會檢查 sha256，在 macOS 上還會檢查程式碼簽章。

   ```sh
   curl -fsSL https://releases.herdr.dev/core/install.sh -o install.sh
   HC_VERSION=0.1.21 HC_INSTALL_DIR=$HOME/.local/bin sh install.sh
   ```

   裝好後，`~/.local/bin/hc --version` 應印出 `hc 0.1.21`。
   <!-- evidence: https://releases.herdr.dev/core/install.sh (HTTP 200); docs/logs/2026-11-19-spike-hc.md:35,37 -->
2. 下載原始碼並建置：

   ```sh
   git clone https://github.com/jhangyu/roosterm.git
   cd roosterm
   python3 scripts/build.py
   ```

   結果是 `build/roosterm`。腳本會先對網頁用戶端執行 `npm ci` 與 `npm run build`，再以 `CGO_ENABLED=0` 執行 `go build -trimpath`，最後印出 `wrote build/roosterm`。
   <!-- evidence: git remote -v; scripts/build.py:77-82,98-99 -->
3. 把執行檔複製到 `~/.local/bin`：

   ```sh
   mkdir -p ~/.local/bin
   cp build/roosterm ~/.local/bin/
   ```

   〈啟動伺服器〉的服務設定檔預期 `hc` 與 `roosterm` 都在 `~/.local/bin`。
   <!-- evidence: deploy/systemd/roosterm.service:7; deploy/launchd/dev.roosterm.serve.plist:11 -->

- 在一台機器上為另一台建置：`python3 scripts/build.py --target linux/arm64` 會產生 `build/roosterm-linux-arm64`；`--target` 接受 `GOOS/GOARCH`。把該檔案複製到主機的 `~/.local/bin/roosterm` 即可。
  <!-- evidence: scripts/build.py:4-5,36-41,94-99 -->
- 自己建置伺服器套件：執行 `python3 scripts/build.py --package darwin/arm64 linux/amd64`，每個目標都會產生 `build/roosterm-server-<os>-<arch>.tar.gz`。
  <!-- evidence: scripts/build.py:6,70-72; docs/logs/contract-server-package.md:8 -->
- 日後更新：重新建置、再複製一次、重啟服務（見〈啟動伺服器〉）。

## 建立登入帳號

本節以手動方式建立你的登入帳號；安裝程式會替你完成這件事。

```sh
~/.local/bin/roosterm passwd
```

- 請在你平常的登入 shell 執行。它會詢問 `username:`、`password:` 與 `repeat password:`（輸入密碼時不會顯示）。
  <!-- evidence: host/cmd/roosterm/passwd.go:39-50,101-107 -->
- 它以 argon2id 雜湊密碼，並寫入 `~/.config/roosterm/config.toml`，內容為預設值加上你目前的 `$SHELL`；檔案權限 0600，資料夾權限 0700，只有你能讀。若 `$SHELL` 是空的，它會停止並顯示 `SHELL is empty; run this from your login shell`。
  <!-- evidence: host/cmd/roosterm/passwd.go:59-65,77-82; host/internal/config/config.go:22-25,119-135 -->
- 完成後會印出 `wrote <path>`。
  <!-- evidence: host/cmd/roosterm/passwd.go:27 -->
- 之後再執行一次，只會更新使用者名稱與密碼，其他設定不變。
  <!-- evidence: host/cmd/roosterm/passwd.go:68-73,63-65 -->
- `passwd` 與 `serve` 都接受 `--config PATH`，改用其他設定檔。預設是 `$XDG_CONFIG_HOME/roosterm/config.toml`（有設定該變數時），否則是 `~/.config/roosterm/config.toml`。
  <!-- evidence: host/cmd/roosterm/main.go:19-22,47; host/internal/config/config.go:48-58 -->
- 更改密碼與登出所有裝置，見[安全性說明](#安全性說明)。

## 設定檔

本節說明每個設定的作用，以及如何修改。

用任何文字編輯器修改 `config.toml`，再重啟 roosterm（見〈啟動伺服器〉）。設定檔只在啟動時讀取一次。
<!-- evidence: host/cmd/roosterm/serve.go:43 -->

範例檔案：

```toml
listen = "127.0.0.1:8787"
hc_socket = ""
ping_interval = "25s"
session_ttl = "720h0m0s"
shell = "/bin/zsh"
username = "me"
password_hash = "$argon2id$v=19$m=65536,t=3,p=4$<salt b64>$<key b64>"
```

| 鍵 | `roosterm passwd` 寫入的預設值 | 說明 |
|---|---|---|
| `listen` | `127.0.0.1:8787` | roosterm 監聽的位址與連接埠（純 HTTP）。反向代理跑在同一台機器時，保持 `127.0.0.1` |
| `hc_socket` | 空 | `hc` socket 的路徑；留空則使用 hc 自己的預設位置 |
| `ping_interval` | `25s` | 伺服器對每條開啟中的終端機連線送 ping 的間隔，避免代理切斷閒置連線 |
| `session_ttl` | `720h0m0s`（30 天） | 登入維持有效的時間 |
| `shell` | 你的 `$SHELL` | 可執行檔的絕對路徑；新的 Shell 分頁以登入 shell（`shell -l`）啟動 |
| `username` | 由 `passwd` 設定 | 唯一的登入使用者 |
| `password_hash` | 由 `passwd` 設定 | argon2id 雜湊；請用 `roosterm passwd` 設定，不要手改 |

<!-- evidence: host/internal/config/config.go:16-20,38-46,71-72,74-81,97-117; docs/api.md:73 -->

- 除 `hc_socket` 外，每個鍵都必填；出現未知的鍵，伺服器會報錯並停止；時間長度用 Go 格式，如 `25s`、`30m`、`720h`，且必須是正值。
  <!-- evidence: host/internal/config/config.go:71-81,97-103 -->
- 登入狀態存放在 `config.toml` 旁的 `sessions.json`（權限 0600）。
  <!-- evidence: host/internal/config/config.go:60-63; host/internal/auth/session.go:43-45 -->

## 啟動伺服器

本節讓 roosterm 跑起來。安裝程式已設定好服務，所以只有手動安裝，或想在前景試跑時才需要這一節。

**切勿在正在使用的主機上執行 `hc server stop`，它會結束每個分頁中的所有程式。** 下面的指令都不會停止 hc。
<!-- evidence: deploy/README.md:5; deploy/install.py -->

### 先在前景試跑

```sh
~/.local/bin/hc server start
~/.local/bin/roosterm serve
```

- 即使 hc 已在執行，`hc server start` 也可以安全重複執行。
  <!-- evidence: deploy/README.md:3 ("idempotent") -->
- 成功時會印出：`listening on 127.0.0.1:8787, hc <socket path>`。
  <!-- evidence: host/cmd/roosterm/serve.go:77 -->
- 按 Ctrl-C 停止 roosterm；分頁中的程式仍在 hc 裡繼續執行。
  <!-- evidence: host/cmd/roosterm/serve.go:54,84-87; docs/design.md:24-25 -->
- 若 hc 沒有在執行，`roosterm serve` 會報錯並結束。
  <!-- evidence: host/internal/hc/supervisor.go:26-32 -->
- 設定 HTTPS 之前，可以先在同一台機器的瀏覽器開啟 `http://localhost:8787` 快速確認，因為瀏覽器視 `localhost` 為安全來源，cookie 可正常使用。
  <!-- evidence: docs/api.md:16 -->
- 安裝服務前，請先停掉這個前景的 roosterm：同一個連接埠只能有一個 roosterm 使用。
  <!-- evidence: deploy/README.md:17 -->

### 在 macOS 以服務執行

本步驟安裝一個 launchd 工作，替你啟動 hc 與 roosterm。

```sh
python3 deploy/install.py --binary build/roosterm    # install and load the launchd job
python3 deploy/install.py status                      # loaded, pid, last exit code, whether the port answers
python3 deploy/install.py restart                     # also: start, stop, uninstall
```

- 請在 roosterm 資料夾中執行。第一個指令就是套件附的同一個安裝程式，指向你自己建置的執行檔；沒有設定檔時，它也會詢問登入與設定。
  <!-- evidence: deploy/install.py:478,387-473 -->
- 這個服務會先啟動 `hc server start`，再啟動 `roosterm serve`，在登入時啟動，崩潰後也會重啟（正常結束則不重啟）。
  <!-- evidence: deploy/launchd/dev.roosterm.serve.plist:9-19; deploy/README.md:15 -->
- 服務檔會放在 `~/Library/LaunchAgents`，日誌位於 `~/Library/Logs/roosterm.log`。
  <!-- evidence: deploy/install.py:45-50,280-290 -->
- 服務以你選擇的登入 shell 加上 `-lc` 執行，安裝程式會把該 shell 寫進服務檔。
  <!-- evidence: deploy/launchd/dev.roosterm.serve.plist:9-10; deploy/install.py:285-288 -->
- `stop` 不會停止 hc；服務會在下次登入或執行 `start` 時重新載入。
  <!-- evidence: deploy/install.py:403-414 -->

### 在 Linux 以服務執行

這裡用兩個 systemd 使用者單元：`herdr-core.service` 執行 `hc server start`；`roosterm.service` 依賴它，並在失敗時重啟 roosterm。安裝程式做的是同樣的事；下面是手動指令。

```sh
mkdir -p ~/.config/systemd/user
cp deploy/systemd/herdr-core.service deploy/systemd/roosterm.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now herdr-core.service roosterm.service
loginctl enable-linger "$USER"   # start at boot without a login session
```

- 日誌：`journalctl --user -u roosterm.service`。
- 停止 roosterm（hc 繼續執行）：`systemctl --user stop roosterm.service`。停止 `herdr-core.service` 也不會停止 hc（該單元設有 `KillMode=process`，停止它不會影響 hc）。
<!-- evidence: deploy/systemd/herdr-core.service:9-10; deploy/systemd/roosterm.service:2-8; deploy/README.md:21-29 -->

## 設定 HTTPS

本節說明反向代理要做什麼。

- roosterm 本身不處理 HTTPS。請在它前面放一個帶 TLS 憑證的反向代理，並轉送到 `listen` 位址。可以用 Caddy 或 nginx 之類的代理。
  <!-- evidence: host/cmd/roosterm/serve.go:71; docs/api.md:16 -->
- 實際使用一定要有 HTTPS：登入 cookie 帶有 `Secure`，瀏覽器只會在 HTTPS（或 `localhost`）下送出它，而且 Android App 只接受 `https://` 位址。
  <!-- evidence: host/internal/auth/cookie.go:33-35; app/lib/hosts.dart:11-20 -->
- 代理必須放行 WebSocket 連線（每個分頁用一條，路徑是 `/ws/tabs/{id}`）。
  <!-- evidence: host/internal/web/server.go:35 -->
- 若代理會關閉閒置連線，請把它的閒置逾時設得比 `ping_interval`（預設 25 秒）更長。
  <!-- evidence: docs/design.md:31; host/internal/config/config.go:18 -->

## 在瀏覽器使用

本節說明如何用瀏覽器連上 roosterm。

開啟你的 HTTPS 網址並登入。側邊欄列出你的工作階段。「New session」可選 Shell、Claude Code 或 Swarm，並提供資料夾選擇器。工作階段可改名或隱藏；已結束的工作階段可重新啟動或關閉。關閉仍在執行的工作階段會終止其中的程式。
<!-- evidence: web/src/workspace/NewSessionDialog.tsx:25-31; web/src/terminal/TerminalPane.tsx:224; docs/api.md:79-83 -->

## Android App

本節把預先建置的 APK 裝到手機並連線。

1. 開啟 [GitHub Releases](https://github.com/jhangyu/roosterm/releases)，下載其中一個 APK。每個版本有三個檔案：
   - `us.jhangy.roosterm-arm64-v8a.apk`：大多數現行手機。
   - `us.jhangy.roosterm-armeabi-v7a.apk`：較舊的 32 位元手機。
   - `us.jhangy.roosterm-x86_64.apk`：模擬器與 x86 裝置。
   <!-- evidence: gh release view app-v0.0.0 --repo jhangyu/roosterm (assets); scripts/ci.py:83-93; app/android/app/build.gradle.kts:18 -->
2. 在手機上開啟該檔案，Android 詢問時，允許從這個來源安裝（側載，即不經 Play Store 安裝 APK）。Releases 的 APK 都用同一把固定金鑰簽章，所以新版 APK 可以直接覆蓋安裝舊版，並保留你儲存的主機。
   <!-- evidence: app/android/app/build.gradle.kts:30-57; docs/logs/contract-server-package.md:10 -->
3. 在 App 中新增主機：填入名稱與 `https://` 位址（`http://` 會被拒絕，因為登入 cookie 帶有 `Secure`）。開啟該主機後登入。
   <!-- evidence: app/lib/hosts.dart:11-20 -->

- App 只載入該主機位址下的頁面。指向其他網站的連結會被擋下，並顯示該網址的提示。App 只有 Android 版，沒有 iOS 版。
  <!-- evidence: app/lib/hosts.dart:24-32; app/lib/host_page.dart:29-34; app/README.md:3 -->
- 已安裝 0.0.0 版的手機必須先解除安裝一次，因為那個 APK 是用用完即丟的金鑰簽章，App 內儲存的主機會遺失。之後的版本可直接覆蓋更新。
  <!-- evidence: docs/logs/contract-server-package.md:10; gh release view app-v0.0.0 -->
- 想自己建置 App，見[自行建置 Android App](#自行建置-android-app)。

## 安全性說明

能登入的人，就擁有你使用者身分的完整 shell。請使用 HTTPS 與強密碼。
<!-- evidence: docs/api.md:73; docs/design.md:18 -->

- 單一帳號，argon2id 密碼雜湊存於 `config.toml`。登入限流：失敗次數分兩個桶計算，帶有有效 `roosterm_device` cookie 的瀏覽器一桶，其餘一桶；單一桶連續失敗 5 次會鎖定該桶的 `/login` 60 秒（HTTP 429 並附 `Retry-After`），登入成功會重置計數。此限制不是依 IP。
  <!-- evidence: host/internal/auth/ratelimit.go:8-11; host/internal/web/login.go:30-33; host/cmd/roosterm/serve.go:67; docs/api.md:89 -->
- 已知裝置 cookie `roosterm_device`：登入成功時設定，`Path=/login`、400 天、HttpOnly、Secure、SameSite=Strict。它只用來選擇限流桶，不授予任何存取權；更改密碼後，舊的 cookie 即失效。
  <!-- evidence: host/internal/auth/cookie.go:13-31 -->
- Session cookie `roosterm_session`：HttpOnly、Secure、SameSite=Strict；伺服器只保存 token 的 SHA-256。`sessions.json` 權限最多只能是擁有者讀寫（0600 或更嚴格），否則伺服器會拒絕啟動。
  <!-- evidence: host/internal/auth/cookie.go:11,33-35; host/internal/auth/session.go:26,43-45 -->
- 更改密碼：先執行 `roosterm passwd`，再重啟 roosterm。重啟之前，舊密碼與所有 session 仍然有效；重啟之後，所有裝置都會登出。要登出所有裝置，再執行一次 `roosterm passwd` 並重啟。
  <!-- evidence: host/cmd/roosterm/passwd.go:28; host/internal/auth/session.go:32-33,54-58; host/cmd/roosterm/serve.go:61 -->
- 跨來源防護：瀏覽器標示為來自其他來源的 POST、PATCH、DELETE 請求（含 `/login`、`/logout`）會以 403、kind `cross_origin` 拒絕。JSON 請求本文必須帶 `Content-Type: application/json`，否則回 415；用腳本呼叫時必須送出此標頭。
  <!-- evidence: host/internal/web/server.go:41-56; docs/api.md:7,28-29 -->
- 對外暴露的內容：分頁的 REST 端點、每個分頁一條 WebSocket，以及僅限家目錄下的資料夾列表（解析後落在家目錄之外的路徑，含透過 symlink 者，會以 403 拒絕）。
  <!-- evidence: docs/api.md:33,86 -->
- 乾淨的環境：分頁中啟動的程式不會繼承 hc core 的 `TMUX`、`TMUX_PANE` 與 `CLAUDE*` 變數。
  <!-- evidence: host/internal/api/scrub.go:5-9 -->
- 再次提醒：切勿在正在使用的主機上執行 `hc server stop`，它會結束每個分頁中的所有程式。
  <!-- evidence: deploy/README.md:5 -->

## 自行建置 Android App

本節說明如何自己建置 App，而不是下載 Releases 的版本。

需要 Flutter SDK（Dart `^3.12.2`）。
<!-- evidence: app/pubspec.yaml:8 -->

```sh
cd app
flutter analyze
flutter build apk
```

- 一般的 `flutter build apk` 只建置 arm64。`python3 scripts/ci.py build android` 會建置三種 ABI（各一個 APK），輸出到 `app/build/dist/`，檔名與 Releases 的檔案相同。
  <!-- evidence: app/android/app/build.gradle.kts:23-27; app/android/build.gradle.kts:3-10; scripts/ci.py:7,83-105 -->
- 本機的 release 建置預設用 debug 金鑰簽章，除非設定了 `ANDROID_KEYSTORE_FILE`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS` 與 `ANDROID_KEY_PASSWORD`。debug 簽章的 APK 不能覆蓋安裝 release 版，反之亦然；只設定其中一部分時，建置會報錯並停止。
  <!-- evidence: app/android/app/build.gradle.kts:30-57 -->

## 發布與 CI

本節說明如何發布新版本。

1. `python3 scripts/ci.py bump patch`（或 `minor`、`major`，或明確的 `X.Y.Z+N`，如 `bump 0.0.0+1`）會更新 `app/pubspec.yaml`、提交，並建立標籤 `app-vX.Y.Z`；它不會 push。加上 `--dry-run` 只顯示變更。
   <!-- evidence: scripts/ci.py:8-9,191-216 -->
2. 已追蹤檔案有未提交的變更（未追蹤檔案不計），或標籤已存在時，它會拒絕執行。
   <!-- evidence: scripts/ci.py:202-206 -->
3. `git push origin HEAD app-vX.Y.Z` 會觸發 `.github/workflows/android.yml`：先跑測試防護（`ci.py guard`；CI 永不跑測試），再建置各 ABI 的 APK（`ci.py build android`）與四個伺服器套件（`ci.py build server`），最後發佈一個標題為 `roosterm X.Y.Z` 的 GitHub Release，附上三個 APK 與四個套件。建置需要下面說明的四個簽章 secrets，缺少時會失敗。
   <!-- evidence: .github/workflows/android.yml:33-45; scripts/ci.py:82-105,232; scripts/ci_guard.py:2-6 -->
4. 本機 hook：執行 `git config core.hooksPath .githooks` 後，每次 push 都會跑 `python3 scripts/ci.py prepush`（防護加上同一套建置，輸出在 `app/build/dist/`）。
   <!-- evidence: .githooks/pre-push; ci.py:7,243,256 -->

### 維護者：固定簽章金鑰

本步驟建立簽署每個 release APK 的唯一金鑰，只需在你自己的機器上做一次。

```sh
mkdir -p ~/.config/roosterm
keytool -genkeypair -v -storetype PKCS12 -keystore ~/.config/roosterm/release-key.jks \
  -alias roosterm -keyalg RSA -keysize 4096 -validity 10000 -dname "CN=roosterm"
base64 -i ~/.config/roosterm/release-key.jks | gh secret set ANDROID_KEYSTORE_BASE64
gh secret set ANDROID_KEYSTORE_PASSWORD
gh secret set ANDROID_KEY_PASSWORD
printf roosterm | gh secret set ANDROID_KEY_ALIAS
```

- 金鑰檔放在 `~/.config/roosterm/release-key.jks`。金鑰檔與其密碼只讓你自己能讀。PKCS12 的金鑰密碼與儲存庫密碼相同，所以兩個 secret 填同一個值。
  <!-- evidence: docs/logs/contract-server-package.md:29; keytool -help -->
- 四個 GitHub Actions secrets 是 `ANDROID_KEYSTORE_BASE64`、`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS` 與 `ANDROID_KEY_PASSWORD`。CI 把 base64 secret 還原成暫存金鑰檔，並以 `ANDROID_KEYSTORE_FILE` 傳給 Gradle。
  <!-- evidence: docs/logs/contract-server-package.md:10; scripts/ci.py:48-78; app/android/app/build.gradle.kts:30-57 -->
- 請備份金鑰檔與密碼（密碼管理器加一份離線副本）。**若遺失金鑰，所有手機都無法覆蓋更新：每位使用者都必須解除安裝後重新安裝 App。** 不要把金鑰提交進儲存庫；`app/android/.gitignore` 已忽略 `*.jks`。
  <!-- evidence: app/android/.gitignore:12-13 -->

## 開發與測試

本節列出開發時用的檢查指令。

```sh
python3 scripts/test.py                          # gofmt, go vet, go test -race, with time budgets
cd web && npx tsc -b && npx oxlint src           # web type check and lint
cd app && flutter analyze                        # Android app analysis
```

`scripts/test.py` 在下列情況會失敗：測試程式碼出現 sleep 類字樣、任一測試超過 1 秒、整體超過 40 秒，或有測試被跳過。網頁用戶端沒有提交任何測試。
<!-- evidence: scripts/test.py:16-19,28-37,40-110; web/package.json:8-9 -->

## API 與設計文件

HTTP 與 WebSocket 的契約（端點、cookie、錯誤格式、關閉碼）見 [docs/api.md](docs/api.md)。hc 通訊協定筆記見 [docs/hc-bridge.md](docs/hc-bridge.md)，設計摘要見 [docs/design.md](docs/design.md)。
<!-- evidence: docs/api.md:1-3; docs/hc-bridge.md:1-3; docs/design.md:1 -->

## 專案結構

本節列出主要目錄的內容。

```
host/cmd/roosterm/   main, serve, passwd
host/internal/       api, apierr, atomicfile, auth, bridge, config, hc, swarm, web
web/src/             api, components, lib, screens, terminal, workspace, styles
app/lib/             Flutter host list and WebView pages
deploy/              install.py, systemd units, launchd plist, deploy/README.md
docs/                design.md, api.md, hc-bridge.md, logs/
scripts/             build.py, test.py, ci.py, ci_guard.py
.github/workflows/   android.yml
.githooks/           pre-push
```
<!-- evidence: ls host/internal web/src app/lib scripts .github/workflows .githooks -->

- `host/` 是 Go 伺服器：登入、分頁的 REST、每個分頁一條 WebSocket，以及內嵌的網頁檔案。它不持有任何程序，也不保存分頁狀態。
- `web/` 是 React + Vite + xterm.js 用戶端；按鍵列與重新連線邏輯在這裡，並與 App 共用。
- `app/` 是 Flutter WebView 外殼加主機清單；WebView 負責保存 session cookie。
<!-- evidence: docs/design.md:15-17; web/package.json -->
