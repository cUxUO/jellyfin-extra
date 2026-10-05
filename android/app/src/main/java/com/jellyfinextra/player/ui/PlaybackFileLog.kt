package com.jellyfinextra.player.ui

import android.content.Context
import androidx.media3.common.Format
import androidx.media3.common.PlaybackException
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DecoderCounters
import androidx.media3.exoplayer.DecoderReuseEvaluation
import androidx.media3.exoplayer.analytics.AnalyticsListener
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * debug 版的播放紀錄，寫在 app 私有目錄的 playback.log（每次播放覆寫）。
 * 這台平板的 logcat 很快被洗掉，改寫檔案才能可靠地事後讀取：
 * adb shell run-as com.jellyfinextra.player cat files/playback.log
 */
@UnstableApi
class PlaybackFileLog(context: Context) : AnalyticsListener {
    private val file = File(context.filesDir, "playback.log").apply { writeText("") }
    private val time = SimpleDateFormat("HH:mm:ss", Locale.US)

    fun log(message: String) {
        runCatching { file.appendText("${time.format(Date())} $message\n") }
    }

    override fun onVideoDecoderInitialized(eventTime: AnalyticsListener.EventTime, decoderName: String, initializedTimestampMs: Long, initializationDurationMs: Long) =
        log("video decoder $decoderName (${initializationDurationMs}ms)")

    override fun onAudioDecoderInitialized(eventTime: AnalyticsListener.EventTime, decoderName: String, initializedTimestampMs: Long, initializationDurationMs: Long) =
        log("audio decoder $decoderName")

    override fun onVideoInputFormatChanged(eventTime: AnalyticsListener.EventTime, format: Format, decoderReuseEvaluation: DecoderReuseEvaluation?) =
        log("video format ${format.sampleMimeType} ${format.codecs} ${format.width}x${format.height} ${format.frameRate}fps")

    override fun onDroppedVideoFrames(eventTime: AnalyticsListener.EventTime, droppedFrames: Int, elapsedMs: Long) =
        log("dropped $droppedFrames frames in ${elapsedMs}ms")

    override fun onPlaybackStateChanged(eventTime: AnalyticsListener.EventTime, state: Int) {
        val name = when (state) { 1 -> "IDLE"; 2 -> "BUFFERING"; 3 -> "READY"; 4 -> "ENDED"; else -> "$state" }
        log("state $name at ${eventTime.currentPlaybackPositionMs}ms")
    }

    override fun onPlayerError(eventTime: AnalyticsListener.EventTime, error: PlaybackException) =
        log("error ${error.errorCodeName}: ${error.message}")

    override fun onAudioCodecError(eventTime: AnalyticsListener.EventTime, audioCodecError: Exception) =
        log("audio codec error: $audioCodecError")

    override fun onAudioSinkError(eventTime: AnalyticsListener.EventTime, audioSinkError: Exception) =
        log("audio sink error: $audioSinkError")

    override fun onAudioUnderrun(eventTime: AnalyticsListener.EventTime, bufferSize: Int, bufferSizeMs: Long, elapsedSinceLastFeedMs: Long) =
        log("audio underrun (${elapsedSinceLastFeedMs}ms since last feed)")

    override fun onAudioInputFormatChanged(eventTime: AnalyticsListener.EventTime, format: Format, decoderReuseEvaluation: DecoderReuseEvaluation?) =
        log("audio format ${format.sampleMimeType} ${format.channelCount}ch ${format.sampleRate}Hz")

    override fun onLoadError(
        eventTime: AnalyticsListener.EventTime,
        loadEventInfo: androidx.media3.exoplayer.source.LoadEventInfo,
        mediaLoadData: androidx.media3.exoplayer.source.MediaLoadData,
        error: java.io.IOException,
        wasCanceled: Boolean,
    ) = log("load error: $error")

    override fun onIsLoadingChanged(eventTime: AnalyticsListener.EventTime, isLoading: Boolean) =
        log("loading=$isLoading buffered=${eventTime.totalBufferedDurationMs}ms")

    override fun onVideoDisabled(eventTime: AnalyticsListener.EventTime, decoderCounters: DecoderCounters) =
        log("video totals: rendered ${decoderCounters.renderedOutputBufferCount}, dropped ${decoderCounters.droppedBufferCount}, max consecutive dropped ${decoderCounters.maxConsecutiveDroppedBufferCount}")
}
