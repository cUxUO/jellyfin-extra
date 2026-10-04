#import "JXJellyfin.h"
#import "JXEndpoints.h"
#import "JXHTTP.h"
#import "JXSettings.h"

static NSString *const kListFields = @"PrimaryImageAspectRatio,Overview,Genres,CommunityRating,OfficialRating,ChildCount";

@implementation JXJellyfin

+ (instancetype)current {
	NSURL *base = JXEndpoints.jellyfinBase;
	return base ? [[JXJellyfin alloc] initWithBase:base] : nil;
}

- (instancetype)initWithBase:(NSURL *)base {
	if ((self = [super init])) _base = base;
	return self;
}

- (NSString *)token { return JXSettings.shared.token; }
- (NSString *)userId { return JXSettings.shared.userId; }

- (NSURL *)url:(NSString *)path query:(NSDictionary<NSString *, NSString *> *)query {
	NSURLComponents *c = [NSURLComponents componentsWithURL:[NSURL URLWithString:path relativeToURL:self.base].absoluteURL resolvingAgainstBaseURL:NO];
	NSMutableArray *items = [NSMutableArray array];
	[query enumerateKeysAndObjectsUsingBlock:^(NSString *k, NSString *v, BOOL *stop) {
		[items addObject:[NSURLQueryItem queryItemWithName:k value:v]];
	}];
	if (items.count) c.queryItems = items;
	return c.URL;
}

- (void)items:(NSURL *)url completion:(JXItemsCompletion)completion {
	[JXHTTP getJSON:url token:self.token completion:^(id json, NSError *error) {
		if (error) { completion(nil, error); return; }
		id arr = [json isKindOfClass:NSDictionary.class] ? json[@"Items"] : json;
		completion([JXItem itemsWithJSON:[arr isKindOfClass:NSArray.class] ? arr : @[]], nil);
	}];
}

- (void)authenticateUser:(NSString *)user password:(NSString *)password
              completion:(void (^)(NSString *, NSString *, NSString *, NSError *))completion {
	NSURL *u = [self url:@"Users/AuthenticateByName" query:nil];
	[JXHTTP request:@"POST" url:u body:@{@"Username": user ?: @"", @"Pw": password ?: @""} token:nil timeout:0
	     completion:^(NSData *data, NSError *error) {
		if (error) { completion(nil, nil, nil, error); return; }
		NSDictionary *o = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
		NSDictionary *u2 = [o isKindOfClass:NSDictionary.class] ? o[@"User"] : nil;
		NSString *token = [o isKindOfClass:NSDictionary.class] ? o[@"AccessToken"] : nil;
		if (!token.length || ![u2 isKindOfClass:NSDictionary.class]) {
			completion(nil, nil, nil, [NSError errorWithDomain:JXHTTPErrorDomain code:0 userInfo:@{NSLocalizedDescriptionKey: @"登入回應格式不對"}]);
			return;
		}
		completion(token, u2[@"Id"], u2[@"Name"], nil);
	}];
}

- (void)userViews:(JXItemsCompletion)completion {
	[self items:[self url:@"UserViews" query:@{@"userId": self.userId}] completion:completion];
}

- (void)resume:(JXItemsCompletion)completion {
	[self items:[self url:@"UserItems/Resume" query:@{@"userId": self.userId, @"limit": @"12", @"mediaTypes": @"Video", @"fields": kListFields}]
	 completion:completion];
}

- (void)nextUpForSeries:(NSString *)seriesId limit:(NSInteger)limit completion:(JXItemsCompletion)completion {
	NSMutableDictionary *q = [@{@"userId": self.userId, @"limit": @(limit).stringValue, @"fields": kListFields} mutableCopy];
	if (seriesId) q[@"seriesId"] = seriesId;
	[self items:[self url:@"Shows/NextUp" query:q] completion:completion];
}

- (void)latestInParent:(NSString *)parentId completion:(JXItemsCompletion)completion {
	[self items:[self url:@"Items/Latest" query:@{@"userId": self.userId, @"parentId": parentId, @"limit": @"16", @"fields": kListFields}]
	 completion:completion];
}

