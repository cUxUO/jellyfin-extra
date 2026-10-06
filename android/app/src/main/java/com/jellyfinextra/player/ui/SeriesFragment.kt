package com.jellyfinextra.player.ui

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import android.widget.Toast
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import coil3.load
import com.google.android.material.button.MaterialButton
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.google.android.material.progressindicator.LinearProgressIndicator
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import com.jellyfinextra.player.net.catching
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch

/** 影集詳情：上半部是影集資訊與「繼續」按鈕，下面是季的切換與集數列表。點集數直接播放。 */
class SeriesFragment : Fragment(R.layout.fragment_series) {
    private lateinit var seriesId: String
    private var seasonId: String? = null
    private var loaded = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        seriesId = requireArguments().getString(ARG_SERIES)!!
        seasonId = savedInstanceState?.getString(ARG_SEASON) ?: requireArguments().getString(ARG_SEASON)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putString(ARG_SEASON, seasonId)
    }

    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        root.findViewById<View>(R.id.back).setOnClickListener { parentFragmentManager.popBackStack() }
        root.findViewById<RecyclerView>(R.id.episodes).layoutManager = LinearLayoutManager(requireContext())
        load(root)
    }

    override fun onResume() {
        super.onResume()
        if (loaded) view?.let { load(it) }
    }

    private class Data(val api: JellyfinApi, val series: Item, val seasons: List<Item>, val next: Item?)

    private fun load(root: View) {
        val loading = root.findViewById<View>(R.id.loading)
        if (!loaded) loading.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = catching {
                coroutineScope {
                    val api = requireContext().jellyfinApi()
                    val series = async { api.item(seriesId) }
                    val seasons = async { api.seasons(seriesId) }
                    // 「繼續」的對象：有看到一半的集數就是那集，否則是下一集，再不然是第一集
                    val next = async { runCatching { api.nextUp(seriesId, 1).firstOrNull() }.getOrNull() }
                    Data(api, series.await(), seasons.await(), next.await())
                }
            }
            loading.visibility = View.GONE
            result.onSuccess { d ->
                loaded = true
                bindHeader(root, d)
                bindSeasons(root, d)
            }.onFailure { e ->
                Toast.makeText(requireContext(), e.message, Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun bindHeader(root: View, d: Data) {
        val s = d.series
        root.findViewById<View>(R.id.scroll).visibility = View.VISIBLE
        s.backdrop?.let { root.findViewById<ImageView>(R.id.backdrop).load(d.api.imageUrl(it, 1280)) }
        root.findViewById<TextView>(R.id.title).text = s.name
        val seasonCount = d.seasons.count { (it.indexNumber ?: 0) > 0 }
        root.findViewById<TextView>(R.id.meta).text = listOfNotNull(
            s.productionYear?.toString(),
            seasonCount.takeIf { it > 0 }?.let { "$it 季" },
            s.officialRating,
            s.communityRating?.let { "★ %.1f".format(java.util.Locale.US, it) },
            s.genres.take(3).joinToString(" / ").takeIf { it.isNotEmpty() },
        ).joinToString(" · ")
        root.findViewById<TextView>(R.id.overview).text = s.overview
        root.findViewById<MaterialButton>(R.id.watched).apply {
            text = if (s.played) "標記為未看" else "全部標記已看"
            setOnClickListener {
                viewLifecycleOwner.lifecycleScope.launch {
                    catching { d.api.setPlayed(s.id, !s.played) }
                        .onSuccess { load(root) }
                        .onFailure { Toast.makeText(requireContext(), it.message, Toast.LENGTH_SHORT).show() }
                }
            }
        }
        bindPlayButton(root, d.next)
    }

    private fun bindPlayButton(root: View, episode: Item?) {
        root.findViewById<MaterialButton>(R.id.play).apply {
            visibility = if (episode == null) View.GONE else View.VISIBLE
            if (episode == null) return
            text = if (episode.resumable) "繼續 ${episodeLabel(episode)} · ${formatTicks(episode.positionTicks)}"
            else "播放 ${episodeLabel(episode)}"
            setOnClickListener { main.play(episode, if (episode.resumable) episode.positionTicks else 0) }
        }
    }

    private fun bindSeasons(root: View, d: Data) {
        val group = root.findViewById<ChipGroup>(R.id.seasons)
        group.removeAllViews()
        // 預設季：參數指定的，否則「繼續」那集所在的季，否則第一個正片季
        val selected = d.seasons.firstOrNull { it.id == seasonId }
            ?: d.seasons.firstOrNull { it.id == d.next?.seasonId }
            ?: d.seasons.firstOrNull { (it.indexNumber ?: 0) > 0 }
            ?: d.seasons.firstOrNull()
        seasonId = selected?.id
        for (season in d.seasons) {
            val chip = Chip(requireContext()).apply {
                text = if (season.childCount > 0) "${season.name} · ${season.childCount} 集" else season.name
                isCheckable = true
                isCheckedIconVisible = false
                chipBackgroundColor = ContextCompat.getColorStateList(context, R.color.chip_bg)
                setTextColor(ContextCompat.getColorStateList(context, R.color.chip_text))
                chipStrokeColor = ContextCompat.getColorStateList(context, R.color.chip_stroke)
                chipStrokeWidth = resources.displayMetrics.density
                isChecked = season.id == seasonId
                setOnCheckedChangeListener { _, checked ->
                    if (checked && seasonId != season.id) {
                        seasonId = season.id
                        loadEpisodes(root, d.api)
                    }
                }
            }
            group.addView(chip)
        }
        loadEpisodes(root, d.api)
    }

    private fun loadEpisodes(root: View, api: JellyfinApi) {
        val season = seasonId ?: return
        val list = root.findViewById<RecyclerView>(R.id.episodes)
        viewLifecycleOwner.lifecycleScope.launch {
            catching { api.episodes(seriesId, season) }
                .onSuccess { eps -> list.adapter = EpisodeAdapter(api, eps) { ep -> main.play(ep, if (ep.resumable) ep.positionTicks else 0) } }
                .onFailure { Toast.makeText(requireContext(), it.message, Toast.LENGTH_SHORT).show() }
        }
    }

    companion object {
        private const val ARG_SERIES = "series"
        private const val ARG_SEASON = "season"

        fun of(seriesId: String, seasonId: String?) = SeriesFragment().apply {
            arguments = Bundle().apply {
                putString(ARG_SERIES, seriesId)
                putString(ARG_SEASON, seasonId)
            }
        }
    }
}

private class EpisodeAdapter(
    private val api: JellyfinApi,
    private val items: List<Item>,
    private val onClick: (Item) -> Unit,
) : RecyclerView.Adapter<EpisodeAdapter.Holder>() {
    class Holder(v: View) : RecyclerView.ViewHolder(v) {
        val image: ImageView = v.findViewById(R.id.image)
        val badge: View = v.findViewById(R.id.badge)
        val progress: LinearProgressIndicator = v.findViewById(R.id.progress)
        val title: TextView = v.findViewById(R.id.title)
        val meta: TextView = v.findViewById(R.id.meta)
        val overview: TextView = v.findViewById(R.id.overview)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int) =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_episode, parent, false))

    override fun getItemCount() = items.size

    override fun onBindViewHolder(h: Holder, position: Int) {
        val ep = items[position]
        h.image.loadImage(api, ep.wide, 384)
        // 有些集數的名稱本身就是「第 N 集」，不要重複加
        val no = ep.indexNumber
        h.title.text = if (no != null && !ep.name.contains(no.toString())) "$no. ${ep.name}" else ep.name
        h.meta.text = listOfNotNull(
            ep.runTimeTicks.takeIf { it > 0 }?.let { formatDuration(it) },
            if (ep.resumable) "看到 ${formatTicks(ep.positionTicks)}" else null,
        ).joinToString(" · ")
        h.overview.text = ep.overview
        h.overview.visibility = if (ep.overview.isNullOrBlank()) View.GONE else View.VISIBLE
        h.badge.visibility = if (ep.played) View.VISIBLE else View.GONE
        bindProgress(h.progress, ep)
        h.itemView.contentDescription = h.title.text
        h.itemView.setOnClickListener { onClick(ep) }
    }
}
