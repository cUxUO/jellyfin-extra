package com.jellyfinextra.player.net

import com.jellyfinextra.player.data.Settings
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject

/** Windows 轉碼伺服器的 API，見 server/internal/api。 */
class XcodeApi(
    private val http: OkHttpClient,
    private val settings: Settings,
    private val base: HttpUrl,
) {
    data class Session(
        val id: String,
        val playlist: HttpUrl,
        /** 播放清單的 0 秒對應片中的這個位置。 */
        val startTimeTicks: Long,
        val runTimeTicks: Long,
        val width: Int,
        val height: Int,
    )

    suspend fun create(itemId: String, profile: String, startTimeTicks: Long): Session {
        val body = JSONObject()
            .put("itemId", itemId)
            .put("profile", profile)
            .put("startTimeTicks", startTimeTicks)
        val req = Request.Builder()
            .url(base.resolve("v1/sessions")!!)
            .header("Authorization", Http.authHeader(settings))
            .post(Http.jsonBody(body))
            .build()
        val o = try {
            JSONObject(http.fetch(req))
        } catch (e: HttpException) {
            throw HttpException(e.code, describe(e))
        }
        val video = o.optJSONObject("video") ?: JSONObject()
        return Session(
            id = o.getString("sessionId"),
            // 回傳的是相對路徑，接在基底網址後面；經反向代理掛在 /xcode/ 下也適用
            playlist = base.resolve(o.getString("playlist"))!!,
            startTimeTicks = o.optLong("startTimeTicks"),
            runTimeTicks = o.optLong("runTimeTicks"),
            width = video.optInt("width"),
            height = video.optInt("height"),
        )
    }

    suspend fun delete(sessionId: String) {
        val req = Request.Builder()
            .url(base.resolve("v1/sessions/$sessionId")!!)
            .delete()
            .build()
        http.fetch(req)
    }

    private fun describe(e: HttpException) = when (e.code) {
        401 -> "登入已過期，請重新登入"
        404 -> "找不到這個項目，或沒有權限"
        422 -> "轉碼伺服器不支援這個片源（${e.message}）"
        503 -> "轉碼伺服器忙碌中，請稍後再試"
        else -> "轉碼失敗（${e.message}）"
    }
}
