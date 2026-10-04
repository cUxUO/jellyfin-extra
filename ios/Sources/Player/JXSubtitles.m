#import "JXSubtitles.h"

@implementation JXCue
@end

@implementation JXSubtitles

/// 00:01:02.345 或 01:02.345
static BOOL parseTime(NSString *s, double *out) {
	NSArray *parts = [[s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet] componentsSeparatedByString:@":"];
	if (parts.count < 2 || parts.count > 3) return NO;
	double v = 0;
	for (NSString *p in parts) v = v * 60 + [[p stringByReplacingOccurrencesOfString:@"," withString:@"."] doubleValue];
	*out = v;
	return YES;
}

+ (NSArray<JXCue *> *)parseVTT:(NSData *)data {
	NSString *s = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
	if (!s) return @[];
	s = [s stringByReplacingOccurrencesOfString:@"\r\n" withString:@"\n"];
	s = [s stringByReplacingOccurrencesOfString:@"\r" withString:@"\n"];
	NSRegularExpression *tags = [NSRegularExpression regularExpressionWithPattern:@"<[^>]*>" options:0 error:nil];
	NSMutableArray<JXCue *> *cues = [NSMutableArray array];
	for (NSString *block in [s componentsSeparatedByString:@"\n\n"]) {
		NSArray<NSString *> *lines = [block componentsSeparatedByString:@"\n"];
		NSUInteger i = 0;
		while (i < lines.count && [lines[i] rangeOfString:@"-->"].location == NSNotFound) i++;
		if (i >= lines.count) continue; // 標頭、NOTE、STYLE 區塊
		NSArray *timing = [lines[i] componentsSeparatedByString:@"-->"];
		NSString *endPart = [[timing[1] stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet] componentsSeparatedByString:@" "].firstObject;
		double start, end;
		if (!parseTime(timing[0], &start) || !parseTime(endPart, &end)) continue;
		NSMutableArray *text = [NSMutableArray array];
		for (NSUInteger j = i + 1; j < lines.count; j++) {
			NSString *line = [tags stringByReplacingMatchesInString:lines[j] options:0 range:NSMakeRange(0, lines[j].length) withTemplate:@""];
			line = [[[line stringByReplacingOccurrencesOfString:@"&amp;" withString:@"&"]
			         stringByReplacingOccurrencesOfString:@"&lt;" withString:@"<"] stringByReplacingOccurrencesOfString:@"&gt;" withString:@">"];
			line = [line stringByReplacingOccurrencesOfString:@"&nbsp;" withString:@" "];
			line = [line stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
			if (line.length) [text addObject:line];
		}
		if (!text.count) continue;
		JXCue *c = [[JXCue alloc] init];
		c.start = start;
		c.end = end;
		c.text = [text componentsJoinedByString:@"\n"];
		[cues addObject:c];
	}
	// ASS 轉出的 VTT 不一定依時間排序
	[cues sortUsingComparator:^NSComparisonResult(JXCue *a, JXCue *b) { return a.start < b.start ? NSOrderedAscending : a.start > b.start ? NSOrderedDescending : NSOrderedSame; }];
	return cues;
}

+ (NSString *)textAt:(double)t cues:(NSArray<JXCue *> *)cues {
	// 二分搜尋第一個 start > t 的位置，往回找仍在顯示中的 cue
	NSUInteger lo = 0, hi = cues.count;
	while (lo < hi) {
		NSUInteger mid = (lo + hi) / 2;
		if (cues[mid].start <= t) lo = mid + 1; else hi = mid;
	}
	NSMutableArray *active = [NSMutableArray array];
	// 長 cue 可能比前面好幾個短 cue 更早開始，往回看一段距離
	for (NSInteger i = (NSInteger)lo - 1, seen = 0; i >= 0 && seen < 40; i--, seen++) {
		JXCue *c = cues[i];
		if (c.end > t && ![active containsObject:c.text]) [active insertObject:c.text atIndex:0];
	}
	return active.count ? [active componentsJoinedByString:@"\n"] : nil;
}

@end
