# Roosterm 設計規格

## 目標
讓使用者從瀏覽器或 Android App，經 HTTPS 連回自己的主機，在完整的終端機裡使用 Claude Code 與任意 shell 程式；用戶端斷線或主機服務重啟都不中斷正在執行的程式。

## 背景與要避免的缺陷
先前的嘗試（holyclaude、cloudcli、AIWebIDE，已刪除）的問題，直接轉為需求：
- 綁太多服務 → 主機端只有 `roosterm` 單一執行檔加 herdr-core，不用 Docker、不用資料庫、執行期不需要 Node。
- 斷線後 session 無法恢復；resume Claude Code 會產生新行程，PID 改變，連不回 `tmux -L claude-swarm-<lead pid>` 上的 swarm worker → claude 行程在斷線與 roosterm 重啟時都必須存活，PID 不變。
- 自建 AI harness 不如直接用 Claude Code → roosterm 只是終端機外殼，不解析、不包裝 Claude Code。

研究依據：`docs/research/herder.md`（herdr 與 herdr-core）、`docs/research/roamgate.md`（Roamgate）。

## 需求
1. 主機端：`roosterm`（Go 單一執行檔）加上 herdr-core（`hc`，鎖定 0.1.21）。
2. 用戶端與主機之間只走 HTTP(S)，WebSocket 經 HTTP 升級；使用者的 reverse proxy 負責對外網路。
3. 用戶端：瀏覽器，以及 Flutter Android App。
4. 完整互動式終端機：Claude Code 介面正確顯示；可 `cd`、`exit`、執行任意程式。
5. 多分頁；可一鍵開分頁執行 `tmux -L claude-swarm-<pid> a`。
6. 用戶端斷線、roosterm 重啟或當掉、hc 核心重啟，分頁內的程式都存活且 PID 不變。
7. 帳號密碼登入，單一帳號。
8. 主機作業系統：macOS 與 Linux，程式碼不分岔。

## 不做
- 多帳號、權限分級。
- 推播通知、Firebase。
- Windows 主機。
- 聊天介面、`claude -p` 結構化輸出、任何 AI 助理功能。
- 自己的輸出回放緩衝（畫面還原由 hc 負責）。

## 架構

```
瀏覽器 / Flutter WebView
   │  HTTPS（經使用者的 reverse proxy）
   ▼
roosterm（Go 單一執行檔）
   ├─ 認證：帳密 → session cookie
   ├─ 前端靜態檔（go:embed）
   ├─ REST：分頁的列出、新增、改名、關閉
   └─ WebSocket：每分頁一條，終端機輸入、輸出、視窗大小
   │  Unix socket（JSON-RPC）
   ▼
herdr-core（hc）
   └─ holder 行程：持有所有 PTY，執行 $SHELL -l、claude、tmux attach 等
```

### 元件

| 元件 | 職責 | 依賴 |
|---|---|---|
| `host/`（Go） | 認證；HTTP、WebSocket 與 hc JSON-RPC 之間的轉接；提供前端檔案。本身不持有任何行程 | Go 標準函式庫、一個 WebSocket 套件、`golang.org/x/crypto/argon2` |
| `web/` | 登入頁、分頁列、開新分頁與 swarm 選單、特殊按鍵列、斷線橫條、xterm.js 終端機 | React、Vite、shadcn/ui（Radix + Tailwind）、`@xterm/xterm`、`@xterm/addon-fit` |
| herdr-core | 行程持久、重新 attach 時送出完整畫面 | 主機另行安裝 |
| `app/`（Flutter） | WebView 加主機清單（每台主機一個網址）；cookie 由 WebView 保存 | `webview_flutter` |

- 元件一律使用 shadcn/ui 現成元件，不手刻。圖示用 `python3 ~/ui-kits/find-icon.py` 從圖示包取得，整個介面只用同一套圖示包的同一風格。
- 特殊按鍵列（Esc、Tab、Shift+Tab、Ctrl+C、Ctrl+D、方向鍵）放在網頁內，手機瀏覽器與 App 共用同一實作。
- 所有第三方套件版本在實作時查 registry 與官方文件決定，不憑記憶填寫。

### 分頁模型
- 一個分頁等於一個 hc terminal。分頁清單以 hc 的 `terminal.list` 為唯一來源，roosterm 不另存。
- 新分頁呼叫 `terminal.run`，預設執行 `$SHELL -l`。
- swarm 快速開啟：roosterm 掃描 tmux socket 目錄（`$TMUX_TMPDIR`，未設定時為 `/tmp/tmux-<uid>`），只列出 `claude-swarm-*`；選取後開新分頁執行 `tmux -L <名稱> a`。
- 分頁內程式結束（例如 `exit`）時，分頁顯示「已結束（code N）」，由使用者關閉。

