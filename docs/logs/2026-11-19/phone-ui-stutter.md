# Phone UI stutter: root cause and fix

**Result:** fixed in `c4761db`. The Android app's WebView now uses hybrid composition. The user confirmed on the device that dialog and tab animations are smooth.

## Symptom
In the Android app, every page UI animation (dialog open/close, tab switch) looked like ~4 fps. Terminal text output looked normal. Desktop browsers were fine.

## Evidence (device: Mali-G925, System WebView 157.0.8103.50, debug app build, HEAD d01991d)
| Check | Result |
|---|---|
| Desktop Chrome, phone emulation, CPU x4–x6 | React re-renders: about 1 commit per 400 output messages, react-dom about 5% of main thread. Resize chain: fires only on real size change. xterm DOM renderer: rendering 10–18% of main thread; batching writes per animation frame gave no gain. All three refuted. |
| Phone app, idle/`top` load, 22 s | 1266 of 1325 page frames (95.5%) recalc style without paint, caused by xterm cursor blink (`cursorBlink: true`, `web/src/terminal/TerminalPane.tsx:129`). Each frame 2–4 ms; display holds 60 fps. Real but cheap; not the stutter. |
| Phone app, dialog/tab animations, 14 s | gfxinfo frame p50 6 / p95 22 / p99 150 ms. WebView dropped 251 of 846 frames (30%). Android UI-thread frames up to 213 ms with under 3 ms of WebView work inside. Page main thread max 16 ms, layout max 1.1 ms, paint max 1.3 ms. |
| Same URL in phone Chrome (user observation) | Much smoother than the app. |

## Root cause
`webview_flutter` on Android defaulted to the texture embedding (`initSurfaceAndroidView`): each WebView frame went through a 1260x2687 buffer that Flutter composited. The stalls were on the Android/Flutter side, not in page CSS.

## Fix
`app/lib/host_page.dart` builds the widget with `AndroidWebViewWidgetCreationParams(displayWithHybridComposition: true)` (webview_flutter_android 4.14.1, `android_webview_controller.dart:1311-1340` uses `initExpensiveAndroidView`). `flutter analyze` RC=0, `flutter build apk --debug` RC=0, `adb install -r` Success.

## Known limits
- After-fix confirmation is the user's observation on the device; no after-fix frame capture was taken.
- The cursor-blink redundant frames remain (cheap: 2–4 ms per frame, but they keep the app busy while the terminal is focused).

## Incident during measurement
Test roosterm instances shared the user's hc core. `GET /api/tabs` lists every roosterm-labelled hc terminal (`host/internal/api/tabs.go:66`), and the page auto-selects a tab. A test driver attached to, resized and deleted the user's Claude Code tab t32. Measurements must never start claude processes and must not act on tabs they did not create.
