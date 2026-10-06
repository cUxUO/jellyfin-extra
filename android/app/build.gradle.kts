import com.android.build.api.variant.FilterConfiguration
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
}

// 發布簽章：金鑰在主機的 ~/.config/jellyfin-extra（devenv 掛進容器），不進版控。
// 每一版都必須用同一把，遺失就無法覆蓋安裝更新。
val signingDir = File(System.getProperty("user.home"), ".config/jellyfin-extra")
val signing = Properties().apply {
    File(signingDir, "signing.properties").takeIf { it.exists() }?.inputStream()?.use { load(it) }
}

// 依 ABI 分開的 APK：versionCode = 基礎版號 × 10 + 這個值。64 位元的較大，
// 同時支援兩種的裝置會從 32 位元版更新到 64 位元版，反之不會。
val abiCodes = mapOf("armeabi-v7a" to 1, "arm64-v8a" to 2)

android {
    namespace = "com.jellyfinextra.player"
    // androidx.core 1.19 需要 compileSdk 37；實際執行的下限由 minSdk 決定
    compileSdk = 37

    defaultConfig {
        applicationId = "com.jellyfinextra.player"
        minSdk = 24
        targetSdk = 34
        // 發布新版時兩個都要改；versionName 也是 GitHub Release 的 tag（v0.2.0）
        versionCode = 9
        versionName = "0.2.7"
        buildConfigField("String", "UPDATE_REPO", "\"${providers.gradleProperty("updateRepo").get()}\"")
    }

    signingConfigs {
        if (signing.isNotEmpty()) {
            create("release") {
                storeFile = File(signingDir, signing.getProperty("storeFile"))
                storePassword = signing.getProperty("storePassword")
                keyAlias = signing.getProperty("keyAlias")
                keyPassword = signing.getProperty("keyPassword")
            }
        }
    }

    // Conscrypt 帶有各平台的 native 函式庫，分開打包可以讓 APK 小很多。只做兩種 ARM：
    // ZenPad（MT8163）是 arm64-v8a，armeabi-v7a 給 32 位元的舊裝置
    splits {
        abi {
            isEnable = true
            reset()
            include(*abiCodes.keys.toTypedArray())
            isUniversalApk = false
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        isCoreLibraryDesugaringEnabled = true
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release")
        }
    }
}

androidComponents {
    onVariants { variant ->
        variant.outputs.forEach { output ->
            val abi = output.filters.find { it.filterType == FilterConfiguration.FilterType.ABI }?.identifier
            val base = output.versionCode.orNull ?: 0
            output.versionCode.set(base * 10 + (abiCodes[abi] ?: 0))
        }
    }
}

// 沒有簽章金鑰時，release 會產生未簽章、無法安裝的 APK；直接報錯比較清楚
tasks.matching { it.name.startsWith("assemble") && it.name.endsWith("Release") }.configureEach {
    doFirst {
        check(signing.isNotEmpty()) { "找不到 ${signingDir}/signing.properties，無法簽章 release（見 CLAUDE.md）" }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.appcompat)
    implementation(libs.androidx.recyclerview)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.media3.exoplayer)
    implementation(libs.media3.exoplayer.hls)
    implementation(libs.media3.ui)
    implementation(libs.media3.datasource.okhttp)
    implementation(libs.media3.ffmpeg.decoder)
    implementation(libs.okhttp)
    implementation(libs.conscrypt.android)
    implementation(libs.material)
    implementation(libs.androidx.fragment.ktx)
    implementation(libs.coil)
    implementation(libs.coil.network.okhttp)
    coreLibraryDesugaring(libs.desugar.jdk.libs)
    testImplementation(libs.junit)
}
