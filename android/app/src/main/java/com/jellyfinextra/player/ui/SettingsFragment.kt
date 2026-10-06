package com.jellyfinextra.player.ui

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.fragment.app.Fragment
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.jellyfinextra.player.BuildConfig
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.data.DeviceCaps
import com.jellyfinextra.player.data.Quality
import com.jellyfinextra.player.data.SubtitleSize
import com.jellyfinextra.player.net.Updater

class SettingsFragment : Fragment(R.layout.fragment_settings) {
    override fun onViewCreated(root: View, savedInstanceState: Bundle?) {
        bind(root)
        root.findViewById<View>(R.id.editConnection).setOnClickListener {
            startActivity(Intent(requireContext(), LoginActivity::class.java))
        }
        root.findViewById<View>(R.id.logout).setOnClickListener { main.logout() }
        root.findViewById<View>(R.id.quality).setOnClickListener { pickQuality() }
        root.findViewById<View>(R.id.subtitleSize).setOnClickListener { pickSubtitleSize() }
        root.findViewById<View>(R.id.checkUpdate).setOnClickListener {
            val status = root.findViewById<TextView>(R.id.updateStatus)
            UpdatePrompt.check(requireActivity() as AppCompatActivity, manual = true) { status.text = it }
        }
    }

    private fun pickQuality() {
        val s = app.settings
        val options = Quality.entries.filter { it.maxHeight <= DeviceCaps.maxTranscodeHeight }
        MaterialAlertDialogBuilder(requireContext())
            .setTitle("預設畫質")
            .setSingleChoiceItems(options.map { it.label }.toTypedArray(), options.indexOf(s.quality)) { d, which ->
                s.quality = options[which]
                view?.let { bind(it) }
                d.dismiss()
            }
            .show()
    }

    private fun pickSubtitleSize() {
        val s = app.settings
        val options = SubtitleSize.entries
        MaterialAlertDialogBuilder(requireContext())
            .setTitle("字幕大小")
            .setSingleChoiceItems(options.map { it.label }.toTypedArray(), options.indexOf(s.subtitleSize)) { d, which ->
                s.subtitleSize = options[which]
                view?.let { bind(it) }
                d.dismiss()
            }
            .show()
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
        root.findViewById<TextView>(R.id.decoders).text = DeviceCaps.describe()
        root.findViewById<TextView>(R.id.quality).text = s.quality.label
        root.findViewById<TextView>(R.id.subtitleSize).text = s.subtitleSize.label
        val abi = runCatching { Updater(requireContext(), app.http).abi }.getOrNull()
        root.findViewById<TextView>(R.id.version).text =
            listOfNotNull("Jellyfin Extra ${BuildConfig.VERSION_NAME}", abi, if (BuildConfig.DEBUG) "debug" else null).joinToString(" · ")
    }
}
