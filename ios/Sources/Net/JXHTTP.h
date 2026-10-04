#import <Foundation/Foundation.h>

typedef void (^JXDataCompletion)(NSData *data, NSError *error);
typedef void (^JXJSONCompletion)(id json, NSError *error);

extern NSString *const JXHTTPErrorDomain;

/// HTTP 共用：驗證標頭、基底網址正規化、錯誤轉換。完成回呼一律在主執行緒。
@interface JXHTTP : NSObject
+ (NSURLSession *)session;
/// Jellyfin 的驗證標頭；轉碼伺服器也讀同一個標頭裡的 Token。token 為 nil 時不帶 Token（登入用）。
+ (NSString *)authHeaderWithToken:(NSString *)token;
/// 把使用者輸入的位址正規化成以 / 結尾的基底網址。
+ (NSURL *)baseURL:(NSString *)s;
+ (NSURLSessionDataTask *)request:(NSString *)method url:(NSURL *)url body:(id)jsonBody token:(NSString *)token
                          timeout:(NSTimeInterval)timeout completion:(JXDataCompletion)completion;
+ (NSURLSessionDataTask *)getJSON:(NSURL *)url token:(NSString *)token completion:(JXJSONCompletion)completion;
@end
