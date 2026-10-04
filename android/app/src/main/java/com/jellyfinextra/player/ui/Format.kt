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
