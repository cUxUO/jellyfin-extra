#import "JXSeriesViewController.h"
#import "JXChips.h"
#import "JXFormat.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXPlayerViewController.h"
#import "JXRootViewController.h"
#import "JXTheme.h"
#import "UIImageView+JX.h"

@interface JXEpisodeCell : UITableViewCell
- (void)configure:(JXItem *)ep api:(JXJellyfin *)api;
@end

@implementation JXEpisodeCell {
	UIImageView *_thumb;
	UIProgressView *_progress;
	UILabel *_badge, *_title, *_meta, *_overview;
}

- (instancetype)initWithStyle:(UITableViewCellStyle)style reuseIdentifier:(NSString *)rid {
	if ((self = [super initWithStyle:style reuseIdentifier:rid])) {
		self.backgroundColor = UIColor.clearColor;
		UIView *sel = [[UIView alloc] init];
		sel.backgroundColor = JXTheme.surface;
		self.selectedBackgroundView = sel;
		_thumb = [[UIImageView alloc] init];
		_thumb.contentMode = UIViewContentModeScaleAspectFill;
		_thumb.clipsToBounds = YES;
		_thumb.layer.cornerRadius = 8;
		_thumb.backgroundColor = JXTheme.placeholder;
		_progress = [[UIProgressView alloc] initWithProgressViewStyle:UIProgressViewStyleBar];
		_progress.progressTintColor = JXTheme.accent;
		_progress.trackTintColor = JXColorA(0, 0.5);
		_badge = [[UILabel alloc] init];
		_badge.text = @"已看";
		_badge.font = [JXTheme fontOfSize:11 weight:UIFontWeightBold];
		_badge.textColor = JXTheme.text;
		_badge.textAlignment = NSTextAlignmentCenter;
		_badge.backgroundColor = JXColorA(0x0E1014, 0.8);
		_badge.layer.cornerRadius = 4;
		_badge.clipsToBounds = YES;
		_title = [[UILabel alloc] init];
		_title.font = [JXTheme fontOfSize:17 weight:UIFontWeightMedium];
		_title.textColor = JXTheme.text;
		_meta = [[UILabel alloc] init];
		_meta.font = [JXTheme fontOfSize:13 weight:UIFontWeightRegular];
		_meta.textColor = JXTheme.textTertiary;
		_overview = [[UILabel alloc] init];
		_overview.font = [JXTheme fontOfSize:14 weight:UIFontWeightRegular];
		_overview.textColor = JXTheme.textSecondary;
		_overview.numberOfLines = 2;
		for (UIView *v in @[_thumb, _progress, _badge, _title, _meta, _overview]) [self.contentView addSubview:v];
	}
	return self;
}

- (void)layoutSubviews {
	[super layoutSubviews];
	CGFloat w = self.contentView.bounds.size.width;
	CGFloat x = 48, y = 12;
	_thumb.frame = CGRectMake(x, y, 192, 108);
	_progress.frame = CGRectMake(x + 4, y + 102, 184, 3);
	_badge.frame = CGRectMake(x + 192 - 46, y + 8, 38, 18);
	CGFloat tx = x + 192 + 20, tw = w - tx - 40;
	CGSize metaSize = [_meta sizeThatFits:CGSizeMake(tw, 20)];
	CGFloat titleW = MIN([_title sizeThatFits:CGSizeMake(tw, 22)].width, tw - (metaSize.width ? metaSize.width + 12 : 0));
	_title.frame = CGRectMake(tx, y + 2, titleW, 22);
	_meta.frame = CGRectMake(tx + titleW + 12, y + 5, metaSize.width, 18);
	CGSize os = [_overview sizeThatFits:CGSizeMake(tw, 60)];
	_overview.frame = CGRectMake(tx, y + 32, tw, MIN(os.height, 44));
}

- (void)configure:(JXItem *)ep api:(JXJellyfin *)api {
	[_thumb jx_setImageURL:[api imageURL:ep.wide maxWidth:384]];
	// 有些集數的名稱本身就是「第 N 集」，不要重複加
	NSInteger no = ep.indexNumber;
	_title.text = (no > 0 && ![ep.name containsString:@(no).stringValue]) ? [NSString stringWithFormat:@"%ld. %@", (long)no, ep.name] : ep.name;
	NSMutableArray *m = [NSMutableArray array];
	if (ep.runTimeTicks > 0) [m addObject:JXFormatDuration(ep.runTimeTicks)];
	if (ep.resumable) [m addObject:[@"看到 " stringByAppendingString:JXFormatTicks(ep.positionTicks)]];
	_meta.text = [m componentsJoinedByString:@" · "];
	_overview.text = ep.overview;
	_badge.hidden = !ep.played;
	_progress.hidden = !ep.resumable;
	_progress.progress = JXProgress(ep);
	self.accessibilityLabel = _title.text;
	[self setNeedsLayout];
}