- (void)library:(JXItem *)view sort:(JXSort)sort genre:(NSString *)genre
     completion:(void (^)(NSArray<JXItem *> *, NSInteger, NSError *))completion {
	NSArray *sorts = @[@[@"DateCreated,SortName", @"Descending"], @[@"SortName", @"Ascending"],
	                   @[@"ProductionYear,SortName", @"Descending"], @[@"CommunityRating,SortName", @"Descending"]];
	NSMutableDictionary *q = [@{@"userId": self.userId, @"parentId": view.itemId, @"sortBy": sorts[sort][0],
	                            @"sortOrder": sorts[sort][1], @"fields": kListFields} mutableCopy];
	NSString *types = [view.collectionType isEqualToString:@"movies"] ? @"Movie" : [view.collectionType isEqualToString:@"tvshows"] ? @"Series" : nil;
	if (types) { q[@"recursive"] = @"true"; q[@"includeItemTypes"] = types; }
	if (genre) q[@"genres"] = genre;
	[JXHTTP getJSON:[self url:@"Items" query:q] token:self.token completion:^(id json, NSError *error) {
		if (error) { completion(nil, 0, error); return; }
		NSArray *items = [JXItem itemsWithJSON:json[@"Items"]];
		NSInteger total = [json[@"TotalRecordCount"] isKindOfClass:NSNumber.class] ? [json[@"TotalRecordCount"] integerValue] : items.count;
		completion(items, total, nil);
	}];
}

- (void)genresInParent:(NSString *)parentId completion:(void (^)(NSArray<NSString *> *, NSError *))completion {
	[JXHTTP getJSON:[self url:@"Genres" query:@{@"userId": self.userId, @"parentId": parentId, @"sortBy": @"SortName"}]
	          token:self.token completion:^(id json, NSError *error) {
		NSMutableArray *names = [NSMutableArray array];
		for (NSDictionary *g in json[@"Items"]) if ([g[@"Name"] isKindOfClass:NSString.class]) [names addObject:g[@"Name"]];
		completion(names, error);
	}];
}

- (void)search:(NSString *)term completion:(JXItemsCompletion)completion {
	[self items:[self url:@"Items" query:@{@"userId": self.userId, @"searchTerm": term, @"recursive": @"true",
	                                       @"includeItemTypes": @"Movie,Series,Episode", @"limit": @"60", @"fields": kListFields}]
	 completion:completion];
}

- (void)children:(JXItem *)parent completion:(JXItemsCompletion)completion {
	if ([parent.type isEqualToString:@"Series"]) { [self seasons:parent.itemId completion:completion]; return; }
	if ([parent.type isEqualToString:@"Season"]) { [self episodes:parent.seriesId ?: parent.itemId season:parent.itemId completion:completion]; return; }
	if ([parent.type isEqualToString:@"Playlist"]) {
		[self items:[self url:[NSString stringWithFormat:@"Playlists/%@/Items", parent.itemId] query:@{@"userId": self.userId, @"fields": kListFields}]
		 completion:completion];
		return;
	}
	[self items:[self url:@"Items" query:@{@"userId": self.userId, @"parentId": parent.itemId, @"sortBy": @"IsFolder,SortName",
	                                       @"sortOrder": @"Ascending", @"fields": kListFields}]
	 completion:completion];
}

- (void)seasons:(NSString *)seriesId completion:(JXItemsCompletion)completion {
	[self items:[self url:[NSString stringWithFormat:@"Shows/%@/Seasons", seriesId] query:@{@"userId": self.userId, @"fields": kListFields}]
	 completion:completion];
}

- (void)episodes:(NSString *)seriesId season:(NSString *)seasonId completion:(JXItemsCompletion)completion {
	[self items:[self url:[NSString stringWithFormat:@"Shows/%@/Episodes", seriesId]
	                query:@{@"userId": self.userId, @"seasonId": seasonId, @"fields": kListFields}]
	 completion:completion];
}

