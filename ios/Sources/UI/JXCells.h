#import <UIKit/UIKit.h>
@class JXItem, JXJellyfin;

/// 直式海報卡片（2:3），標題與年份在下方。
@interface JXPosterCell : UICollectionViewCell
+ (CGFloat)textHeight;
- (void)configure:(JXItem *)item api:(JXJellyfin *)api;
@end

/// 橫式卡片（16:9，繼續觀看、下一集）。
@interface JXWideCell : UICollectionViewCell
+ (CGFloat)textHeight;
- (void)configure:(JXItem *)item api:(JXJellyfin *)api;
@end

/// 首頁的一列：標題、右側動作，以及橫向捲動的卡片。
@interface JXRowView : UIView <UICollectionViewDataSource, UICollectionViewDelegate>
@property (nonatomic, copy) void (^onSelect)(JXItem *item);
@property (nonatomic, copy) void (^onAction)(void);
- (instancetype)initWithTitle:(NSString *)title wide:(BOOL)wide api:(JXJellyfin *)api items:(NSArray<JXItem *> *)items;
- (void)setActionTitle:(NSString *)title;
@end

/// 海報格線用的 flow layout：依寬度決定欄數，海報寬約 130pt。
UICollectionViewFlowLayout *JXGridLayout(void);
void JXUpdateGridLayout(UICollectionViewFlowLayout *layout, CGFloat width);
