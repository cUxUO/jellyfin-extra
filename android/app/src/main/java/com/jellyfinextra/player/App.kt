package com.jellyfinextra.player

import android.app.Application
import android.content.Context
import com.jellyfinextra.player.data.Settings
import com.jellyfinextra.player.net.Http
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import okhttp3.HttpUrl
import okhttp3.OkHttpClient

class App : Application() {
    lateinit var settings: Settings
        private set

    val http: OkHttpClient by lazy { Http.client() }

    /** 不隨 Activity 結束而取消的工作，例如離開播放畫面後回報停止、刪除轉碼 session。 */
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    /** 目前選用的 Jellyfin 位址（內網優先），由 [com.jellyfinextra.player.net.Endpoints] 決定。 */
    @Volatile
    var jellyfinBase: HttpUrl? = null

    /** 離開播放畫面後的收尾（回報停止、刪除 session）；列表重新載入前先等它完成，進度才是最新的。 */
    @Volatile
    var playbackCleanup: Job? = null

    override fun onCreate() {
        super.onCreate()
        settings = Settings(this)
    }
}

val Context.app: App get() = applicationContext as App
