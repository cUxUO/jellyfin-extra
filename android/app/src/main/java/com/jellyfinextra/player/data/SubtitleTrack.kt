package com.jellyfinextra.player.data

import java.util.Locale

/** Jellyfin 的一條字幕串流。 */
data class SubtitleTrack(
    val index: Int,
    val title: String,
    val language: String?,
    val codec: String,
    val isExternal: Boolean,
    val isDefault: Boolean,
    val isForced: Boolean,
) {
    /** 點陣圖字幕（藍光 PGS、DVD）只能由轉碼伺服器燒進畫面；文字字幕由 app 自己顯示。 */
    val isImage: Boolean get() = codec.lowercase() in IMAGE_CODECS

    /** 外掛的圖形字幕在 Jellyfin 主機上，轉碼伺服器讀不到，無法燒錄。 */
    val playable: Boolean get() = !isImage || !isExternal

    companion object {
        private val IMAGE_CODECS = setOf("pgssub", "hdmv_pgs_subtitle", "dvdsub", "dvd_subtitle", "dvbsub", "dvb_subtitle", "xsub")
    }
}

/**
 * 預設字幕的挑選規則：
 * 1. 符合偏好語言的文字字幕（Jellyfin 的字幕語言偏好，沒設定就用裝置語言）；
 *    中文再依裝置地區分繁簡（台港澳偏好繁體），標示預設的加分，「特效／歌詞」之類的部分字幕扣分。
 * 2. 否則用標示為強制或預設的字幕（可能是圖形字幕）。
 * 3. 都沒有就不顯示。
 */
object SubtitleChooser {
    private val CHINESE_CODES = setOf("zho", "chi", "zh", "cmn", "yue")
    private val CHINESE_TITLE = Regex("""中文|繁|简|簡|chinese|\b(cht|chs|tc|sc)\b""", RegexOption.IGNORE_CASE)
    private val TRADITIONAL = Regex("""繁|正體|traditional|big5|\b(cht|tc)\b""", RegexOption.IGNORE_CASE)
    private val SIMPLIFIED = Regex("""简|簡體|simplified|\b(chs|sc|gb)\b""", RegexOption.IGNORE_CASE)
    private val PARTIAL = Regex("""signs|songs|forced|特效|歌詞|歌词|\bsdh\b""", RegexOption.IGNORE_CASE)

    fun choose(tracks: List<SubtitleTrack>, preferredLanguage: String?, locale: Locale = Locale.getDefault()): SubtitleTrack? {
        val usable = tracks.filter { it.playable }
        val lang = preferredLanguage?.takeIf { it.isNotBlank() }?.lowercase() ?: runCatching { locale.isO3Language }.getOrDefault("")
        val wantChinese = lang in CHINESE_CODES
        val preferTraditional = locale.country.uppercase() in setOf("TW", "HK", "MO")

        val matching = usable.filter { !it.isImage && matchesLanguage(it, lang, wantChinese) }
        if (matching.isNotEmpty()) {
            return matching.maxBy { score(it, wantChinese, preferTraditional) }
        }
        return usable.firstOrNull { it.isForced } ?: usable.firstOrNull { it.isDefault }
    }

    private fun matchesLanguage(t: SubtitleTrack, lang: String, wantChinese: Boolean): Boolean {
        val code = t.language?.lowercase()
        return if (wantChinese) code in CHINESE_CODES || CHINESE_TITLE.containsMatchIn(t.title) else code == lang
    }

    private fun score(t: SubtitleTrack, chinese: Boolean, preferTraditional: Boolean): Int {
        var s = 0
        if (chinese) {
            val traditional = TRADITIONAL.containsMatchIn(t.title)
            val simplified = SIMPLIFIED.containsMatchIn(t.title)
            if (preferTraditional && traditional) s += 4
            if (!preferTraditional && simplified) s += 4
            if (preferTraditional && simplified && !traditional) s -= 1
        }
        if (t.isDefault) s += 1
        if (PARTIAL.containsMatchIn(t.title)) s -= 3
        return s
    }
}
