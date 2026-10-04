#import <UIKit/UIKit.h>
#import "JXTracks.h"

/// 詳情頁的音軌／字幕選擇（從按鈕彈出的選單）。
@interface JXTrackPicker : NSObject
+ (void)pickAudioFrom:(UIViewController *)vc source:(UIView *)source tracks:(NSArray<JXAudioTrack *> *)tracks
              current:(NSInteger)current onPick:(void (^)(JXAudioTrack *track))onPick;
/// onPick 收到 nil 表示關閉字幕。
+ (void)pickSubtitleFrom:(UIViewController *)vc source:(UIView *)source tracks:(NSArray<JXSubtitleTrack *> *)tracks
                 current:(JXSubtitleTrack *)current onPick:(void (^)(JXSubtitleTrack *track))onPick;
+ (NSString *)subtitleLabel:(JXSubtitleTrack *)t;
@end
