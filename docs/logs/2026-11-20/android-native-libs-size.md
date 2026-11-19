# app/build 原生函式庫肥大原因（實測）

## 結論
正常的 Flutter debug 建置行為，非專案設定錯誤。1.3 GB 的 `merged_native_libs` 是「未 strip 的 Flutter debug 引擎」×3 個 ABI，加上 arm64 專屬的 Vulkan 驗證層。最終 APK 內的 .so 已被 AGP strip（merged 1.3 GB → stripped 118 MB），所以 APK 只有 151 MB。無需改動設定。

## 範圍
- 變體：只有 `debug`（merged_native_libs/、stripped_native_libs/ 下都只有 debug；沒有 profile/release）。
- ABI：libflutter 為 arm64-v8a、armeabi-v7a、x86_64 三種；x86 只有 libdartjni 與 libdatastore_shared_counter（Flutter debug 引擎不提供 x86）。
- 合計 24 個 .so（12 merged + 12 stripped）= 4 ABI（libflutter 僅 3 個）× debug 1 變體 × 兩個階段目錄。

## 主要 .so（路徑前綴 `app/build/app/intermediates/`）
| 檔案 | ABI | merged 大小 | stripped 大小 | merged 是否 strip |
|---|---|---|---|---|
| merged_native_libs/debug/mergeDebugNativeLibs/out/lib/…/libflutter.so | arm64-v8a | 392,541,920 | 37,584,608 | 否（with debug_info） |
| 同上 | x86_64 | 387,349,064 | 38,828,136 | 否 |
| 同上 | armeabi-v7a | 346,245,564 | 31,959,500 | 否 |
| 同上 libVkLayer_khronos_validation.so | arm64-v8a | 248,644,248 | 15,240,080 | 否（with debug_info） |
| libdartjni.so | 4 ABI | 0.20–0.24 MB | 0.06–0.10 MB | 否 |
| libdatastore_shared_counter.so | 4 ABI | ~8–10 KB | ~6–8 KB | 否 |

merged 合計 ≈ 1.375 GB（du 顯示 1.3G）；stripped 合計 118M（du）。libflutter×3 + VkLayer 佔 merged 99.9% 以上。

## 為何大（section 實測，arm64 libflutter.so）
- `.debug_info` 174 MB、`.debug_str` 71 MB、`.debug_loc` 54 MB、`.debug_line` 25 MB、`.debug_ranges` 12 MB，合計約 336 MB 是 DWARF；真正的程式碼 `.text` 僅 9.4 MB、`.rodata` 23.8 MB。
- strip 後只剩 `.rodata`/`.text`/`.eh_frame` 等 → 37.6 MB。同 BuildID（a9d0a2bd…）證明是同一檔 strip 前後。
- VkLayer_khronos_validation.so：`.debug_info` 96 MB + `.debug_str` 75 MB + … → 248 MB，strip 後 15 MB。此檔來自 Flutter 的 `io.flutter:arm64_v8a_debug` artifact（gradle cache 內該 jar 同時含 libflutter.so 392,541,920 與 VkLayer 248,644,248，位元組數與 merged 完全相同），非專案依賴；是 Flutter debug 引擎給 Impeller/Vulkan 除錯用。
- 引擎來自 `~/.gradle/caches/modules-2/files-2.1/io.flutter/*`，jar 內本身即未 strip，專案沒有 plugin 帶入大型原生庫（webview_flutter_android 使用系統 WebView，不內含 .so；只有 libdartjni、libdatastore_shared_counter 這類 KB 級）。

## 重複與雙 APK
- merged_native_libs（未 strip）→ stripped_native_libs（strip 後，APK 實際打包）：同一組檔案的前後版本，非專案重複設定，是 AGP 的中間產物。
- `outputs/apk/debug/app-debug.apk` 與 `outputs/flutter-apk/app-debug.apk`：`cmp` 相同、大小同為 151,626,023；inode 不同（6089481192 vs 6089481201），是實體副本。前者是 Gradle 輸出，後者是 flutter 工具複製出來給 `flutter install` 用。→ APK 共 2×151 MB ≈ 303 MB。
- APK 內容（unzip -l）：libflutter 37.6+32.0+38.8 MB、VkLayer 15.2 MB、`assets/flutter_assets/kernel_blob.bin` 52.9 MB（debug JIT kernel，release 為 AOT 的 libapp.so 取代）。

## 設定檢查
- `app/android/app/build.gradle.kts`：無 abiFilters、無 splits、無 packagingOptions／jniLibs 設定（全檔 1–48 行，僅 namespace、Java17、release 用 debug 簽署）。因此預設打包 Flutter debug 支援的全部 ABI。
- `app/pubspec.yaml`：依賴 webview_flutter、webview_flutter_android、shared_preferences、flutter_svg，皆非肥大來源。
- `docs/logs/2026-11-19/phone-ui-stutter.md:20`：hybrid composition 僅是 Dart 參數（`displayWithHybridComposition: true`），不影響原生庫大小。

## release 大小估計
- 本機實測：gradle cache 內 release 引擎 jar（未 strip）libflutter 為 arm64 163,774,472、armeabi-v7a 149,950,396、x86_64 164,285,328 bytes；AGP strip 後的 release 大小我沒有實測，**未驗證**（一般預期為十幾 MB／ABI，僅屬推測）。
- 未建置 release（紅線），故 `--release --split-per-abi` 的 APK 大小無實測值，**未驗證**。release 無 VkLayer、無 kernel_blob（改 AOT libapp.so），且 split 後每個 APK 只含單一 ABI，必然比 151 MB 小很多，量級需實建確認。

## 建議
- **無需動作**。debug APK 大小是 Flutter 預設行為；中間 merged_native_libs 是可再生的建置產物，刪除 `app/build` 安全（`flutter build` 會重生）。
- 可選（非必要）：debug 只給手機（arm64）時，建置用 `flutter build apk --debug --target-platform android-arm64` 可省 armeabi-v7a 與 x86_64，APK 約降到 ~55 MB 級（由上表各項相加：37.6+15.2+0.1+kernel 52.9 的壓縮後尺寸；整體未實測）。不建議加 abiFilters 進 gradle（會讓 release 也受影響）。

## 不確定項
- strip 後 release 實際大小、`--target-platform` 後 APK 實際大小：未建置，未驗證。
- 「flutter 工具複製 APK」的機制為依檔案實況（兩個同內容實體副本）推論，未讀 flutter 工具原始碼。

## 後續處置

- 預設只建 arm64-v8a：commit df093d9。實測 `app/build` 1.9 GB → 996 MB，debug APK 145 MB → 77 MB。
