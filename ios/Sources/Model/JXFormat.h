#import <Foundation/Foundation.h>
@class JXItem;

/// Jellyfin 的時間單位：1 tick = 100ns。
#define JX_TICKS_PER_SECOND 10000000LL

/// 播放位置，例如 22:00、1:06:46。
NSString *JXFormatTime(double seconds);
NSString *JXFormatTicks(long long ticks);
/// 片長，例如「1 小時 41 分」「24 分」。
NSString *JXFormatDuration(long long ticks);
/// 「剩 1 小時 19 分」。
NSString *JXFormatRemaining(JXItem *item);
float JXProgress(JXItem *item);
/// 集數短標，例如「第 2 集」，季號不是 1 時加上「S2 · 」。
NSString *JXEpisodeLabel(JXItem *item);
/// 卡片標題：集數用影集名稱。
NSString *JXDisplayTitle(JXItem *item);
/// 年份 · 片長 · 分級 · ★ 評分 · 類型。
NSString *JXMetaLine(JXItem *item, BOOL includeGenres);
