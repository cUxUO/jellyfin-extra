package com.jellyfinextra.player.net

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class UpdaterTest {
    @Test
    fun versionCompare() {
        assertTrue(Updater.isNewer("0.2.1", "0.2.0"))
        assertTrue(Updater.isNewer("v0.2.10", "0.2.9"))
        assertTrue(Updater.isNewer("1.0", "0.9.9"))
        assertFalse(Updater.isNewer("0.2.0", "0.2.0"))
        assertFalse(Updater.isNewer("0.2.0", "0.2.0-debug"))
        assertFalse(Updater.isNewer("0.1.9", "0.2.0"))
    }
}
