package com.jellyfinextra.player.ui

import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import android.view.inputmethod.EditorInfo
import android.widget.TextView
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.GridLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.JellyfinApi.Item
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** 共用的海報格線頁骨架。 */
abstract class GridFragment : Fragment(R.layout.fragment_grid) {
    protected var adapter: PosterAdapter? = null

    protected fun setupGrid(root: View, columnDp: Int = 160) {
        val grid = root.findViewById<RecyclerView>(R.id.grid)
        grid.layoutManager = GridLayoutManager(requireContext(), gridColumns(columnDp))
        val d = resources.displayMetrics.density
        grid.addItemDecoration(GridGap((10 * d).toInt(), (16 * d).toInt()))
    }

    protected fun show(root: View, items: List<Item>, empty: String) {
        adapter?.items = items
        root.findViewById<TextView>(R.id.message).apply {
            text = empty
            visibility = if (items.isEmpty()) View.VISIBLE else View.GONE
        }
    }

    protected fun showError(root: View, e: Throwable) {
        root.findViewById<TextView>(R.id.message).apply {
            text = e.message
            visibility = View.VISIBLE
        }
    }
}

/** 播放清單與其他資料夾的內容。 */
class FolderFragment : GridFragment() {
    private var loaded = false

    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        val a = requireArguments()
        root.findViewById<TextView>(R.id.title).text = a.getString(ARG_NAME)
        root.findViewById<View>(R.id.back).apply {
            visibility = View.VISIBLE
            setOnClickListener { parentFragmentManager.popBackStack() }
        }
        setupGrid(root)
        load(root)
    }

    override fun onResume() {
        super.onResume()
        if (loaded) view?.let { load(it) }
    }

    private fun load(root: View) {
        val a = requireArguments()
        val parent = Item.stub(a.getString(ARG_ID)!!, a.getString(ARG_TYPE).orEmpty(), a.getString(ARG_NAME).orEmpty(), null)
        val loading = root.findViewById<View>(R.id.loading)
        if (!loaded) loading.visibility = View.VISIBLE
        viewLifecycleOwner.lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = runCatching {
                val api = requireContext().jellyfinApi()
                if (adapter == null) {
                    adapter = PosterAdapter(api, fill = true) { main.openItem(it) }
                    root.findViewById<RecyclerView>(R.id.grid).adapter = adapter
                }
                api.children(parent)
            }
            loading.visibility = View.GONE
            result.onSuccess { loaded = true; show(root, it, "這裡沒有項目") }.onFailure { showError(root, it) }
        }
    }

    companion object {
        private const val ARG_ID = "id"
        private const val ARG_TYPE = "type"
        private const val ARG_NAME = "name"

        fun of(item: Item) = FolderFragment().apply {
            arguments = Bundle().apply {
                putString(ARG_ID, item.id)
                putString(ARG_TYPE, item.type)
                putString(ARG_NAME, item.name)
            }
        }
    }
}

/** 搜尋：輸入停頓 0.4 秒後自動搜尋，按鍵盤的搜尋鍵立即搜尋。 */
class SearchFragment : GridFragment() {
    private var searchJob: Job? = null

    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        root.findViewById<View>(R.id.title).visibility = View.GONE
        root.findViewById<View>(R.id.searchBox).visibility = View.VISIBLE
        setupGrid(root)
        val input = root.findViewById<TextView>(R.id.searchInput)
        input.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) = Unit
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) = Unit
            override fun afterTextChanged(s: Editable?) = search(root, s.toString(), debounce = true)
        })
        input.setOnEditorActionListener { v, actionId, _ ->
            if (actionId == EditorInfo.IME_ACTION_SEARCH) {
                search(root, v.text.toString(), debounce = false)
                true
            } else false
        }
        input.requestFocus()
    }

    private fun search(root: View, term: String, debounce: Boolean) {
        searchJob?.cancel()
        val q = term.trim()
        if (q.isEmpty()) {
            show(root, emptyList(), "輸入片名或影集名稱")
            return
        }
        searchJob = viewLifecycleOwner.lifecycleScope.launch {
            if (debounce) delay(400)
            val result = runCatching {
                val api = requireContext().jellyfinApi()
                if (adapter == null) {
                    adapter = PosterAdapter(api, fill = true) { main.openItem(it) }
                    root.findViewById<RecyclerView>(R.id.grid).adapter = adapter
                }
                api.search(q)
            }
            result.onSuccess { show(root, it, "找不到「$q」") }.onFailure { showError(root, it) }
        }
    }
}

/** 欄數：內容區寬度（扣掉平板的左側導覽列與左右留白）除以每欄大約的寬度；手機至少 3 欄海報。 */
fun Fragment.gridColumns(columnDp: Int = 160, min: Int = 3): Int {
    val res = resources
    val nav = if (requireContext().isPhone) 0 else 88
    val padding = 2 * (res.getDimension(R.dimen.page_padding) / res.displayMetrics.density).toInt()
    return ((res.configuration.screenWidthDp - nav - padding) / columnDp).coerceAtLeast(min)
}

/** 手機的「媒體庫」頁：列出所有媒體庫（平板直接放在左側導覽列）。 */
class LibrariesFragment : Fragment(R.layout.fragment_grid) {
    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        root.findViewById<TextView>(R.id.title).text = "媒體庫"
        root.findViewById<View>(R.id.searchBox)?.visibility = View.GONE
        val grid = root.findViewById<RecyclerView>(R.id.grid)
        grid.layoutManager = GridLayoutManager(requireContext(), gridColumns(columnDp = 170, min = 1))
        val d = resources.displayMetrics.density
        grid.addItemDecoration(GridGap((10 * d).toInt(), (16 * d).toInt()))
        lifecycleScope.launch {
            val api = runCatching { requireContext().jellyfinApi() }.getOrNull() ?: return@launch
            grid.adapter = WideCardAdapter(api, fill = true) { main.showView(it) }.apply { items = main.libraries }
        }
    }
}
