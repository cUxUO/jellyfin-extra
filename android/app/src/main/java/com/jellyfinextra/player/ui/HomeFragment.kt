package com.jellyfinextra.player.ui

import android.graphics.Rect
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import coil3.load
import com.google.android.material.button.MaterialButton
import com.google.android.material.progressindicator.LinearProgressIndicator
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch

/** 首頁：最上面是主打項目（繼續觀看的第一部，沒有就用最新的電影），下面是各列。 */
class HomeFragment : Fragment(R.layout.fragment_home) {
    private var loaded = false

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        load(view)
    }

    // 從播放畫面回來時更新進度與繼續觀看
    override fun onResume() {
        super.onResume()
        if (loaded) view?.let { load(it) }
    }

    private fun load(root: View) {
        val loading = root.findViewById<View>(R.id.loading)
        val message = root.findViewById<TextView>(R.id.message)
        if (!loaded) loading.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = runCatching {
                // coroutineScope：任一請求失敗時把例外交給 runCatching，而不是讓整個畫面的協程失敗
                coroutineScope {
                    val api = requireContext().jellyfinApi()
                    val views = api.userViews().filter { it.collectionType == "movies" || it.collectionType == "tvshows" }
                    val resume = async { api.resume() }
                    val nextUp = async { api.nextUp() }
                    val latest = views.map { v -> async { v to api.latest(v.id) } }
                    Data(api, resume.await(), nextUp.await(), latest.awaitAll())
                }
            }
            loading.visibility = View.GONE
            result.onSuccess { d ->
                loaded = true
                message.visibility = View.GONE
                bind(root, d)
            }.onFailure { e ->
                if (!loaded) {
                    message.text = e.message
                    message.visibility = View.VISIBLE
                }
            }
        }
    }

    private class Data(
        val api: JellyfinApi,
        val resume: List<Item>,
        val nextUp: List<Item>,
        val latest: List<Pair<Item, List<Item>>>,
    )

    private fun bind(root: View, d: Data) {
        root.findViewById<View>(R.id.scroll).visibility = View.VISIBLE
        val hero = d.resume.firstOrNull() ?: d.latest.firstOrNull { it.first.collectionType == "movies" }?.second?.firstOrNull()
        bindHero(root, d.api, hero)

        val sections = root.findViewById<LinearLayout>(R.id.sections)
        sections.removeAllViews()
        if (d.resume.isNotEmpty()) addWideRow(sections, d.api, "繼續觀看", d.resume)
        if (d.nextUp.isNotEmpty()) addWideRow(sections, d.api, "下一集", d.nextUp)
        for ((view, items) in d.latest) {
            if (items.isEmpty()) continue
            val row = addRow(sections, "最新加入 · ${view.name}")
            row.action("全部") { main.showView(view) }
            row.list.adapter = PosterAdapter(d.api) { main.openItem(it) }.also { it.items = items }
        }
    }

    private fun bindHero(root: View, api: JellyfinApi, item: Item?) {
        val hero = root.findViewById<View>(R.id.hero)
        if (item == null) {
            hero.visibility = View.GONE
            return
        }
        hero.visibility = View.VISIBLE
        val image = root.findViewById<ImageView>(R.id.heroImage)
        item.backdrop?.let { image.load(api.imageUrl(it, 1280)) }
        root.findViewById<TextView>(R.id.heroEyebrow).text = if (item.resumable) "繼續觀看" else "最新加入"
        root.findViewById<TextView>(R.id.heroTitle).text =
            if (item.type == "Episode") "${displayTitle(item)}　${episodeLabel(item)}" else item.name
        root.findViewById<TextView>(R.id.heroMeta).text = metaLine(item)
        val progressRow = root.findViewById<View>(R.id.heroProgressRow)
        if (item.resumable) {
            progressRow.visibility = View.VISIBLE
            root.findViewById<LinearProgressIndicator>(R.id.heroProgress).progress = progressPermille(item)
            root.findViewById<TextView>(R.id.heroRemaining).text = formatRemaining(item)
        } else {
            progressRow.visibility = View.GONE
        }
        root.findViewById<MaterialButton>(R.id.heroPlay).apply {
            text = if (item.resumable) "繼續播放 ${formatTicks(item.positionTicks)}" else "播放"
            setOnClickListener { main.play(item, if (item.resumable) item.positionTicks else 0) }
        }
        root.findViewById<MaterialButton>(R.id.heroDetails).setOnClickListener {
            // 集數的詳細資訊是所屬影集
            if (item.type == "Episode" && item.seriesId != null) main.push(SeriesFragment.of(item.seriesId, item.seasonId))
            else main.openItem(item)
        }
    }

    private fun addWideRow(parent: LinearLayout, api: JellyfinApi, title: String, items: List<Item>) {
        val row = addRow(parent, title)
        row.list.adapter = WideCardAdapter(api) { main.openItem(it) }.also { it.items = items }
    }

    private class Row(val view: View, val list: RecyclerView) {
        fun action(text: String, onClick: () -> Unit) {
            view.findViewById<TextView>(R.id.sectionAction).apply {
                this.text = text
                visibility = View.VISIBLE
                setOnClickListener { onClick() }
            }
        }
    }

    private fun addRow(parent: LinearLayout, title: String): Row {
        val v = LayoutInflater.from(parent.context).inflate(R.layout.section_row, parent, false)
        v.findViewById<TextView>(R.id.sectionTitle).text = title
        val list = v.findViewById<RecyclerView>(R.id.sectionList)
        list.layoutManager = LinearLayoutManager(parent.context, LinearLayoutManager.HORIZONTAL, false)
        list.addItemDecoration(HorizontalGap(dp(20)))
        parent.addView(v)
        return Row(v, list)
    }

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
}

/** 橫列卡片之間的間距。 */
class HorizontalGap(private val gap: Int) : RecyclerView.ItemDecoration() {
    override fun getItemOffsets(outRect: Rect, view: View, parent: RecyclerView, state: RecyclerView.State) {
        if (parent.getChildAdapterPosition(view) > 0) outRect.left = gap
    }
}
