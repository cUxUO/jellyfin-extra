#import "JXItem.h"

@implementation JXImageRef
+ (instancetype)refWithItem:(NSString *)itemId type:(NSString *)type tag:(NSString *)tag {
	JXImageRef *r = [[JXImageRef alloc] init];
	r.itemId = itemId;
	r.type = type;
	r.tag = tag;
	return r;
}
@end

static NSString *S(NSDictionary *o, NSString *key) {
	id v = o[key];
	return [v isKindOfClass:NSString.class] ? v : nil;
}

static NSInteger I(NSDictionary *o, NSString *key) {
	id v = o[key];
	return [v isKindOfClass:NSNumber.class] ? [v integerValue] : 0;
}

static long long L(NSDictionary *o, NSString *key) {
	id v = o[key];
	return [v isKindOfClass:NSNumber.class] ? [v longLongValue] : 0;
}

@implementation JXItem

- (BOOL)playable {
	return !self.isFolder && [@[@"Movie", @"Episode", @"Video", @"MusicVideo"] containsObject:self.type ?: @""];
}

- (BOOL)resumable {
	return self.positionTicks > 0 && !self.played;
}

+ (instancetype)itemWithJSON:(NSDictionary *)o {
	JXItem *it = [[JXItem alloc] init];
	it.itemId = S(o, @"Id");
	it.name = S(o, @"Name") ?: @"";
	it.type = S(o, @"Type") ?: @"";
	it.isFolder = [o[@"IsFolder"] boolValue];
	it.collectionType = S(o, @"CollectionType");
	it.seriesId = S(o, @"SeriesId");
	it.seriesName = S(o, @"SeriesName");
	it.seasonId = S(o, @"SeasonId");
	it.indexNumber = I(o, @"IndexNumber");
	it.parentIndexNumber = I(o, @"ParentIndexNumber");
	it.productionYear = I(o, @"ProductionYear");
	it.runTimeTicks = L(o, @"RunTimeTicks");
	it.childCount = I(o, @"ChildCount");
	it.overview = S(o, @"Overview");
	it.originalTitle = S(o, @"OriginalTitle");
	it.officialRating = S(o, @"OfficialRating");
	id rating = o[@"CommunityRating"];
	it.communityRating = [rating isKindOfClass:NSNumber.class] ? [rating doubleValue] : 0;
	id genres = o[@"Genres"];
	it.genres = [genres isKindOfClass:NSArray.class] ? genres : @[];

	NSDictionary *ud = [o[@"UserData"] isKindOfClass:NSDictionary.class] ? o[@"UserData"] : nil;
	it.positionTicks = L(ud, @"PlaybackPositionTicks");
	it.played = [ud[@"Played"] boolValue];
	it.unplayedCount = I(ud, @"UnplayedItemCount");

	NSDictionary *tags = [o[@"ImageTags"] isKindOfClass:NSDictionary.class] ? o[@"ImageTags"] : @{};
	NSArray *backdropTags = [o[@"BackdropImageTags"] isKindOfClass:NSArray.class] ? o[@"BackdropImageTags"] : @[];
	JXImageRef *ownPrimary = tags[@"Primary"] ? [JXImageRef refWithItem:it.itemId type:@"Primary" tag:tags[@"Primary"]] : nil;
	JXImageRef *ownThumb = tags[@"Thumb"] ? [JXImageRef refWithItem:it.itemId type:@"Thumb" tag:tags[@"Thumb"]] : nil;
	JXImageRef *ownBackdrop = backdropTags.count ? [JXImageRef refWithItem:it.itemId type:@"Backdrop" tag:backdropTags[0]] : nil;
	JXImageRef *parentBackdrop = nil;
	NSString *pb = S(o, @"ParentBackdropItemId");
	if (pb) {
		NSArray *pt = [o[@"ParentBackdropImageTags"] isKindOfClass:NSArray.class] ? o[@"ParentBackdropImageTags"] : @[];
		parentBackdrop = [JXImageRef refWithItem:pb type:@"Backdrop" tag:pt.firstObject];
	}
	JXImageRef *seriesPoster = (it.seriesId && S(o, @"SeriesPrimaryImageTag"))
		? [JXImageRef refWithItem:it.seriesId type:@"Primary" tag:S(o, @"SeriesPrimaryImageTag")] : nil;

	BOOL episode = [it.type isEqualToString:@"Episode"];
	it.poster = episode ? (seriesPoster ?: ownPrimary) : ownPrimary;
	it.wide = episode ? (ownPrimary ?: parentBackdrop) : (ownThumb ?: ownBackdrop ?: ownPrimary);
	it.backdrop = ownBackdrop ?: parentBackdrop;
	return it;
}

+ (NSArray<JXItem *> *)itemsWithJSON:(NSArray *)a {
	NSMutableArray *out = [NSMutableArray array];
	for (id o in a) if ([o isKindOfClass:NSDictionary.class]) [out addObject:[self itemWithJSON:o]];
	return out;
}

@end
