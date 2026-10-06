package com.jellyfinextra.player.ui

import org.junit.Assert.assertEquals
import org.junit.Test

class FormatTest {
    @Test
    fun resolutionLabelUsesTheBoxTheFrameFitsIn() {
        assertEquals("1080p", resolutionLabel(1920, 1080))
        // 寬銀幕片：放進 1080p 的框，高度只有 802
        assertEquals("1080p · 1920×802", resolutionLabel(1920, 802))
        assertEquals("720p · 1280×536", resolutionLabel(1280, 536))
        assertEquals("480p · 852×356", resolutionLabel(852, 356))
        assertEquals("360p · 640×268", resolutionLabel(640, 268))
        // 4:3 與編碼器對齊後的尺寸
        assertEquals("1080p · 1440×1080", resolutionLabel(1440, 1080))
        assertEquals("1080p · 1920×1088", resolutionLabel(1920, 1088))
        assertEquals("720p", resolutionLabel(0, 720))
    }
}
