package com.jellyfinextra.player.data

import android.content.Context
import java.util.UUID

/**
 * 使用者設定與登入狀態，存在 app 私有的 SharedPreferences。
 * 位址由使用者輸入，不寫死在程式碼。
 */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    var lanJellyfin: String
        get() = prefs.getString(KEY_LAN_JELLYFIN, "")!!
        set(v) = prefs.edit().putString(KEY_LAN_JELLYFIN, v.trim()).apply()

    var lanXcode: String
        get() = prefs.getString(KEY_LAN_XCODE, "")!!
        set(v) = prefs.edit().putString(KEY_LAN_XCODE, v.trim()).apply()

    /** 外部網址，例如 https://example.com；Jellyfin 在根路徑，轉碼伺服器在 /xcode/。 */
    var external: String
        get() = prefs.getString(KEY_EXTERNAL, "")!!
        set(v) = prefs.edit().putString(KEY_EXTERNAL, v.trim()).apply()

    /** 預設畫質；播放中可在畫質面板暫時切換。 */
    var quality: Quality
        get() = Quality.of(prefs.getString(KEY_QUALITY, null))
        set(v) = prefs.edit().putString(KEY_QUALITY, v.name).apply()

    /** 文字字幕大小；播放中也可在字幕面板調整，兩邊共用同一個值。 */
    var subtitleSize: SubtitleSize
        get() = SubtitleSize.of(prefs.getString(KEY_SUBTITLE_SIZE, null))
        set(v) = prefs.edit().putString(KEY_SUBTITLE_SIZE, v.name).apply()

    /** 上次檢查更新的時間（毫秒），自動檢查每天最多一次。 */
    var lastUpdateCheck: Long
        get() = prefs.getLong(KEY_LAST_UPDATE_CHECK, 0)
        set(v) = prefs.edit().putLong(KEY_LAST_UPDATE_CHECK, v).apply()

    val token: String get() = prefs.getString(KEY_TOKEN, "")!!
    val userId: String get() = prefs.getString(KEY_USER_ID, "")!!
    val userName: String get() = prefs.getString(KEY_USER_NAME, "")!!
    val loggedIn: Boolean get() = token.isNotEmpty() && userId.isNotEmpty()

    /** Jellyfin 用 DeviceId 區分裝置與播放 session，安裝後固定不變。 */
    val deviceId: String
        get() = prefs.getString(KEY_DEVICE_ID, null)
            ?: UUID.randomUUID().toString().also { prefs.edit().putString(KEY_DEVICE_ID, it).apply() }

    fun saveLogin(token: String, userId: String, userName: String) {
        prefs.edit()
            .putString(KEY_TOKEN, token)
            .putString(KEY_USER_ID, userId)
            .putString(KEY_USER_NAME, userName)
            .apply()
    }

    fun logout() {
        prefs.edit().remove(KEY_TOKEN).remove(KEY_USER_ID).remove(KEY_USER_NAME).apply()
    }

    companion object {
        private const val KEY_LAN_JELLYFIN = "lan_jellyfin"
        private const val KEY_LAN_XCODE = "lan_xcode"
        private const val KEY_EXTERNAL = "external"
        private const val KEY_QUALITY = "quality"
        private const val KEY_SUBTITLE_SIZE = "subtitle_size"
        private const val KEY_LAST_UPDATE_CHECK = "last_update_check"
        private const val KEY_TOKEN = "token"
        private const val KEY_USER_ID = "user_id"
        private const val KEY_USER_NAME = "user_name"
        private const val KEY_DEVICE_ID = "device_id"
    }
}
