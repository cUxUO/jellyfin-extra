package com.jellyfinextra.player.ui

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.View
import android.view.WindowManager
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.annotation.OptIn
import androidx.appcompat.app.AlertDialog
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
import com.jellyfinextra.player.data.SubtitleChooser
import com.jellyfinextra.player.data.SubtitleTrack
import com.jellyfinextra.player.net.Endpoints
import com.jellyfinextra.player.net.Http
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.PlaybackReport
import com.jellyfinextra.player.net.XcodeApi
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.UUID

/**
 * 播放：優先請轉碼伺服器即時轉成 HLS；轉碼伺服器離線時改走 Jellyfin 原始檔直接播放。
 * 播放狀態回報給 Jellyfin，讓觀看紀錄與進度同步。
 *
 * 字幕：文字字幕由 [SubtitleOverlay] 自己畫；圖形字幕請轉碼伺服器燒進畫面，
 * 切換到或離開圖形字幕時在目前位置重建轉碼 session。
 */
@OptIn(UnstableApi::class)
class PlayerActivity : AppCompatActivity() {
    private lateinit var playerView: PlayerView
    private lateinit var status: TextView
    private lateinit var subtitleButton: Button
    private lateinit var subtitleStatus: TextView
    private lateinit var subtitleOverlay: SubtitleOverlay

    private lateinit var itemId: String
    private var startTicks = 0L

    private var player: ExoPlayer? = null
    private var jellyfin: JellyfinApi? = null
    private var xcode: XcodeApi? = null
    private var session: XcodeApi.Session? = null

    private var mediaInfo: JellyfinApi.MediaInfo? = null
    private var currentSubtitle: SubtitleTrack? = null
    private var subtitleJob: Job? = null

