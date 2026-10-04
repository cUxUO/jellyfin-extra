#import <UIKit/UIKit.h>

/// 電影詳情：背景圖、簡介、播放按鈕，以及開播前選音軌與字幕。
@interface JXMovieViewController : UIViewController
- (instancetype)initWithItemId:(NSString *)itemId;
@end
