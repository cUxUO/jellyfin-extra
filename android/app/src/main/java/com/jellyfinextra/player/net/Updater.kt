package com.jellyfinextra.player.net

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.content.FileProvider
import androidx.core.content.pm.PackageInfoCompat
import com.jellyfinextra.player.BuildConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.IOException
import java.security.MessageDigest

/**
 * 自動更新：查公開 repo（BuildConfig.UPDATE_REPO）的 Release，
 * 有新版就下載符合這個安裝版本 ABI 的 APK，再交給系統安裝程式（Android 不允許 app 自己無聲安裝）。
 * 發布方式見 tools/release-android.sh。
 */
class Updater(private val context: Context, private val http: OkHttpClient) {
    data class Release(
        val version: String,
        val notes: String,
        val apk: HttpUrl,
        val apkName: String,
        val size: Long,
        /** GitHub 提供的 SHA-256（asset 的 digest 欄位），沒有時不驗證。 */
        val sha256: String?,
    )

    /** 這個安裝版本的 ABI：versionCode 的個位數（見 app/build.gradle.kts 的 abiCodes）。 */
    val abi: String?
        get() {
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            return ABI_CODES[(PackageInfoCompat.getLongVersionCode(info) % 10).toInt()]
        }

    /**
     * 最近幾個 Release 中最新的、有這個 ABI 的 APK 者；比目前版本新才回傳。
     * 不只看 latest：同一個 repo 之後可能也發布伺服器等其他東西，那些 Release 沒有 APK。
     */
    suspend fun newerRelease(): Release? {
        val req = Request.Builder()
            .url("https://api.github.com/repos/${BuildConfig.UPDATE_REPO}/releases?per_page=20")
            .header("Accept", "application/vnd.github+json")
            .build()
        val releases = JSONArray(http.fetch(req))
        val abi = abi ?: return null
        var best: Release? = null
        for (i in 0 until releases.length()) {
            val o = releases.getJSONObject(i)
            if (o.optBoolean("draft") || o.optBoolean("prerelease")) continue
            val r = apkRelease(o, abi) ?: continue
            if (best == null || isNewer(r.version, best.version)) best = r
        }
        return best?.takeIf { isNewer(it.version, BuildConfig.VERSION_NAME) }
    }

    private fun apkRelease(o: JSONObject, abi: String): Release? {
        val version = o.getString("tag_name").removePrefix("v")
        val assets = o.optJSONArray("assets") ?: return null
        for (i in 0 until assets.length()) {
            val a = assets.getJSONObject(i)
            val name = a.getString("name")
            if (!name.startsWith("JellyfinExtra-") || !name.endsWith("-$abi.apk")) continue
            return Release(
                version = version,
                notes = o.optString("body").trim(),
                apk = a.getString("browser_download_url").toHttpUrl(),
                apkName = name,
                size = a.optLong("size"),
                sha256 = a.optString("digest").takeIf { it.startsWith("sha256:") }?.removePrefix("sha256:"),
            )
        }
        return null
    }

    /** 下載到 app 的快取目錄（已下載且校驗相符就沿用），回傳 APK 檔。 */
    suspend fun download(r: Release, onProgress: (Int) -> Unit = {}): File = withContext(Dispatchers.IO) {
        val dir = File(context.cacheDir, "updates").apply { mkdirs() }
        dir.listFiles()?.filter { it.name != r.apkName }?.forEach { it.delete() } // 舊版的安裝檔
        val file = File(dir, r.apkName)
        if (file.exists() && file.length() == r.size && (r.sha256 == null || sha256(file) == r.sha256)) return@withContext file

        val tmp = File(dir, "${r.apkName}.part")
        http.newCall(Request.Builder().url(r.apk).build()).execute().use { resp ->
            if (!resp.isSuccessful) throw HttpException(resp.code, "下載失敗：HTTP ${resp.code}")
            val total = resp.body.contentLength().takeIf { it > 0 } ?: r.size
            resp.body.byteStream().use { input ->
                tmp.outputStream().use { out ->
                    val buf = ByteArray(64 * 1024)
                    var done = 0L
                    var last = -1
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        out.write(buf, 0, n)
                        done += n
                        val pct = if (total > 0) (done * 100 / total).toInt() else 0
                        if (pct != last) {
                            last = pct
                            onProgress(pct)
                        }
                    }
                }
            }
        }
        if (r.sha256 != null && sha256(tmp) != r.sha256) {
            tmp.delete()
            throw IOException("下載的檔案校驗不符，已刪除")
        }
        if (!tmp.renameTo(file)) throw IOException("無法儲存安裝檔")
        file
    }

    /**
     * 開啟系統安裝程式。Android 8 以上第一次要使用者允許這個 app「安裝不明應用程式」，
     * 沒允許時先帶到那個設定頁，回來後再安裝一次。
     */
    fun install(file: File): Boolean {
        if (Build.VERSION.SDK_INT >= 26 && !context.packageManager.canRequestPackageInstalls()) {
            context.startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
            return false
        }
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.updates", file)
        context.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        )
        return true
    }

    private fun sha256(f: File): String {
        val md = MessageDigest.getInstance("SHA-256")
        f.inputStream().use { input ->
            val buf = ByteArray(64 * 1024)
            while (true) {
                val n = input.read(buf)
                if (n < 0) break
                md.update(buf, 0, n)
            }
        }
        return md.digest().joinToString("") { "%02x".format(it) }
    }

    companion object {
        private val ABI_CODES = mapOf(1 to "armeabi-v7a", 2 to "arm64-v8a")

        /** 版本號逐段比較（0.2.10 比 0.2.9 新），忽略 -debug 之類的後綴。 */
        fun isNewer(latest: String, current: String): Boolean {
            fun parts(v: String) = v.removePrefix("v").substringBefore('-').split('.').map { it.toIntOrNull() ?: 0 }
            val a = parts(latest)
            val b = parts(current)
            for (i in 0 until maxOf(a.size, b.size)) {
                val x = a.getOrElse(i) { 0 }
                val y = b.getOrElse(i) { 0 }
                if (x != y) return x > y
            }
            return false
        }
    }
}
