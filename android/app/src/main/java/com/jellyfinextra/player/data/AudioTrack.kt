package com.jellyfinextra.player.data

/** Jellyfin 的一條音軌。轉碼伺服器一律轉成 AAC 立體聲，切換音軌要重建轉碼 session。 */
data class AudioTrack(
    val index: Int,
    val title: String,
    val isDefault: Boolean,
)
