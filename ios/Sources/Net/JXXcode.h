#import <Foundation/Foundation.h>

@interface JXXcodeSession : NSObject
@property (nonatomic, copy) NSString *sessionId;
/// 整部片的 VOD 清單（0 秒就是片頭），可以拖曳到任何位置；片段在請求時才轉出。
@property (nonatomic, strong) NSURL *playlist;
@property (nonatomic) NSInteger width;
@property (nonatomic) NSInteger height;
@property (nonatomic) BOOL hwDecode;
/// 燒進畫面的字幕（Jellyfin Index），-1 表示沒有。
@property (nonatomic) NSInteger burnedSubtitle;
/// 實際轉出的音軌，-1 表示沒有音軌。
@property (nonatomic) NSInteger audioIndex;
@end

/// Windows 轉碼伺服器的 API，見 server/internal/api。
@interface JXXcode : NSObject
- (instancetype)initWithBase:(NSURL *)base;
/// burnSubtitle、audioIndex 傳 -1 表示不指定。
- (void)createSession:(NSString *)itemId profile:(NSString *)profile startTicks:(long long)startTicks
         burnSubtitle:(NSInteger)burnSubtitle audioIndex:(NSInteger)audioIndex
           completion:(void (^)(JXXcodeSession *session, NSError *error))completion;
- (void)deleteSession:(NSString *)sessionId;
/// completion 在主執行緒呼叫，不論成功與否。
- (void)deleteSession:(NSString *)sessionId completion:(void (^)(void))completion;
@end
