package com.jellyfinextra.player

import android.app.Application
import android.content.Context
import androidx.fragment.app.Fragment
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.SingletonImageLoader
import coil3.disk.DiskCache
import coil3.disk.directory
import coil3.memory.MemoryCache
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import coil3.request.crossfade
import com.jellyfinextra.player.data.Settings
import com.jellyfinextra.player.net.Http
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import okhttp3.HttpUrl
import okhttp3.OkHttpClient

class App : Application(), SingletonImageLoader.Factory {
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

    /**
     * 圖片共用同一個 OkHttp。這台平板的 app 只有 128MB heap，記憶體快取壓在 20%，
     * 磁碟快取讓重開 app 時海報不必重抓。
     */
    override fun newImageLoader(context: PlatformContext): ImageLoader = ImageLoader.Builder(context)
        .components { add(OkHttpNetworkFetcherFactory(callFactory = { http })) }
        .memoryCache { MemoryCache.Builder().maxSizePercent(context, 0.2).build() }
        .diskCache { DiskCache.Builder().directory(cacheDir.resolve("images")).maxSizeBytes(150L * 1024 * 1024).build() }
        .crossfade(150)
        .build()
}

val Context.app: App get() = applicationContext as App

val Fragment.app: App get() = requireContext().app
