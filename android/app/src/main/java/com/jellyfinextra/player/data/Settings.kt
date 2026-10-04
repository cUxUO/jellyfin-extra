package com.jellyfinextra.player.data

import android.content.Context
import android.os.Build
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

    /** 送給轉碼伺服器的裝置規格名稱，對應 server/internal/profile。 */
    var profile: String
        get() = prefs.getString(KEY_PROFILE, null) ?: defaultProfile()
        set(v) = prefs.edit().putString(KEY_PROFILE, v).apply()

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
        val PROFILES = listOf("zenpad10", "ipad-air1")

        private const val KEY_LAN_JELLYFIN = "lan_jellyfin"
        private const val KEY_LAN_XCODE = "lan_xcode"
        private const val KEY_EXTERNAL = "external"
        private const val KEY_PROFILE = "profile"
        private const val KEY_TOKEN = "token"
        private const val KEY_USER_ID = "user_id"
        private const val KEY_USER_NAME = "user_name"
        private const val KEY_DEVICE_ID = "device_id"

        // 目前只有 ZenPad 10（P028）一台 Android 裝置
        private fun defaultProfile() = if (Build.MODEL == "P028") "zenpad10" else PROFILES.first()
    }
}
