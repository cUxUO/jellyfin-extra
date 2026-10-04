# iPad 播放程式（Theos application，Objective-C）。見 CLAUDE.md 的 iOS 一節。
ARCHS = arm64
TARGET := iphone:clang:12.4:12.0
INSTALL_TARGET_PROCESSES = JellyfinExtra

include $(THEOS)/makefiles/common.mk

APPLICATION_NAME = JellyfinExtra
JellyfinExtra_FILES = $(wildcard Sources/*/*.m)
JellyfinExtra_FRAMEWORKS = UIKit Foundation AVFoundation CoreMedia QuartzCore
JellyfinExtra_CFLAGS = -fobjc-arc -ISources/App -ISources/Net -ISources/Model -ISources/UI -ISources/Player

include $(THEOS_MAKE_PATH)/application.mk

# 安裝後讓主畫面重新掃描 /Applications，圖示才會出現
after-install::
	install.exec "uicache -p /Applications/JellyfinExtra.app || uicache"
