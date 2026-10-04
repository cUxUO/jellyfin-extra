#import <Foundation/Foundation.h>

@interface JXCue : NSObject
@property (nonatomic) double start;
@property (nonatomic) double end;
@property (nonatomic, copy) NSString *text;
@end

/// 最簡 WebVTT 解析：只取時間與文字，去掉標籤與位置設定。
/// VTT 是 Jellyfin 從 ASS/SRT 轉來的，原本的位置與樣式已經不準，統一顯示在底部置中。
@interface JXSubtitles : NSObject
+ (NSArray<JXCue *> *)parseVTT:(NSData *)data;
/// 目前時間應顯示的文字（多個 cue 同時出現時依開始時間換行接起來）；cues 須依開始時間排序。
+ (NSString *)textAt:(double)t cues:(NSArray<JXCue *> *)cues;
@end
