package com.jellyfinextra.player.net

import android.os.Build
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.data.Settings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.util.concurrent.TimeUnit

/** 伺服器回傳非 2xx。message 是給使用者看的說明，不含 token。 */
class HttpException(val code: Int, message: String) : IOException(message)

object Http {
    private val JSON = "application/json; charset=utf-8".toMediaType()

    fun client(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        // 轉碼伺服器建立 session 時要等第一批片段，最長約 30 秒
        .readTimeout(40, TimeUnit.SECONDS)
        .build()

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
