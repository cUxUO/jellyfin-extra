#import <UIKit/UIKit.h>

/// 橫向捲動的膠囊選項（類型篩選、季）。選中的填琥珀色。
@interface JXChips : UIScrollView
@property (nonatomic, copy) void (^onSelect)(NSInteger index);
@property (nonatomic) NSInteger selectedIndex;
- (void)setTitles:(NSArray<NSString *> *)titles;
@end
