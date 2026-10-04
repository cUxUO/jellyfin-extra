#import "UIImageView+JX.h"
#import <objc/runtime.h>
#import "JXHTTP.h"

static const void *kTaskKey = &kTaskKey;
static const void *kURLKey = &kURLKey;

static NSCache<NSURL *, UIImage *> *JXImageCache(void) {
	static NSCache *c;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		c = [[NSCache alloc] init];
		c.totalCostLimit = 40 * 1024 * 1024; // iPad Air 1 只有 1GB 記憶體
	});
	return c;
}

/// 在背景把圖片畫一次，讓 UIKit 拿到已解碼的點陣圖。
static UIImage *JXDecoded(UIImage *img) {
	if (!img) return nil;
	UIGraphicsImageRendererFormat *f = [UIGraphicsImageRendererFormat defaultFormat];
	f.scale = img.scale;
	f.opaque = YES;
	return [[[UIGraphicsImageRenderer alloc] initWithSize:img.size format:f] imageWithActions:^(UIGraphicsImageRendererContext *ctx) {
		[img drawAtPoint:CGPointZero];
	}];
}

@implementation UIImageView (JX)

- (void)jx_setImageURL:(NSURL *)url {
	NSURLSessionDataTask *old = objc_getAssociatedObject(self, kTaskKey);
	[old cancel];
	objc_setAssociatedObject(self, kURLKey, url, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	if (!url) { self.image = nil; return; }

	UIImage *cached = [JXImageCache() objectForKey:url];
	if (cached) { self.image = cached; return; }
	self.image = nil;

	__weak UIImageView *weakSelf = self;
	NSURLSessionDataTask *task = [JXHTTP.session dataTaskWithURL:url completionHandler:^(NSData *data, NSURLResponse *resp, NSError *error) {
		if (error || !data.length) return;
		UIImage *img = JXDecoded([UIImage imageWithData:data scale:UIScreen.mainScreen.scale]);
		if (!img) return;
		[JXImageCache() setObject:img forKey:url cost:(NSUInteger)(img.size.width * img.size.height * img.scale * img.scale * 4)];
		dispatch_async(dispatch_get_main_queue(), ^{
			UIImageView *iv = weakSelf;
			if (!iv || ![objc_getAssociatedObject(iv, kURLKey) isEqual:url]) return;
			iv.alpha = 0;
			iv.image = img;
			[UIView animateWithDuration:0.15 animations:^{ iv.alpha = 1; }];
		});
	}];
	objc_setAssociatedObject(self, kTaskKey, task, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[task resume];
}

@end
