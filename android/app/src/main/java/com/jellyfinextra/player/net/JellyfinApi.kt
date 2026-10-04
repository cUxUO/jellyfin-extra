package com.jellyfinextra.player.net

import com.jellyfinextra.player.data.Settings
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONArray
import org.json.JSONObject

/** 播放程式用到的 Jellyfin API 子集（已對照 Jellyfin 12.1 的 OpenAPI）。 */
class JellyfinApi(
    private val http: OkHttpClient,
    private val settings: Settings,
    val base: HttpUrl,
) {
    data class Login(val token: String, val userId: String, val userName: String)

    data class Item(
        val id: String,
        val name: String,
        val type: String,
        val isFolder: Boolean,
        val seriesId: String?,
        val seriesName: String?,
        val indexNumber: Int?,
        val parentIndexNumber: Int?,
        val productionYear: Int?,
        val runTimeTicks: Long,
        val positionTicks: Long,
        val played: Boolean,
    ) {
        val playable: Boolean get() = !isFolder && type in PLAYABLE_TYPES

        companion object {
            private val PLAYABLE_TYPES = setOf("Movie", "Episode", "Video", "MusicVideo")

            /** 只帶瀏覽子項目需要的欄位，用於 Activity 之間傳遞。 */
            fun stub(id: String, type: String, name: String, seriesId: String?) = Item(
                id = id, name = name, type = type, isFolder = true, seriesId = seriesId, seriesName = null,
                indexNumber = null, parentIndexNumber = null, productionYear = null,
                runTimeTicks = 0, positionTicks = 0, played = false,
            )

            fun from(o: JSONObject): Item {
                val userData = o.optJSONObject("UserData")
                return Item(
                    id = o.getString("Id"),
                    name = o.optString("Name"),
                    type = o.optString("Type"),
                    isFolder = o.optBoolean("IsFolder"),
                    seriesId = o.optStringOrNull("SeriesId"),
                    seriesName = o.optStringOrNull("SeriesName"),
                    indexNumber = o.optIntOrNull("IndexNumber"),
                    parentIndexNumber = o.optIntOrNull("ParentIndexNumber"),
                    productionYear = o.optIntOrNull("ProductionYear"),
                    runTimeTicks = o.optLong("RunTimeTicks"),
                    positionTicks = userData?.optLong("PlaybackPositionTicks") ?: 0,
                    played = userData?.optBoolean("Played") ?: false,
                )
            }
        }
    }

    suspend fun authenticate(user: String, password: String): Login {
        val body = JSONObject().put("Username", user).put("Pw", password)
        val req = Request.Builder()
            .url(base.resolve("Users/AuthenticateByName")!!)
            .header("Authorization", Http.authHeader(settings, token = ""))
            .post(Http.jsonBody(body))
            .build()
        val o = JSONObject(http.fetch(req))
        val u = o.getJSONObject("User")
        return Login(o.getString("AccessToken"), u.getString("Id"), u.optString("Name"))
    }

    suspend fun userViews(): List<Item> =
        items(url("UserViews").addQueryParameter("userId", settings.userId))

    /** 依類型取子項目：影集列出季，季列出集，其他資料夾照名稱排序。 */
    suspend fun children(parent: Item): List<Item> = when (parent.type) {
        "Series" -> items(url("Shows/${parent.id}/Seasons").addQueryParameter("userId", settings.userId))
        "Season" -> items(
            url("Shows/${parent.seriesId ?: parent.id}/Episodes")
                .addQueryParameter("userId", settings.userId)
                .addQueryParameter("seasonId", parent.id)
        )
        else -> items(
            url("Items")
                .addQueryParameter("userId", settings.userId)
                .addQueryParameter("parentId", parent.id)
                .addQueryParameter("sortBy", "IsFolder,SortName")
                .addQueryParameter("sortOrder", "Ascending")
        )
    }

    suspend fun item(id: String): Item {
        val u = url("Items/$id").addQueryParameter("userId", settings.userId).build()
        return Item.from(JSONObject(http.fetch(get(u))))
    }

    /** 原始檔網址，轉碼伺服器離線時的退路；驗證靠請求標頭，網址裡不放 token。 */
    fun directStreamUrl(itemId: String): HttpUrl =
        url("Videos/$itemId/stream").addQueryParameter("static", "true").build()

    suspend fun reportStart(r: PlaybackReport) = post("Sessions/Playing", r.toJson(includeState = true))
    suspend fun reportProgress(r: PlaybackReport) = post("Sessions/Playing/Progress", r.toJson(includeState = true))
    suspend fun reportStopped(r: PlaybackReport) = post("Sessions/Playing/Stopped", r.toJson(includeState = false))

    data class PlaybackReport(
        val itemId: String,
        val playSessionId: String,
        val positionTicks: Long,
        val isPaused: Boolean,
        val playMethod: String, // Transcode 或 DirectPlay
    ) {
        fun toJson(includeState: Boolean): JSONObject {
            val o = JSONObject()
                .put("ItemId", itemId)
                .put("PlaySessionId", playSessionId)
                .put("PositionTicks", positionTicks)
            if (includeState) {
                o.put("IsPaused", isPaused).put("PlayMethod", playMethod).put("CanSeek", true)
            }
            return o
        }
    }

    private fun url(path: String): HttpUrl.Builder = base.resolve(path)!!.newBuilder()

    private fun get(url: HttpUrl) = Request.Builder().url(url).header("Authorization", Http.authHeader(settings)).build()

    private suspend fun items(url: HttpUrl.Builder): List<Item> {
        val arr: JSONArray = JSONObject(http.fetch(get(url.build()))).optJSONArray("Items") ?: JSONArray()
        return List(arr.length()) { Item.from(arr.getJSONObject(it)) }
    }

    private suspend fun post(path: String, body: JSONObject) {
        val req = Request.Builder()
            .url(base.resolve(path)!!)
            .header("Authorization", Http.authHeader(settings))
            .post(Http.jsonBody(body))
            .build()
        http.fetch(req)
    }
}

private fun JSONObject.optStringOrNull(key: String): String? =
    if (has(key) && !isNull(key)) getString(key) else null

private fun JSONObject.optIntOrNull(key: String): Int? =
    if (has(key) && !isNull(key)) getInt(key) else null
