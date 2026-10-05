#import "JXXcode.h"
#import "JXHTTP.h"
#import "JXSettings.h"

@implementation JXXcodeSession
@end

@implementation JXXcode {
	NSURL *_base;
}

- (instancetype)initWithBase:(NSURL *)base {
	if ((self = [super init])) _base = base;
	return self;
}

- (NSURL *)url:(NSString *)path { return [NSURL URLWithString:path relativeToURL:_base].absoluteURL; }

- (void)createSession:(NSString *)itemId profile:(NSString *)profile startTicks:(long long)startTicks
         burnSubtitle:(NSInteger)burnSubtitle audioIndex:(NSInteger)audioIndex
           completion:(void (^)(JXXcodeSession *, NSError *))completion {
	NSMutableDictionary *body = [@{@"itemId": itemId, @"profile": profile, @"startTimeTicks": @(startTicks)} mutableCopy];
	if (burnSubtitle >= 0) body[@"subtitleStreamIndex"] = @(burnSubtitle);
	if (audioIndex >= 0) body[@"audioStreamIndex"] = @(audioIndex);
	[JXHTTP request:@"POST" url:[self url:@"v1/sessions"] body:body token:JXSettings.shared.token timeout:0
	     completion:^(NSData *data, NSError *error) {
		if (error) { completion(nil, [self describe:error]); return; }
		NSDictionary *o = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
		if (![o isKindOfClass:NSDictionary.class] || ![o[@"sessionId"] isKindOfClass:NSString.class]) {
			completion(nil, [NSError errorWithDomain:JXHTTPErrorDomain code:0 userInfo:@{NSLocalizedDescriptionKey: @"轉碼伺服器回應格式不對"}]);
			return;
		}
		JXXcodeSession *s = [[JXXcodeSession alloc] init];
		s.sessionId = o[@"sessionId"];
		// 回傳的是相對路徑，接在基底網址後面；經反向代理掛在 /xcode/ 下也適用
		s.playlist = [self url:o[@"playlist"]];
		NSDictionary *v = [o[@"video"] isKindOfClass:NSDictionary.class] ? o[@"video"] : @{};
		s.width = [v[@"width"] integerValue];
		s.height = [v[@"height"] integerValue];
		s.hwDecode = v[@"hwDecode"] ? [v[@"hwDecode"] boolValue] : YES;
		s.burnedSubtitle = o[@"subtitleStreamIndex"] ? [o[@"subtitleStreamIndex"] integerValue] : -1;
		s.audioIndex = o[@"audioStreamIndex"] ? [o[@"audioStreamIndex"] integerValue] : -1;
		completion(s, nil);
	}];
}

- (void)deleteSession:(NSString *)sessionId {
	[self deleteSession:sessionId completion:nil];
}

- (void)deleteSession:(NSString *)sessionId completion:(void (^)(void))completion {
	[JXHTTP request:@"DELETE" url:[self url:[NSString stringWithFormat:@"v1/sessions/%@", sessionId]] body:nil token:nil timeout:10
	     completion:^(NSData *data, NSError *error) { if (completion) completion(); }];
}

- (NSError *)describe:(NSError *)e {
	NSString *msg;
	switch (e.code) {
		case 401: msg = @"登入已過期，請重新登入"; break;
		case 404: msg = @"找不到這個項目，或沒有權限"; break;
		case 422: msg = [NSString stringWithFormat:@"轉碼伺服器不支援這個片源（%@）", e.localizedDescription]; break;
		case 503: msg = @"轉碼伺服器忙碌中，請稍後再試"; break;
		default: msg = [NSString stringWithFormat:@"轉碼失敗（%@）", e.localizedDescription]; break;
	}
	return [NSError errorWithDomain:e.domain code:e.code userInfo:@{NSLocalizedDescriptionKey: msg}];
}

@end
