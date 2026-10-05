package com.jellyfinextra.player.data

/** 片源的影像資訊（Jellyfin MediaStream）。 */
data class SourceVideo(
    val codec: String,
    val width: Int,
    val height: Int,
    val bitDepth: Int,
    val frameRate: Float,
    /** Jellyfin 的 VideoRange：SDR、HDR；空字串表示不明。 */
    val range: String,
)

/** 判斷能否不經轉碼、直接播放原始檔。 */
object DirectPlay {
    /** 裝置的解碼能力；實作是 [DeviceCaps]，測試時可替換。 */
    interface Decoders {
        /** 有硬體解碼器能解這個格式、尺寸、幀率與位元深度。 */
        fun video(codec: String, width: Int, height: Int, frameRate: Float, bitDepth: Int): Boolean
        /** 有解碼器（軟解也可以）能解這個音訊格式。 */
        fun audio(codec: String): Boolean
    }

    /** ExoPlayer 能讀的容器（Jellyfin 的 Container 可能是逗號分隔的別名清單）。 */
    private val CONTAINERS = setOf("mkv", "matroska", "webm", "mp4", "m4v", "mov", "mpegts", "ts")

    /** 再高就算能解，Wi-Fi 與舊平板的讀取也吃不消。 */
    const val MAX_BITRATE = 40_000_000L

    /**
     * 回傳 null 表示可以直接播放，否則是不行的原因（顯示與記錄用）。
     * [audioIndex] 是使用者選的音軌（null 表示預設）；直接播放只能播容器的預設音軌。
     * [subtitle] 是要顯示的字幕；圖形字幕要轉碼伺服器燒錄。
     */
    fun check(
        container: String,
        bitrate: Long,
        video: SourceVideo?,
        audio: List<AudioTrack>,
        audioIndex: Int?,
        subtitle: SubtitleTrack?,
        d: Decoders,
    ): String? {
        if (container.split(',').none { it.trim().lowercase() in CONTAINERS }) return "容器 $container"
        val v = video ?: return "沒有影像資訊"
        // 這些舊平板的螢幕都是 SDR，HDR／杜比視界直接播放顏色會錯，交給轉碼伺服器 tonemap
        if (v.range.isNotEmpty() && !v.range.equals("SDR", ignoreCase = true)) return "HDR"
        if (!d.video(v.codec, v.width, v.height, v.frameRate, v.bitDepth)) {
            val depth = if (v.bitDepth > 8) " ${v.bitDepth}-bit" else ""
            return "${v.codec.uppercase()} ${v.width}×${v.height}$depth 無硬解"
        }
        if (bitrate > MAX_BITRATE) return "位元率 ${bitrate / 1_000_000} Mbps 過高"
        val default = audio.firstOrNull { it.isDefault } ?: audio.firstOrNull()
        if (audioIndex != null && audioIndex != default?.index) return "非預設音軌"
        if (default != null && !d.audio(default.codec)) return "音訊 ${default.codec.uppercase()} 無解碼器"
        if (subtitle?.isImage == true) return "圖形字幕要燒錄"
        return null
    }
}
