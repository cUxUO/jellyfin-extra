package com.jellyfinextra.player.ui

import android.content.Context
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.jellyfinextra.player.data.AudioTrack
import com.jellyfinextra.player.data.SubtitleTrack

/** 詳情頁在播放前選音軌、字幕用的對話框；播放中改用播放畫面的側邊面板。 */
object TrackPicker {
    fun audio(context: Context, tracks: List<AudioTrack>, current: Int?, onPick: (AudioTrack) -> Unit) {
        if (tracks.isEmpty()) return
        val checked = tracks.indexOfFirst { it.index == current }.coerceAtLeast(0)
        MaterialAlertDialogBuilder(context)
            .setTitle("音軌")
            .setSingleChoiceItems(tracks.map { it.title }.toTypedArray<CharSequence>(), checked) { d, which ->
                d.dismiss()
                onPick(tracks[which])
            }
            .show()
    }

    /** [current] 為 null 表示關閉字幕。 */
    fun subtitle(context: Context, tracks: List<SubtitleTrack>, current: SubtitleTrack?, onPick: (SubtitleTrack?) -> Unit) {
        val usable = tracks.filter { it.playable }
        val labels = listOf("關閉") + usable.map { subtitleLabel(it) }
        val checked = current?.let { cur -> usable.indexOfFirst { it.index == cur.index } + 1 } ?: 0
        MaterialAlertDialogBuilder(context)
            .setTitle("字幕")
            .setSingleChoiceItems(labels.toTypedArray<CharSequence>(), checked) { d, which ->
                d.dismiss()
                onPick(if (which == 0) null else usable[which - 1])
            }
            .show()
    }

    fun subtitleLabel(t: SubtitleTrack) = if (t.isImage) "${t.title}（圖形，燒進畫面）" else t.title
}