@end

@interface JXSeriesViewController () <UITableViewDataSource, UITableViewDelegate>
@end

@implementation JXSeriesViewController {
	NSString *_seriesId, *_seasonId;
	JXJellyfin *_api;
	JXItem *_series, *_next;
	NSArray<JXItem *> *_seasons, *_episodes;
	UITableView *_table;
	UIView *_header;
	UIImageView *_backdrop;
	CAGradientLayer *_left, *_bottom;
	UIView *_scrimHost;
	UILabel *_title, *_meta, *_overview;
	UIButton *_play, *_watched;
	JXChips *_chips;
	UIActivityIndicatorView *_spinner;
	BOOL _loaded;
}

- (instancetype)initWithSeries:(NSString *)seriesId season:(NSString *)seasonId {
	if ((self = [super init])) {
		_seriesId = seriesId;
		_seasonId = seasonId;
	}
	return self;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	_api = JXJellyfin.current;

	_table = [[UITableView alloc] initWithFrame:CGRectZero style:UITableViewStylePlain];
	_table.translatesAutoresizingMaskIntoConstraints = NO;
	_table.backgroundColor = UIColor.clearColor;
	_table.separatorColor = JXColor(0x1E222A);
	_table.separatorInset = UIEdgeInsetsMake(0, 48, 0, 40);
	_table.rowHeight = 132;
	_table.dataSource = self;
	_table.delegate = self;
	_table.contentInsetAdjustmentBehavior = UIScrollViewContentInsetAdjustmentNever;
	[_table registerClass:JXEpisodeCell.class forCellReuseIdentifier:@"e"];
	_table.hidden = YES;

	UIButton *back = [JXTheme roundIconButton:JXIcons.back size:44 label:@"返回"];
	[back addTarget:self action:@selector(back) forControlEvents:UIControlEventTouchUpInside];
	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;
	[self.view addSubview:_table];
	[self.view addSubview:back];
	[self.view addSubview:_spinner];
	[NSLayoutConstraint activateConstraints:@[
		[_table.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_table.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_table.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_table.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[back.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor constant:32],
		[back.topAnchor constraintEqualToAnchor:self.view.safeAreaLayoutGuide.topAnchor constant:16],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
	]];
	[self buildHeader];
	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(load) name:JXPlaybackDidFinishNotification object:nil];
	[self load];
}

