package com.jellyfinextra.player.net

import android.os.Build
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.data.Settings
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.conscrypt.Conscrypt
import org.json.JSONObject
import java.io.IOException
import java.security.KeyStore
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager

/** 伺服器回傳非 2xx。message 是給使用者看的說明，不含 token。 */
class HttpException(val code: Int, message: String) : IOException(message)

/**
 * 同 runCatching，但不攔下協程取消。畫面關掉或重建（手機啟動時從橫向轉成直立）時請求被取消，
 * runCatching 會把它當成失敗，顯示「Job was cancelled」或在已卸離的 Fragment 上操作畫面。
 */
inline fun <T> catching(block: () -> T): Result<T> = try {
    Result.success(block())
} catch (e: CancellationException) {
    throw e
} catch (e: Exception) {
    Result.failure(e)
}

object Http {
    private val JSON = "application/json; charset=utf-8".toMediaType()

    fun client(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        // 轉碼伺服器建立 session 時要等第一批片段，最長約 30 秒
        .readTimeout(40, TimeUnit.SECONDS)
        .useConscrypt()
        .build()

    /**
     * TLS 改用 app 內建的 Conscrypt（BoringSSL）：這台 Android 7.0 的系統 TLS 只有 TLS 1.0–1.2、
     * 曲線只有 P-256，對目前的 ECDSA P-384 憑證必定握手失敗（alert 40）。
     * 憑證信任仍用系統的 TrustManager，所以 network_security_config（內建的 ISRG 根憑證）照樣生效。
     * Jellyfin、轉碼伺服器、ExoPlayer 與 Coil 都共用這個 client。
     */
    private fun OkHttpClient.Builder.useConscrypt(): OkHttpClient.Builder {
        val tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        tmf.init(null as KeyStore?)
        val trust = tmf.trustManagers.filterIsInstance<X509TrustManager>().first()
        val tls = SSLContext.getInstance("TLS", Conscrypt.newProvider())
        tls.init(null, arrayOf(trust), null)
        return sslSocketFactory(tls.socketFactory, trust)
    }

    /** 探測位址用：連不上就快速放棄，改試下一個位址。 */
    fun probeClient(base: OkHttpClient): OkHttpClient = base.newBuilder()
        .connectTimeout(1500, TimeUnit.MILLISECONDS)
        .readTimeout(2, TimeUnit.SECONDS)
        .callTimeout(3, TimeUnit.SECONDS)
        .build()

    /** Jellyfin 的驗證標頭；轉碼伺服器也讀同一個標頭裡的 Token。 */
    fun authHeader(settings: Settings, token: String = settings.token): String {
        val device = Build.MODEL.replace("\"", "")
        val head = "MediaBrowser Client=\"jellyfin-extra\", Device=\"$device\", " +
            "DeviceId=\"${settings.deviceId}\", Version=\"${BuildConfig.VERSION_NAME}\""
        return if (token.isEmpty()) head else "$head, Token=\"$token\""
    }

    fun jsonBody(o: JSONObject) = o.toString().toRequestBody(JSON)

    /** 把使用者輸入的位址正規化成以 / 結尾的基底網址，讓相對路徑能正確接上。 */
    fun baseUrl(s: String): HttpUrl? {
        val t = s.trim()
        if (t.isEmpty()) return null
        return (if (t.endsWith("/")) t else "$t/").toHttpUrlOrNull()
    }
}

/** 在 IO 執行緒送出請求，回傳 body 字串；非 2xx 丟 [HttpException]。 */
suspend fun OkHttpClient.fetch(request: Request): String = withContext(Dispatchers.IO) {
    newCall(request).execute().use { resp ->
        val body = resp.body.string()
        if (!resp.isSuccessful) {
            val detail = runCatching { JSONObject(body).optString("error") }.getOrNull().orEmpty()
            throw HttpException(resp.code, "HTTP ${resp.code}" + if (detail.isNotEmpty()) "：$detail" else "")
        }
        body
    }
}
