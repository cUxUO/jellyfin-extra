#import <Foundation/Foundation.h>

@interface JXAudioTrack : NSObject
@property (nonatomic) NSInteger index;
@property (nonatomic, copy) NSString *title;
@property (nonatomic) BOOL isDefault;
@end

/// Jellyfin 的一條字幕串流。
@interface JXSubtitleTrack : NSObject
@property (nonatomic) NSInteger index;
@property (nonatomic, copy) NSString *title;
@property (nonatomic, copy) NSString *language;
@property (nonatomic, copy) NSString *codec;
@property (nonatomic) BOOL isExternal;
@property (nonatomic) BOOL isDefault;
@property (nonatomic) BOOL isForced;
/// 點陣圖字幕（PGS、DVD）只能由轉碼伺服器燒進畫面；文字字幕由 app 自己顯示。
@property (nonatomic, readonly) BOOL isImage;
/// 外掛的圖形字幕在 Jellyfin 主機上，轉碼伺服器讀不到，無法燒錄。
@property (nonatomic, readonly) BOOL playable;
@end

@interface JXMediaInfo : NSObject
@property (nonatomic, copy) NSString *mediaSourceId;
@property (nonatomic, copy) NSArray<JXAudioTrack *> *audio;
@property (nonatomic, copy) NSArray<JXSubtitleTrack *> *subtitles;
/// 片源畫質的簡短描述，例如「4K Dolby Vision」。
@property (nonatomic, copy) NSString *videoDescription;
+ (instancetype)infoWithPlaybackInfo:(NSDictionary *)json;
@end

/// 預設字幕的挑選規則，與 Android 版 SubtitleChooser 相同：
/// 1. 符合偏好語言的文字字幕（Jellyfin 偏好，沒設定就用裝置語言）；中文依地區分繁簡，預設加分、部分字幕扣分。
/// 2. 否則用標示為強制或預設的字幕。 3. 都沒有就不顯示。
@interface JXSubtitleChooser : NSObject
+ (JXSubtitleTrack *)choose:(NSArray<JXSubtitleTrack *> *)tracks preferredLanguage:(NSString *)pref locale:(NSLocale *)locale;
@end
