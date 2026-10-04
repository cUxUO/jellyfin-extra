package com.jellyfinextra.player.ui

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.View
import android.view.WindowManager
import android.widget.TextView
import androidx.annotation.OptIn
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.lifecycleScope
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.PlayerView
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.Endpoints
import com.jellyfinextra.player.net.Http
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.PlaybackReport
import com.jellyfinextra.player.net.XcodeApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.UUID

/**
 * 播放：優先請轉碼伺服器即時轉成 HLS；轉碼伺服器離線時改走 Jellyfin 原始檔直接播放。
 * 播放狀態回報給 Jellyfin，讓觀看紀錄與進度同步。
 */
@OptIn(UnstableApi::class)
class PlayerActivity : AppCompatActivity() {
    private lateinit var playerView: PlayerView
    private lateinit var status: TextView

    private lateinit var itemId: String
    private var startTicks = 0L

    private var player: ExoPlayer? = null
    private var jellyfin: JellyfinApi? = null
    private var xcode: XcodeApi? = null
    private var session: XcodeApi.Session? = null

    /** 播放器的 0 秒對應片中的這個位置（轉碼從 startTicks 開始，直接播放則是 0）。 */
    private var baseTicks = 0L
    private var playMethod = "Transcode"
    private val playSessionId = UUID.randomUUID().toString()
    private var reportedStart = false
    private var progressJob: Job? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_player)
        playerView = findViewById(R.id.playerView)
        status = findViewById(R.id.status)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        hideSystemBars()

        itemId = intent.getStringExtra(EXTRA_ITEM_ID)!!
        startTicks = intent.getLongExtra(EXTRA_START_TICKS, 0)
        status.text = getString(R.string.preparing)
        lifecycleScope.launch { prepare() }
    }

    private suspend fun prepare() {
        val app = this.app
        val jfBase = app.jellyfinBase ?: Endpoints.jellyfin(app)
        if (jfBase == null) {
            fail("連不到 Jellyfin")
            return
        }
        val jf = JellyfinApi(app.http, app.settings, jfBase)
        jellyfin = jf

        // 轉碼伺服器與 Jellyfin 都讀這個標頭；直接播放時原始檔請求也要帶它
        val dataSourceFactory = OkHttpDataSource.Factory(app.http)
            .setDefaultRequestProperties(mapOf("Authorization" to Http.authHeader(app.settings)))

        val mediaItem: MediaItem
        var startPositionMs = 0L
        val xcodeBase = Endpoints.xcode(app)
        if (xcodeBase != null) {
            val api = XcodeApi(app.http, app.settings, xcodeBase)
            xcode = api
            val s = try {
                api.create(itemId, app.settings.profile, startTicks)
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
                return
            }
            session = s
            baseTicks = s.startTimeTicks
            mediaItem = MediaItem.Builder()
                .setUri(s.playlist.toString())
                .setMimeType(MimeTypes.APPLICATION_M3U8)
                // 轉碼中的清單是 EVENT 型態，ExoPlayer 會當直播處理；固定 1 倍速，避免它為了追直播邊緣而加速
                .setLiveConfiguration(
                    MediaItem.LiveConfiguration.Builder().setMinPlaybackSpeed(1f).setMaxPlaybackSpeed(1f).build()
                )
                .build()
        } else {
            status.text = getString(R.string.direct_play)
            playMethod = "DirectPlay"
            baseTicks = 0
            startPositionMs = startTicks / TICKS_PER_MS
            mediaItem = MediaItem.fromUri(jf.directStreamUrl(itemId).toString())
        }

        val p = ExoPlayer.Builder(this)
            .setMediaSourceFactory(DefaultMediaSourceFactory(dataSourceFactory))
            .build()
        // debug 版把解碼器名稱、掉幀等寫進 files/playback.log，用來確認是否為硬體解碼
        if (BuildConfig.DEBUG) p.addAnalyticsListener(PlaybackFileLog(this))
        player = p
        playerView.player = p
        p.addListener(listener)
        p.setMediaItem(mediaItem, startPositionMs)
        p.prepare()
        p.playWhenReady = true
    }

    private val listener = object : Player.Listener {
        override fun onPlaybackStateChanged(state: Int) {
            when (state) {
                Player.STATE_READY -> {
                    status.visibility = View.GONE
                    if (!reportedStart) {
                        reportedStart = true
                        report { reportStart(it) }
                        startProgressLoop()
                    }
                }
                Player.STATE_ENDED -> finish()
                else -> Unit
            }
        }

        override fun onIsPlayingChanged(isPlaying: Boolean) {
            if (reportedStart) report { reportProgress(it) }
        }

        override fun onPlayerError(error: PlaybackException) {
            fail("播放錯誤：${error.errorCodeName}")
        }
    }

    private fun currentReport(): PlaybackReport {
        val p = player
        val pos = baseTicks + (p?.currentPosition ?: 0) * TICKS_PER_MS
        return PlaybackReport(itemId, playSessionId, pos, isPaused = p?.isPlaying != true, playMethod = playMethod)
    }

    /** 回報失敗不影響播放，只是 Jellyfin 的紀錄不會更新。 */
    private fun report(call: suspend JellyfinApi.(PlaybackReport) -> Unit) {
        val jf = jellyfin ?: return
        val r = currentReport()
        app.appScope.launch { runCatching { jf.call(r) } }
    }

    private fun startProgressLoop() {
        progressJob = lifecycleScope.launch {
            while (isActive) {
                delay(10_000)
                report { reportProgress(it) }
            }
        }
    }

    private fun fail(message: String) {
        status.visibility = View.VISIBLE
        status.text = message
        player?.pause()
    }

    override fun onStop() {
        super.onStop()
        player?.pause()
    }

    override fun onDestroy() {
        super.onDestroy()
        progressJob?.cancel()
        val p = player ?: return
        val r = currentReport()
        p.removeListener(listener)
        p.release()
        player = null

        // Activity 已結束，改在 app 層級完成收尾：回報停止、停掉轉碼釋出 GPU 名額
        val jf = jellyfin
        val api = xcode
        val s = session
        app.playbackCleanup = app.appScope.launch {
            if (reportedStart && jf != null) runCatching { jf.reportStopped(r) }
            if (api != null && s != null) runCatching { api.delete(s.id) }
        }
    }

    private fun hideSystemBars() {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        WindowInsetsControllerCompat(window, window.decorView).apply {
            hide(WindowInsetsCompat.Type.systemBars())
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        }
    }

    companion object {
        private const val EXTRA_ITEM_ID = "item_id"
        private const val EXTRA_START_TICKS = "start_ticks"

        fun intent(context: Context, item: JellyfinApi.Item, startTicks: Long) =
            Intent(context, PlayerActivity::class.java)
                .putExtra(EXTRA_ITEM_ID, item.id)
                .putExtra(EXTRA_START_TICKS, startTicks)
    }
}
