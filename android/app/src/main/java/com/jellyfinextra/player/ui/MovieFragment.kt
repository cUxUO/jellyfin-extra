package com.jellyfinextra.player.ui

import android.os.Bundle
import android.view.View
import android.widget.ImageView
import android.widget.TextView
import android.widget.Toast
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import coil3.load
import com.google.android.material.button.MaterialButton
import com.google.android.material.progressindicator.LinearProgressIndicator
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.data.SubtitleChooser
import com.jellyfinextra.player.data.SubtitleTrack
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch

/** 電影詳情：背景圖、資訊、播放按鈕，播放前可先選音軌與字幕。 */
class MovieFragment : Fragment(R.layout.fragment_movie) {
    private lateinit var itemId: String
    private var item: Item? = null
    private var info: JellyfinApi.MediaInfo? = null
    private var audioIndex: Int? = null
    private var subtitle: SubtitleTrack? = null
    private var subtitleAuto = true
    private var loaded = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        itemId = requireArguments().getString(ARG_ID)!!
    }

    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        root.findViewById<View>(R.id.back).setOnClickListener { parentFragmentManager.popBackStack() }
        load(root)
    }

    override fun onResume() {
        super.onResume()
        if (loaded) view?.let { load(it) }
    }

    private fun load(root: View) {
        val loading = root.findViewById<View>(R.id.loading)
        if (!loaded) loading.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = runCatching {
                coroutineScope {
                    val api = requireContext().jellyfinApi()
                    val item = async { api.item(itemId) }
                    val info = async { runCatching { api.mediaInfo(itemId) }.getOrNull() }
                    val pref = async { runCatching { api.subtitleLanguagePreference() }.getOrNull() }
                    Triple(api, item.await(), Pair(info.await(), pref.await()))
                }
            }
            loading.visibility = View.GONE
            result.onSuccess { (api, item, extra) ->
                val (info, pref) = extra
                if (!loaded && info != null) {
                    // 預設值只在第一次載入時決定，之後保留使用者的選擇
                    audioIndex = info.audio.firstOrNull { it.isDefault }?.index ?: info.audio.firstOrNull()?.index
                    subtitle = SubtitleChooser.choose(info.subtitles, pref)
                }
                loaded = true
                this@MovieFragment.item = item
                this@MovieFragment.info = info
                bind(root, api, item)
            }.onFailure { e ->
                Toast.makeText(requireContext(), e.message, Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun bind(root: View, api: JellyfinApi, item: Item) {
        root.findViewById<View>(R.id.scroll).visibility = View.VISIBLE
        item.backdrop?.let { root.findViewById<ImageView>(R.id.backdrop).load(api.imageUrl(it, 1280)) }
        root.findViewById<TextView>(R.id.title).text = item.name
        root.findViewById<TextView>(R.id.originalTitle).apply {
            text = item.originalTitle
            visibility = if (item.originalTitle.isNullOrBlank() || item.originalTitle == item.name) View.GONE else View.VISIBLE
        }
        root.findViewById<TextView>(R.id.meta).text = metaLine(item, includeGenres = true)
        root.findViewById<TextView>(R.id.overview).apply {
            text = item.overview
            visibility = if (item.overview.isNullOrBlank()) View.GONE else View.VISIBLE
        }

        root.findViewById<MaterialButton>(R.id.play).apply {
            text = if (item.resumable) "繼續播放 ${formatTicks(item.positionTicks)}" else "播放"
            setOnClickListener { play(item, if (item.resumable) item.positionTicks else 0) }
        }
        root.findViewById<View>(R.id.fromStart).apply {
            visibility = if (item.resumable) View.VISIBLE else View.GONE
            setOnClickListener { play(item, 0) }
        }
        root.findViewById<MaterialButton>(R.id.watched).apply {
            contentDescription = if (item.played) "標記為未看" else "標記為已看"
            setIconTintResource(if (item.played) R.color.accent else R.color.text)
            setOnClickListener { togglePlayed(root, api, item) }
        }
        val progressRow = root.findViewById<View>(R.id.progressRow)
        if (item.resumable) {
            progressRow.visibility = View.VISIBLE
            root.findViewById<LinearProgressIndicator>(R.id.progress).progress = progressPermille(item)
            root.findViewById<TextView>(R.id.remaining).text = formatRemaining(item)
        } else {
            progressRow.visibility = View.GONE
        }
        bindTracks(root)
    }

    private fun bindTracks(root: View) {
        val info = info
        val audioView = root.findViewById<TextView>(R.id.audioValue)
        val subView = root.findViewById<TextView>(R.id.subtitleValue)
        val audioPicker = root.findViewById<View>(R.id.audioPicker)
        val subPicker = root.findViewById<View>(R.id.subtitlePicker)
        if (info == null) {
            audioPicker.visibility = View.GONE
            subPicker.visibility = View.GONE
            return
        }
        audioPicker.visibility = if (info.audio.isEmpty()) View.GONE else View.VISIBLE
        audioView.text = info.audio.firstOrNull { it.index == audioIndex }?.title ?: "預設"
        audioPicker.setOnClickListener {
            TrackPicker.audio(requireContext(), info.audio, audioIndex) { audioIndex = it.index; bindTracks(root) }
        }
        subPicker.visibility = if (info.subtitles.isEmpty()) View.GONE else View.VISIBLE
        val sub = subtitle
        subView.text = when {
            sub == null -> "關閉"
            subtitleAuto -> "${TrackPicker.subtitleLabel(sub)}（自動選擇）"
            else -> TrackPicker.subtitleLabel(sub)
        }
        subPicker.setOnClickListener {
            TrackPicker.subtitle(requireContext(), info.subtitles, subtitle) {
                subtitle = it
                subtitleAuto = false
                bindTracks(root)
            }
        }
        root.findViewById<TextView>(R.id.tech).text =
            info.videoDescription?.takeIf { it.isNotEmpty() }?.let { "片源 $it · 在這台裝置以轉碼播放" }.orEmpty()
    }

    private fun play(item: Item, startTicks: Long) {
        val sub = subtitle
        main.play(
            item, startTicks,
            audioIndex = audioIndex ?: PlayerActivity.AUDIO_DEFAULT,
            subtitle = sub?.index ?: PlayerActivity.SUBTITLE_OFF,
        )
    }

    private fun togglePlayed(root: View, api: JellyfinApi, item: Item) {
        viewLifecycleOwner.lifecycleScope.launch {
            runCatching { api.setPlayed(item.id, !item.played) }
                .onSuccess { load(root) }
                .onFailure { Toast.makeText(requireContext(), it.message, Toast.LENGTH_SHORT).show() }
        }
    }

    companion object {
        private const val ARG_ID = "id"

        fun of(item: Item) = MovieFragment().apply { arguments = Bundle().apply { putString(ARG_ID, item.id) } }
    }
}
