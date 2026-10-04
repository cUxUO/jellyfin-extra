#import <UIKit/UIKit.h>
@class JXItem;

/// 播放結束（已回報停止）後發出，列表與詳情頁收到時重新載入進度。
extern NSString *const JXPlaybackDidFinishNotification;

/// 主畫面：左側導覽列加內容區（UINavigationController，隱藏導覽列）。
/// 導覽列的媒體庫項目依 Jellyfin 的 UserViews 產生；播放另以全螢幕呈現。
@interface JXRootViewController : UIViewController
- (void)openItem:(JXItem *)item;
- (void)playItem:(JXItem *)item startTicks:(long long)ticks audio:(NSInteger)audio subtitle:(NSInteger)subtitle;
- (void)showView:(JXItem *)view;
- (void)push:(UIViewController *)vc;
- (void)logout;
/// 連線設定（已登入時預先填好目前的值）。
- (void)presentLogin;
@end

JXRootViewController *JXRoot(void);
