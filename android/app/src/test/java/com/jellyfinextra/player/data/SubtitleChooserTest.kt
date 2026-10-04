package com.jellyfinextra.player.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Locale

class SubtitleChooserTest {
    private val taiwan = Locale.TAIWAN
    private val china = Locale.CHINA

    private fun track(
        index: Int, title: String, lang: String? = null, codec: String = "subrip",
        external: Boolean = false, default: Boolean = false, forced: Boolean = false,
    ) = SubtitleTrack(index, title, lang, codec, external, default, forced)

    // 荒野機器人的實際字幕串流（節錄）
    private val wildRobot = listOf(
        track(4, "English - SUBRIP", "eng"),
        track(5, "SDH - English - SUBRIP", "eng"),
        track(6, "SDH / Positional / PGS - English - PGSSUB", "eng", codec = "PGSSUB"),
        track(7, "Traditional - Chinese - SUBRIP", "zho"),
        track(8, "Danish - SUBRIP", "dan"),
        track(9, "Dutch - SUBRIP", "nld"),
    )

    @Test
    fun picksTraditionalChineseFromDeviceLocale() {
        assertEquals(7, SubtitleChooser.choose(wildRobot, null, taiwan)?.index)
    }

    @Test
    fun jellyfinPreferenceWinsAndSdhIsAvoided() {
        assertEquals(4, SubtitleChooser.choose(wildRobot, "eng", taiwan)?.index)
    }

    @Test
    fun animeExternalTraditionalVsEmbeddedSimplified() {
        val tracks = listOf(
            track(0, "繁體中文 - 未定義 - ASS - 外部", codec = "ass", external = true),
            track(3, "简日双语 - Chinese - 預設 - ASS", "zho", codec = "ass", default = true),
        )
        assertEquals(0, SubtitleChooser.choose(tracks, null, taiwan)?.index)
        assertEquals(3, SubtitleChooser.choose(tracks, null, china)?.index)
    }

    @Test
    fun partialTracksLoseToFullOnes() {
        val tracks = listOf(
            track(2, "繁體中文 Signs & Songs - ASS", "zho", codec = "ass", default = true),
            track(3, "繁體中文 - ASS", "zho", codec = "ass"),
        )
        assertEquals(3, SubtitleChooser.choose(tracks, null, taiwan)?.index)
    }

    @Test
    fun imageSubtitlesOnlyWhenFlagged() {
        val pgsOnly = listOf(track(2, "Japanese - PGSSUB", "jpn", codec = "PGSSUB"))
        assertNull(SubtitleChooser.choose(pgsOnly, null, taiwan))
        val flagged = listOf(track(2, "Japanese - PGSSUB", "jpn", codec = "PGSSUB", default = true))
        assertEquals(2, SubtitleChooser.choose(flagged, null, taiwan)?.index)
    }

    @Test
    fun externalImageSubtitlesAreNeverChosen() {
        val tracks = listOf(track(0, "Chinese - PGSSUB - 外部", "zho", codec = "PGSSUB", external = true, default = true))
        assertNull(SubtitleChooser.choose(tracks, null, taiwan))
    }

    @Test
    fun dutchIsNotChinese() {
        // 「Dutch」裡有 tc，不能被當成繁中（tc）
        val tracks = listOf(track(9, "Dutch - SUBRIP", "nld"))
        assertNull(SubtitleChooser.choose(tracks, null, taiwan))
    }
}
