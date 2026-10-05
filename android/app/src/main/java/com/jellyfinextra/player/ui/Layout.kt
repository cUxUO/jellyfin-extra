package com.jellyfinextra.player.ui

import android.app.Activity
import android.content.Context
import android.content.pm.ActivityInfo

/**
 * 手機（最短邊 < 600dp）用直立版面，平板維持橫向。
 * 直立版面放在 layout-port／values-port，所以平板的畫面不受影響；播放畫面一律橫向。
 */
val Context.isPhone: Boolean get() = resources.configuration.smallestScreenWidthDp < 600

/** 在 setContentView 之前呼叫；方向不符時系統會旋轉並重建 Activity，載入對應方向的版面。 */
fun Activity.applyPageOrientation() {
    requestedOrientation = if (isPhone) ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT else ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
}