- (void)buildHeader {
	_header = [[UIView alloc] initWithFrame:CGRectMake(0, 0, 100, 386)];
	_backdrop = [[UIImageView alloc] init];
	_backdrop.translatesAutoresizingMaskIntoConstraints = NO;
	_backdrop.contentMode = UIViewContentModeScaleAspectFill;
	_backdrop.clipsToBounds = YES;
	_backdrop.backgroundColor = JXTheme.surface;
	_scrimHost = [[UIView alloc] init];
	_scrimHost.translatesAutoresizingMaskIntoConstraints = NO;
	_left = JXTheme.leftScrim;
	_bottom = JXTheme.bottomScrim;
	[_scrimHost.layer addSublayer:_left];
	[_scrimHost.layer addSublayer:_bottom];

	_title = [JXTheme labelWithSize:38 weight:UIFontWeightBold color:JXTheme.text];
	_meta = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textBody];
	_overview = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textSecondary];
	_overview.numberOfLines = 2;
	_play = [JXTheme primaryButton:@"播放" icon:[JXIcons playOfSize:18]];
	[_play addTarget:self action:@selector(playNext) forControlEvents:UIControlEventTouchUpInside];
	_watched = [JXTheme outlineButton:@"全部標記已看"];
	[_watched addTarget:self action:@selector(toggleWatched) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *buttons = [[UIStackView alloc] initWithArrangedSubviews:@[_play, _watched]];
	buttons.spacing = 12;
	UIStackView *info = [[UIStackView alloc] initWithArrangedSubviews:@[_title, _meta, _overview, buttons]];
	info.translatesAutoresizingMaskIntoConstraints = NO;
	info.axis = UILayoutConstraintAxisVertical;
	info.alignment = UIStackViewAlignmentLeading;
	info.spacing = 10;
	[info setCustomSpacing:14 afterView:_overview];

	_chips = [[JXChips alloc] init];
	_chips.contentInset = UIEdgeInsetsMake(0, 48, 0, 40);
	__weak typeof(self) weakSelf = self;
	_chips.onSelect = ^(NSInteger i) { [weakSelf selectSeason:i]; };

	for (UIView *v in @[_backdrop, _scrimHost, info, _chips]) [_header addSubview:v];
	[NSLayoutConstraint activateConstraints:@[
		[_backdrop.leadingAnchor constraintEqualToAnchor:_header.leadingAnchor],
		[_backdrop.trailingAnchor constraintEqualToAnchor:_header.trailingAnchor],
		[_backdrop.topAnchor constraintEqualToAnchor:_header.topAnchor],
		[_backdrop.heightAnchor constraintEqualToConstant:316],
		[_scrimHost.leadingAnchor constraintEqualToAnchor:_backdrop.leadingAnchor],
		[_scrimHost.trailingAnchor constraintEqualToAnchor:_backdrop.trailingAnchor],
		[_scrimHost.topAnchor constraintEqualToAnchor:_backdrop.topAnchor],
		[_scrimHost.bottomAnchor constraintEqualToAnchor:_backdrop.bottomAnchor],
		[info.leadingAnchor constraintEqualToAnchor:_header.leadingAnchor constant:48],
		[info.bottomAnchor constraintEqualToAnchor:_backdrop.bottomAnchor constant:-20],
		[info.widthAnchor constraintLessThanOrEqualToConstant:640],
		[_overview.widthAnchor constraintLessThanOrEqualToConstant:640],
		[_chips.topAnchor constraintEqualToAnchor:_backdrop.bottomAnchor constant:16],
		[_chips.leadingAnchor constraintEqualToAnchor:_header.leadingAnchor],
		[_chips.trailingAnchor constraintEqualToAnchor:_header.trailingAnchor],
	]];
	_table.tableHeaderView = _header;
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	_left.frame = _scrimHost.bounds;
	_bottom.frame = _scrimHost.bounds;
	if (_header.frame.size.width != _table.bounds.size.width) {
		_header.frame = CGRectMake(0, 0, _table.bounds.size.width, 386);
		_table.tableHeaderView = _header;
	}
}

- (void)back { [self.navigationController popViewControllerAnimated:YES]; }

- (void)load {
	if (!_loaded) [_spinner startAnimating];
	dispatch_group_t g = dispatch_group_create();
	__block JXItem *series = nil, *next = nil;
	__block NSArray *seasons = nil;
	__block NSError *err = nil;
	dispatch_group_enter(g);
	[_api item:_seriesId completion:^(JXItem *item, NSError *e) { series = item; if (e) err = e; dispatch_group_leave(g); }];
	dispatch_group_enter(g);
	[_api seasons:_seriesId completion:^(NSArray<JXItem *> *items, NSError *e) { seasons = items; if (e) err = e; dispatch_group_leave(g); }];
	// 「繼續」的對象：有看到一半的集數就是那集，否則是下一集，再不然是第一集
	dispatch_group_enter(g);
	[_api nextUpForSeries:_seriesId limit:1 completion:^(NSArray<JXItem *> *items, NSError *e) { next = items.firstObject; dispatch_group_leave(g); }];
	__weak typeof(self) weakSelf = self;
	dispatch_group_notify(g, dispatch_get_main_queue(), ^{
		typeof(self) s = weakSelf;
		if (!s) return;
		[s->_spinner stopAnimating];
		if (!series || err) {
			UIAlertController *a = [UIAlertController alertControllerWithTitle:nil message:err.localizedDescription ?: @"載入失敗" preferredStyle:UIAlertControllerStyleAlert];
			[a addAction:[UIAlertAction actionWithTitle:@"好" style:UIAlertActionStyleDefault handler:nil]];
			[s presentViewController:a animated:YES completion:nil];
			return;
		}
		s->_loaded = YES;
		s->_series = series;
		s->_seasons = seasons;
		s->_next = next;
		s->_table.hidden = NO;
		[s bindHeader];
		[s bindSeasons];
	});
}

