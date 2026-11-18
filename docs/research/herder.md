# herdr / herdr-core 調查（user 所稱 "herder"）

調查日：2026-11-19。方法：讀原始碼（未建置、未執行）。clone 於 `scratch/research/herder-src`（herdr）與 `scratch/research/herdr-core-src`（herdr-core）。

## 1. 身分

正確拼法是 **herdr**（無 e），非 "herder"。同一組織有兩個相關 repo：

| 項目 | URL | stars (gh api, 2026-11-19) | 最後 push | 授權 | 用途 |
|---|---|---|---|---|---|
| herdrdev/herdr | https://github.com/herdrdev/herdr | 51,722 | 2026-11-07 | Apache-2.0 | 給 coding agent 用的終端多工器（TUI，Rust，單一執行檔） |
| herdrdev/herdr-core (`hc`) | https://github.com/herdrdev/herdr-core | 797 | 2026-11-16 | Apache-2.0 | 無 UI 的 headless 核心：在斷線後仍存活的終端，JSON-RPC API |
| ogulcancelik/herdr | 重導向至 herdrdev/herdr | — | — | — | 舊位址 |

與 user 描述對照：「popular」成立（51.7k stars）。「designed for this problem」**部分成立**：herdr 設計給本機/SSH 的 TUI，不提供 web/HTTP；真正對應「持久終端 + 自建 client」的是 **herdr-core**。

## 2. 機制摘要

**herdr（TUI 產品）**
- client-server：背景 server 持有 PTY，client 可 detach/reattach（README.md:31、`ctrl+b q` 脫離）。
- server/機器重啟後只還原版面，**原行程不存活**（README.md:31 原文 "the original processes do not survive"；docs/next/README.md:31 同）。
- 有 `handoff_runtime.rs`（src/handoff_runtime.rs:1-30）：server 替換（升級）時移交 PTY/子行程，但這是升級路徑，非 crash 保證。
- 無 HTTP/WebSocket：src 與 Cargo.toml 中 grep websocket/tungstenite/axum/hyper/TcpListener 無命中（僅 "hyperlink"/"hyper" 按鍵修飾符誤命中）。IPC 用 `interprocess`（Cargo.toml:36）。
- 以 portable-pty 持有 PTY（Cargo.toml:39），TUI 用 ratatui。

**herdr-core（`hc`）**
- 三層：CLI/viewer → core（單一行程，解析畫面、API）→ **holder 行程**（擁有所有 PTY、是每個程式的父行程；crates/herdr-core/src/holder.rs:1-4）。
- holder 以 `setsid` 脫離啟動（platform/unix.rs:202-212），「outlives crashed cores」（unix.rs:216 註解）；holder 自己重啟當掉的 core（holder.rs:3-4），並保留每終端約 1 MB 輸出尾（holder.rs:14 `TAIL_BYTES`）供重建畫面。
- core 重啟/升級不殺程式：README.md「Run it and come back」「`hc server restart` … without ending programs」；docs/security.md「Holder processes own the terminals so a core restart or update can leave programs running」。
- 重新 attach：`terminal.attach` 回傳 channel，先送「first paint」（目前畫面、可選歷史、狀態、標題）再送原始輸出（schema/README.md「Attach streams」；viewers.rs:99 `first_paint`）。畫面由內嵌 ghostty-vt 解析維護（crates/ghostty-vt）。
- API：JSON-RPC 2.0（逐行 JSON）＋ framed 二進位模式（channel/CREDIT/RESIZE/FOCUS 框，schema/README.md「Framed mode」），流量控制、observer/viewer 區分、多 viewer（最後活躍者決定尺寸）。
- 傳輸：**只有 Unix socket / Windows named pipe**（docs/security.md「core listens on a local Unix socket, not a public TCP port」）；`hc connect` 把協定接到 stdin/stdout 以走 SSH（schema/README.md「Connecting」）。crates 內 grep 無 websocket/axum/hyper/TCP 監聽（只有 tests 用 TcpListener 127.0.0.1，tests/windows.rs:185）。
- 無驗證層：「There is no separate API password or per-client permission check」（docs/security.md）。
- 依賴精簡：tokio、serde、clap、libc、ghostty-vt（crates/herdr-core/Cargo.toml:12-20），無 HTTP 棧。
- 子行程結束語意：結束終端對程式與前景 job 送 hangup；`nohup`/`setsid` 脫離者可續跑（README.md「Agents run agents」段）。`hc run --keep`：程式遺失時重啟新 shell，不重跑任務（README.md）。
- 版本狀態：「preview (protocol generation 1, version 0.x)」（schema/README.md）。

## 3. R1–R7 評級

