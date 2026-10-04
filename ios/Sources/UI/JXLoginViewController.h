#import <UIKit/UIKit.h>

/// 連線設定與登入。位址、帳號存在 JXSettings；密碼不保存。
@interface JXLoginViewController : UIViewController
@property (nonatomic, copy) void (^onLoggedIn)(void);
/// 有設定時顯示「取消」（已登入、從設定頁進來）。
@property (nonatomic, copy) void (^onCancel)(void);
@end
