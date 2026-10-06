package com.jellyfinextra.player.data

/**
 * 播放畫質。
 * - [AUTO]：內網時能直接播放就播原始檔，否則轉成多軌 HLS、由播放器依網速切換解析度（上限是螢幕大小）。
 * - [ORIGINAL]：能直接播放就播原始檔，否則以裝置能硬解的最高解析度轉碼。
 * - [DIRECT]：不管能不能直接播放一律播原始檔（例如想看 HDR 或保留原始音軌）；播不起來時改最高畫質轉碼。
 * - 其他：固定解析度轉碼（放進 16:9 的框）。
 */
enum class Quality(val label: String, val maxHeight: Int) {
    AUTO("自動（依網速）", 0),
    ORIGINAL("原始畫質", 0),
    DIRECT("原檔（不轉碼）", 0),
    P1080("1080p", 1080),
    P720("720p", 720),
    P480("480p", 480);

    /** 固定解析度時送給轉碼伺服器的框寬。 */
    val maxWidth: Int get() = maxHeight * 16 / 9

    companion object {
        fun of(name: String?): Quality = entries.firstOrNull { it.name == name } ?: AUTO
    }
}
