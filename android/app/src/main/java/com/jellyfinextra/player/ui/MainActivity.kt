package com.jellyfinextra.player.ui

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.view.Menu
import android.view.View
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.fragment.app.Fragment
import androidx.fragment.app.FragmentManager
import androidx.lifecycle.lifecycleScope
import com.google.android.material.bottomnavigation.BottomNavigationView
import com.google.android.material.navigation.NavigationBarView
import com.jellyfinextra.player.App
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.net.Endpoints
import com.jellyfinextra.player.net.HttpException
import com.jellyfinextra.player.net.JellyfinApi
import com.jellyfinextra.player.net.JellyfinApi.Item
import kotlinx.coroutines.launch

/**
 * 主畫面：導覽列加內容區。平板是左側導覽列，媒體庫項目依 Jellyfin 的 UserViews 產生；
 * 手機（直立）是底部導覽列，最多 5 項，所以媒體庫集中在「媒體庫」頁。
 * 詳情頁疊在內容區的返回堆疊上，播放另開 [PlayerActivity]。
 */
class MainActivity : AppCompatActivity() {
    private lateinit var rail: NavigationBarView
    private var views: List<Item> = emptyList()

    /** 底部導覽列（手機直立）：媒體庫不逐一放進導覽列。 */
    private val compactNav: Boolean get() = rail is BottomNavigationView

    /** 使用者的媒體庫（UserViews），手機的「媒體庫」頁列出它們。 */
    val libraries: List<Item> get() = views

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        applyPageOrientation()
        if (!app.settings.loggedIn) {
            startActivity(Intent(this, LoginActivity::class.java))
            finish()
            return
        }
        setContentView(R.layout.activity_main)
        rail = findViewById(R.id.rail)
        rail.setOnItemSelectedListener { showRoot(it.itemId); true }
        rail.setOnItemReselectedListener { popToRoot() }
        lifecycleScope.launch { setup(firstStart = savedInstanceState == null) }
    }

    private suspend fun setup(firstStart: Boolean) {
        val loading = findViewById<View>(R.id.loading)
        loading.visibility = View.VISIBLE
        val result = runCatching {
            val base = Endpoints.jellyfin(app) ?: error("連不到 Jellyfin（內網與外部網址都失敗）")
            JellyfinApi(app.http, app.settings, base).userViews()
        }
        loading.visibility = View.GONE
        result.onSuccess { v ->
            views = v
            buildMenu()
            // 建選單時第一項已自動選取，設定 selectedItemId 不會再觸發監聽，直接顯示首頁。
            // 重建（例如手機啟動時從橫向轉成直立）時若內容區還沒有頁面，也要顯示首頁
            if (firstStart || supportFragmentManager.findFragmentById(R.id.content) == null) showRoot(ID_HOME)
            if (firstStart) UpdatePrompt.autoCheck(this)
        }.onFailure { e ->
            if (e is HttpException && e.code == 401) {
                logout()
                return
            }
            Toast.makeText(this, e.message, Toast.LENGTH_LONG).show()
        }
    }

    private fun buildMenu() {
        val menu = rail.menu
        menu.clear()
        menu.add(Menu.NONE, ID_HOME, 0, "首頁").setIcon(R.drawable.ic_home)
        if (compactNav) {
            menu.add(Menu.NONE, ID_LIBRARIES, 1, "媒體庫").setIcon(R.drawable.ic_folder)
            menu.add(Menu.NONE, ID_SEARCH, 50, "搜尋").setIcon(R.drawable.ic_search)
            menu.add(Menu.NONE, ID_SETTINGS, 51, "設定").setIcon(R.drawable.ic_settings)
            return
        }
        // 導覽列最多 7 項：首頁、搜尋、設定之外留給媒體庫
        views.take(MAX_VIEWS).forEachIndexed { i, v ->
            val icon = when (v.collectionType) {
                "movies" -> R.drawable.ic_film
                "tvshows" -> R.drawable.ic_tv
                "playlists" -> R.drawable.ic_list
                else -> R.drawable.ic_folder
            }
            // 播放清單庫在 Jellyfin 預設叫「Playlists」，改用中文
            val label = if (v.collectionType == "playlists") "清單" else v.name
            menu.add(Menu.NONE, ID_VIEW_BASE + i, i + 1, label).setIcon(icon)
        }
        menu.add(Menu.NONE, ID_SEARCH, 50, "搜尋").setIcon(R.drawable.ic_search)
        menu.add(Menu.NONE, ID_SETTINGS, 51, "設定").setIcon(R.drawable.ic_settings)
    }

    private fun rootFragment(id: Int): Fragment = when (id) {
        ID_HOME -> HomeFragment()
        ID_SEARCH -> SearchFragment()
        ID_SETTINGS -> SettingsFragment()
        ID_LIBRARIES -> LibrariesFragment()
        else -> LibraryFragment.of(views[id - ID_VIEW_BASE])
    }

    private fun showRoot(id: Int) {
        supportFragmentManager.popBackStack(null, FragmentManager.POP_BACK_STACK_INCLUSIVE)
        supportFragmentManager.beginTransaction()
            .replace(R.id.content, rootFragment(id))
            .commit()
    }

    private fun popToRoot() {
        supportFragmentManager.popBackStack(null, FragmentManager.POP_BACK_STACK_INCLUSIVE)
    }

    fun push(fragment: Fragment) {
        supportFragmentManager.beginTransaction()
            .replace(R.id.content, fragment)
            .addToBackStack(null)
            .commit()
    }

    /** 首頁上的媒體庫列可以直接切到那個媒體庫。 */
    fun showView(view: Item) {
        val i = views.indexOfFirst { it.id == view.id }
        if (i >= 0 && !compactNav) rail.selectedItemId = ID_VIEW_BASE + i else push(LibraryFragment.of(view))
    }

    /** 點到項目時的去向：電影開詳情，影集與季開影集頁，集數直接播放，其他資料夾開清單。 */
    fun openItem(item: Item) {
        when (item.type) {
            "Movie", "Video", "MusicVideo" -> push(MovieFragment.of(item))
            "Series" -> push(SeriesFragment.of(item.id, null))
            "Season" -> push(SeriesFragment.of(item.seriesId ?: return, item.id))
            "Episode" -> play(item, if (item.resumable) item.positionTicks else 0)
            "CollectionFolder", "UserView" -> showView(item)
            else -> push(FolderFragment.of(item))
        }
    }

    fun play(item: Item, startTicks: Long, audioIndex: Int = PlayerActivity.AUDIO_DEFAULT, subtitle: Int = PlayerActivity.SUBTITLE_AUTO) {
        startActivity(PlayerActivity.intent(this, item, startTicks, audioIndex, subtitle))
    }

    fun logout() {
        app.settings.logout()
        startActivity(Intent(this, LoginActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK))
        finish()
    }

    companion object {
        private const val ID_HOME = 1
        private const val ID_SEARCH = 2
        private const val ID_SETTINGS = 3
        private const val ID_LIBRARIES = 4
        private const val ID_VIEW_BASE = 100
        private const val MAX_VIEWS = 4
    }
}

val Fragment.main: MainActivity get() = requireActivity() as MainActivity

/** 解析好的 Jellyfin API；位址在 [MainActivity] 啟動時已探測過，這裡只在程序重建時才會再探測。 */
suspend fun Context.jellyfinApi(): JellyfinApi {
    val app = applicationContext as App
    val base = app.jellyfinBase ?: Endpoints.jellyfin(app) ?: error("連不到 Jellyfin")
    return JellyfinApi(app.http, app.settings, base)
}
