#import <Foundation/Foundation.h>

/// debug 版的播放紀錄（同 Android 的 playback.log），每次播放覆寫。release 版不寫。
/// 讀取：ssh ipad cat /var/mobile/Library/Caches/com.jellyfinextra.player/playback.log
void JXPlaybackLogReset(void);
void JXPlaybackLog(NSString *format, ...) NS_FORMAT_FUNCTION(1, 2);
