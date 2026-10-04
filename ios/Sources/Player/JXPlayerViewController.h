#import <UIKit/UIKit.h>
@class JXItem;

/// 讓轉碼伺服器挑預設音軌。
static const NSInteger JXAudioDefault = -1;
/// 依偏好自動挑字幕。
static const NSInteger JXSubtitleAuto = -2;
static const NSInteger JXSubtitleOff = -1;

/// 播放：優先請轉碼伺服器即時轉成 HLS；轉碼伺服器離線時改走 Jellyfin 原始檔直接播放。
/// 播放狀態回報給 Jellyfin，讓觀看紀錄與進度同步。
///
/// 字幕：文字字幕由 app 自己畫；圖形字幕請轉碼伺服器燒進畫面。
/// 音軌與燒錄字幕都在轉碼時決定，切換時在目前位置重建轉碼 session。
@interface JXPlayerViewController : UIViewController
- (instancetype)initWithItem:(JXItem *)item startTicks:(long long)ticks audio:(NSInteger)audio subtitle:(NSInteger)subtitle;
@end