- (void)bindHeader {
	JXItem *s = _series;
	if (!_backdrop.image) [_backdrop jx_setImageURL:[_api imageURL:s.backdrop maxWidth:1600]];
	_title.text = s.name;
	NSInteger seasonCount = 0;
	for (JXItem *x in _seasons) if (x.indexNumber > 0) seasonCount++;
	NSMutableArray *p = [NSMutableArray array];
	if (s.productionYear) [p addObject:@(s.productionYear).stringValue];
	if (seasonCount) [p addObject:[NSString stringWithFormat:@"%ld 季", (long)seasonCount]];
	if (s.officialRating.length) [p addObject:s.officialRating];
	if (s.communityRating > 0) [p addObject:[NSString stringWithFormat:@"★ %.1f", s.communityRating]];
	if (s.genres.count) [p addObject:[[s.genres subarrayWithRange:NSMakeRange(0, MIN(3, s.genres.count))] componentsJoinedByString:@" / "]];
	_meta.text = [p componentsJoinedByString:@" · "];
	_overview.text = s.overview;
	[_watched setTitle:s.played ? @"標記為未看" : @"全部標記已看" forState:UIControlStateNormal];
	_play.hidden = _next == nil;
	if (_next) {
		NSString *t = _next.resumable
			? [NSString stringWithFormat:@"繼續 %@ · %@", JXEpisodeLabel(_next), JXFormatTicks(_next.positionTicks)]
			: [@"播放 " stringByAppendingString:JXEpisodeLabel(_next)];
		[_play setTitle:t forState:UIControlStateNormal];
	}
}

- (void)bindSeasons {
	// 預設季：參數指定的，否則「繼續」那集所在的季，否則第一個正片季
	NSInteger sel = NSNotFound;
	for (NSString *want in @[_seasonId ?: @"", _next.seasonId ?: @""]) {
		if (sel != NSNotFound || !want.length) continue;
		sel = [_seasons indexOfObjectPassingTest:^BOOL(JXItem *x, NSUInteger i, BOOL *stop) { return [x.itemId isEqualToString:want]; }];
	}
	if (sel == NSNotFound) sel = [_seasons indexOfObjectPassingTest:^BOOL(JXItem *x, NSUInteger i, BOOL *stop) { return x.indexNumber > 0; }];
	if (sel == NSNotFound && _seasons.count) sel = 0;
	NSMutableArray *titles = [NSMutableArray array];
	for (JXItem *x in _seasons) [titles addObject:x.childCount > 0 ? [NSString stringWithFormat:@"%@ · %ld 集", x.name, (long)x.childCount] : x.name];
	[_chips setTitles:titles];
	if (sel == NSNotFound) return;
	_chips.selectedIndex = sel;
	_seasonId = _seasons[sel].itemId;
	[self loadEpisodes];
}

- (void)selectSeason:(NSInteger)i {
	_seasonId = _seasons[i].itemId;
	[self loadEpisodes];
}

- (void)loadEpisodes {
	NSString *season = _seasonId;
	__weak typeof(self) weakSelf = self;
	[_api episodes:_seriesId season:season completion:^(NSArray<JXItem *> *items, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s || ![season isEqualToString:s->_seasonId]) return;
		s->_episodes = items ?: @[];
		[s->_table reloadData];
	}];
}

- (void)playNext {
	if (_next) [JXRoot() playItem:_next startTicks:_next.resumable ? _next.positionTicks : 0 audio:JXAudioDefault subtitle:JXSubtitleAuto];
}

- (void)toggleWatched {
	__weak typeof(self) weakSelf = self;
	[_api setPlayed:_seriesId played:!_series.played completion:^(NSError *error) { [weakSelf load]; }];
}

- (NSInteger)tableView:(UITableView *)tv numberOfRowsInSection:(NSInteger)section { return _episodes.count; }

- (UITableViewCell *)tableView:(UITableView *)tv cellForRowAtIndexPath:(NSIndexPath *)ip {
	JXEpisodeCell *c = [tv dequeueReusableCellWithIdentifier:@"e" forIndexPath:ip];
	[c configure:_episodes[ip.row] api:_api];
	return c;
}

- (void)tableView:(UITableView *)tv didSelectRowAtIndexPath:(NSIndexPath *)ip {
	[tv deselectRowAtIndexPath:ip animated:YES];
	JXItem *ep = _episodes[ip.row];
	[JXRoot() playItem:ep startTicks:ep.resumable ? ep.positionTicks : 0 audio:JXAudioDefault subtitle:JXSubtitleAuto];
}

@end
