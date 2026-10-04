#import <UIKit/UIKit.h>
@class JXItem;

/// 海報格線：資料夾、播放清單、季，或搜尋結果。
@interface JXFolderViewController : UIViewController
+ (instancetype)folder:(JXItem *)item;
+ (instancetype)search;
@end
