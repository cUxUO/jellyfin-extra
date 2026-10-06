#import <Foundation/Foundation.h>

/// 使用者設定與登入狀態（NSUserDefaults）。位址由使用者輸入，不寫死在程式碼。
@interface JXSettings : NSObject
+ (instancetype)shared;
@property (nonatomic, copy) NSString *lanJellyfin;
@property (nonatomic, copy) NSString *lanXcode;
/// 外部網址，例如 https://example.com；Jellyfin 在根路徑，轉碼伺服器在 /xcode/。
@property (nonatomic, copy) NSString *external;
/// 送給轉碼伺服器的裝置規格名稱，對應 server/internal/profile。
@property (nonatomic, copy) NSString *profile;
@property (nonatomic, readonly) NSString *token;
@property (nonatomic, readonly) NSString *userId;
@property (nonatomic, readonly) NSString *userName;
/// 文字字幕大小的選項（小、中、大、特大），規則同 Android 的 SubtitleSize。
+ (NSArray<NSString *> *)subtitleSizeLabels;
/// 目前選的字幕大小，subtitleSizeLabels 的索引，預設 1（中，原本的大小）。
@property (nonatomic) NSInteger subtitleSize;
/// 字級倍率，1 是原本的大小。
@property (nonatomic, readonly) double subtitleScale;
/// Jellyfin 用 DeviceId 區分裝置，安裝後固定不變。
@property (nonatomic, readonly) NSString *deviceId;
@property (nonatomic, readonly) BOOL loggedIn;
- (void)saveToken:(NSString *)token userId:(NSString *)userId userName:(NSString *)userName;
- (void)logout;
@end
