package com.jellyfinextra.player.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ImageView
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import coil3.load
import coil3.request.allowRgb565
import com.google.android.material.progressindicator.LinearProgressIndicator
import com.jellyfinextra.player.R
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item

/** 依顯示大小向 Jellyfin 要圖；沒有圖時保留底色。 */
fun ImageView.loadImage(api: JellyfinApi, ref: JellyfinApi.ImageRef?, maxWidth: Int) {
    if (ref == null) {
        setImageDrawable(null)
        return
    }
    // 海報與縮圖不需要透明度，用 RGB_565 省一半記憶體
    load(api.imageUrl(ref, maxWidth)) { allowRgb565(true) }
}

/** 直式海報卡片。[fill] 為 true 時寬度填滿格線欄寬，否則是橫列裡的固定寬度。 */
class PosterAdapter(
    private val api: JellyfinApi,
    private val fill: Boolean = false,
    private val onClick: (Item) -> Unit,
) : RecyclerView.Adapter<PosterAdapter.Holder>() {
    var items: List<Item> = emptyList()
        set(value) {
            field = value
            notifyDataSetChanged()
        }

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val image: ImageView = view.findViewById(R.id.image)
        val badge: TextView = view.findViewById(R.id.badge)
        val progress: LinearProgressIndicator = view.findViewById(R.id.progress)
        val title: TextView = view.findViewById(R.id.title)
        val subtitle: TextView = view.findViewById(R.id.subtitle)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): Holder {
        val v = LayoutInflater.from(parent.context).inflate(R.layout.item_poster, parent, false)
        if (fill) v.layoutParams = v.layoutParams.apply { width = ViewGroup.LayoutParams.MATCH_PARENT }
        return Holder(v)
    }

    override fun getItemCount() = items.size

    override fun onBindViewHolder(h: Holder, position: Int) {
        val item = items[position]
        h.image.loadImage(api, item.poster, 300)
        h.title.text = displayTitle(item)
        h.subtitle.text = when (item.type) {
            "Episode" -> episodeLabel(item)
            "Series" -> item.productionYear?.toString().orEmpty()
            "Playlist", "Folder", "BoxSet" -> if (item.childCount > 0) "${item.childCount} 項" else ""
            else -> item.productionYear?.toString().orEmpty()
        }
        bindProgress(h.progress, item)
        when {
            item.played -> h.badge.show("已看")
            item.type == "Series" && item.unplayedCount > 0 -> h.badge.show("${item.unplayedCount} 集未看")
            else -> h.badge.visibility = View.GONE
        }
        h.itemView.contentDescription = displayTitle(item)
        h.itemView.setOnClickListener { onClick(item) }
    }
}

/** 橫式卡片（繼續觀看、下一集）。 */
class WideCardAdapter(
    private val api: JellyfinApi,
    private val onClick: (Item) -> Unit,
) : RecyclerView.Adapter<WideCardAdapter.Holder>() {
    var items: List<Item> = emptyList()
        set(value) {
            field = value
            notifyDataSetChanged()
        }

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val image: ImageView = view.findViewById(R.id.image)
        val progress: LinearProgressIndicator = view.findViewById(R.id.progress)
        val title: TextView = view.findViewById(R.id.title)
        val subtitle: TextView = view.findViewById(R.id.subtitle)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int) =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_card_wide, parent, false))

    override fun getItemCount() = items.size

    override fun onBindViewHolder(h: Holder, position: Int) {
        val item = items[position]
        h.image.loadImage(api, item.wide, 520)
        h.title.text = displayTitle(item)
        val rest = if (item.resumable) formatRemaining(item) else item.runTimeTicks.takeIf { it > 0 }?.let { formatDuration(it) }
        h.subtitle.text = listOfNotNull(if (item.type == "Episode") episodeLabel(item) else null, rest).joinToString(" · ")
        bindProgress(h.progress, item)
        h.itemView.contentDescription = displayTitle(item)
        h.itemView.setOnClickListener { onClick(item) }
    }
}

fun bindProgress(bar: LinearProgressIndicator, item: Item) {
    if (item.resumable) {
        bar.visibility = View.VISIBLE
        bar.progress = progressPermille(item)
    } else {
        bar.visibility = View.GONE
    }
}

private fun TextView.show(text: String) {
    this.text = text
    visibility = View.VISIBLE
}