    private var playMethod = "Transcode"
    private val playSessionId = UUID.randomUUID().toString()
    private var reportedStart = false
    private var progressJob: Job? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_player)
        playerView = findViewById(R.id.playerView)
        status = findViewById(R.id.status)
        subtitleButton = findViewById(R.id.subtitleButton)
        subtitleStatus = findViewById(R.id.subtitleStatus)
        subtitleOverlay = SubtitleOverlay(findViewById(R.id.subtitles))
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        hideSystemBars()

        subtitleButton.setOnClickListener { showSubtitleMenu() }
        // 字幕按鈕跟著播放控制列一起出現、隱藏
        playerView.setControllerVisibilityListener(PlayerView.ControllerVisibilityListener { visibility ->
            val hasSubtitles = mediaInfo?.subtitles?.isNotEmpty() == true
            subtitleButton.visibility = if (visibility == View.VISIBLE && hasSubtitles) View.VISIBLE else View.GONE
            subtitleOverlay.setRaised(visibility == View.VISIBLE)
        })

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

        // 字幕清單或偏好拿不到時照樣播放，只是沒有字幕
        val info = runCatching { jf.mediaInfo(itemId) }.getOrNull()
        mediaInfo = info
        val preference = runCatching { jf.subtitleLanguagePreference() }.getOrNull()
        var subtitle = info?.let { SubtitleChooser.choose(it.subtitles, preference) }

        // 轉碼伺服器與 Jellyfin 都讀這個標頭；直接播放時原始檔請求也要帶它
        val dataSourceFactory = OkHttpDataSource.Factory(app.http)
            .setDefaultRequestProperties(mapOf("Authorization" to Http.authHeader(app.settings)))

        val mediaItem: MediaItem
        // 轉碼與直接播放的時間軸都是整部片，從要求的位置開始播即可
        val startPositionMs = startTicks / TICKS_PER_MS
        val xcodeBase = Endpoints.xcode(app)
        if (xcodeBase != null) {
            val api = XcodeApi(app.http, app.settings, xcodeBase)
            xcode = api
            val s = try {
                api.create(itemId, app.settings.profile, startTicks, burnSubtitle = subtitle?.takeIf { it.isImage }?.index)
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
                return
            }
            session = s
            mediaItem = hlsItem(s)
        } else {
            status.text = getString(R.string.direct_play)
            playMethod = "DirectPlay"
            mediaItem = MediaItem.fromUri(jf.directStreamUrl(itemId).toString())
            // 直接播放無法燒錄圖形字幕
            if (subtitle?.isImage == true) subtitle = null
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

        subtitleOverlay.attach(p)
        currentSubtitle = subtitle
        if (subtitle != null && !subtitle.isImage) loadTextSubtitle(subtitle)
    }

    private fun hlsItem(s: XcodeApi.Session) = MediaItem.Builder()
        .setUri(s.playlist.toString())
        .setMimeType(MimeTypes.APPLICATION_M3U8)
        .build()

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
            // 片長以 Jellyfin 記錄為準，實際檔案可能短一點點，最後一段不存在時當作播完
            val p = player
            if (p != null && p.duration > 0 && p.currentPosition > p.duration - END_TOLERANCE_MS) {
                finish()
                return
            }
            fail("播放錯誤：${error.errorCodeName}")
        }
    }

    // ---- 字幕 ----

    private fun showSubtitleMenu() {
        val tracks = mediaInfo?.subtitles?.filter { it.playable } ?: return
        val labels = listOf(getString(R.string.subtitles_off)) + tracks.map {
            if (it.isImage) it.title + getString(R.string.subtitle_image_suffix) else it.title
        }
        val checked = currentSubtitle?.let { cur -> tracks.indexOfFirst { it.index == cur.index } + 1 } ?: 0
        AlertDialog.Builder(this)
            .setTitle(R.string.subtitles)
            .setSingleChoiceItems(labels.toTypedArray<CharSequence>(), checked) { dialog, which ->
                dialog.dismiss()
                applySubtitle(if (which == 0) null else tracks[which - 1])
            }
            .show()
    }

    private fun applySubtitle(track: SubtitleTrack?) {
        if (track?.isImage == true && xcode == null) {
            Toast.makeText(this, R.string.subtitle_image_needs_xcode, Toast.LENGTH_SHORT).show()
            return
        }
        currentSubtitle = track
        subtitleJob?.cancel()
        subtitleStatus.visibility = View.GONE
        subtitleOverlay.clear()

        // 燒錄狀態改變（換成圖形字幕、換另一條圖形字幕、或離開圖形字幕）才需要重建轉碼
        val burnNow = session?.burnedSubtitle ?: -1
        val burnWanted = track?.takeIf { it.isImage }?.index ?: -1
        if (xcode != null && burnWanted != burnNow) restartSession(burnWanted.takeIf { it >= 0 })
        if (track != null && !track.isImage) loadTextSubtitle(track)
    }

    private fun loadTextSubtitle(track: SubtitleTrack) {
        val jf = jellyfin ?: return
        val info = mediaInfo ?: return
        // 外掛字幕通常不到一秒；內嵌字幕可能要等 Jellyfin 讀完整個檔案，提示一直顯示到載入完成
        subtitleJob = lifecycleScope.launch {
            val hint = launch {
                delay(800)
                subtitleStatus.text = getString(if (track.isExternal) R.string.subtitle_loading else R.string.subtitle_loading_embedded)
                subtitleStatus.visibility = View.VISIBLE
            }
            try {
                val vtt = jf.subtitleVtt(itemId, info.mediaSourceId, track.index)
                if (currentSubtitle == track) subtitleOverlay.show(vtt)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                Toast.makeText(this@PlayerActivity, getString(R.string.subtitle_failed, e.message), Toast.LENGTH_LONG).show()
            } finally {
                hint.cancel()
                subtitleStatus.visibility = View.GONE
            }
        }
    }

    /** 在目前位置建立新的轉碼 session（燒錄字幕改變時），播放器接著播，舊 session 隨後刪除。 */
    private fun restartSession(burnSubtitle: Int?) {
        val api = xcode ?: return
        val p = player ?: return
        val old = session
        val positionMs = p.currentPosition
        status.text = getString(R.string.preparing)
        status.visibility = View.VISIBLE
        lifecycleScope.launch {
            val s = try {
                api.create(itemId, app.settings.profile, positionMs * TICKS_PER_MS, burnSubtitle)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
                return@launch
            }
            session = s
            p.setMediaItem(hlsItem(s), positionMs)
            p.prepare()
            if (old != null) app.appScope.launch { runCatching { api.delete(old.id) } }
        }
    }

    // ---- 播放回報 ----

    private fun currentReport(): PlaybackReport {
        val p = player
        val pos = (p?.currentPosition ?: 0) * TICKS_PER_MS
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
        subtitleOverlay.release()
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
        private const val END_TOLERANCE_MS = 10_000L

        fun intent(context: Context, item: JellyfinApi.Item, startTicks: Long) =
            Intent(context, PlayerActivity::class.java)
                .putExtra(EXTRA_ITEM_ID, item.id)
                .putExtra(EXTRA_START_TICKS, startTicks)
    }
}
