package com.jellyfinextra.player.ui

import android.content.res.ColorStateList
import android.graphics.Rect
import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.button.MaterialButtonToggleGroup
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import com.jellyfinextra.player.net.JellyfinApi.Sort
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/** 媒體庫：海報格線，可排序、依類型篩選。播放清單庫等其他媒體庫只列內容、不顯示篩選。 */
class LibraryFragment : Fragment(R.layout.fragment_library) {
    private lateinit var library: Item
    private var sort = Sort.Added
    private var genre: String? = null
    private var adapter: PosterAdapter? = null
    private var loadJob: Job? = null
    private var loaded = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val a = requireArguments()
        library = Item(
            id = a.getString(ARG_ID)!!, name = a.getString(ARG_NAME).orEmpty(), type = "CollectionFolder",
            isFolder = true, collectionType = a.getString(ARG_COLLECTION),
        )
        sort = Sort.entries[savedInstanceState?.getInt(STATE_SORT) ?: 0]
        genre = savedInstanceState?.getString(STATE_GENRE)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putInt(STATE_SORT, sort.ordinal)
        outState.putString(STATE_GENRE, genre)
    }

    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        // 播放清單庫在 Jellyfin 預設叫「Playlists」，改用中文（導覽列顯示「清單」）
        root.findViewById<TextView>(R.id.title).text =
            if (library.collectionType == "playlists") getString(R.string.playlists_title) else library.name
        val filterable = library.collectionType == "movies" || library.collectionType == "tvshows"
        root.findViewById<View>(R.id.controls).visibility = if (filterable) View.VISIBLE else View.GONE

        val grid = root.findViewById<RecyclerView>(R.id.grid)
        grid.layoutManager = GridLayoutManager(requireContext(), spanCount())
        grid.addItemDecoration(GridGap(dp(10), dp(16)))

        val sortGroup = root.findViewById<MaterialButtonToggleGroup>(R.id.sort)
        val sortIds = listOf(R.id.sortAdded, R.id.sortName, R.id.sortYear, R.id.sortRating)
        sortGroup.check(sortIds[sort.ordinal])
        sortGroup.addOnButtonCheckedListener { _, id, checked ->
            if (checked) {
                sort = Sort.entries[sortIds.indexOf(id)]
                loadItems(root)
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            val api = runCatching { requireContext().jellyfinApi() }.getOrNull() ?: return@launch
            adapter = PosterAdapter(api, fill = true) { main.openItem(it) }
            grid.adapter = adapter
            loadItems(root)
            if (filterable) runCatching { api.genres(library.id) }.onSuccess { buildGenres(root, it) }
        }
    }

    override fun onResume() {
        super.onResume()
        if (loaded) view?.let { loadItems(it) }
    }

    private fun buildGenres(root: View, names: List<String>) {
        val group = root.findViewById<ChipGroup>(R.id.genres)
        group.removeAllViews()
        (listOf(null) + names).forEach { name ->
            val chip = Chip(requireContext()).apply {
                text = name ?: "全部"
                isCheckable = true
                isCheckedIconVisible = false
                chipBackgroundColor = ContextCompat.getColorStateList(context, R.color.chip_bg)
                setTextColor(ContextCompat.getColorStateList(context, R.color.chip_text))
                chipStrokeColor = ContextCompat.getColorStateList(context, R.color.chip_stroke)
                chipStrokeWidth = dp(1).toFloat()
                rippleColor = ColorStateList.valueOf(0x22FFFFFF)
                minHeight = dp(40)
                isChecked = name == genre
                setOnCheckedChangeListener { _, checked ->
                    if (checked && genre != name) {
                        genre = name
                        loadItems(root)
                    }
                }
            }
            group.addView(chip)
        }
    }

    private fun loadItems(root: View) {
        val a = adapter ?: return
        val loading = root.findViewById<View>(R.id.loading)
        val message = root.findViewById<TextView>(R.id.message)
        val count = root.findViewById<TextView>(R.id.count)
        loadJob?.cancel()
        if (!loaded) loading.visibility = View.VISIBLE
        loadJob = viewLifecycleOwner.lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = runCatching { requireContext().jellyfinApi().library(library, sort, genre) }
            loading.visibility = View.GONE
            result.onSuccess { page ->
                loaded = true
                a.items = page.items
                count.text = countLabel(page.total)
                message.visibility = if (page.items.isEmpty()) View.VISIBLE else View.GONE
                message.text = "這裡沒有項目"
            }.onFailure { e ->
                message.text = e.message
                message.visibility = View.VISIBLE
            }
        }
    }

    private fun countLabel(n: Int) = when (library.collectionType) {
        "movies", "tvshows" -> "$n 部"
        else -> "$n 項"
    }

    /** 依寬度決定欄數：海報約 140dp 寬加間距，1280×800 的 ZenPad 是 7 欄。 */
    private fun spanCount(): Int {
        val widthDp = resources.configuration.screenWidthDp - 88 - 60
        return (widthDp / 160).coerceAtLeast(3)
    }

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()

    companion object {
        private const val ARG_ID = "id"
        private const val ARG_NAME = "name"
        private const val ARG_COLLECTION = "collection"
        private const val STATE_SORT = "sort"
        private const val STATE_GENRE = "genre"

        fun of(view: Item) = LibraryFragment().apply {
            arguments = Bundle().apply {
                putString(ARG_ID, view.id)
                putString(ARG_NAME, view.name)
                putString(ARG_COLLECTION, view.collectionType)
            }
        }
    }
}

/** 格線間距：每格左右各半，上下固定。 */
class GridGap(private val half: Int, private val rowGap: Int) : RecyclerView.ItemDecoration() {
    override fun getItemOffsets(outRect: Rect, view: View, parent: RecyclerView, state: RecyclerView.State) {
        outRect.left = half
        outRect.right = half
        outRect.bottom = rowGap
    }
}
