#import "JXEndpoints.h"
#import "JXHTTP.h"
#import "JXSettings.h"

static NSURL *gJellyfinBase;

@implementation JXEndpoints

+ (NSURL *)jellyfinBase { return gJellyfinBase; }
+ (void)setJellyfinBase:(NSURL *)base { gJellyfinBase = base; }

+ (void)resolveJellyfin:(void (^)(NSURL *))completion {
	[self resolveJellyfinWithLAN:JXSettings.shared.lanJellyfin external:JXSettings.shared.external completion:completion];
}

+ (void)resolveJellyfinWithLAN:(NSString *)lan external:(NSString *)external completion:(void (^)(NSURL *))completion {
	NSMutableArray *bases = [NSMutableArray array];
	NSURL *a = [JXHTTP baseURL:lan], *b = [JXHTTP baseURL:external];
	if (a) [bases addObject:a];
	if (b) [bases addObject:b];
	[self firstReachable:bases path:@"System/Info/Public" completion:^(NSURL *base) {
		if (base) gJellyfinBase = base;
		completion(base);
	}];
}

+ (void)resolveXcode:(void (^)(NSURL *))completion {
	NSMutableArray *bases = [NSMutableArray array];
	NSURL *lan = [JXHTTP baseURL:JXSettings.shared.lanXcode];
	NSURL *ext = [JXHTTP baseURL:JXSettings.shared.external];
	if (lan) [bases addObject:lan];
	if (ext) [bases addObject:[NSURL URLWithString:@"xcode/" relativeToURL:ext].absoluteURL];
	[self firstReachable:bases path:@"healthz" completion:completion];
}

/// 依序探測，連不上就快速放棄改試下一個。
+ (void)firstReachable:(NSArray<NSURL *> *)bases path:(NSString *)path completion:(void (^)(NSURL *))completion {
	if (!bases.count) { completion(nil); return; }
	NSURL *base = bases.firstObject;
	NSURL *probe = [NSURL URLWithString:path relativeToURL:base].absoluteURL;
	[JXHTTP request:@"GET" url:probe body:nil token:nil timeout:2.5 completion:^(NSData *data, NSError *error) {
		if (!error) { completion(base); return; }
		[self firstReachable:[bases subarrayWithRange:NSMakeRange(1, bases.count - 1)] path:path completion:completion];
	}];
}

@end
