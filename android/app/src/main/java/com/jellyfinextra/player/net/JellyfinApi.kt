package com.jellyfinextra.player.net

import com.jellyfinextra.player.data.AudioTrack
import com.jellyfinextra.player.data.Settings
import com.jellyfinextra.player.data.SubtitleTrack
import java.util.concurrent.TimeUnit
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

    /** 圖片：項目 ID 加上 tag（tag 變了代表圖換了，放進網址當快取鍵）。 */
    data class ImageRef(val itemId: String, val type: String, val tag: String?)

    data class Item(
        val id: String,
        val name: String,
        val type: String,
        val isFolder: Boolean,
        val collectionType: String? = null,
        val seriesId: String? = null,
        val seriesName: String? = null,
        val seasonId: String? = null,
        val indexNumber: Int? = null,
        val parentIndexNumber: Int? = null,
        val productionYear: Int? = null,
        val runTimeTicks: Long = 0,
        val positionTicks: Long = 0,
        val played: Boolean = false,
        val unplayedCount: Int = 0,
        val childCount: Int = 0,
        val overview: String? = null,
        val originalTitle: String? = null,
        val genres: List<String> = emptyList(),
        val communityRating: Double? = null,
        val officialRating: String? = null,
        /** 直式海報（2:3）；集數用影集的海報。 */
        val poster: ImageRef? = null,
        /** 橫式縮圖（16:9）：集數是劇照，電影／影集是 Thumb，沒有就用背景圖。 */
        val wide: ImageRef? = null,
        /** 背景圖；集數用所屬影集的。 */
        val backdrop: ImageRef? = null,
    ) {
        val playable: Boolean get() = !isFolder && type in PLAYABLE_TYPES
        val resumable: Boolean get() = positionTicks > 0 && !played

        companion object {
            private val PLAYABLE_TYPES = setOf("Movie", "Episode", "Video", "MusicVideo")

            /** 只帶開啟頁面需要的欄位，用於 Fragment 參數。 */
            fun stub(id: String, type: String, name: String, seriesId: String?) = Item(
                id = id, name = name, type = type, isFolder = type != "Movie" && type != "Episode", seriesId = seriesId,
            )

            fun from(o: JSONObject): Item {
                val id = o.getString("Id")
                val type = o.optString("Type")
                val userData = o.optJSONObject("UserData")
                val tags = o.optJSONObject("ImageTags")
                val backdropTags = o.optJSONArray("BackdropImageTags")
                val ownPrimary = tags?.optStringOrNull("Primary")?.let { ImageRef(id, "Primary", it) }
                val ownThumb = tags?.optStringOrNull("Thumb")?.let { ImageRef(id, "Thumb", it) }
                val ownBackdrop = backdropTags?.takeIf { it.length() > 0 }?.let { ImageRef(id, "Backdrop", it.getString(0)) }
                val parentBackdrop = o.optStringOrNull("ParentBackdropItemId")?.let { pid ->
                    ImageRef(pid, "Backdrop", o.optJSONArray("ParentBackdropImageTags")?.optString(0))
                }
                val seriesPoster = o.optStringOrNull("SeriesId")?.let { sid ->
                    o.optStringOrNull("SeriesPrimaryImageTag")?.let { ImageRef(sid, "Primary", it) }
                }
                val isEpisode = type == "Episode"
                return Item(
                    id = id,
                    name = o.optString("Name"),
                    type = type,
                    isFolder = o.optBoolean("IsFolder"),
                    collectionType = o.optStringOrNull("CollectionType"),
                    seriesId = o.optStringOrNull("SeriesId"),
                    seriesName = o.optStringOrNull("SeriesName"),
                    seasonId = o.optStringOrNull("SeasonId"),
                    indexNumber = o.optIntOrNull("IndexNumber"),
                    parentIndexNumber = o.optIntOrNull("ParentIndexNumber"),
                    productionYear = o.optIntOrNull("ProductionYear"),
                    runTimeTicks = o.optLong("RunTimeTicks"),
                    positionTicks = userData?.optLong("PlaybackPositionTicks") ?: 0,
                    played = userData?.optBoolean("Played") ?: false,
                    unplayedCount = userData?.optInt("UnplayedItemCount") ?: 0,
                    childCount = o.optInt("ChildCount"),
                    overview = o.optStringOrNull("Overview"),
                    originalTitle = o.optStringOrNull("OriginalTitle"),
                    genres = o.optJSONArray("Genres")?.let { a -> List(a.length()) { a.getString(it) } } ?: emptyList(),
                    communityRating = if (o.has("CommunityRating") && !o.isNull("CommunityRating")) o.getDouble("CommunityRating") else null,
                    officialRating = o.optStringOrNull("OfficialRating"),
                    poster = if (isEpisode) seriesPoster ?: ownPrimary else ownPrimary,
                    wide = if (isEpisode) ownPrimary ?: parentBackdrop else ownThumb ?: ownBackdrop ?: ownPrimary,
                    backdrop = ownBackdrop ?: parentBackdrop,
                )
            }
        }
    }

    /** 每個畫面需要的欄位；沒列的欄位 Jellyfin 不一定會回傳。 */
    private val listFields = "PrimaryImageAspectRatio,Overview,Genres,CommunityRating,OfficialRating,ChildCount"

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

    // ---- 首頁 ----

    suspend fun resume(limit: Int = 12): List<Item> = items(
        url("UserItems/Resume")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("limit", limit.toString())
            .addQueryParameter("mediaTypes", "Video")
            .addQueryParameter("fields", listFields)
    )

    suspend fun nextUp(seriesId: String? = null, limit: Int = 12): List<Item> = items(
        url("Shows/NextUp")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("limit", limit.toString())
            .addQueryParameter("fields", listFields)
            .apply { if (seriesId != null) addQueryParameter("seriesId", seriesId) }
    )

    /** 某個媒體庫最新加入的項目（影集會合併成影集本身）。 */
    suspend fun latest(parentId: String, limit: Int = 16): List<Item> {
        val u = url("Items/Latest")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("parentId", parentId)
            .addQueryParameter("limit", limit.toString())
            .addQueryParameter("fields", listFields)
            .build()
        val arr = JSONArray(http.fetch(get(u)))
        return List(arr.length()) { Item.from(arr.getJSONObject(it)) }
    }

    // ---- 媒體庫 ----

    enum class Sort(val sortBy: String, val order: String) {
        Added("DateCreated,SortName", "Descending"),
        Name("SortName", "Ascending"),
        Year("ProductionYear,SortName", "Descending"),
        Rating("CommunityRating,SortName", "Descending"),
    }

    data class Page(val items: List<Item>, val total: Int)

    /** 媒體庫內容：電影庫列電影、節目庫列影集，其他（例如播放清單庫）列直接的子項目。 */
    suspend fun library(view: Item, sort: Sort, genre: String?): Page {
        val types = when (view.collectionType) {
            "movies" -> "Movie"
            "tvshows" -> "Series"
            else -> null
        }
        val u = url("Items")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("parentId", view.id)
            .addQueryParameter("sortBy", sort.sortBy)
            .addQueryParameter("sortOrder", sort.order)
            .addQueryParameter("fields", listFields)
            .apply {
                if (types != null) addQueryParameter("recursive", "true").addQueryParameter("includeItemTypes", types)
                if (genre != null) addQueryParameter("genres", genre)
            }
            .build()
        val o = JSONObject(http.fetch(get(u)))
        val arr = o.optJSONArray("Items") ?: JSONArray()
        return Page(List(arr.length()) { Item.from(arr.getJSONObject(it)) }, o.optInt("TotalRecordCount", arr.length()))
    }

    suspend fun genres(parentId: String): List<String> {
        val u = url("Genres")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("parentId", parentId)
            .addQueryParameter("sortBy", "SortName")
            .build()
        val arr = JSONObject(http.fetch(get(u))).optJSONArray("Items") ?: JSONArray()
        return List(arr.length()) { arr.getJSONObject(it).optString("Name") }
    }

    suspend fun search(term: String): List<Item> = items(
        url("Items")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("searchTerm", term)
            .addQueryParameter("recursive", "true")
            .addQueryParameter("includeItemTypes", "Movie,Series,Episode")
            .addQueryParameter("limit", "60")
            .addQueryParameter("fields", listFields)
    )

    /** 依類型取子項目：影集列出季，季列出集，播放清單列出內容，其他資料夾照名稱排序。 */
    suspend fun children(parent: Item): List<Item> = when (parent.type) {
        "Series" -> seasons(parent.id)
        "Season" -> episodes(parent.seriesId ?: parent.id, parent.id)
        "Playlist" -> items(
            url("Playlists/${parent.id}/Items")
                .addQueryParameter("userId", settings.userId)
                .addQueryParameter("fields", listFields)
        )
        else -> items(
            url("Items")
                .addQueryParameter("userId", settings.userId)
                .addQueryParameter("parentId", parent.id)
                .addQueryParameter("sortBy", "IsFolder,SortName")
                .addQueryParameter("sortOrder", "Ascending")
                .addQueryParameter("fields", listFields)
        )
    }

    suspend fun seasons(seriesId: String): List<Item> = items(
        url("Shows/$seriesId/Seasons")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("fields", listFields)
    )

    suspend fun episodes(seriesId: String, seasonId: String): List<Item> = items(
        url("Shows/$seriesId/Episodes")
            .addQueryParameter("userId", settings.userId)
            .addQueryParameter("seasonId", seasonId)
            .addQueryParameter("fields", listFields)
    )

    // ---- 詳情 ----

    suspend fun item(id: String): Item {
        val u = url("Items/$id").addQueryParameter("userId", settings.userId).build()
        return Item.from(JSONObject(http.fetch(get(u))))
    }

    suspend fun setPlayed(itemId: String, played: Boolean) {
        val req = Request.Builder()
            .url(url("UserPlayedItems/$itemId").addQueryParameter("userId", settings.userId).build())
            .header("Authorization", Http.authHeader(settings))
            .apply { if (played) post(Http.jsonBody(JSONObject())) else delete() }
            .build()
        http.fetch(req)
    }

    data class MediaInfo(
        val mediaSourceId: String,
        val audio: List<AudioTrack>,
        val subtitles: List<SubtitleTrack>,
        val videoDescription: String?,
    )

    suspend fun mediaInfo(itemId: String): MediaInfo {
        val u = url("Items/$itemId/PlaybackInfo").addQueryParameter("userId", settings.userId).build()
        val src = JSONObject(http.fetch(get(u))).getJSONArray("MediaSources").getJSONObject(0)
        val streams = src.optJSONArray("MediaStreams") ?: JSONArray()
        val all = (0 until streams.length()).map { streams.getJSONObject(it) }
        val subs = all.filter { it.optString("Type") == "Subtitle" }.map {
            SubtitleTrack(
                index = it.getInt("Index"),
                title = it.optStringOrNull("DisplayTitle") ?: it.optString("Codec"),
                language = it.optStringOrNull("Language"),
                codec = it.optString("Codec"),
                isExternal = it.optBoolean("IsExternal"),
                isDefault = it.optBoolean("IsDefault"),
                isForced = it.optBoolean("IsForced"),
            )
        }
        val audio = all.filter { it.optString("Type") == "Audio" }.map {
            AudioTrack(
                index = it.getInt("Index"),
                title = it.optStringOrNull("DisplayTitle") ?: it.optString("Codec"),
                isDefault = it.optBoolean("IsDefault"),
            )
        }
        val video = all.firstOrNull { it.optString("Type") == "Video" }
        return MediaInfo(src.getString("Id"), audio, subs, video?.let { describeVideo(it) })
    }

    /** 片源畫質的簡短描述，例如「4K Dolby Vision」「1080p HEVC 10-bit」。 */
    private fun describeVideo(v: JSONObject): String {
        val h = v.optInt("Height")
        val w = v.optInt("Width")
        val res = when {
            w >= 3200 || h >= 2000 -> "4K"
            h >= 1000 || w >= 1800 -> "1080p"
            h >= 700 -> "720p"
            h > 0 -> "${h}p"
            else -> ""
        }
        val range = when (val t = v.optString("VideoRangeType")) {
            "SDR", "", "Unknown" -> null
            "DOVI", "DOVIWithEL", "DOVIWithHDR10", "DOVIWithHLG", "DOVIWithSDR", "DOVIWithELHDR10Plus", "DOVIWithHDR10Plus" -> "Dolby Vision"
            else -> t
        }
        val codec = v.optString("Codec").uppercase().takeIf { range == null && it.isNotEmpty() }
        val depth = v.optInt("BitDepth").takeIf { range == null && it > 8 }?.let { "$it-bit" }
        return listOfNotNull(res.takeIf { it.isNotEmpty() }, range, codec, depth).joinToString(" ")
    }

    /** 使用者在 Jellyfin 設定的字幕語言偏好（ISO 639-2，例如 chi），沒設定時回傳 null。 */
    suspend fun subtitleLanguagePreference(): String? {
        val o = JSONObject(http.fetch(get(url("Users/Me").build())))
        return o.optJSONObject("Configuration")?.optStringOrNull("SubtitleLanguagePreference")?.takeIf { it.isNotBlank() }
    }

    /**
     * 文字字幕轉成 WebVTT。外掛字幕約 0.1 秒；內嵌字幕要讓 Jellyfin 讀完整個檔案抽出來
     * （1GB 約 10 秒，60GB 的藍光原盤約 8 分鐘），之後 Jellyfin 會快取。
     * 不設讀取逾時：中途放棄的請求會讓 Jellyfin 停掉抽取，下次又得從頭讀。
     */
    suspend fun subtitleVtt(itemId: String, mediaSourceId: String, index: Int): ByteArray {
        val u = url("Videos/$itemId/$mediaSourceId/Subtitles/$index/0/Stream.vtt").build()
        val slow = http.newBuilder().readTimeout(0, TimeUnit.MILLISECONDS).build()
        return slow.fetch(get(u)).toByteArray()
    }

    /** 圖片網址：依顯示寬度向 Jellyfin 要縮好的圖，舊平板不必解碼原始大圖。圖片 API 不需要驗證。 */
    fun imageUrl(ref: ImageRef, maxWidth: Int): String =
        url("Items/${ref.itemId}/Images/${ref.type}")
            .addQueryParameter("maxWidth", maxWidth.toString())
            .addQueryParameter("quality", "85")
            .apply { if (ref.tag != null) addQueryParameter("tag", ref.tag) }
            .build()
            .toString()

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
