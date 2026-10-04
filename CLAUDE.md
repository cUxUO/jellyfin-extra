# 專案環境說明

本機的實際位址、網域與測試資料在不進版控的 CLAUDE.local.md：@CLAUDE.local.md

## 專案目標

讓舊裝置能順暢播放 Jellyfin 的影片：由 Windows 的 GPU 負責即時轉碼，各裝置用專屬播放程式。

```
播放程式 ──① 登入、瀏覽、回報進度──▶ Linux Jellyfin（Docker，反向代理後對外 HTTPS）
   │                                    ▲
   │ ② 開始播放（itemId、token、       │ ③ 驗證 token、PlaybackInfo
   │    裝置能力、起始時間、音軌、字幕） │ ④ 取原始檔：/Videos/{id}/stream?static=true（走內網 HTTP 8096）
   ▼                                    │
Windows 轉碼伺服器（Go，原生 exe）──────┘
   │ ⑤ ffmpeg：NVDEC 解碼 → NVENC 編碼 → HLS
   ▼
⑥ 播放程式拉 HLS（對外經同一個反向代理轉發）
```

- Linux 上的 Jellyfin 是弱主機、沒有轉碼能力，**不修改它**；媒體檔在 NAS，Jellyfin 掛載使用。
- 轉碼伺服器用播放程式帶來的 token 向 Jellyfin 取資料，權限由 Jellyfin 把關。
- ffmpeg 參數一律由伺服器組出，不接受用戶端傳入；token 不得寫入 log。
- 影片來源寫成可替換的介面，日後可能加上直接讀 NAS（SMB）。
- Windows 關機時，播放程式要能退回 Jellyfin 本身的直接播放。
- **內網優先**：播放程式設定兩組位址，先試內網直連（Jellyfin 的 8096、Windows xcode 的 8097，實際位址見 CLAUDE.local.md；放在 app 設定，不寫死在程式碼），連不上才改用外部 `https://<網域>`（Jellyfin 本體與 `/xcode/`）。理由：iPad 解析不到網域（見 iOS 一節），ZenPad 系統 TLS 不支援目前的 P-384 憑證；兩台大概也無法安裝 Tailscale（待查證）。

## 執行環境

- 你在 Docker 容器內執行（Ubuntu 22.04，使用者 `dev`，沒有 root / sudo），專案目錄是 `/work`。容器設定在 `devenv/`。
- 主機是 Arch Linux，你不能也不需要操作主機。
- 需要新工具時，不要自己 `apt install`，請告訴使用者，並建議改 `devenv/Dockerfile`。
- 工具鏈在 volume 內：Android SDK 在 `~/Android/Sdk`，Theos 在 `$THEOS`，Go 快取在 `~/go`。

## 目標裝置

| 平台 | 目標 | 連線方式 |
|---|---|---|
| Android | ASUS ZenPad 10（P028，MediaTek MT8163），Android 7.0（API 24），1280×800，實機 | USB，`adb` |
| iOS | iPad Air 1（A7），iOS 12.5.7，checkra1n 越獄，rootful，ARCHS=arm64 | SSH，主機別名 `ipad`；USB log 用 `idevicesyslog` |
| Windows | Windows 11，NVIDIA RTX 3070 Ti，實機 | SSH，主機別名 `Windows` |

## Windows 轉碼伺服器（Go）

```bash
# 交叉編譯
cd server && GOOS=windows GOARCH=amd64 go build -o ../build/xcode.exe ./cmd/xcode

# 部署與執行
scp build/xcode.exe Windows:'C:/dev/jellyfin-extra-test/'
ssh Windows 'C:\dev\jellyfin-extra-test\xcode.exe -config C:\dev\jellyfin-extra-test\xcode.json'

# 停止：ssh 中斷後 xcode.exe 不一定會跟著結束，一律確認並結束殘留行程
ssh Windows 'taskkill /f /im xcode.exe'
```

- 程式結構（`server/`）：
  - `cmd/xcode`：主程式，設定檔 `xcode.json`（不進版控，範本 `server/xcode.example.json`）。用 Job Object 讓 ffmpeg 跟著 xcode.exe 結束。
  - `cmd/fakejf`：開發用假 Jellyfin，讀 `items.json` 用本機檔案當片源，只供端對端測試。
  - `internal/api`：`POST /v1/sessions`、`GET /v1/sessions/{id}/{file}`、`DELETE /v1/sessions/{id}`，回傳相對路徑。
  - `internal/jellyfin`：Jellyfin API 子集。`internal/source`：127.0.0.1 上的原始檔 proxy，替 ffmpeg 附 token。
  - `internal/profile`：裝置規格與轉碼計畫。`internal/ffmpeg`：產生 ffmpeg 參數。`internal/session`：ffmpeg 生命週期與閒置回收。
