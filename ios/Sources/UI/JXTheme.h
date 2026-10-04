#import <UIKit/UIKit.h>

/// 深色底加琥珀色強調，與 Android 版及設計稿一致。
@interface JXTheme : NSObject
@property (class, readonly) UIColor *bg;
@property (class, readonly) UIColor *surface;
@property (class, readonly) UIColor *surfaceHigh;
@property (class, readonly) UIColor *line;
@property (class, readonly) UIColor *outline;
@property (class, readonly) UIColor *text;
@property (class, readonly) UIColor *textSecondary;
@property (class, readonly) UIColor *textTertiary;
@property (class, readonly) UIColor *textBody;
@property (class, readonly) UIColor *accent;
@property (class, readonly) UIColor *onAccent;
@property (class, readonly) UIColor *progressTrack;
@property (class, readonly) UIColor *placeholder;

+ (UIFont *)fontOfSize:(CGFloat)size weight:(UIFontWeight)weight;
+ (UILabel *)labelWithSize:(CGFloat)size weight:(UIFontWeight)weight color:(UIColor *)color;
/// 琥珀色膠囊按鈕（主要動作）。
+ (UIButton *)primaryButton:(NSString *)title icon:(UIImage *)icon;
/// 外框膠囊按鈕（次要動作）。
+ (UIButton *)outlineButton:(NSString *)title;
/// 圓形半透明圖示按鈕（返回等）。
+ (UIButton *)roundIconButton:(UIImage *)icon size:(CGFloat)size label:(NSString *)label;
/// 左側壓暗的漸層，讓背景圖上的文字讀得清楚。
+ (CAGradientLayer *)leftScrim;
+ (CAGradientLayer *)bottomScrim;
@end

UIColor *JXColor(uint32_t rgb);
UIColor *JXColorA(uint32_t rgb, CGFloat alpha);
