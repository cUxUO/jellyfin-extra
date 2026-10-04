package com.jellyfinextra.player.ui

import android.content.Intent
import android.os.Bundle
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.jellyfinextra.player.R
import com.jellyfinextra.player.app
import com.jellyfinextra.player.data.Settings
import com.jellyfinextra.player.net.Endpoints
import com.jellyfinextra.player.net.HttpException
import com.jellyfinextra.player.net.JellyfinApi
import kotlinx.coroutines.launch

class LoginActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_login)
        title = getString(R.string.login_title)

        val settings = app.settings
        val lanJellyfin = findViewById<EditText>(R.id.lanJellyfin)
        val lanXcode = findViewById<EditText>(R.id.lanXcode)
        val external = findViewById<EditText>(R.id.external)
        val profile = findViewById<Spinner>(R.id.profile)
        val username = findViewById<EditText>(R.id.username)
        val password = findViewById<EditText>(R.id.password)
        val login = findViewById<Button>(R.id.login)
        val status = findViewById<TextView>(R.id.status)

        lanJellyfin.setText(settings.lanJellyfin)
        lanXcode.setText(settings.lanXcode)
        external.setText(settings.external)
        username.setText(settings.userName)
        profile.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, Settings.PROFILES)
        profile.setSelection(Settings.PROFILES.indexOf(settings.profile).coerceAtLeast(0))

        login.setOnClickListener {
            settings.lanJellyfin = lanJellyfin.text.toString()
            settings.lanXcode = lanXcode.text.toString()
            settings.external = external.text.toString()
            settings.profile = profile.selectedItem as String
            val user = username.text.toString().trim()
            val pw = password.text.toString()

            login.isEnabled = false
            status.text = getString(R.string.logging_in)
            lifecycleScope.launch {
                val result = runCatching {
                    val base = Endpoints.jellyfin(app) ?: error("連不到 Jellyfin，請檢查位址")
                    JellyfinApi(app.http, settings, base).authenticate(user, pw)
                }
                result.onSuccess {
                    settings.saveLogin(it.token, it.userId, it.userName)
                    startActivity(
                        Intent(this@LoginActivity, BrowseActivity::class.java)
                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
                    )
                    finish()
                }.onFailure { e ->
                    login.isEnabled = true
                    status.text = when {
                        e is HttpException && e.code == 401 -> "帳號或密碼錯誤"
                        else -> "登入失敗：${e.message}"
                    }
                }
            }
        }
    }
}
