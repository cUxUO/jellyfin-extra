#import "JXTracks.h"

@implementation JXAudioTrack
@end

@implementation JXSubtitleTrack
- (BOOL)isImage {
	return [@[@"pgssub", @"hdmv_pgs_subtitle", @"dvdsub", @"dvd_subtitle", @"dvbsub", @"dvb_subtitle", @"xsub"]
	           containsObject:self.codec.lowercaseString ?: @""];
}
- (BOOL)playable { return !self.isImage || !self.isExternal; }
@end

@implementation JXMediaInfo

+ (instancetype)infoWithPlaybackInfo:(NSDictionary *)json {
	NSArray *sources = json[@"MediaSources"];
	NSDictionary *src = [sources isKindOfClass:NSArray.class] ? sources.firstObject : nil;
	if (![src isKindOfClass:NSDictionary.class]) return nil;
	JXMediaInfo *info = [[JXMediaInfo alloc] init];
	info.mediaSourceId = src[@"Id"];
	NSMutableArray *audio = [NSMutableArray array], *subs = [NSMutableArray array];
	NSDictionary *video = nil;
	for (NSDictionary *s in src[@"MediaStreams"]) {
		if (![s isKindOfClass:NSDictionary.class]) continue;
		NSString *type = s[@"Type"];
		NSString *title = [s[@"DisplayTitle"] isKindOfClass:NSString.class] ? s[@"DisplayTitle"] : ([s[@"Codec"] isKindOfClass:NSString.class] ? s[@"Codec"] : @"");
		if ([type isEqualToString:@"Audio"]) {
			JXAudioTrack *t = [[JXAudioTrack alloc] init];
			t.index = [s[@"Index"] integerValue];
			t.title = title;
			t.isDefault = [s[@"IsDefault"] boolValue];
			[audio addObject:t];
		} else if ([type isEqualToString:@"Subtitle"]) {
			JXSubtitleTrack *t = [[JXSubtitleTrack alloc] init];
			t.index = [s[@"Index"] integerValue];
			t.title = title;
			t.language = [s[@"Language"] isKindOfClass:NSString.class] ? s[@"Language"] : nil;
			t.codec = [s[@"Codec"] isKindOfClass:NSString.class] ? s[@"Codec"] : @"";
			t.isExternal = [s[@"IsExternal"] boolValue];
			t.isDefault = [s[@"IsDefault"] boolValue];
			t.isForced = [s[@"IsForced"] boolValue];
			[subs addObject:t];
		} else if ([type isEqualToString:@"Video"] && !video) {
			video = s;
		}
	}
	info.audio = audio;
	info.subtitles = subs;
	info.videoDescription = [self describeVideo:video];
	return info;
}

+ (NSString *)describeVideo:(NSDictionary *)v {
	if (!v) return nil;
	NSInteger w = [v[@"Width"] integerValue], h = [v[@"Height"] integerValue];
	NSString *res = (w >= 3200 || h >= 2000) ? @"4K" : (h >= 1000 || w >= 1800) ? @"1080p" : (h >= 700) ? @"720p" : (h > 0 ? [NSString stringWithFormat:@"%ldp", (long)h] : nil);
	NSString *rangeType = [v[@"VideoRangeType"] isKindOfClass:NSString.class] ? v[@"VideoRangeType"] : @"";
	NSString *range = nil;
	if ([rangeType hasPrefix:@"DOVI"]) range = @"Dolby Vision";
	else if (rangeType.length && ![rangeType isEqualToString:@"SDR"] && ![rangeType isEqualToString:@"Unknown"]) range = rangeType;
	NSMutableArray *parts = [NSMutableArray array];
	if (res) [parts addObject:res];
	if (range) [parts addObject:range];
	else {
		NSString *codec = [v[@"Codec"] isKindOfClass:NSString.class] ? [v[@"Codec"] uppercaseString] : nil;
		if (codec.length) [parts addObject:codec];
		NSInteger depth = [v[@"BitDepth"] integerValue];
		if (depth > 8) [parts addObject:[NSString stringWithFormat:@"%ld-bit", (long)depth]];
	}
	return [parts componentsJoinedByString:@" "];
}

@end

@implementation JXSubtitleChooser

static BOOL matches(NSString *pattern, NSString *s) {
	if (!s.length) return NO;
	return [s rangeOfString:pattern options:NSRegularExpressionSearch | NSCaseInsensitiveSearch].location != NSNotFound;
}

+ (JXSubtitleTrack *)choose:(NSArray<JXSubtitleTrack *> *)tracks preferredLanguage:(NSString *)pref locale:(NSLocale *)locale {
	NSSet *chineseCodes = [NSSet setWithArray:@[@"zho", @"chi", @"zh", @"cmn", @"yue"]];
	NSString *chineseTitle = @"中文|繁|简|簡|chinese|\\b(cht|chs|tc|sc)\\b";
	NSString *traditional = @"繁|正體|traditional|big5|\\b(cht|tc)\\b";
	NSString *simplified = @"简|簡體|simplified|\\b(chs|sc|gb)\\b";
	NSString *partial = @"signs|songs|forced|特效|歌詞|歌词|\\bsdh\\b";

	NSString *lang = pref.length ? pref.lowercaseString : [locale objectForKey:NSLocaleLanguageCode];
	// 裝置語言是兩字母碼（zh），Jellyfin 偏好是三字母碼（chi）；兩者都收
	BOOL wantChinese = [chineseCodes containsObject:lang ?: @""];
	NSString *region = [[locale objectForKey:NSLocaleCountryCode] uppercaseString] ?: @"";
	// 繁體中文環境的 locale 常是 zh-Hant 不帶地區；腳本碼也算
	NSString *script = [locale objectForKey:NSLocaleScriptCode] ?: @"";
	BOOL preferTraditional = [@[@"TW", @"HK", @"MO"] containsObject:region] || [script isEqualToString:@"Hant"];

	JXSubtitleTrack *best = nil;
	NSInteger bestScore = NSIntegerMin;
	for (JXSubtitleTrack *t in tracks) {
		if (!t.playable || t.isImage) continue;
		NSString *code = t.language.lowercaseString ?: @"";
		BOOL match = wantChinese ? ([chineseCodes containsObject:code] || matches(chineseTitle, t.title)) : [code isEqualToString:lang];
		if (!match) continue;
		NSInteger s = 0;
		if (wantChinese) {
			BOOL trad = matches(traditional, t.title), simp = matches(simplified, t.title);
			if (preferTraditional && trad) s += 4;
			if (!preferTraditional && simp) s += 4;
			if (preferTraditional && simp && !trad) s -= 1;
		}
		if (t.isDefault) s += 1;
		if (matches(partial, t.title)) s -= 3;
		if (s > bestScore) { best = t; bestScore = s; }
	}
	if (best) return best;
	for (JXSubtitleTrack *t in tracks) if (t.playable && t.isForced) return t;
	for (JXSubtitleTrack *t in tracks) if (t.playable && t.isDefault) return t;
	return nil;
}

@end