| 需求 | herdr (TUI) | herdr-core (`hc`) | 證據 |
|---|---|---|---|
| R1 單一執行檔＋主機既有工具 | partial（單一執行檔，但其 server 是 TUI 產品，不是為被包裝而設） | **yes**：`hc` 單檔、靜態 Linux build，自動啟 core/holder，無 Docker/外部服務 | README.md（core）「Linux builds are static」；Cargo.toml 依賴 |
| R2 全 HTTP(S)/WS 經反向代理 | no | **no（需自寫橋接）**：只有 Unix socket/pipe/stdio | docs/security.md；grep 無 HTTP 棧 |
| R3 瀏覽器＋Flutter | no | no（無 web client；但 API 為 JSON-RPC＋OpenRPC schema，語言無關，可自建 client） | schema/README.md「Build your own client」 |
| R4 完整 TUI 保真＋任意 shell | partial（自己的 TUI 外殼） | **yes**：核心保存並重播真實 VT 狀態（ghostty-vt），first paint＋raw output；不 wrap 程式 | schema/README.md；viewers.rs:99；README.md「Core draws nothing」 |
| R5 多分頁、tab 內 `tmux -L … a` | partial | **yes**：每個 terminal 是獨立 PTY，shell 內可跑 tmux（巢狀）；未實測 | README.md「hc run --bg」；（巢狀 tmux 為推論，標記待實測） |
| R6 斷線不殺、同 PID；server 重啟不殺 | partial（server 重啟後行程不存活） | **yes（文件＋碼一致，未實測）**：holder 為 PTY 父行程、setsid 脫離、core 重啟/升級保留程式；同一行程故 PID 不變 | holder.rs:1-4；unix.rs:202-216；security.md；README.md |
| R7 不是 AI harness | partial（有 agent 偵測與編排功能，但不 wrap agent） | **yes**：「Core draws nothing」，狀態來自程式自報 OSC 7501，不辨識品牌 | README.md「Status comes from the program」 |

R6 風險：holder 本身若被 SIGKILL/OOM 則其下所有程式一併消失（單點）；未實測，原文只說 holder 異常消失時「inherited cleanup tokens」會終結被遺棄程式（README.md「Agents run agents」段）。

## 4. 建議

**(b) 以 herdr-core 為底座，在其上自建 web shell 橋接**（非 herdr TUI 本體；herdr 本體選 (d)）。

理由：
1. R6 是三次失敗的主因，hc 的 holder/core 分離正是解此問題且已實作（斷線、core 重啟、升級皆不殺程式）；自己重做要重寫 PTY holder＋畫面重建，工作量大。
2. 缺口只有 R2/R3：需自寫一個薄橋接（WebSocket ↔ hc 的 Unix socket framed 協定，或透過 `hc connect` 子行程）。協定逐行 JSON-RPC＋文件完整的 framed 二進位，橋接工作量小；畫面還原已由 core 做（first paint），瀏覽器端只需 xterm.js 類元件，Flutter 端同理。
3. R7 吻合：core 不畫 UI、不辨識 agent。
4. 注意：Apache-2.0；0.x preview，協定宣稱只增不破（schema/README.md），但專案很新（需鎖版本）；無驗證層，認證必須由 Roosterm 橋接＋反向代理自行負責。
5. 不選 (a)：無 HTTP。不選 (c)/(d) 純借鑑：hc 已涵蓋最難的一塊。
6. 替代：若不想依賴第三方 0.x 專案，則 (c)——借鑑 holder/core 兩層＋first paint 設計自寫。

信心：**中**。未實測項（需 scratch 驗證）：巢狀 `tmux -L claude-swarm-<pid>`、核心 kill 後同 PID、macOS 上 holder 存活（macos_launchd.rs 有專屬路徑）、holder 被殺的後果。

## 5. 來源

- https://github.com/herdrdev/herdr（commit ee5bfb5，2026-10-29）
- https://github.com/herdrdev/herdr-core（commit c751780，2026-11-17）
- gh api repos/herdrdev/herdr、herdr-core（2026-11-19）
- 網搜確認拼法與背景：https://www.tecmint.com/herdr-run-ai-coding-agents-in-linux-terminal/ 、https://mer.vin/2026/07/herdr-explained-terminal-multiplexer-for-running-multiple-ai-coding-agents/（二手，僅供身分識別，未作為結論依據）
- 本地檔：herdr-core-src/{README.md, docs/security.md, docs/machines.md, schema/README.md, crates/herdr-core/src/{holder.rs, platform/unix.rs, viewers.rs, restart.rs}}；herder-src/{README.md, Cargo.toml, src/handoff_runtime.rs}
