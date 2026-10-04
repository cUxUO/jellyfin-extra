package com.jellyfinextra.player.ui

import android.graphics.Color
import android.os.Handler
import android.os.Looper
import androidx.media3.common.Player
import androidx.media3.common.text.Cue
import androidx.media3.common.util.UnstableApi
import androidx.media3.extractor.text.CuesWithTiming
import androidx.media3.extractor.text.SubtitleParser
import androidx.media3.extractor.text.webvtt.WebvttParser
import androidx.media3.ui.CaptionStyleCompat
import androidx.media3.ui.SubtitleView

/**
 * 自己顯示文字字幕：WebVTT 解析成帶時間的 cue，依播放位置每 100ms 更新一次。
 * 不經過 ExoPlayer 的字幕軌，所以字幕晚到（Jellyfin 抽取內嵌字幕要讀完整個檔案）或切換字幕都不必重建播放。
 */
@UnstableApi
class SubtitleOverlay(private val view: SubtitleView) {
    private var cues: List<CuesWithTiming> = emptyList()
    private var player: Player? = null
    private val handler = Handler(Looper.getMainLooper())

    private val tick = object : Runnable {
        override fun run() {
            render()
            handler.postDelayed(this, 100)
        }
    }

    init {
        view.setStyle(
            CaptionStyleCompat(
                Color.WHITE, Color.TRANSPARENT, Color.TRANSPARENT,
                CaptionStyleCompat.EDGE_TYPE_OUTLINE, Color.BLACK, null,
            )
        )
        view.setFractionalTextSize(SubtitleView.DEFAULT_TEXT_SIZE_FRACTION * 1.1f)
        // VTT 是 Jellyfin 從 ASS 轉來的，原本的位置與樣式已經不準，統一用底部置中
        view.setApplyEmbeddedStyles(false)
    }

    /**
     * 播放控制列出現時把整個字幕層往上移，免得蓋住進度條。
     * （WebVTT cue 的位置不受 SubtitleView 的 bottomPaddingFraction 影響，所以直接位移 view。）
     */
    fun setRaised(raised: Boolean) {
        val px = RAISE_DP * view.resources.displayMetrics.density
        view.animate().translationY(if (raised) -px else 0f).setDuration(150).start()
    }

    fun attach(player: Player) {
        this.player = player
        handler.removeCallbacks(tick)
        handler.post(tick)
    }

    /** 換上新的字幕內容（WebVTT 原文）。 */
    fun show(vtt: ByteArray) {
        val parsed = ArrayList<CuesWithTiming>()
        WebvttParser().parse(vtt, 0, vtt.size, SubtitleParser.OutputOptions.allCues()) { parsed.add(it) }
        // ASS 轉出的 VTT 不一定依時間排序
        cues = parsed.sortedBy { it.startTimeUs }
        render()
    }

    fun clear() {
        cues = emptyList()
        view.setCues(emptyList())
    }

    fun release() {
        handler.removeCallbacks(tick)
        player = null
    }

    private companion object {
        const val RAISE_DP = 100f
    }

    private fun render() {
        val p = player ?: return
        val now = p.currentPosition * 1000
        val active = ArrayList<Cue>()
        for (c in cues) {
            if (c.startTimeUs > now) break
            if (c.endTimeUs > now) active.addAll(c.cues)
        }
        view.setCues(active)
    }
}
