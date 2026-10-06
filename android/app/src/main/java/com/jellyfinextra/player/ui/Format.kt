package com.jellyfinextra.player.ui

import com.jellyfinextra.player.net.JellyfinApi.Item
import java.util.Locale

/** Jellyfin 的時間單位：1 tick = 100ns。 */
const val TICKS_PER_MS = 10_000L
private const val TICKS_PER_MINUTE = 60 * 1000 * TICKS_PER_MS

/** 播放位置，例如 22:00、1:06:46。 */
fun formatTicks(ticks: Long): String {
    val total = ticks / TICKS_PER_MS / 1000
    val h = total / 3600
    val m = total % 3600 / 60
    val s = total % 60
    return if (h > 0) "%d:%02d:%02d".format(Locale.US, h, m, s) else "%d:%02d".format(Locale.US, m, s)
}

/** 片長，例如「1 小時 41 分」「24 分」。 */
fun formatDuration(ticks: Long): String {
    val minutes = ((ticks + TICKS_PER_MINUTE / 2) / TICKS_PER_MINUTE).toInt()
    val h = minutes / 60
    val m = minutes % 60
    return when {
        h > 0 && m > 0 -> "$h 小時 $m 分"
        h > 0 -> "$h 小時"
        else -> "$m 分"
    }
}

/** 還沒看完的部分，例如「剩 1 小時 19 分」。 */
fun formatRemaining(item: Item): String = "剩 " + formatDuration((item.runTimeTicks - item.positionTicks).coerceAtLeast(0))

/** 0–1000 的進度，給進度條用。 */
fun progressPermille(item: Item): Int =
    if (item.runTimeTicks <= 0) 0 else (item.positionTicks * 1000 / item.runTimeTicks).toInt().coerceIn(0, 1000)

/** 集數的短標，例如「第 2 集」；季號不是 1 時加上季，例如「S2 · 第 3 集」。 */
fun episodeLabel(item: Item): String {
    val ep = item.indexNumber?.let { "第 $it 集" } ?: item.name
    val season = item.parentIndexNumber
    return if (season != null && season != 1 && season != 0) "S$season · $ep" else ep
}

/** 畫質檔位（16:9 框的高度），由小到大。 */
private val RESOLUTION_TIERS = listOf(360, 480, 720, 1080, 1440, 2160)

/**
 * 轉碼畫面的畫質，例如「1080p」「1080p · 1920×802」。檔位是放得進的最小 16:9 框
 * （寬銀幕片 1920×802 也是 1080p，和畫質選項一致），不是剛好 16:9 的檔位尺寸時才附上實際尺寸。
 */
fun resolutionLabel(width: Int, height: Int): String {
    // 編碼器會把尺寸對齊到偶數或 16 的倍數，留一點餘裕
    val tier = RESOLUTION_TIERS.firstOrNull { width <= it * 16 / 9 + 2 && height <= it + 8 } ?: height
    val standard = height == tier && width >= tier * 16 / 9 - 2 // 剛好是 16:9 的檔位尺寸
    return if (width <= 0 || standard) "${tier}p" else "${tier}p · $width×$height"
}

/** 卡片與詳情頁的標題：集數用影集名稱。 */
fun displayTitle(item: Item): String = if (item.type == "Episode") item.seriesName ?: item.name else item.name

/** 年份、片長、分級、評分組成的資訊列。 */
fun metaLine(item: Item, includeGenres: Boolean = true): String = listOfNotNull(
    item.productionYear?.toString(),
    item.runTimeTicks.takeIf { it > 0 && item.type != "Series" }?.let { formatDuration(it) },
    item.officialRating,
    item.communityRating?.let { "★ %.1f".format(Locale.US, it) },
    item.genres.take(3).joinToString(" / ").takeIf { includeGenres && it.isNotEmpty() },
).joinToString(" · ")