- (void)item:(NSString *)itemId completion:(JXItemCompletion)completion {
	[JXHTTP getJSON:[self url:[NSString stringWithFormat:@"Items/%@", itemId] query:@{@"userId": self.userId}]
	          token:self.token completion:^(id json, NSError *error) {
		completion([json isKindOfClass:NSDictionary.class] ? [JXItem itemWithJSON:json] : nil, error);
	}];
}

- (void)setPlayed:(NSString *)itemId played:(BOOL)played completion:(JXErrorCompletion)completion {
	NSURL *u = [self url:[NSString stringWithFormat:@"UserPlayedItems/%@", itemId] query:@{@"userId": self.userId}];
	[JXHTTP request:played ? @"POST" : @"DELETE" url:u body:played ? @{} : nil token:self.token timeout:0
	     completion:^(NSData *data, NSError *error) { completion(error); }];
}

- (void)mediaInfo:(NSString *)itemId completion:(void (^)(JXMediaInfo *, NSError *))completion {
	[JXHTTP getJSON:[self url:[NSString stringWithFormat:@"Items/%@/PlaybackInfo", itemId] query:@{@"userId": self.userId}]
	          token:self.token completion:^(id json, NSError *error) {
		completion([json isKindOfClass:NSDictionary.class] ? [JXMediaInfo infoWithPlaybackInfo:json] : nil, error);
	}];
}

- (void)subtitleLanguagePreference:(void (^)(NSString *))completion {
	[JXHTTP getJSON:[self url:@"Users/Me" query:nil] token:self.token completion:^(id json, NSError *error) {
		NSString *pref = json[@"Configuration"][@"SubtitleLanguagePreference"];
		completion([pref isKindOfClass:NSString.class] && pref.length ? pref : nil);
	}];
}

- (NSURLSessionDataTask *)subtitleVTT:(NSString *)itemId source:(NSString *)sourceId index:(NSInteger)index
                           completion:(void (^)(NSData *, NSError *))completion {
	NSURL *u = [self url:[NSString stringWithFormat:@"Videos/%@/%@/Subtitles/%ld/0/Stream.vtt", itemId, sourceId, (long)index] query:nil];
	// 中途放棄會讓 Jellyfin 停掉抽取，下次又得從頭讀，所以給到一小時
	return [JXHTTP request:@"GET" url:u body:nil token:self.token timeout:3600 completion:completion];
}

- (NSURL *)imageURL:(JXImageRef *)ref maxWidth:(NSInteger)maxWidth {
	if (!ref) return nil;
	NSMutableDictionary *q = [@{@"maxWidth": @(maxWidth).stringValue, @"quality": @"85"} mutableCopy];
	if (ref.tag) q[@"tag"] = ref.tag;
	return [self url:[NSString stringWithFormat:@"Items/%@/Images/%@", ref.itemId, ref.type] query:q];
}

- (NSURL *)directStreamURL:(NSString *)itemId {
	return [self url:[NSString stringWithFormat:@"Videos/%@/stream", itemId] query:@{@"static": @"true"}];
}

- (void)reportPlayback:(NSString *)what item:(NSString *)itemId session:(NSString *)playSessionId
              position:(long long)ticks paused:(BOOL)paused method:(NSString *)method completion:(JXErrorCompletion)completion {
	NSMutableDictionary *body = [@{@"ItemId": itemId, @"PlaySessionId": playSessionId, @"PositionTicks": @(ticks)} mutableCopy];
	if (![what isEqualToString:@"Stopped"]) {
		body[@"IsPaused"] = @(paused);
		body[@"PlayMethod"] = method;
		body[@"CanSeek"] = @YES;
	}
	NSString *path = [what isEqualToString:@"Start"] ? @"Sessions/Playing" : [@"Sessions/Playing/" stringByAppendingString:what];
	[JXHTTP request:@"POST" url:[self url:path query:nil] body:body token:self.token timeout:0
	     completion:^(NSData *data, NSError *error) { if (completion) completion(error); }];
}

@end
