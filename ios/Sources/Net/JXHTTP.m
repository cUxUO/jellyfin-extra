#import "JXHTTP.h"
#import <UIKit/UIKit.h>
#import "JXSettings.h"

NSString *const JXHTTPErrorDomain = @"JXHTTP";

@implementation JXHTTP

+ (NSURLSession *)session {
	static NSURLSession *s;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		NSURLSessionConfiguration *c = NSURLSessionConfiguration.defaultSessionConfiguration;
		c.timeoutIntervalForRequest = 40; // 轉碼伺服器建立 session 時要等第一批片段，最長約 30 秒
		c.HTTPMaximumConnectionsPerHost = 6;
		s = [NSURLSession sessionWithConfiguration:c];
	});
	return s;
}

+ (NSString *)authHeaderWithToken:(NSString *)token {
	NSString *version = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"0";
	NSString *device = [UIDevice.currentDevice.model stringByReplacingOccurrencesOfString:@"\"" withString:@""];
	NSString *head = [NSString stringWithFormat:@"MediaBrowser Client=\"jellyfin-extra\", Device=\"%@\", DeviceId=\"%@\", Version=\"%@\"",
	                  device, JXSettings.shared.deviceId, version];
	return token.length ? [head stringByAppendingFormat:@", Token=\"%@\"", token] : head;
}

+ (NSURL *)baseURL:(NSString *)s {
	NSString *t = [s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
	if (!t.length) return nil;
	if (![t hasSuffix:@"/"]) t = [t stringByAppendingString:@"/"];
	NSURL *u = [NSURL URLWithString:t];
	return (u.scheme && u.host) ? u : nil;
}

+ (NSURLSessionDataTask *)request:(NSString *)method url:(NSURL *)url body:(id)jsonBody token:(NSString *)token
                          timeout:(NSTimeInterval)timeout completion:(JXDataCompletion)completion {
	NSMutableURLRequest *req = [NSMutableURLRequest requestWithURL:url];
	req.HTTPMethod = method;
	if (timeout > 0) req.timeoutInterval = timeout;
	[req setValue:[self authHeaderWithToken:token] forHTTPHeaderField:@"Authorization"];
	[req setValue:@"application/json" forHTTPHeaderField:@"Accept"];
	if (jsonBody) {
		req.HTTPBody = [NSJSONSerialization dataWithJSONObject:jsonBody options:0 error:nil];
		[req setValue:@"application/json; charset=utf-8" forHTTPHeaderField:@"Content-Type"];
	}
	NSURLSessionDataTask *task = [self.session dataTaskWithRequest:req completionHandler:^(NSData *data, NSURLResponse *resp, NSError *error) {
		NSError *err = error;
		NSInteger code = [resp isKindOfClass:NSHTTPURLResponse.class] ? ((NSHTTPURLResponse *)resp).statusCode : 0;
		if (!err && (code < 200 || code >= 300)) {
			NSString *detail = nil;
			id json = data.length ? [NSJSONSerialization JSONObjectWithData:data options:0 error:nil] : nil;
			if ([json isKindOfClass:NSDictionary.class] && [json[@"error"] isKindOfClass:NSString.class]) detail = json[@"error"];
			NSString *msg = detail ? [NSString stringWithFormat:@"HTTP %ld：%@", (long)code, detail] : [NSString stringWithFormat:@"HTTP %ld", (long)code];
			err = [NSError errorWithDomain:JXHTTPErrorDomain code:code userInfo:@{NSLocalizedDescriptionKey: msg}];
		}
		dispatch_async(dispatch_get_main_queue(), ^{ completion(err ? nil : (data ?: [NSData data]), err); });
	}];
	[task resume];
	return task;
}

+ (NSURLSessionDataTask *)getJSON:(NSURL *)url token:(NSString *)token completion:(JXJSONCompletion)completion {
	return [self request:@"GET" url:url body:nil token:token timeout:0 completion:^(NSData *data, NSError *error) {
		if (error) { completion(nil, error); return; }
		NSError *perr = nil;
		id json = [NSJSONSerialization JSONObjectWithData:data options:0 error:&perr];
		completion(json, perr);
	}];
}

@end
