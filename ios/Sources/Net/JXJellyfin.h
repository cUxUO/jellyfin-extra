#import <Foundation/Foundation.h>
#import "JXItem.h"
#import "JXTracks.h"

typedef NS_ENUM(NSInteger, JXSort) { JXSortAdded, JXSortName, JXSortYear, JXSortRating };

typedef void (^JXItemsCompletion)(NSArray<JXItem *> *items, NSError *error);
typedef void (^JXItemCompletion)(JXItem *item, NSError *error);
typedef void (^JXErrorCompletion)(NSError *error);

/// 播放程式用到的 Jellyfin API 子集（對照 Jellyfin 12.1 的 OpenAPI，與 Android 版相同）。
@interface JXJellyfin : NSObject
@property (nonatomic, readonly) NSURL *base;
/// 目前解析好的位址；尚未解析時為 nil。
+ (instancetype)current;
- (instancetype)initWithBase:(NSURL *)base;

- (void)authenticateUser:(NSString *)user password:(NSString *)password
              completion:(void (^)(NSString *token, NSString *userId, NSString *userName, NSError *error))completion;
- (void)userViews:(JXItemsCompletion)completion;
- (void)resume:(JXItemsCompletion)completion;
- (void)nextUpForSeries:(NSString *)seriesId limit:(NSInteger)limit completion:(JXItemsCompletion)completion;
- (void)latestInParent:(NSString *)parentId completion:(JXItemsCompletion)completion;
- (void)library:(JXItem *)view sort:(JXSort)sort genre:(NSString *)genre
     completion:(void (^)(NSArray<JXItem *> *items, NSInteger total, NSError *error))completion;
- (void)genresInParent:(NSString *)parentId completion:(void (^)(NSArray<NSString *> *names, NSError *error))completion;
- (void)search:(NSString *)term completion:(JXItemsCompletion)completion;
- (void)children:(JXItem *)parent completion:(JXItemsCompletion)completion;
- (void)seasons:(NSString *)seriesId completion:(JXItemsCompletion)completion;
- (void)episodes:(NSString *)seriesId season:(NSString *)seasonId completion:(JXItemsCompletion)completion;
- (void)item:(NSString *)itemId completion:(JXItemCompletion)completion;
- (void)setPlayed:(NSString *)itemId played:(BOOL)played completion:(JXErrorCompletion)completion;
- (void)mediaInfo:(NSString *)itemId completion:(void (^)(JXMediaInfo *info, NSError *error))completion;
- (void)subtitleLanguagePreference:(void (^)(NSString *pref))completion;
/// 文字字幕轉成 WebVTT。內嵌字幕要讓 Jellyfin 讀完整個檔案抽出來（大檔可能好幾分鐘），不設逾時。
- (NSURLSessionDataTask *)subtitleVTT:(NSString *)itemId source:(NSString *)sourceId index:(NSInteger)index
                           completion:(void (^)(NSData *data, NSError *error))completion;
/// 依顯示寬度（像素）向 Jellyfin 要縮好的圖。圖片 API 不需要驗證。
- (NSURL *)imageURL:(JXImageRef *)ref maxWidth:(NSInteger)maxWidth;
/// 原始檔網址，轉碼伺服器離線時的退路。
- (NSURL *)directStreamURL:(NSString *)itemId;

- (void)reportPlayback:(NSString *)what item:(NSString *)itemId session:(NSString *)playSessionId
              position:(long long)ticks paused:(BOOL)paused method:(NSString *)method completion:(JXErrorCompletion)completion;
@end