- 測試：`cd server && go test -race ./...`（容器內，Linux）。Windows 專屬程式碼至少要過 `GOOS=windows go vet ./...`。
- 端對端測試：Windows 測試目錄已有 `clips\`（SDR HEVC 1080p、HDR10 HEVC 4K、10-bit H.264 各 60 秒）、`items.json`、指向 fakejf 的 `xcode.fake.json`。先跑 `fakejf.exe -items items.json`（token `devtoken`，聽 127.0.0.1:18096），再跑 `xcode.exe -config xcode.fake.json`。
- 真實 Jellyfin（12.1.0）：`xcode.json` 指向內網 8096（不經 NPM）。測試用 token 以 `tools/jf-login.sh <內網網址> <帳號>` 取得，存在容器的 `~/.config/jellyfin-extra/token`；讀取時用 `$(cat ~/.config/jellyfin-extra/token)`，不要印出、不要寫進檔案或 log。
- 對外：Linux 主機上的 Nginx Proxy Manager（Docker）把 `https://<網域>/xcode/` 轉到 Windows 8097，設定在 Jellyfin Proxy Host 的 Advanced 分頁。
- HLS 播放清單由 API 加上 `#EXT-X-START:TIME-OFFSET=0`，否則轉碼中的 EVENT 清單會被播放端當直播，從最新片段開始播。
- ffmpeg 用 jellyfin-ffmpeg 8.1.3 的 win64 版，放在 `C:\dev\jellyfin-extra-test\ffmpeg\`。
- 3070 Ti：NVENC 可編 H.264 / HEVC（不能編 AV1），NVDEC 可解 H.264 / HEVC / VP9 / AV1。消費級驅動有同時編碼數上限。
- 遠端預設 shell 是 cmd，主控台編碼是 Big5（cp950）：需要中文輸出時先 `chcp 65001`，或用 PowerShell。連線時的 post-quantum 警告是 Windows 內建 OpenSSH 較舊所致，可忽略。
- 無法操作 GUI；需要 GUI 才能看的問題，請整理成文字描述請使用者查看。用 Event Log、PowerShell 等命令列方式診斷。
- 防火牆規則、Windows 服務註冊、驅動更新等在測試目錄以外的設定，由使用者操作。

## Android 播放程式

專案在 `android/`：Kotlin、View 系統（不用 Compose，舊平板較輕）、Media3 ExoPlayer + OkHttp、JSON 用內建 `org.json`。
套件 `com.jellyfinextra.player`；`ui/`（Browse、Login、Player）、`net/`（Endpoints 內網優先、JellyfinApi、XcodeApi）、`data/Settings`。
版本：AGP 9.4.1（內建 Kotlin，不另外套用 kotlin-android 外掛）、Gradle 9.8.0（wrapper 已鎖 SHA-256）、compileSdk 37（androidx.core 1.19 要求）、targetSdk 34、minSdk 24。版本集中在 `gradle/libs.versions.toml`。

```bash
cd android
adb devices                                  # 確認實機已連線並授權
./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb logcat -d | tail -200                    # 先看最近的 log，不要無限串流
adb logcat --pid=$(adb shell pidof -s <package>) -d
```

- `minSdk 24`。目前沒有 native 程式碼，不設 `abiFilters`；日後加 native 時 ZenPad 實際是 arm64-v8a。
- 播放回報：Jellyfin 會把沒有自己轉碼工作的 `PlayMethod: Transcode` 改成 DirectPlay（後台顯示用），不影響進度紀錄，不用處理。
- 確認解碼器：debug 版把解碼器名稱、格式、掉幀寫進 app 私有的 `files/playback.log`（每次播放覆寫），用 `adb shell run-as com.jellyfinextra.player cat files/playback.log` 讀。硬解為 `OMX.MTK.VIDEO.DECODER.*`。這台的 logcat 緩衝很小，不要依賴 logcat。
- 開發時預填連線設定：debug 版可用 `adb shell run-as com.jellyfinextra.player` 寫入 `shared_prefs/settings.xml`（只放位址與 profile，不放 token；密碼由使用者在裝置上輸入）。
- Java 8+ API（如 `java.time`）靠 core library desugaring，不要假設 API 26 以上才有的 API 可用。
- Android 7.0 沒有原生 TLS 1.3，系統憑證庫也沒有 Let's Encrypt 的 ISRG Root X1（7.1.1 才加入）：連 HTTPS 時用 `network_security_config` 內建所需根憑證。`targetSdk >= 24` 預設不信任使用者安裝的 CA。
- 實測 ZenPad 系統 TLS（Conscrypt）的 ClientHello：只有 TLS 1.0–1.2，曲線**只有 P-256**（沒有 X25519、P-384）。目前網域憑證是 ECDSA P-384，系統 TLS 握手必定失敗（alert 40）。播放程式的所有 HTTPS（Jellyfin 與 xcode）要走 app 內建的 Conscrypt + OkHttp，ExoPlayer 用 OkHttpDataSource，不要用系統的 HttpURLConnection。
- 裝置實際支援的硬體解碼格式以 `MediaCodecList` 實測為準，不要只憑規格表。目前實測（`/system/etc/media_codecs*.xml` 與 logcat）：
  - 影像硬解：H.264（`OMX.MTK.VIDEO.DECODER.AVC`，最高 1920×1088）、HEVC（`OMX.MTK.VIDEO.DECODER.HEVC`，最高 1920×1088）、MPEG-4、H.263。VP8 / VP9 只有軟解，沒有 AV1。
  - 音訊：AAC、MP3、FLAC、Vorbis、Opus、DTS；沒有 AC3 / E-AC3。
  - 裝置 ABI 是 `arm64-v8a,armeabi-v7a,armeabi`。
- 容器內的 adb 收不到 USB 熱插拔事件：手機接上後 `adb devices` 是空的，先 `adb kill-server && adb start-server`。
- 模擬器不可用於 armeabi-v7a native 測試，一律用實機。
- native 除錯：把 NDK 內的 32 位元 `lldb-server` 推到 `/data/local/tmp`，再 `adb forward`。

## iOS 播放程式（Theos app）

```makefile
ARCHS = arm64
TARGET := iphone:clang:12.4:12.0
INSTALL_TARGET_PROCESSES = <App 名稱>
include $(THEOS)/makefiles/common.mk
APPLICATION_NAME = <App 名稱>
include $(THEOS_MAKE_PATH)/application.mk
```

```bash
make clean package                           # 產生 .deb 到 packages/
make package install THEOS_DEVICE_IP=ipad    # 安裝到裝置
ssh ipad uicache                             # 主畫面沒出現圖示時
idevicesyslog | grep -i <關鍵字>              # 即時 log（USB，需 usbmuxd socket 有效，容器要用 compose.ios.yaml 啟動）
```

- 語言用 Objective-C，不用 Swift。iOS 12 是 rootful，不設 `THEOS_PACKAGE_SCHEME`。
- 播放用 AVPlayer（原生支援 HLS）。A7 沒有 HEVC 硬解，送 H.264（最高約 1080p）+ AAC。
- SDK 用 `$THEOS/sdks/iPhoneOS12.4.sdk`，不要隨意換成更新的 SDK。
- 越獄重開機後會失效，連不上裝置時先問使用者是否需要重新用 checkra1n 越獄（DFU 只能由使用者手動操作）。
- 連線失敗時用 `ssh -v ipad` 看演算法相容問題，iOS 12 的 sshd 較舊。
- 裝置是 iPad4,1（Wi-Fi 版），iOS 12.5.7（16H81），OpenSSH 8.4（Cydia），已裝 uikittools（`uiopen`、`uicache`、`sbreload`）。系統沒有 `awk` 等常見工具，遠端指令用 `grep`、`cut`，或在容器端處理輸出。
- 用 `ssh ipad uiopen <URL>` 可以讓 Safari 開網址；Safari 是 12.1.2，太新的網頁功能不可用。
- iPad 的 DNS 會先問路由器發的中華電信 IPv6 DNS（公開 DNS 查不到我們的網域，回 NXDOMAIN），而且 iPad 的 DNS 設定改不了；在 `/etc/hosts` 加對應也無效（已還原）。iPad 一律用內網 IP 連線。
- 不要建議使用者開關飛航模式：這台 iPad 會當機一段時間。
- 在 iPad 上執行任何指令（包括 `uiopen` 開網頁）前，先說明並等使用者同意。

## 權限邊界（請遵守）

- SSH 只允許連 `ipad` 與 `Windows` 這兩個別名，不要連其他主機，也不要新增主機或金鑰設定。
- iPad 上只動自己安裝的套件（播放程式在 `/Applications/` 下的 app、必要時 `/Library/MobileSubstrate/` 內的 tweak）。不要修改系統檔、不要重開機、不要改 root 密碼或 SSH 設定、不要移除越獄元件。
- Windows 上只在 `C:\dev\jellyfin-extra-test` 內操作，不要刪除或修改該目錄以外的東西。
- 不要修改 Linux Jellyfin 主機、NAS 與反向代理的設定；需要調整時整理成步驟請使用者操作。
- 破壞性或不可逆的指令（`rm -rf`、`git push --force`、`git reset --hard` 等）執行前先說明並等待確認。
- 不要把金鑰、token、Jellyfin 網址、裝置 IP 之類的資訊寫進程式碼或提交到 git，改用不進版控的設定檔或環境變數。
- 修改建置設定（Gradle、Makefile、go.mod 的工具鏈、`devenv/`）前先說明原因。
