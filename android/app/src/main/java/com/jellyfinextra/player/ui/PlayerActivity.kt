package com.jellyfinextra.player.ui

import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.util.DisplayMetrics
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.TextView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.annotation.OptIn
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.lifecycleScope
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.exoplayer.trackselection.DefaultTrackSelector
import androidx.media3.exoplayer.upstream.DefaultBandwidthMeter
import androidx.media3.ui.PlayerView
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.data.AudioTrack
import com.jellyfinextra.player.data.DeviceCaps
import com.jellyfinextra.player.data.DirectPlay
import com.jellyfinextra.player.data.Quality
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
 * 播放：依畫質設定（[Quality]）與裝置實測的解碼能力（[DeviceCaps]）決定：
 * - 直接播放：原始檔能硬解（[DirectPlay]）且選「原始畫質」，或選「自動」且在內網時，向 Jellyfin 取原始檔。
 * - 轉碼：請轉碼伺服器即時轉成 HLS（HEVC 或 H.264 依硬解能力）；「自動」時是多軌，播放器依網速切換解析度。
 * - 轉碼伺服器離線時一律直接播放。
 * 播放狀態回報給 Jellyfin，讓觀看紀錄與進度同步。
 *
 * 字幕：文字字幕由 [SubtitleOverlay] 自己畫；圖形字幕請轉碼伺服器燒進畫面。
 * 音軌與燒錄字幕都在轉碼時決定，切換時在目前位置重建轉碼 session（或改回直接播放）。
 */
@OptIn(UnstableApi::class)
class PlayerActivity : AppCompatActivity() {
    private lateinit var playerView: PlayerView
    private lateinit var status: TextView
    private lateinit var subtitleStatus: TextView
    private lateinit var subtitleOverlay: SubtitleOverlay
    private lateinit var panelScrim: View
    private lateinit var panelTitle: TextView
    private lateinit var panelList: RecyclerView

    private lateinit var itemId: String
    private var startTicks = 0L
    private var requestedAudio = AUDIO_DEFAULT
    private var requestedSubtitle = SUBTITLE_AUTO

    private var player: ExoPlayer? = null
    private var jellyfin: JellyfinApi? = null
    private var xcode: XcodeApi? = null
    private var session: XcodeApi.Session? = null

    private var mediaInfo: JellyfinApi.MediaInfo? = null
    private var currentSubtitle: SubtitleTrack? = null
    private var currentAudio: Int? = null
    private var subtitleJob: Job? = null

    private var playMethod = "Transcode"
    private lateinit var quality: Quality
    /** Jellyfin 走內網：「自動」畫質時才直接播放原始檔（外部連線頻寬不一定夠）。 */
    private var onLan = false
    private var fileLog: PlaybackFileLog? = null
    private val playSessionId = UUID.randomUUID().toString()
    private var reportedStart = false
    private var progressJob: Job? = null

