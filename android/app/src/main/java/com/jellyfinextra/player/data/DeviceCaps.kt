package com.jellyfinextra.player.data

import android.media.MediaCodecInfo
import android.media.MediaCodecInfo.CodecProfileLevel
import android.media.MediaCodecList
import android.os.Build
import org.json.JSONArray
import org.json.JSONObject

/**
 * 裝置實測的解碼能力（MediaCodecList），不憑規格表。
 * 決定轉碼伺服器送 HEVC 還是 H.264、最大解析度，以及能不能直接播放原始檔。
 */
object DeviceCaps : DirectPlay.Decoders {
    /** 一個硬體影像解碼器能解的最大尺寸與是否支援 10-bit。 */
    data class VideoDecoder(val codec: String, val name: String, val maxWidth: Int, val maxHeight: Int, val tenBit: Boolean)

    private val VIDEO_MIME = mapOf(
        "h264" to "video/avc",
        "hevc" to "video/hevc",
        "vp9" to "video/x-vnd.on2.vp9",
        "av1" to "video/av01",
        "mpeg4" to "video/mp4v-es",
        "mpeg2video" to "video/mpeg2",
    )

    private val AUDIO_MIME = mapOf(
        "aac" to "audio/mp4a-latm",
        "mp3" to "audio/mpeg",
        "mp2" to "audio/mpeg-L2",
        "flac" to "audio/flac",
        "opus" to "audio/opus",
        "vorbis" to "audio/vorbis",
        "ac3" to "audio/ac3",
        "eac3" to "audio/eac3",
        "dts" to "audio/vnd.dts",
        "truehd" to "audio/true-hd",
        "alac" to "audio/alac",
    )

    private val decoders: List<MediaCodecInfo> by lazy {
        MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.filter { !it.isEncoder }
    }

    /** 各格式能力最好的硬體解碼器。 */
    val video: List<VideoDecoder> by lazy {
        VIDEO_MIME.mapNotNull { (codec, mime) ->
            hardware(mime).mapNotNull { info ->
                val caps = runCatching { info.getCapabilitiesForType(mime) }.getOrNull() ?: return@mapNotNull null
                val vc = caps.videoCapabilities ?: return@mapNotNull null
                VideoDecoder(codec, info.name, vc.supportedWidths.upper, vc.supportedHeights.upper, tenBit(codec, caps))
            }.maxByOrNull { it.maxWidth * it.maxHeight }
        }
    }

    private fun hardware(mime: String) = decoders.filter { info ->
        info.supportedTypes.any { it.equals(mime, ignoreCase = true) } && isHardware(info) && !info.name.endsWith(".secure")
    }

    private fun isHardware(info: MediaCodecInfo): Boolean {
        if (Build.VERSION.SDK_INT >= 29) return info.isHardwareAccelerated
        val n = info.name.lowercase()
        return !(n.startsWith("omx.google.") || n.startsWith("c2.android.") || n.contains(".sw.") || n.contains("ffmpeg"))
    }

    private fun tenBit(codec: String, caps: MediaCodecInfo.CodecCapabilities): Boolean {
        val profiles = when (codec) {
            "hevc" -> setOf(CodecProfileLevel.HEVCProfileMain10, CodecProfileLevel.HEVCProfileMain10HDR10)
            "h264" -> setOf(CodecProfileLevel.AVCProfileHigh10)
            "vp9" -> setOf(CodecProfileLevel.VP9Profile2, CodecProfileLevel.VP9Profile3)
            "av1" -> setOf(2) // AV1ProfileMain10（API 29 才有常數）
            else -> emptySet()
        }
        return caps.profileLevels.any { it.profile in profiles }
    }

    override fun video(codec: String, width: Int, height: Int, frameRate: Float, bitDepth: Int): Boolean {
        val c = codec.lowercase()
        val mime = VIDEO_MIME[c] ?: return false
        if (bitDepth > 8 && video.none { it.codec == c && it.tenBit }) return false
        return hardware(mime).any { info ->
            val vc = runCatching { info.getCapabilitiesForType(mime).videoCapabilities }.getOrNull() ?: return@any false
            // 有些片源寬高是奇數，解碼器要求對齊；以向上對齊到 2 的尺寸判斷
            val w = (width + 1) and 1.inv()
            val h = (height + 1) and 1.inv()
            if (frameRate > 0) vc.areSizeAndRateSupported(w, h, frameRate.toDouble()) else vc.isSizeSupported(w, h)
        }
    }

    override fun audio(codec: String): Boolean {
        val c = codec.lowercase()
        if (c.startsWith("pcm_")) return true // ExoPlayer 自己處理 PCM
        val mime = AUDIO_MIME[c] ?: return false
        return decoders.any { info -> info.supportedTypes.any { it.equals(mime, ignoreCase = true) } }
    }

    /** 最大可硬解的高度（轉碼的 H.264／HEVC 取較大者），畫質選單用來隱藏裝置解不了的選項。 */
    val maxTranscodeHeight: Int
        get() = video.filter { it.codec == "h264" || it.codec == "hevc" }.maxOfOrNull { minOf(it.maxWidth, it.maxHeight) } ?: 1080

    /** 送給轉碼伺服器的能力（server/internal/profile.Capabilities）；伺服器只編得出 H.264 與 HEVC。 */
    fun xcodeCapabilities(): JSONObject {
        val list = JSONArray()
        video.filter { it.codec == "h264" || it.codec == "hevc" }.forEach {
            list.put(JSONObject().put("codec", it.codec).put("maxWidth", it.maxWidth).put("maxHeight", it.maxHeight))
        }
        return JSONObject().put("decoders", list)
    }

    /** 設定頁顯示用，例如「HEVC 1920×1088 · H.264 1920×1088」。 */
    fun describe(): String = video.joinToString(" · ") {
        val depth = if (it.tenBit) " 10-bit" else ""
        "${it.codec.uppercase()} ${it.maxWidth}×${it.maxHeight}$depth"
    }.ifEmpty { "沒有偵測到硬體影像解碼器" }
}
