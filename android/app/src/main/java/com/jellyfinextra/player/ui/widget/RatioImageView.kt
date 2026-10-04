package com.jellyfinextra.player.ui.widget

import android.content.Context
import android.util.AttributeSet
import com.google.android.material.imageview.ShapeableImageView
import com.jellyfinextra.player.R

/** 固定比例的圓角圖片：寬度由版面決定，高 = 寬 × ratio（海報 1.5、橫式縮圖 0.5625）。 */
class RatioImageView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyle: Int = 0,
) : ShapeableImageView(context, attrs, defStyle) {
    var ratio: Float = 1.5f
        set(value) {
            field = value
            requestLayout()
        }

    init {
        context.obtainStyledAttributes(attrs, R.styleable.RatioImageView).apply {
            ratio = getFloat(R.styleable.RatioImageView_ratio, 1.5f)
            recycle()
        }
    }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val w = MeasureSpec.getSize(widthMeasureSpec)
        setMeasuredDimension(w, (w * ratio).toInt())
    }
}
