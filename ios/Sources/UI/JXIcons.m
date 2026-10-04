#import "JXIcons.h"

typedef void (^JXDraw)(UIBezierPath *p);

@implementation JXIcons

/// stroke 為 NO 時填滿（播放、暫停等實心圖示）。
+ (UIImage *)iconOfSize:(CGFloat)size stroke:(BOOL)stroke draw:(JXDraw)draw {
	UIGraphicsImageRenderer *r = [[UIGraphicsImageRenderer alloc] initWithSize:CGSizeMake(size, size)];
	UIImage *img = [r imageWithActions:^(UIGraphicsImageRendererContext *ctx) {
		CGContextScaleCTM(ctx.CGContext, size / 24.0, size / 24.0);
		UIBezierPath *p = [UIBezierPath bezierPath];
		p.lineWidth = 1.8;
		p.lineCapStyle = kCGLineCapRound;
		p.lineJoinStyle = kCGLineJoinRound;
		draw(p);
		[UIColor.blackColor set];
		if (stroke) [p stroke]; else [p fill];
	}];
	return [img imageWithRenderingMode:UIImageRenderingModeAlwaysTemplate];
}

+ (UIImage *)stroke:(JXDraw)draw { return [self iconOfSize:24 stroke:YES draw:draw]; }

static void line(UIBezierPath *p, CGFloat x1, CGFloat y1, CGFloat x2, CGFloat y2) {
	[p moveToPoint:CGPointMake(x1, y1)];
	[p addLineToPoint:CGPointMake(x2, y2)];
}

static void poly(UIBezierPath *p, NSArray<NSValue *> *pts, BOOL close) {
	[pts enumerateObjectsUsingBlock:^(NSValue *v, NSUInteger i, BOOL *stop) {
		if (i == 0) [p moveToPoint:v.CGPointValue]; else [p addLineToPoint:v.CGPointValue];
	}];
	if (close) [p closePath];
}

static NSValue *P(CGFloat x, CGFloat y) { return [NSValue valueWithCGPoint:CGPointMake(x, y)]; }

static void circle(UIBezierPath *p, CGFloat cx, CGFloat cy, CGFloat r) {
	[p appendPath:[UIBezierPath bezierPathWithOvalInRect:CGRectMake(cx - r, cy - r, r * 2, r * 2)]];
}

static void rrect(UIBezierPath *p, CGFloat x, CGFloat y, CGFloat w, CGFloat h, CGFloat r) {
	[p appendPath:[UIBezierPath bezierPathWithRoundedRect:CGRectMake(x, y, w, h) cornerRadius:r]];
}

+ (UIImage *)home {
	return [self stroke:^(UIBezierPath *p) {
		poly(p, @[P(3, 11), P(12, 4), P(21, 11), P(21, 21), P(15, 21), P(15, 15), P(9, 15), P(9, 21), P(3, 21)], YES);
	}];
}

+ (UIImage *)film {
	return [self stroke:^(UIBezierPath *p) {
		rrect(p, 3, 4, 18, 16, 2);
		line(p, 7, 4, 7, 20); line(p, 17, 4, 17, 20);
		line(p, 3, 9, 7, 9); line(p, 3, 15, 7, 15); line(p, 17, 9, 21, 9); line(p, 17, 15, 21, 15);
	}];
}

+ (UIImage *)tv {
	return [self stroke:^(UIBezierPath *p) {
		rrect(p, 3, 5, 18, 12, 2);
		line(p, 8, 21, 16, 21); line(p, 12, 17, 12, 21);
	}];
}

+ (UIImage *)list {
	return [self stroke:^(UIBezierPath *p) {
		line(p, 8, 6, 21, 6); line(p, 8, 12, 21, 12); line(p, 8, 18, 21, 18);
		line(p, 3.5, 6, 3.6, 6); line(p, 3.5, 12, 3.6, 12); line(p, 3.5, 18, 3.6, 18);
	}];
}

+ (UIImage *)folder {
	return [self stroke:^(UIBezierPath *p) {
		poly(p, @[P(3, 7), P(3, 19), P(21, 19), P(21, 8), P(11, 8), P(9, 5), P(3, 5)], YES);
	}];
}

+ (UIImage *)search {
	return [self stroke:^(UIBezierPath *p) {
		circle(p, 11, 11, 7);
		line(p, 20, 20, 16.5, 16.5);
	}];
}

+ (UIImage *)settings {
	return [self stroke:^(UIBezierPath *p) {
		line(p, 4, 6, 13, 6); line(p, 17, 6, 20, 6);
		line(p, 4, 12, 7, 12); line(p, 11, 12, 20, 12);
		line(p, 4, 18, 15, 18); line(p, 19, 18, 20, 18);
		circle(p, 15, 6, 2); circle(p, 9, 12, 2); circle(p, 17, 18, 2);
	}];
}

+ (UIImage *)back {
	return [self stroke:^(UIBezierPath *p) { poly(p, @[P(15, 5), P(8, 12), P(15, 19)], NO); }];
}

+ (UIImage *)check {
	return [self stroke:^(UIBezierPath *p) { poly(p, @[P(5, 12), P(10, 17), P(19, 7)], NO); }];
}

+ (UIImage *)close {
	return [self stroke:^(UIBezierPath *p) { line(p, 6, 6, 18, 18); line(p, 18, 6, 6, 18); }];
}

+ (UIImage *)subtitles {
	return [self stroke:^(UIBezierPath *p) {
		rrect(p, 3, 5, 18, 14, 2);
		line(p, 7, 13, 11, 13); line(p, 13, 13, 17, 13); line(p, 7, 16, 17, 16);
	}];
}

+ (UIImage *)audio {
	return [self stroke:^(UIBezierPath *p) {
		line(p, 4, 10, 4, 14); line(p, 8, 7, 8, 17); line(p, 12, 4, 12, 20); line(p, 16, 8, 16, 16); line(p, 20, 11, 20, 13);
	}];
}

+ (UIImage *)play { return [self playOfSize:24]; }
+ (UIImage *)pause { return [self pauseOfSize:24]; }

+ (UIImage *)playOfSize:(CGFloat)size {
	return [self iconOfSize:size stroke:NO draw:^(UIBezierPath *p) { poly(p, @[P(7, 4), P(20, 12), P(7, 20)], YES); }];
}

+ (UIImage *)pauseOfSize:(CGFloat)size {
	return [self iconOfSize:size stroke:NO draw:^(UIBezierPath *p) { rrect(p, 6, 4, 4, 16, 1); rrect(p, 14, 4, 4, 16, 1); }];
}

+ (UIImage *)rewind {
	return [self iconOfSize:24 stroke:NO draw:^(UIBezierPath *p) {
		poly(p, @[P(11, 6), P(3, 12), P(11, 18)], YES);
		poly(p, @[P(21, 6), P(13, 12), P(21, 18)], YES);
	}];
}

+ (UIImage *)forward {
	return [self iconOfSize:24 stroke:NO draw:^(UIBezierPath *p) {
		poly(p, @[P(3, 6), P(11, 12), P(3, 18)], YES);
		poly(p, @[P(13, 6), P(21, 12), P(13, 18)], YES);
	}];
}

@end