## 資料流
1. `POST /login`：驗證帳密，發 session cookie（HttpOnly、Secure、SameSite=Strict）。之後所有 HTTP 請求與 WebSocket 升級都先驗 cookie，失敗回 401，WebSocket 在升級前就拒絕。
2. `GET /api/tabs` → `terminal.list`。
3. `POST /api/tabs`（可帶初始指令）→ `terminal.run`；`PATCH /api/tabs/{id}` → `terminal.label`；`DELETE /api/tabs/{id}` → `terminal.kill`。
4. `GET /api/swarms` → 列出 `claude-swarm-*` socket。
5. `WS /ws/tabs/{id}` → `terminal.attach`：先收完整一幀畫面，再收即時輸出。鍵盤輸入用 binary frame，對應 `terminal.send`；視窗大小用 JSON 控制訊息。
6. 同一分頁可同時被多個用戶端開啟，視窗大小以最後一次變更為準。

hc 方法的實際參數與串流格式由一次性測試第 5 項確認後，寫入 `docs/hc-bridge.md`，作為轉接層的規格。

## 重新連線
- WebSocket 斷線時顯示「已斷線」橫條與重新連線按鈕。
- 收到瀏覽器 `online` 或 `visibilitychange`（回到前景）事件時自動重新連線。不使用定時重試。
- 重新連線等於重新 attach，由 hc 送出完整畫面。
- WebSocket 定期送 ping，避免 proxy 閒置切斷；間隔為設定檔的具名設定值。

## 錯誤處理
所有錯誤明確顯示，不默默降級。錯誤訊息包含操作名稱、受影響的分頁 id、期望值與實際值。

| 情況 | 行為 |
|---|---|
| 啟動時找不到 hc socket | roosterm 以非零代碼結束，訊息列出預期路徑與實際狀況 |
| 執行中 hc 核心重啟 | 用檔案監看偵測 socket 重新出現後重連 hc；期間所有分頁顯示「主機核心離線」 |
| 分頁程式結束 | WebSocket 以自訂關閉代碼關閉並附結束代碼 |
| hc 回傳錯誤 | 原樣轉給前端顯示 |
| 登入失敗過多 | 回 429，附可重試時間 |

## 認證
- `roosterm passwd` 設定帳號與密碼；密碼以 argon2id 雜湊後存在設定檔。
- session 存成檔案（權限 0600），roosterm 重啟後不需重新登入；session 有到期時間。
- 登入失敗速率限制；時鐘可注入，測試用假時鐘。

## 設定與部署
- 指令：`roosterm passwd`、`roosterm serve`。
- 設定檔：`~/.config/roosterm/config.toml`，內容為監聽位址、hc socket 路徑、ping 間隔、session 到期時間、帳號與密碼雜湊。
- `deploy/` 附 launchd plist 與 systemd user unit 範例；hc 的自動啟動方式依一次性測試第 4 項結果補入。
- Node 只在建置期打包前端；主機執行期不需要。

## 專案結構
```
roosterm/
├─ host/     Go module：cmd/roosterm、internal/{auth,hc,web}
├─ web/      React + Vite + shadcn/ui，建置產物由 host 以 go:embed 打包
├─ app/      Flutter：WebView 與主機清單
├─ deploy/   launchd、systemd 範例
├─ scripts/  test.py、build.py
└─ docs/
```

## 開工前的一次性測試
在 `scratch/` 執行，不 commit；結論與精簡 log 寫入 `docs/logs/`。

1. 在 hc terminal 中執行 `claude`，介面正確顯示並完成一次互動。
2. 另開 terminal 執行 `tmux -L claude-swarm-<pid> a`，swarm 畫面正確。
3. `kill -9` hc 核心後，claude PID 不變，重新 attach 後畫面完整。
4. macOS 與 Linux（本機的 Linux 容器或 VM，只用於測試）上 holder 行程都能存活；確認 hc 的開機自動啟動方式。
5. 記錄 `terminal.attach`、`terminal.send`、`terminal.run`、`terminal.list`、`terminal.label`、`terminal.kill` 的實際參數與串流格式，以及視窗大小變更與程式結束事件的傳遞方式。

第 1–3 項任一不過，停止實作，改回透明 tmux 方案（`tmux -L roosterm`，快捷鍵前綴設為 None、關閉狀態列、分頁內清除 `TMUX`）並重新設計持久層。

## 測試
入口：`scripts/test.py`，內部呼叫 `go test`；全套件 40 秒內完成，並行執行，單一測試上限 1 秒；不連外網、不用真實時鐘。

常駐測試只收：
- 認證：argon2id 驗證、cookie 屬性、session 到期、失敗速率限制、未登入時 WebSocket 升級被拒。
- 轉接：以測試內的假 hc（Unix socket）驗證 WebSocket 與 JSON-RPC 的訊息轉換、結束代碼傳遞、hc 離線行為。
- swarm socket 掃描：只列出 `claude-swarm-*`。

不收 UI、瀏覽器或截圖測試。
