# 開發容器：Android 7.0 (armeabi-v7a) + Windows 交叉編譯 + iOS 12 越獄 tweak (Theos)
# 注意：這份檔案沒有實際建置測試過，第一次 build 若某個套件或步驟失敗，請依錯誤訊息調整。

# 選 22.04：Theos 的 Linux toolchain 可能需要 libtinfo5，24.04 已移除該套件
FROM ubuntu:22.04

ARG DEBIAN_FRONTEND=noninteractive
# 要和主機使用者的 UID 一致（Arch 第一個使用者通常是 1000），
# 這樣掛載進來的專案檔案權限正常，USB 裝置的 ACL 也會對容器內的使用者生效
ARG UID=1000
# 想鎖定 Frida 版本時填，例如 16.1.11（iOS 12 的相容性要實測）；留空則裝最新版
ARG FRIDA_TOOLS_VERSION=
# Go：Windows 轉碼伺服器用。升級時兩個值一起改，checksum 見 https://go.dev/dl/?mode=json
ARG GO_VERSION=1.27.1
ARG GO_SHA256=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445

RUN apt-get update && apt-get install -y --no-install-recommends \
    # 通用
    ca-certificates curl git unzip zip xz-utils rsync file jq less nano tmux \
    openssh-client build-essential cmake ninja-build pkg-config python3 python3-pip pipx \
    # Android
    openjdk-17-jdk-headless \
    # Theos / 越獄
    fakeroot dpkg perl libplist-utils libplist-dev libssl-dev libtinfo5 \
    libimobiledevice-utils libusbmuxd-tools ideviceinstaller usbutils \
    # Windows 交叉編譯（mingw-w64 與 clang/lld）
    clang lld mingw-w64 \
 && rm -rf /var/lib/apt/lists/*

# ldid：Theos 簽章必備，從 Procursus 版原始碼編譯
RUN git clone --depth 1 --recursive https://github.com/ProcursusTeam/ldid.git /tmp/ldid \
 && make -C /tmp/ldid -j"$(nproc)" \
 && make -C /tmp/ldid install PREFIX=/usr/local \
 && rm -rf /tmp/ldid

# Go（官方 tarball，驗證 checksum）
RUN curl -fsSLo /tmp/go.tgz "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
 && echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm /tmp/go.tgz

# 非 root 使用者。預先建立 volume 掛載點並設好擁有者，
# 否則 Docker 建立的具名 volume 會是 root 擁有，dev 使用者寫不進去
RUN useradd -m -u ${UID} -s /bin/bash dev \
 && mkdir -p /work /home/dev/Android /home/dev/.gradle /home/dev/.android \
             /home/dev/.ssh /home/dev/.claude /home/dev/theos /home/dev/go \
 && chown -R dev:dev /work /home/dev \
 && chmod 700 /home/dev/.ssh

USER dev
WORKDIR /work

ENV ANDROID_HOME=/home/dev/Android/Sdk \
    THEOS=/home/dev/theos \
    CLAUDE_CONFIG_DIR=/home/dev/.claude \
    GOPATH=/home/dev/go \
    GOCACHE=/home/dev/go/cache
ENV PATH=/home/dev/.local/bin:/usr/local/go/bin:${GOPATH}/bin:${ANDROID_HOME}/cmdline-tools/latest/bin:${ANDROID_HOME}/platform-tools:${THEOS}/bin:${PATH}

# Frida client（裝置端 frida-server 版本必須與此一致）
RUN pipx install "frida-tools${FRIDA_TOOLS_VERSION:+==$FRIDA_TOOLS_VERSION}"

# Claude Code（原生安裝腳本，安裝到 ~/.local/bin）
# 若此步驟失敗或官方做法已變動，請以官方文件為準；備案是裝 Node.js 後用 npm 安裝
RUN curl -fsSL https://claude.ai/install.sh | bash

# 其他語言工具鏈依需要再加，例如：
# RUN curl https://sh.rustup.rs | sh -s -- -y --profile minimal \
#  && ~/.cargo/bin/cargo install cargo-xwin
# （dotnet 等也可以在這裡加）

CMD ["sleep", "infinity"]
