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
        /** 整部片的 VOD 清單（0 秒就是片頭），可以拖曳到任何位置；片段在請求時才轉出。 */
        val playlist: HttpUrl,
        /** 建立 session 時要求的起點，伺服器已先轉好這個位置；播放器要自己跳過去。 */
        val startTimeTicks: Long,
        val runTimeTicks: Long,
        val width: Int,
        val height: Int,
        /** 燒進畫面的字幕（Jellyfin Index），-1 表示沒有。 */
        val burnedSubtitle: Int,
        /** 實際轉出的音軌（Jellyfin Index），-1 表示沒有音軌。 */
        val audioIndex: Int,
        /** GPU（NVDEC）解碼；false 表示片源格式要用 CPU 解碼。 */
        val hwDecode: Boolean,
    )

    /**
     * [burnSubtitle] 是要燒進畫面的圖形字幕 Index；文字字幕不經轉碼伺服器。
     * [audioIndex] 是音軌 Index，null 表示由伺服器挑預設音軌。
     */
    suspend fun create(
        itemId: String,
        profile: String,
        startTimeTicks: Long,
        burnSubtitle: Int? = null,
        audioIndex: Int? = null,
    ): Session {
        val body = JSONObject()
            .put("itemId", itemId)
            .put("profile", profile)
            .put("startTimeTicks", startTimeTicks)
        if (burnSubtitle != null) body.put("subtitleStreamIndex", burnSubtitle)
        if (audioIndex != null) body.put("audioStreamIndex", audioIndex)
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
            burnedSubtitle = o.optInt("subtitleStreamIndex", -1),
            audioIndex = o.optInt("audioStreamIndex", -1),
            hwDecode = video.optBoolean("hwDecode", true),
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
