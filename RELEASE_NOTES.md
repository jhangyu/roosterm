## Highlights

**4867f96 → 27cd63f · 28 commits (excluding these notes)**

This release closes a login lockout bypass. Parallel login requests can no longer skip the failed-attempt limit. The folder picker now tells you when folder memory fails, and it marks matching folder names correctly. The terminal banner shows the clock time when a connection was lost. The Android app shows a message when it cannot save the host list. Most other changes clean up the code.

### ✨ Added

- The folder picker shows a notice when folder memory fails to load or save.
- The terminal banner shows the clock time when the connection stopped responding.
- The Android app shows a message when it cannot save the host list.
- API contract (`docs/api.md`) and design overview (`docs/design.md`) added.

### 🛠 Fixed & Improved

- Security: parallel login requests could bypass the login lockout. The limiter now counts an attempt before the password check starts.
- During a lockout, a malformed or empty login body now gets a 400 reply instead of 429.
- A failed folder memory load no longer blocks the next save.
- The folder filter now highlights the right part of a name. Names such as "İstanbul-proj" are no longer mis-marked.
- A restarted tab with a custom name now appears before the rename finishes.
- Creating a swarm tab with an empty swarm name now returns a 400 error. It no longer opens a plain shell.
- A session attach stream is now closed when its reply arrives just as the request is cancelled.
- The host reads the home directory once at start and stops with a clear error if it cannot.
- Terminal banner buttons share one style. Reconnect and Close use the primary hover colour, and Restart gets a muted hover background.

### ⚙️ Internal / Build

- Host: error constructors and handlers share one error type, file writes use one atomic helper, and swarm calls follow the request context.
- Host: a swarm tab request now sends a `swarm` field, and the host builds the attach command.
- Web: shared helpers for tab updates, buttons, banners and session actions. Dead UI code and unused design tokens removed. The shadcn kit is pruned to what is used.
- Web: the folder picker is split into a component and a browser hook.
- App: shared Notice widget, the non-Android webview branch removed, default theme settings dropped. Release builds are documented as debug-signed for sideloading.
- Build: `build.py` keeps `host/internal/web/dist/.gitkeep`, and its package.json fork is removed.
- Tests: new tests for the login limiter, config load and the password command. A slow fake-tmux test is replaced by a fast one.
- Docs: old plans, mockups and research notes removed. The refactor campaign contracts and result logs are in `docs/logs/`.

---

## 版本重點

**4867f96 → 27cd63f** · 共 28 個 commit（不含本說明檔）

此版本修補登入鎖定可被繞過的問題：平行的登入請求不能再略過失敗次數限制。資料夾選擇器在資料夾記憶失敗時會提示，也能正確標示符合的資料夾名稱。終端機橫幅會顯示連線中斷的時間。Android App 在無法儲存主機清單時會顯示訊息。其餘多數變更是程式碼整理。

### ✨ 新增

- 資料夾選擇器在資料夾記憶載入或儲存失敗時會顯示提示。
- 終端機橫幅會顯示連線停止回應的時間。
- Android App 在無法儲存主機清單時會顯示訊息。
- 新增 API 規格（`docs/api.md`）與設計概覽（`docs/design.md`）。

### 🛠 修正與改善

- 安全性：平行的登入請求可繞過登入鎖定。限制器現在會在驗證密碼之前先計入該次嘗試。
- 鎖定期間，格式錯誤或空白的登入內容現在回覆 400，不再是 429。
- 資料夾記憶載入失敗，不再阻擋下一次儲存。
- 資料夾篩選現在會標示正確的名稱片段。「İstanbul-proj」這類名稱不再被錯誤標示。
- 已自訂名稱的分頁重新啟動後，會在重新命名完成前就顯示。
- 以空的 swarm 名稱建立 swarm 分頁，現在回傳 400 錯誤，不再開啟一般 shell。
- 當 attach 串流的回應恰好在請求取消時抵達，現在會正確關閉該串流。
- 主機啟動時只讀取一次家目錄，讀不到時會停止並顯示明確錯誤。
- 終端機橫幅按鈕共用同一組樣式。重新連線與關閉使用主色懸停色，重新啟動新增淡色懸停背景。

### ⚙️ 內部/建置

- 主機：錯誤建構函式與處理函式共用同一種錯誤型別，檔案寫入統一使用原子寫入工具，swarm 呼叫會隨請求的 context 取消。
- 主機：swarm 分頁請求改為送出 `swarm` 欄位，由主機組出 attach 指令。
- 網頁：分頁更新、按鈕、橫幅與工作階段操作改用共用實作。移除無用的 UI 程式碼與未使用的設計 token。精簡 shadcn 元件組，只留下用到的部分。
- 網頁：資料夾選擇器拆為元件與瀏覽 hook。
- App：新增共用 Notice 元件，移除非 Android 的 webview 分支，移除預設主題設定。Release 版以 debug 金鑰簽署（供側載使用），並已寫明。
- 建置：`build.py` 會保留 `host/internal/web/dist/.gitkeep`，並移除其 package.json 分叉。
- 測試：新增登入限制器、設定檔載入與密碼指令的測試。耗時的 fake-tmux 測試改為快速測試。
- 文件：移除舊計畫、模型稿與研究筆記。重構專案的契約與結果紀錄存放於 `docs/logs/`。
