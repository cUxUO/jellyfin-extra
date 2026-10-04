package com.jellyfinextra.player.ui

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.LayoutInflater
import android.view.Menu
import android.view.MenuItem
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.ProgressBar
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.DividerItemDecoration
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.Endpoints
import com.jellyfinextra.player.net.HttpException
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import kotlinx.coroutines.launch

/** 瀏覽媒體庫。沒有 parent 時列出使用者的媒體庫，否則列出 parent 的子項目。 */
class BrowseActivity : AppCompatActivity() {
    private lateinit var list: RecyclerView
    private lateinit var progress: ProgressBar
    private lateinit var messageBox: View
    private lateinit var message: TextView
    private val adapter = ItemAdapter(::onItemClick)
    private var parent: Item? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (!app.settings.loggedIn) {
            startActivity(Intent(this, LoginActivity::class.java))
            finish()
            return
        }
        setContentView(R.layout.activity_browse)
        list = findViewById(R.id.list)
        progress = findViewById(R.id.progress)
        messageBox = findViewById(R.id.messageBox)
        message = findViewById(R.id.message)
        findViewById<Button>(R.id.retry).setOnClickListener { load() }

        list.layoutManager = LinearLayoutManager(this)
        list.addItemDecoration(DividerItemDecoration(this, DividerItemDecoration.VERTICAL))
        list.adapter = adapter

        parent = intent.getStringExtra(EXTRA_ID)?.let {
            Item.stub(it, intent.getStringExtra(EXTRA_TYPE).orEmpty(), intent.getStringExtra(EXTRA_NAME).orEmpty(), intent.getStringExtra(EXTRA_SERIES_ID))
        }
        title = parent?.name ?: app.settings.userName
        supportActionBar?.setDisplayHomeAsUpEnabled(parent != null)
        load()
    }

    // 從播放畫面回來時重新載入，更新觀看進度
    override fun onRestart() {
        super.onRestart()
        load()
    }

    private fun load() {
        progress.visibility = View.VISIBLE
        messageBox.visibility = View.GONE
        lifecycleScope.launch {
            app.playbackCleanup?.join()
            val result = runCatching {
                val base = app.jellyfinBase ?: Endpoints.jellyfin(app) ?: error("連不到 Jellyfin（內網與外部網址都失敗）")
                val api = JellyfinApi(app.http, app.settings, base)
                parent?.let { api.children(it) } ?: api.userViews()
            }
            progress.visibility = View.GONE
            result.onSuccess { items ->
                adapter.submit(items)
                if (items.isEmpty()) showMessage(getString(R.string.empty))
            }.onFailure { e ->
                if (e is HttpException && e.code == 401) {
                    app.settings.logout()
                    startActivity(Intent(this@BrowseActivity, LoginActivity::class.java))
                    finish()
                    return@onFailure
                }
                // 位址可能換了（例如從家裡到外面），下次重新探測
                app.jellyfinBase = null
                showMessage(e.message ?: e.toString())
            }
        }
    }

    private fun showMessage(text: String) {
        message.text = text
        messageBox.visibility = View.VISIBLE
    }

    private fun onItemClick(item: Item) {
        when {
            item.playable -> play(item)
            item.isFolder -> startActivity(intentFor(this, item))
        }
    }

    private fun play(item: Item) {
        if (item.positionTicks <= 0 || item.played) {
            startActivity(PlayerActivity.intent(this, item, 0))
            return
        }
        AlertDialog.Builder(this)
            .setTitle(R.string.resume_title)
            .setItems(arrayOf<CharSequence>(getString(R.string.resume_from, formatTicks(item.positionTicks)), getString(R.string.from_start))) { _, which ->
                startActivity(PlayerActivity.intent(this, item, if (which == 0) item.positionTicks else 0))
            }
            .show()
    }

    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        menuInflater.inflate(R.menu.browse, menu)
        return true
    }

    override fun onOptionsItemSelected(item: MenuItem): Boolean = when (item.itemId) {
        android.R.id.home -> { finish(); true }
        R.id.action_refresh -> { app.jellyfinBase = null; load(); true }
        R.id.action_settings -> { startActivity(Intent(this, LoginActivity::class.java)); true }
        R.id.action_logout -> {
            app.settings.logout()
            startActivity(Intent(this, LoginActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK))
            finish()
            true
        }
        else -> super.onOptionsItemSelected(item)
    }

    companion object {
        private const val EXTRA_ID = "id"
        private const val EXTRA_TYPE = "type"
        private const val EXTRA_NAME = "name"
        private const val EXTRA_SERIES_ID = "series_id"

        fun intentFor(context: Context, item: Item) = Intent(context, BrowseActivity::class.java)
            .putExtra(EXTRA_ID, item.id)
            .putExtra(EXTRA_TYPE, item.type)
            .putExtra(EXTRA_NAME, item.name)
            .putExtra(EXTRA_SERIES_ID, item.seriesId ?: if (item.type == "Series") item.id else null)
    }
}

private class ItemAdapter(private val onClick: (Item) -> Unit) : RecyclerView.Adapter<ItemAdapter.Holder>() {
    private var items: List<Item> = emptyList()

    fun submit(newItems: List<Item>) {
        items = newItems
        notifyDataSetChanged()
    }

    class Holder(view: View) : RecyclerView.ViewHolder(view) {
        val title: TextView = view.findViewById(R.id.title)
        val subtitle: TextView = view.findViewById(R.id.subtitle)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int) =
        Holder(LayoutInflater.from(parent.context).inflate(R.layout.item_media, parent, false))

    override fun getItemCount() = items.size

    override fun onBindViewHolder(holder: Holder, position: Int) {
        val item = items[position]
        holder.title.text = titleOf(item)
        val sub = subtitleOf(item)
        holder.subtitle.text = sub
        holder.subtitle.visibility = if (sub.isEmpty()) View.GONE else View.VISIBLE
        holder.itemView.setOnClickListener { onClick(item) }
    }

    private fun titleOf(item: Item): String = when {
        // 有些集數的名稱本身就是「第 N 集」，不要重複加
        item.type == "Episode" && item.indexNumber != null && !item.name.contains(item.indexNumber.toString()) ->
            "第 ${item.indexNumber} 集　${item.name}"
        item.productionYear != null && item.type in setOf("Movie", "Series") -> "${item.name}（${item.productionYear}）"
        else -> item.name
    }

    private fun subtitleOf(item: Item): String {
        val progress = when {
            item.isFolder -> ""
            item.played -> "已看完"
            item.positionTicks > 0 -> "看到 ${formatTicks(item.positionTicks)} / ${formatTicks(item.runTimeTicks)}"
            item.runTimeTicks > 0 -> formatTicks(item.runTimeTicks)
            else -> ""
        }
        // 在播放清單等混合列表裡，集數要配上影集名稱才認得出來
        val series = if (item.type == "Episode") item.seriesName.orEmpty() else ""
        return listOf(series, progress).filter { it.isNotEmpty() }.joinToString(" · ")
    }
}
