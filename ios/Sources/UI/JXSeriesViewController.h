#import <UIKit/UIKit.h>

/// 影集詳情：上半部是影集資訊與「繼續」按鈕，下面是季的切換與集數列表。點集數直接播放。
@interface JXSeriesViewController : UIViewController
- (instancetype)initWithSeries:(NSString *)seriesId season:(NSString *)seasonId;
@end
