package com.jellyfinextra.player.ui

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.fragment.app.Fragment
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app

class SettingsFragment : Fragment(R.layout.fragment_settings) {
    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        bind(root)
        root.findViewById<View>(R.id.editConnection).setOnClickListener {
            startActivity(Intent(requireContext(), LoginActivity::class.java))
        }
        root.findViewById<View>(R.id.logout).setOnClickListener { main.logout() }
    }

    override fun onResume() {
        super.onResume()
        view?.let { bind(it) }
    }

    private fun bind(root: View) {
        val s = app.settings
        root.findViewById<TextView>(R.id.account).text = s.userName
        root.findViewById<TextView>(R.id.connection).text = listOfNotNull(
            "目前使用：${app.jellyfinBase ?: "尚未連線"}",
            s.lanJellyfin.takeIf { it.isNotEmpty() }?.let { "內網 Jellyfin：$it" },
            s.lanXcode.takeIf { it.isNotEmpty() }?.let { "內網轉碼伺服器：$it" },
            s.external.takeIf { it.isNotEmpty() }?.let { "外部網址：$it" },
        ).joinToString("\n")
        root.findViewById<TextView>(R.id.profile).text = s.profile
        root.findViewById<TextView>(R.id.version).text = "Jellyfin Extra ${BuildConfig.VERSION_NAME}"
    }
}
