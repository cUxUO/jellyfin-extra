package com.jellyfinextra.player.ui

import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.Updater
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/**
 * 檢查更新：有新版就自動下載，下載完詢問是否安裝。
 * 自動檢查只在 release 版、開啟 app 時，每天最多一次；設定頁可以手動檢查。
 */
object UpdatePrompt {
    private const val DAY_MS = 24 * 60 * 60 * 1000L
    private var job: Job? = null

    fun autoCheck(activity: AppCompatActivity) {
        if (BuildConfig.DEBUG) return
        if (System.currentTimeMillis() - activity.app.settings.lastUpdateCheck < DAY_MS) return
        check(activity, manual = false)
    }

    /** [onStatus] 顯示進度（手動檢查時在設定頁）。 */
    fun check(activity: AppCompatActivity, manual: Boolean, onStatus: (String) -> Unit = {}) {
        if (job?.isActive == true) return
        val app = activity.app
        val updater = Updater(app, app.http)
        job = activity.lifecycleScope.launch {
            try {
                if (manual) onStatus("檢查中…")
                val r = updater.newerRelease()
                app.settings.lastUpdateCheck = System.currentTimeMillis()
                if (r == null) {
                    if (manual) onStatus("已是最新版本")
                    return@launch
                }
                onStatus("下載 ${r.version} 中…")
                val file = updater.download(r) { pct -> activity.runOnUiThread { onStatus("下載 ${r.version}：$pct%") } }
                onStatus("${r.version} 已下載")
                if (activity.isFinishing) return@launch
                val note = if (BuildConfig.DEBUG) "\n\n（這是 debug 版，簽章與正式版不同，要先解除安裝才能裝正式版）" else ""
                MaterialAlertDialogBuilder(activity)
                    .setTitle("有新版本 ${r.version}")
                    .setMessage(r.notes.ifEmpty { "已下載完成。" } + "\n\n要現在安裝嗎？$note")
                    .setPositiveButton("安裝") { _, _ ->
                        if (!updater.install(file)) {
                            Toast.makeText(activity, "請允許安裝這個 app 的更新，回來後再到設定按「檢查更新」", Toast.LENGTH_LONG).show()
                        }
                    }
                    .setNegativeButton("稍後", null)
                    .show()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // 自動檢查失敗不打擾使用者（例如沒有網路），下次開啟 app 再試
                if (manual) onStatus("檢查更新失敗：${e.message}")
            }
        }
    }
}
