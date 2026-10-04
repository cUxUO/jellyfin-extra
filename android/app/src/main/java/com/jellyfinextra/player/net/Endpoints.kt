package com.jellyfinextra.player.net

import com.jellyfinextra.player.App
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.Request

/**
 * 內網優先：先試內網直連，連不上才用外部網址。
 * 在家時不經 DNS、TLS 與反向代理；iPad 解析不到網域、Android 7 系統 TLS 不支援目前的憑證，都靠這個繞過。
 */
object Endpoints {
    suspend fun jellyfin(app: App, lan: String = app.settings.lanJellyfin, external: String = app.settings.external): HttpUrl? {
        val candidates = listOfNotNull(Http.baseUrl(lan), Http.baseUrl(external))
        return firstReachable(app, candidates, "System/Info/Public")?.also { app.jellyfinBase = it }
    }

    /** 轉碼伺服器；回傳 null 表示 Windows 沒開，播放時改走 Jellyfin 直接播放。 */
    suspend fun xcode(app: App): HttpUrl? {
        val external = Http.baseUrl(app.settings.external)?.resolve("xcode/")
        val candidates = listOfNotNull(Http.baseUrl(app.settings.lanXcode), external)
        return firstReachable(app, candidates, "healthz")
    }

    private suspend fun firstReachable(app: App, bases: List<HttpUrl>, probePath: String): HttpUrl? {
        val client = Http.probeClient(app.http)
        for (base in bases) {
            val url = base.resolve(probePath) ?: continue
            val ok = withContext(Dispatchers.IO) {
                runCatching {
                    client.newCall(Request.Builder().url(url).build()).execute().use { it.isSuccessful }
                }.getOrDefault(false)
            }
            if (ok) return base
        }
        return null
    }
}
