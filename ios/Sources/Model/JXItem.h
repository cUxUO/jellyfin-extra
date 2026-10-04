#import <Foundation/Foundation.h>

/// 圖片：項目 ID 加上 tag（tag 變了代表圖換了，放進網址當快取鍵）。
@interface JXImageRef : NSObject
@property (nonatomic, copy) NSString *itemId;
@property (nonatomic, copy) NSString *type;
@property (nonatomic, copy) NSString *tag;
+ (instancetype)refWithItem:(NSString *)itemId type:(NSString *)type tag:(NSString *)tag;
@end

/// Jellyfin 的 BaseItemDto 中播放程式用到的欄位。
@interface JXItem : NSObject
@property (nonatomic, copy) NSString *itemId;
@property (nonatomic, copy) NSString *name;
@property (nonatomic, copy) NSString *type;
@property (nonatomic) BOOL isFolder;
@property (nonatomic, copy) NSString *collectionType;
@property (nonatomic, copy) NSString *seriesId;
@property (nonatomic, copy) NSString *seriesName;
@property (nonatomic, copy) NSString *seasonId;
@property (nonatomic) NSInteger indexNumber;        // 0 表示沒有
@property (nonatomic) NSInteger parentIndexNumber;  // 0 表示沒有
@property (nonatomic) NSInteger productionYear;     // 0 表示沒有
@property (nonatomic) long long runTimeTicks;
@property (nonatomic) long long positionTicks;
@property (nonatomic) BOOL played;
@property (nonatomic) NSInteger unplayedCount;
@property (nonatomic) NSInteger childCount;
@property (nonatomic, copy) NSString *overview;
@property (nonatomic, copy) NSString *originalTitle;
@property (nonatomic, copy) NSArray<NSString *> *genres;
@property (nonatomic) double communityRating;       // 0 表示沒有
@property (nonatomic, copy) NSString *officialRating;
/// 直式海報（2:3）；集數用影集的海報。
@property (nonatomic, strong) JXImageRef *poster;
/// 橫式縮圖（16:9）：集數是劇照，電影／影集是 Thumb，沒有就用背景圖。
@property (nonatomic, strong) JXImageRef *wide;
/// 背景圖；集數用所屬影集的。
@property (nonatomic, strong) JXImageRef *backdrop;

@property (nonatomic, readonly) BOOL playable;
@property (nonatomic, readonly) BOOL resumable;

+ (instancetype)itemWithJSON:(NSDictionary *)o;
+ (NSArray<JXItem *> *)itemsWithJSON:(NSArray *)a;
@end
