plugins {
    alias(libs.plugins.android.application) apply false
    // 只放上 classpath、不套用：AGP 9 的內建 Kotlin 會改用這個版本的編譯器
    alias(libs.plugins.kotlin.android) apply false
}
