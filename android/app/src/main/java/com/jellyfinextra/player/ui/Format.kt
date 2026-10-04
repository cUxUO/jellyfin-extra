package com.jellyfinextra.player.ui

/** Jellyfin 的時間單位：1 tick = 100ns。 */
const val TICKS_PER_MS = 10_000L

fun formatTicks(ticks: Long): String {
    val total = ticks / TICKS_PER_MS / 1000
    val h = total / 3600
    val m = total % 3600 / 60
    val s = total % 60
    return if (h > 0) "%d:%02d:%02d".format(h, m, s) else "%d:%02d".format(m, s)
}
