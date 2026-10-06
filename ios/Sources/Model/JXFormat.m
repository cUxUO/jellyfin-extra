#import "JXFormat.h"
#import "JXItem.h"

NSString *JXFormatTime(double seconds) {
	long total = seconds > 0 ? (long)seconds : 0;
	long h = total / 3600, m = total % 3600 / 60, s = total % 60;
	return h > 0 ? [NSString stringWithFormat:@"%ld:%02ld:%02ld", h, m, s] : [NSString stringWithFormat:@"%ld:%02ld", m, s];
}

NSString *JXFormatTicks(long long ticks) {
	return JXFormatTime((double)ticks / JX_TICKS_PER_SECOND);
}

NSString *JXFormatDuration(long long ticks) {
	long minutes = (long)((ticks + 30 * JX_TICKS_PER_SECOND) / (60 * JX_TICKS_PER_SECOND));
	long h = minutes / 60, m = minutes % 60;
	if (h > 0 && m > 0) return [NSString stringWithFormat:@"%ld 小時 %ld 分", h, m];
	if (h > 0) return [NSString stringWithFormat:@"%ld 小時", h];
	return [NSString stringWithFormat:@"%ld 分", m];
}

NSString *JXFormatRemaining(JXItem *item) {
	long long rest = item.runTimeTicks - item.positionTicks;
	return [@"剩 " stringByAppendingString:JXFormatDuration(rest > 0 ? rest : 0)];
}

float JXProgress(JXItem *item) {
	if (item.runTimeTicks <= 0) return 0;
	return MIN(1.0f, MAX(0.0f, (float)((double)item.positionTicks / item.runTimeTicks)));
}

NSString *JXEpisodeLabel(JXItem *item) {
	NSString *ep = item.indexNumber > 0 ? [NSString stringWithFormat:@"第 %ld 集", (long)item.indexNumber] : item.name;
	NSInteger season = item.parentIndexNumber;
	return (season > 1) ? [NSString stringWithFormat:@"S%ld · %@", (long)season, ep] : ep;
}

NSString *JXDisplayTitle(JXItem *item) {
	return [item.type isEqualToString:@"Episode"] ? (item.seriesName ?: item.name) : item.name;
}

NSString *JXMetaLine(JXItem *item, BOOL includeGenres) {
	NSMutableArray *p = [NSMutableArray array];
	if (item.productionYear > 0) [p addObject:@(item.productionYear).stringValue];
	if (item.runTimeTicks > 0 && ![item.type isEqualToString:@"Series"]) [p addObject:JXFormatDuration(item.runTimeTicks)];
	if (item.officialRating.length) [p addObject:item.officialRating];
	if (item.communityRating > 0) [p addObject:[NSString stringWithFormat:@"★ %.1f", item.communityRating]];
	if (includeGenres && item.genres.count) {
		NSArray *g = item.genres.count > 3 ? [item.genres subarrayWithRange:NSMakeRange(0, 3)] : item.genres;
		[p addObject:[g componentsJoinedByString:@" / "]];
	}
	return [p componentsJoinedByString:@" · "];
}

NSString *JXResolutionLabel(NSInteger width, NSInteger height) {
	// 編碼器會把尺寸對齊到偶數或 16 的倍數，留一點餘裕
	static const NSInteger tiers[] = {360, 480, 720, 1080, 1440, 2160};
	NSInteger tier = height;
	for (size_t i = 0; i < sizeof(tiers) / sizeof(tiers[0]); i++) {
		if (width <= tiers[i] * 16 / 9 + 2 && height <= tiers[i] + 8) {
			tier = tiers[i];
			break;
		}
	}
	BOOL standard = height == tier && width >= tier * 16 / 9 - 2; // 剛好是 16:9 的檔位尺寸
	if (width <= 0 || standard) return [NSString stringWithFormat:@"%ldp", (long)tier];
	return [NSString stringWithFormat:@"%ldp · %ld×%ld", (long)tier, (long)width, (long)height];
}
