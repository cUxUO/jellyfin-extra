package com.jellyfinextra.player.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class DirectPlayTest {
    /** ZenPad 實測：H.264／HEVC 8-bit 硬解到 1920×1088，音訊沒有 AC3／E-AC3。 */
    private val zenpad = object : DirectPlay.Decoders {
        override fun video(codec: String, width: Int, height: Int, frameRate: Float, bitDepth: Int) =
            codec in setOf("h264", "hevc") && bitDepth <= 8 && width <= 1920 && height <= 1088
        override fun audio(codec: String) = codec in setOf("aac", "mp3", "flac", "opus", "vorbis", "dts")
    }

    private val hevc1080 = SourceVideo("hevc", 1920, 1080, 8, 23.976f, "SDR")
    private val flac = listOf(AudioTrack(1, "FLAC", isDefault = true, codec = "flac"), AudioTrack(2, "AAC", isDefault = false, codec = "aac"))

    private fun check(
        container: String = "mkv", bitrate: Long = 8_000_000, video: SourceVideo? = hevc1080,
        audio: List<AudioTrack> = flac, audioIndex: Int? = null, subtitle: SubtitleTrack? = null,
    ) = DirectPlay.check(container, bitrate, video, audio, audioIndex, subtitle, zenpad)

    @Test
    fun playableSource() {
        assertNull(check())
        assertNull(check(container = "mov,mp4,m4a,3gp,3g2,mj2"))
        assertNull(check(audioIndex = 1)) // 選的就是預設音軌
        assertNull(check(subtitle = SubtitleTrack(3, "繁中", "chi", "subrip", false, false, false)))
    }

    @Test
    fun reasons() {
        assertEquals("容器 avi", check(container = "avi"))
        assertEquals("HDR", check(video = hevc1080.copy(range = "HDR")))
        assertEquals("HEVC 3840×2160 無硬解", check(video = hevc1080.copy(width = 3840, height = 2160)))
        assertEquals("HEVC 1920×1080 10-bit 無硬解", check(video = hevc1080.copy(bitDepth = 10)))
        assertEquals("位元率 60 Mbps 過高", check(bitrate = 60_000_000))
        assertEquals("非預設音軌", check(audioIndex = 2))
        assertEquals("音訊 EAC3 無解碼器", check(audio = listOf(AudioTrack(1, "DD+", isDefault = true, codec = "eac3"))))
        assertNotNull(check(subtitle = SubtitleTrack(4, "PGS", "chi", "PGSSUB", false, false, false)))
        assertNotNull(check(video = null))
    }

    @Test
    fun noAudioIsFine() {
        assertNull(check(audio = emptyList()))
    }

    @Test
    fun quality() {
        assertEquals(Quality.AUTO, Quality.of(null))
        assertEquals(Quality.P720, Quality.of("P720"))
        assertEquals(1280, Quality.P720.maxWidth)
    }
}