    /** 進背景時關掉轉碼 session 的位置；回到前景時從這裡建立新的。null 表示沒有。 */
    private var suspendedAtMs: Long? = null
    private var suspendedBurn: Int? = null
    private var restoreJob: Job? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_player)
        playerView = findViewById(R.id.playerView)
        status = findViewById(R.id.status)
        subtitleStatus = findViewById(R.id.subtitleStatus)
        subtitleOverlay = SubtitleOverlay(findViewById(R.id.subtitles))
        panelScrim = findViewById(R.id.panelScrim)
        panelTitle = findViewById(R.id.panelTitle)
        panelList = findViewById(R.id.panelList)
        panelList.layoutManager = LinearLayoutManager(this)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        hideSystemBars()

        itemId = intent.getStringExtra(EXTRA_ITEM_ID)!!
        startTicks = intent.getLongExtra(EXTRA_START_TICKS, 0)
        requestedAudio = intent.getIntExtra(EXTRA_AUDIO, AUDIO_DEFAULT)
        requestedSubtitle = intent.getIntExtra(EXTRA_SUBTITLE, SUBTITLE_AUTO)
        // debug 版的自動測試可用 extra 指定畫質（見 src/debug/AndroidManifest.xml）
        quality = intent.getStringExtra(EXTRA_QUALITY)?.takeIf { BuildConfig.DEBUG }?.let { Quality.of(it) } ?: app.settings.quality

        // 控制列裡自訂的按鈕（返回、標題、音軌、字幕）
        playerView.findViewById<TextView>(R.id.playerTitle).text = intent.getStringExtra(EXTRA_TITLE)
        playerView.findViewById<View>(R.id.playerBack).setOnClickListener { finish() }
        playerView.findViewById<View>(R.id.playerSubtitles).setOnClickListener { showSubtitlePanel() }
        playerView.findViewById<View>(R.id.playerAudio).setOnClickListener { showAudioPanel() }
        playerView.findViewById<View>(R.id.playerQuality).setOnClickListener { showQualityPanel() }
        playerView.setControllerVisibilityListener(PlayerView.ControllerVisibilityListener { visibility ->
            subtitleOverlay.setRaised(visibility == View.VISIBLE)
        })

        panelScrim.setOnClickListener { hidePanel() }
        findViewById<View>(R.id.panelClose).setOnClickListener { hidePanel() }
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (panelScrim.visibility == View.VISIBLE) hidePanel() else finish()
            }
        })

        status.text = getString(R.string.preparing)
        lifecycleScope.launch { prepare() }
    }

    /** debug 版的自動測試：am start --activity-single-top 帶 quality 或 audio，等同在面板選擇。 */
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        if (!BuildConfig.DEBUG || player == null) return
        intent.getStringExtra(EXTRA_QUALITY)?.let { applyQuality(Quality.of(it)) }
        val audio = intent.getIntExtra(EXTRA_AUDIO, AUDIO_DEFAULT)
        mediaInfo?.audio?.firstOrNull { it.index == audio }?.let { applyAudio(it) }
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

        // 音軌、字幕清單或偏好拿不到時照樣播放，只是不能切換
        val info = runCatching { jf.mediaInfo(itemId) }.getOrNull()
        mediaInfo = info
        var subtitle: SubtitleTrack? = when (requestedSubtitle) {
            SUBTITLE_OFF -> null
            SUBTITLE_AUTO -> info?.let { SubtitleChooser.choose(it.subtitles, runCatching { jf.subtitleLanguagePreference() }.getOrNull()) }
            else -> info?.subtitles?.firstOrNull { it.index == requestedSubtitle }
        }
        val audio = requestedAudio.takeIf { it != AUDIO_DEFAULT }

        // 轉碼伺服器與 Jellyfin 都讀這個標頭；直接播放時原始檔請求也要帶它
        val dataSourceFactory = OkHttpDataSource.Factory(app.http)
            .setDefaultRequestProperties(mapOf("Authorization" to Http.authHeader(app.settings)))

        // debug 版把解碼器名稱、掉幀、播放方式等寫進 files/playback.log，用來確認是否為硬體解碼
        if (BuildConfig.DEBUG) fileLog = PlaybackFileLog(this).also { it.log("decoders: ${DeviceCaps.describe()}") }

        val mediaItem: MediaItem
        // 轉碼與直接播放的時間軸都是整部片，從要求的位置開始播即可
        val startPositionMs = startTicks / TICKS_PER_MS
        val xcodeBase = Endpoints.xcode(app)
        onLan = jfBase == Http.baseUrl(app.settings.lanJellyfin)
        val directReason = directPlayReason(audio, subtitle)
        fileLog?.log("quality=$quality lan=$onLan direct=${directReason ?: "ok"}")
        if (xcodeBase != null) xcode = XcodeApi(app.http, app.settings, xcodeBase)
        val api = xcode
        if (api != null && !(wantsDirect() && directReason == null)) {
            val s = try {
                api.create(itemId, transcodeOptions(), startTicks, burnSubtitle = subtitle?.takeIf { it.isImage }?.index, audioIndex = audio)
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
                return
            }
            session = s
            currentAudio = s.audioIndex.takeIf { it >= 0 }
            mediaItem = hlsItem(s)
        } else {
            status.text = getString(if (api == null) R.string.direct_play else R.string.loading_original)
            playMethod = "DirectPlay"
            mediaItem = directItem(jf)
            currentAudio = defaultAudio()
            if (api != null) watchDirectPlay()
            // 轉碼伺服器離線時無法燒錄圖形字幕，也無法換音軌（播放器播原始檔的預設音軌）
            if (subtitle?.isImage == true) subtitle = null
        }
        updateStatusLine()
        updateTrackButtons()

        // 字幕一律由 SubtitleOverlay 自己畫，不讓 ExoPlayer 顯示原始檔內嵌的字幕；
        // 不依螢幕大小限制軌道，自適應時由轉碼伺服器決定提供哪些解析度
        val selector = DefaultTrackSelector(this).apply {
            setParameters(buildUponParameters().clearViewportSizeConstraints().setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true))
        }
        // 自適應開播時先用伺服器已轉好的最高畫質；內網頻寬通常足夠，外部連線保守一點
        val meter = DefaultBandwidthMeter.Builder(this)
            .setInitialBitrateEstimate(if (onLan) 20_000_000L else 3_000_000L)
            .build()
        val p = ExoPlayer.Builder(this)
            .setMediaSourceFactory(DefaultMediaSourceFactory(dataSourceFactory))
            .setTrackSelector(selector)
            .setBandwidthMeter(meter)
            .setSeekBackIncrementMs(SEEK_INCREMENT_MS)
            .setSeekForwardIncrementMs(SEEK_INCREMENT_MS)
            .build()
        fileLog?.let { p.addAnalyticsListener(it) }
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

    private fun directItem(jf: JellyfinApi) = MediaItem.fromUri(jf.directStreamUrl(itemId).toString())

    private fun defaultAudio(): Int? = mediaInfo?.audio?.let { a -> (a.firstOrNull { it.isDefault } ?: a.firstOrNull())?.index }

    private fun wantsDirect() = quality == Quality.ORIGINAL || (quality == Quality.AUTO && onLan)

    /** null 表示原始檔能直接播放；[audioIndex] 是使用者選的音軌（null 表示預設）。 */
    private fun directPlayReason(audioIndex: Int?, subtitle: SubtitleTrack?): String? {
        val info = mediaInfo ?: return "沒有片源資訊"
        return DirectPlay.check(info.container, info.bitrate, info.video, info.audio, audioIndex, subtitle, DeviceCaps)
    }

    /** 轉碼的畫質要求：「自動」以螢幕大小為上限並要求多軌，「原始畫質」不設上限，其他放進 16:9 的框。 */
    private fun transcodeOptions(): XcodeApi.Options {
        val (w, h) = when (quality) {
            Quality.AUTO -> screenSize()
            Quality.ORIGINAL -> 0 to 0
            else -> quality.maxWidth to quality.maxHeight
        }
        return XcodeApi.Options(Build.MODEL, DeviceCaps.xcodeCapabilities(), w, h, adaptive = quality == Quality.AUTO)
    }

    /** 橫向的螢幕實際像素，例如 ZenPad 是 1280×800。 */
    private fun screenSize(): Pair<Int, Int> {
        val m = DisplayMetrics()
        @Suppress("DEPRECATION")
        windowManager.defaultDisplay.getRealMetrics(m)
        return maxOf(m.widthPixels, m.heightPixels) to minOf(m.widthPixels, m.heightPixels)
    }

    /** 標題下的狀態，例如「800p · HEVC · GPU 轉碼 · 自動」「直接播放 · HEVC 1080p」。 */
    private fun updateStatusLine() {
        val s = session
        val text = when {
            s != null -> {
                // 自適應時顯示播放器目前選的那一軌
                val height = player?.videoFormat?.height?.takeIf { it > 0 } ?: s.height
                listOfNotNull(
                    "${height}p",
                    if (s.codec == "hevc") "HEVC" else "H.264",
                    "${if (s.hwDecode) "GPU" else "CPU"} 轉碼",
                    if (s.variants > 1) "自動" else null,
                ).joinToString(" · ")
            }
            xcode == null -> "直接播放（轉碼伺服器離線）"
            else -> listOfNotNull("直接播放", mediaInfo?.videoDescription).joinToString(" · ")
        }
        playerView.findViewById<TextView>(R.id.playerStatus).text = text
    }

    private fun updateTrackButtons() {
        val info = mediaInfo
        playerView.findViewById<View>(R.id.playerSubtitles).visibility =
            if (info?.subtitles?.any { it.playable } == true) View.VISIBLE else View.GONE
        // 直接播放時播原始檔，換音軌要靠轉碼伺服器
        playerView.findViewById<View>(R.id.playerAudio).visibility =
            if ((info?.audio?.size ?: 0) > 1 && xcode != null) View.VISIBLE else View.GONE
        // 畫質要靠轉碼伺服器切換
        playerView.findViewById<View>(R.id.playerQuality).visibility = if (xcode != null) View.VISIBLE else View.GONE
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

        override fun onVideoSizeChanged(videoSize: VideoSize) {
            updateStatusLine() // 自適應切換解析度時更新
        }

        override fun onPlayerError(error: PlaybackException) {
            // 進背景時已關掉 session，舊項目的請求會失敗；回前景會換上新 session
            if (suspendedAtMs != null) return
            // 片長以 Jellyfin 記錄為準，實際檔案可能短一點點，最後一段不存在時當作播完
            val p = player
            if (p != null && p.duration > 0 && p.currentPosition > p.duration - END_TOLERANCE_MS) {
                finish()
                return
            }
            fail("播放錯誤：${error.errorCodeName}")
        }
    }

    // ---- 音軌／字幕面板 ----

    private sealed interface Row {
        data class Header(val label: String) : Row
        data class Option(val label: String, val note: String, val selected: Boolean, val onPick: () -> Unit) : Row
    }

    private fun showSubtitlePanel() {
        val tracks = mediaInfo?.subtitles?.filter { it.playable } ?: return
        val cur = currentSubtitle
        val rows = mutableListOf<Row>(Row.Option("關閉", "", cur == null) { applySubtitle(null) })
        val text = tracks.filter { !it.isImage }
        val image = tracks.filter { it.isImage }
        if (text.isNotEmpty()) {
            rows += Row.Header("文字字幕")
            text.forEach { t -> rows += Row.Option(t.title, subtitleNote(t), t.index == cur?.index) { applySubtitle(t) } }
        }
        if (image.isNotEmpty()) {
            rows += Row.Header("圖形字幕（燒進畫面，切換時會重新緩衝）")
            image.forEach { t -> rows += Row.Option(t.title, subtitleNote(t), t.index == cur?.index) { applySubtitle(t) } }
        }
        showPanel("字幕", rows)
    }

    /** Jellyfin 的 DisplayTitle 通常已含格式，這裡只補標題沒有的資訊。 */
    private fun subtitleNote(t: SubtitleTrack) = listOfNotNull(
        t.codec.uppercase().takeIf { it.isNotEmpty() && !t.title.contains(it, ignoreCase = true) },
        if (t.isExternal && !t.title.contains("外部")) "外掛" else null,
        if (t.isForced) "強制" else null,
    ).joinToString(" · ")

    private fun showAudioPanel() {
        val tracks = mediaInfo?.audio ?: return
        val rows = mutableListOf<Row>(Row.Header("切換音軌會重新緩衝"))
        tracks.forEach { t -> rows += Row.Option(t.title, if (t.isDefault) "預設" else "", t.index == currentAudio) { applyAudio(t) } }
        showPanel("音軌", rows)
    }

    private fun showQualityPanel() {
        val maxH = DeviceCaps.maxTranscodeHeight
        val reason = directPlayReason(currentAudio.takeIf { it != defaultAudio() }, currentSubtitle)
        val rows = mutableListOf<Row>(Row.Header("硬解：${DeviceCaps.describe()}"))
        Quality.entries.filter { it.maxHeight <= maxH }.forEach { q ->
            val note = when (q) {
                Quality.AUTO -> if (onLan && reason == null) "內網直接播放原始檔" else "依網速切換解析度"
                Quality.ORIGINAL -> if (reason == null) "直接播放原始檔" else "最高畫質轉碼（$reason）"
                else -> ""
            }
            rows += Row.Option(q.label, note, q == quality) { applyQuality(q) }
        }
        showPanel("畫質", rows)
    }

    private fun showPanel(title: String, rows: List<Row>) {
        panelTitle.text = title
        panelList.adapter = PanelAdapter(rows) { hidePanel() }
        panelScrim.visibility = View.VISIBLE
        val panel = findViewById<View>(R.id.panel)
        panel.translationX = panel.width.takeIf { it > 0 }?.toFloat() ?: (360 * resources.displayMetrics.density)
        panel.animate().translationX(0f).setDuration(180).start()
        playerView.hideController()
    }

    private fun hidePanel() {
        val panel = findViewById<View>(R.id.panel)
        panel.animate().translationX(panel.width.toFloat()).setDuration(150).withEndAction {
            panelScrim.visibility = View.GONE
        }.start()
    }

    private class PanelAdapter(private val rows: List<Row>, private val onPicked: () -> Unit) :
        RecyclerView.Adapter<RecyclerView.ViewHolder>() {
        override fun getItemViewType(position: Int) = if (rows[position] is Row.Header) 0 else 1

        override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
            val layout = if (viewType == 0) R.layout.item_track_header else R.layout.item_track_option
            return object : RecyclerView.ViewHolder(LayoutInflater.from(parent.context).inflate(layout, parent, false)) {}
        }

        override fun getItemCount() = rows.size

        override fun onBindViewHolder(h: RecyclerView.ViewHolder, position: Int) {
            val v = h.itemView
            when (val row = rows[position]) {
                is Row.Header -> (v as TextView).text = row.label
                is Row.Option -> {
                    v.findViewById<TextView>(R.id.label).apply {
                        text = row.label
                        setTextColor(context.getColor(if (row.selected) R.color.accent else R.color.text))
                    }
                    v.findViewById<TextView>(R.id.note).apply {
                        text = row.note
                        visibility = if (row.note.isEmpty()) View.GONE else View.VISIBLE
                    }
                    v.findViewById<View>(R.id.check).visibility = if (row.selected) View.VISIBLE else View.INVISIBLE
                    v.setBackgroundResource(if (row.selected) R.drawable.bg_track_selected else 0)
                    v.setOnClickListener {
                        onPicked()
                        row.onPick()
                    }
                }
            }
        }
    }

    // ---- 字幕與音軌切換 ----

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
        if (xcode != null && burnWanted != burnNow) reconfigure(burnWanted.takeIf { it >= 0 }, currentAudio)
        if (track != null && !track.isImage) loadTextSubtitle(track)
    }

    private fun applyAudio(track: AudioTrack) {
        if (track.index == currentAudio) return
        currentAudio = track.index
        reconfigure(session?.burnedSubtitle?.takeIf { it >= 0 }, track.index)
    }

    private fun applyQuality(q: Quality) {
        if (q == quality) return
        quality = q
        reconfigure(session?.burnedSubtitle?.takeIf { it >= 0 }, currentAudio)
    }

    /** 音軌、燒錄字幕或畫質改變後，決定改成直接播放還是在目前位置重建轉碼 session。 */
    private fun reconfigure(burnSubtitle: Int?, audioIndex: Int?) {
        val chosenAudio = audioIndex?.takeIf { it != defaultAudio() }
        if (xcode != null && burnSubtitle == null && wantsDirect() && directPlayReason(chosenAudio, currentSubtitle) == null) {
            switchToDirect()
        } else {
            restartSession(burnSubtitle, audioIndex)
        }
    }

    /**
     * 直接播放一段時間還沒進入就緒（例如解碼器接受資料卻不輸出），改成轉碼。
     * 已知有問題的解碼器在 DeviceCaps 排除，這裡防的是其他裝置上還沒發現的。
     */
    private fun watchDirectPlay() {
        lifecycleScope.launch {
            delay(DIRECT_PLAY_TIMEOUT_MS)
            val p = player ?: return@launch
            if (session == null && playMethod == "DirectPlay" && p.playbackState != Player.STATE_READY && suspendedAtMs == null) {
                fileLog?.log("direct play not ready after ${DIRECT_PLAY_TIMEOUT_MS}ms, falling back to transcode")
                restartSession(currentSubtitle?.takeIf { it.isImage }?.index, currentAudio)
            }
        }
    }

    /** 改成直接播放原始檔：從目前位置接著播，停掉轉碼 session。 */
    private fun switchToDirect() {
        val jf = jellyfin ?: return
        val p = player ?: return
        val old = session
        val positionMs = p.currentPosition
        session = null
        playMethod = "DirectPlay"
        currentAudio = defaultAudio()
        p.setMediaItem(directItem(jf), positionMs)
        p.prepare()
        updateStatusLine()
        fileLog?.log("switch to direct play at ${positionMs}ms")
        watchDirectPlay()
        val api = xcode
        if (old != null && api != null) app.appScope.launch { runCatching { api.delete(old.id) } }
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

    /** 在目前位置建立新的轉碼 session（燒錄字幕或音軌改變時），播放器接著播，舊 session 隨後刪除。 */
    private fun restartSession(burnSubtitle: Int?, audioIndex: Int?) {
        val api = xcode ?: return
        val p = player ?: return
        val old = session
        val positionMs = p.currentPosition
        status.text = getString(R.string.preparing)
        status.visibility = View.VISIBLE
        lifecycleScope.launch {
            val s = try {
                api.create(itemId, transcodeOptions(), positionMs * TICKS_PER_MS, burnSubtitle, audioIndex)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
                return@launch
            }
            session = s
            playMethod = "Transcode"
            currentAudio = s.audioIndex.takeIf { it >= 0 }
            updateStatusLine()
            fileLog?.log("transcode ${s.codec} ${s.width}x${s.height} variants=${s.variants} at ${positionMs}ms")
            p.setMediaItem(hlsItem(s), positionMs)
            p.prepare()
            if (old != null) app.appScope.launch { runCatching { api.delete(old.id) } }
        }
    }

    // ---- 播放回報 ----

    private fun currentReport(): PlaybackReport {
        val p = player
        val pos = (suspendedAtMs ?: p?.currentPosition ?: 0) * TICKS_PER_MS
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

    override fun onStart() {
        super.onStart()
        restoreSession()
    }

    override fun onStop() {
        super.onStop()
        player?.pause()
        restoreJob?.cancel() // 重建到一半又離開：不要在背景建出 session
        suspendSession()
    }

    /**
     * 進背景（Home、鎖定）時關掉轉碼 session，不在背景佔用 GPU 名額；回到前景再由 [restoreSession] 從同一位置建立新的。
     * 按返回離開時由 onDestroy 收尾；直接播放沒有 session，只暫停。
     */
    private fun suspendSession() {
        if (isFinishing || isChangingConfigurations || suspendedAtMs != null) return
        val api = xcode ?: return
        val s = session ?: return
        val p = player ?: return
        suspendedAtMs = p.currentPosition
        suspendedBurn = s.burnedSubtitle.takeIf { it >= 0 }
        session = null
        if (reportedStart) report { reportProgress(it) }
        p.stop() // 不再向已關掉的 session 要片段
        app.appScope.launch { runCatching { api.delete(s.id) } }
    }

    private fun restoreSession() {
        val positionMs = suspendedAtMs ?: return
        val api = xcode ?: return
        val p = player ?: return
        status.text = getString(R.string.preparing)
        status.visibility = View.VISIBLE
        restoreJob = lifecycleScope.launch {
            val s = try {
                api.create(itemId, transcodeOptions(), positionMs * TICKS_PER_MS, suspendedBurn, currentAudio)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                fail(e.message ?: e.toString()) // 下次回前景會再試
                return@launch
            }
            session = s
            currentAudio = s.audioIndex.takeIf { it >= 0 }
            suspendedAtMs = null
            updateStatusLine()
            // 停在離開時的畫面，由使用者按播放
            p.playWhenReady = false
            p.setMediaItem(hlsItem(s), positionMs)
            p.prepare()
        }
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
        private const val EXTRA_TITLE = "title"
        private const val EXTRA_AUDIO = "audio"
        private const val EXTRA_SUBTITLE = "subtitle"
        private const val EXTRA_QUALITY = "quality"
        private const val END_TOLERANCE_MS = 10_000L
        private const val DIRECT_PLAY_TIMEOUT_MS = 15_000L
        private const val SEEK_INCREMENT_MS = 10_000L

        /** 讓轉碼伺服器挑預設音軌。 */
        const val AUDIO_DEFAULT = -1
        /** 依偏好自動挑字幕。 */
        const val SUBTITLE_AUTO = -2
        const val SUBTITLE_OFF = -1

        fun intent(context: Context, item: JellyfinApi.Item, startTicks: Long, audioIndex: Int, subtitle: Int) =
            Intent(context, PlayerActivity::class.java)
                .putExtra(EXTRA_ITEM_ID, item.id)
                .putExtra(EXTRA_START_TICKS, startTicks)
                .putExtra(EXTRA_TITLE, if (item.type == "Episode") "${displayTitle(item)}　${episodeLabel(item)}" else item.name)
                .putExtra(EXTRA_AUDIO, audioIndex)
                .putExtra(EXTRA_SUBTITLE, subtitle)
    }
}
