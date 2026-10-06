package com.jellyfinextra.player.data

/**
 * 文字字幕的大小（app 自己畫的 WebVTT 字幕）。圖形字幕燒在畫面裡，不受影響。
 * 規則同 iOS 的 JXSettings.subtitleScale：「中」是原本的大小。
 */
enum class SubtitleSize(val label: String, val scale: Float) {
    SMALL("小", 0.8f),
    MEDIUM("中", 1f),
    LARGE("大", 1.25f),
    XLARGE("特大", 1.5f);

    companion object {
        fun of(name: String?): SubtitleSize = entries.firstOrNull { it.name == name } ?: MEDIUM
    }
}
