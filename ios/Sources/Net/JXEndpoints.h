#import <Foundation/Foundation.h>

/// 內網優先：先試內網直連，連不上才用外部網址。
/// 這台 iPad 解析不到網域（DNS 先問中華電信的 IPv6 DNS），實際上一律走內網。
@interface JXEndpoints : NSObject
/// 目前選用的 Jellyfin 位址，由 resolveJellyfin 決定。
@property (class, nonatomic, strong) NSURL *jellyfinBase;
+ (void)resolveJellyfin:(void (^)(NSURL *base))completion;
+ (void)resolveJellyfinWithLAN:(NSString *)lan external:(NSString *)external completion:(void (^)(NSURL *base))completion;
/// 轉碼伺服器；回傳 nil 表示 Windows 沒開，播放時改走 Jellyfin 直接播放。
+ (void)resolveXcode:(void (^)(NSURL *base))completion;
@end
