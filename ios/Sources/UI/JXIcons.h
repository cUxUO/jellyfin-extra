#import <UIKit/UIKit.h>

/// 線條圖示，以 24×24 座標繪製後依尺寸縮放；回傳 template 圖，顏色跟著 tintColor。
@interface JXIcons : NSObject
+ (UIImage *)home;
+ (UIImage *)film;
+ (UIImage *)tv;
+ (UIImage *)list;
+ (UIImage *)folder;
+ (UIImage *)search;
+ (UIImage *)settings;
+ (UIImage *)back;
+ (UIImage *)check;
+ (UIImage *)close;
+ (UIImage *)subtitles;
+ (UIImage *)audio;
+ (UIImage *)play;
+ (UIImage *)pause;
+ (UIImage *)rewind;
+ (UIImage *)forward;
+ (UIImage *)playOfSize:(CGFloat)size;
+ (UIImage *)pauseOfSize:(CGFloat)size;
@end
