#import <UIKit/UIKit.h>

@interface UIImageView (JX)
/// 載入網路圖片：記憶體快取命中就直接顯示，否則背景下載並在背景解碼（避免捲動時在主執行緒解碼）。
/// 重複呼叫（例如 cell 重用）會取消前一個請求。url 為 nil 時清空。
- (void)jx_setImageURL:(NSURL *)url;
@end
