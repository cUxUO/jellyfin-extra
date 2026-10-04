# 主機與容器設定步驟（Arch Linux）

這份文件說明主機要裝什麼，以及第一次使用容器的步驟。`Dockerfile` 和 `compose.yaml` 沒有實際建置測試過，哪一步出錯就依訊息調整。

## 一、主機端（只裝這些）

```bash
sudo pacman -Syu
sudo pacman -S --needed docker docker-compose docker-buildx git openssh android-udev usbmuxd
sudo systemctl enable --now docker.socket
```

- **docker 群組**：`sudo usermod -aG docker $USER`，重新登入後生效。注意加入 docker 群組等於擁有主機的 root 等級權限，如果在意這點，可以改用 rootless Podman，但 USB 與 socket 掛載的設定會更麻煩。
- **adb 不要裝在主機**：容器內自帶 `adb`，主機同時跑 adb server 會搶佔手機。若不小心裝了，先 `adb kill-server`。
- **checkra1n**：從官網下載 Linux 版執行檔，放到 `~/.local/bin/checkra1n` 並 `chmod +x`。它不是 pacman 套件。
- **ssh-agent**：確認 `echo $SSH_AUTH_SOCK` 有值，並 `ssh-add` 你的金鑰（`ssh-keygen -t ed25519` 產生）。

## 二、第一次建置

```bash
cd devenv
docker compose build
```

## 三、每次的使用順序

以下指令都在 `devenv/` 內執行。

1. 接上 Android 手機（要開 USB 偵錯）。
2. 只做 Android / Windows 時，直接啟動容器並進入：

```bash
docker compose up -d
docker compose exec dev bash
```

3. 要做 iOS 時：先接上 iPad，用 checkra1n 越獄（DFU 模式需手動操作，重開機後要重做）；越獄完成、iPad 正常進入系統後，確認 usbmuxd 在跑：`ls /run/usbmuxd`。再疊加 `compose.ios.yaml` 啟動（容器已在跑也可以直接執行，compose 會重建容器以加上掛載）：

```bash
docker compose -f compose.yaml -f compose.ios.yaml up -d
docker compose exec dev bash
```

4. 收工：`docker compose down`（volume 會保留）。

用 `compose.ios.yaml` 時，如果 `ls /run/usbmuxd` 找不到檔案，容器會因為 `create_host_path: false` 而啟動失敗，這是刻意的，避免 Docker 在主機上自動建立一個 root 擁有的空目錄。iPad 拔除後 usbmuxd 可能退出，重插後 socket 會重建，這時執行 `docker compose -f compose.yaml -f compose.ios.yaml restart`。

## 四、容器內的一次性安裝

以下都在 `docker compose exec dev bash` 之內執行，結果會存在 volume，不用重做。

### Android SDK

到 <https://developer.android.com/studio#command-line-tools-only> 複製 Linux 版 command-line tools 的下載連結：

```bash
mkdir -p ~/Android/Sdk/cmdline-tools && cd /tmp
curl -LO '<官網的 commandlinetools-linux-xxxx_latest.zip 連結>'
unzip commandlinetools-linux-*_latest.zip -d ~/Android/Sdk/cmdline-tools
mv ~/Android/Sdk/cmdline-tools/cmdline-tools ~/Android/Sdk/cmdline-tools/latest

yes | sdkmanager --licenses
sdkmanager "platform-tools" "platforms;android-34" "build-tools;34.0.0" "cmake;3.22.1"
sdkmanager --list | grep ndk          # 挑一個近期穩定版本
sdkmanager "ndk;<版本>"
```

### Theos

```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/theos/theos/master/bin/install-theos)"
```

`THEOS` 已設為 `~/theos`。安裝腳本會下載 toolchain 和 SDK，細節以官方文件為準。完成後 `ls $THEOS/sdks` 確認有 12.x 或 13.x 的 SDK，沒有的話自行放入。

### Claude Code 登入

```bash
claude
```

登入狀態存在 `claude-config` volume。`CLAUDE_CONFIG_DIR` 這個環境變數我是憑印象設定的，若登入狀態沒有留住，請對照官方文件確認設定目錄的做法。

### SSH 主機別名

編輯容器內的 `~/.ssh/config`（存在 `dev-ssh` volume）：

```
Host ipad
    HostName <iPad 的 IP>
    User root
    # iOS 12 的 sshd 若報演算法錯誤，在這裡加 HostKeyAlgorithms / KexAlgorithms，以 ssh -v 的訊息為準

Host Windows
    HostName <Windows 的 IP>
    User <專用帳號>
```

先在 iPad 上改掉預設密碼 `alpine`，再 `ssh-copy-id ipad`，之後可關掉密碼登入。Windows 端需啟用 OpenSSH Server。

### 驗證

```bash
adb devices
adb shell getprop ro.build.version.sdk      # 應為 24
adb shell getprop ro.product.cpu.abilist    # 應包含 armeabi-v7a
idevice_id -l                               # 應列出 iPad
ssh ipad uname -a
ssh Windows hostname
echo $THEOS && ls $THEOS/sdks
```

## 五、Claude Code 權限設定（建議）

在專案的 `.claude/settings.json` 限縮 Claude Code 能自動執行的指令，範例如下。語法請以官方文件為準，我沒有逐項核對：

```json
{
  "permissions": {
    "allow": [
      "Bash(make:*)",
      "Bash(./gradlew:*)",
      "Bash(adb devices)",
      "Bash(adb install:*)",
      "Bash(adb logcat:*)",
      "Bash(idevicesyslog:*)",
      "Bash(ssh ipad:*)",
      "Bash(ssh Windows:*)",
      "Bash(scp:*)"
    ],
    "deny": [
      "Bash(sudo:*)",
      "Bash(rm -rf /:*)"
    ]
  }
}
```

## 六、清理

```bash
docker compose down -v        # 連同所有 volume（SDK、Theos、登入狀態）一起刪除
docker image rm devenv:latest
docker builder prune
```

主機上只會留下 docker、android-udev、usbmuxd、git、openssh 和 checkra1n。
