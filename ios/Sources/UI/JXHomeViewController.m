#import "JXHomeViewController.h"
#import "JXCells.h"
#import "JXFormat.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXPlayerViewController.h"
#import "JXRootViewController.h"
#import "JXSeriesViewController.h"
#import "JXTheme.h"
#import "UIImageView+JX.h"

@implementation JXHomeViewController {
	UIScrollView *_scroll;
	UIStackView *_stack;
	UIActivityIndicatorView *_spinner;
	UILabel *_message;
	CAGradientLayer *_heroLeft, *_heroBottom;
	UIView *_heroImageHost;
	BOOL _loaded;
	JXItem *_heroItem;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	_scroll = [[UIScrollView alloc] init];
	_scroll.translatesAutoresizingMaskIntoConstraints = NO;
	_scroll.contentInsetAdjustmentBehavior = UIScrollViewContentInsetAdjustmentNever;
	_stack = [[UIStackView alloc] init];
	_stack.translatesAutoresizingMaskIntoConstraints = NO;
	_stack.axis = UILayoutConstraintAxisVertical;
	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;
	_message = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.textSecondary];
	_message.numberOfLines = 0;
	_message.textAlignment = NSTextAlignmentCenter;
	[self.view addSubview:_scroll];
	[_scroll addSubview:_stack];
	[self.view addSubview:_spinner];
	[self.view addSubview:_message];
	[NSLayoutConstraint activateConstraints:@[
		[_scroll.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_scroll.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_scroll.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_scroll.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_stack.leadingAnchor constraintEqualToAnchor:_scroll.leadingAnchor],
		[_stack.trailingAnchor constraintEqualToAnchor:_scroll.trailingAnchor],
		[_stack.topAnchor constraintEqualToAnchor:_scroll.topAnchor],
		[_stack.bottomAnchor constraintEqualToAnchor:_scroll.bottomAnchor constant:-32],
		[_stack.widthAnchor constraintEqualToAnchor:_scroll.widthAnchor],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
		[_message.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_message.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
		[_message.widthAnchor constraintLessThanOrEqualToConstant:520],
	]];
	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(load) name:JXPlaybackDidFinishNotification object:nil];
	[self load];
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	_heroLeft.frame = _heroImageHost.bounds;
	_heroBottom.frame = _heroImageHost.bounds;
}

- (void)load {
	JXJellyfin *api = JXJellyfin.current;
	if (!api) return;
	if (!_loaded) [_spinner startAnimating];
	__block NSArray *resume = @[], *nextUp = @[];
	__block NSError *firstError = nil;
	NSMutableDictionary<NSString *, NSArray *> *latest = [NSMutableDictionary dictionary];
	dispatch_group_t g = dispatch_group_create();
	void (^track)(NSError *) = ^(NSError *e) { if (e && !firstError) firstError = e; };

	dispatch_group_enter(g);
	[api resume:^(NSArray<JXItem *> *items, NSError *error) { resume = items ?: @[]; track(error); dispatch_group_leave(g); }];
	dispatch_group_enter(g);
	[api nextUpForSeries:nil limit:12 completion:^(NSArray<JXItem *> *items, NSError *error) { nextUp = items ?: @[]; track(error); dispatch_group_leave(g); }];
	__block NSArray<JXItem *> *views = @[];
	dispatch_group_enter(g);
	[api userViews:^(NSArray<JXItem *> *items, NSError *error) {
		track(error);
		views = [items filteredArrayUsingPredicate:[NSPredicate predicateWithBlock:^BOOL(JXItem *v, NSDictionary *b) {
			return [v.collectionType isEqualToString:@"movies"] || [v.collectionType isEqualToString:@"tvshows"];
		}]] ?: @[];
		for (JXItem *v in views) {
			dispatch_group_enter(g);
			[api latestInParent:v.itemId completion:^(NSArray<JXItem *> *its, NSError *e2) {
				latest[v.itemId] = its ?: @[];
				track(e2);
				dispatch_group_leave(g);
			}];
		}
		dispatch_group_leave(g);
	}];

	__weak typeof(self) weakSelf = self;
	dispatch_group_notify(g, dispatch_get_main_queue(), ^{
		typeof(self) s = weakSelf;
		if (!s) return;
		[s->_spinner stopAnimating];
		if (firstError && !s->_loaded) {
			s->_message.text = firstError.localizedDescription;
			return;
		}
		if (firstError) return;
		s->_loaded = YES;
		s->_message.text = nil;
		[s bindResume:resume nextUp:nextUp views:views latest:latest api:api];
	});
}

- (void)bindResume:(NSArray<JXItem *> *)resume nextUp:(NSArray<JXItem *> *)nextUp views:(NSArray<JXItem *> *)views
            latest:(NSDictionary<NSString *, NSArray *> *)latest api:(JXJellyfin *)api {
	for (UIView *v in _stack.arrangedSubviews) [v removeFromSuperview];
	JXItem *hero = resume.firstObject;
	if (!hero) for (JXItem *v in views) if ([v.collectionType isEqualToString:@"movies"] && [latest[v.itemId] count]) { hero = latest[v.itemId][0]; break; }
	if (hero) [_stack addArrangedSubview:[self heroFor:hero api:api]];

	void (^open)(JXItem *) = ^(JXItem *item) { [JXRoot() openItem:item]; };
	if (resume.count) {
		JXRowView *r = [[JXRowView alloc] initWithTitle:@"繼續觀看" wide:YES api:api items:resume];
		r.onSelect = open;
		[_stack addArrangedSubview:r];
	}
	if (nextUp.count) {
		JXRowView *r = [[JXRowView alloc] initWithTitle:@"下一集" wide:YES api:api items:nextUp];
		r.onSelect = open;
		[_stack addArrangedSubview:r];
	}
	for (JXItem *v in views) {
		NSArray *items = latest[v.itemId];
		if (!items.count) continue;
		JXRowView *r = [[JXRowView alloc] initWithTitle:[@"最新加入 · " stringByAppendingString:v.name] wide:NO api:api items:items];
		r.onSelect = open;
		[r setActionTitle:@"全部"];
		r.onAction = ^{ [JXRoot() showView:v]; };
		[_stack addArrangedSubview:r];
	}
	if (!_stack.arrangedSubviews.count) _message.text = @"媒體庫是空的";
}

- (UIView *)heroFor:(JXItem *)item api:(JXJellyfin *)api {
	UIView *hero = [[UIView alloc] init];
	hero.translatesAutoresizingMaskIntoConstraints = NO;
	hero.clipsToBounds = YES;
	[hero.heightAnchor constraintEqualToConstant:400].active = YES;

	UIImageView *image = [[UIImageView alloc] init];
	image.translatesAutoresizingMaskIntoConstraints = NO;
	image.contentMode = UIViewContentModeScaleAspectFill;
	image.clipsToBounds = YES;
	image.backgroundColor = JXTheme.surface;
	[image jx_setImageURL:[api imageURL:item.backdrop ?: item.wide maxWidth:1600]];
	UIView *host = [[UIView alloc] init];
	host.translatesAutoresizingMaskIntoConstraints = NO;
	_heroImageHost = host;
	_heroLeft = JXTheme.leftScrim;
	_heroBottom = JXTheme.bottomScrim;
	[host.layer addSublayer:_heroLeft];
	[host.layer addSublayer:_heroBottom];

	UILabel *eyebrow = [JXTheme labelWithSize:13 weight:UIFontWeightSemibold color:JXTheme.accent];
	eyebrow.text = item.resumable ? @"繼續觀看" : @"最新加入";
	UILabel *title = [JXTheme labelWithSize:38 weight:UIFontWeightBold color:JXTheme.text];
	title.numberOfLines = 2;
	BOOL episode = [item.type isEqualToString:@"Episode"];
	title.text = episode ? [NSString stringWithFormat:@"%@　%@", JXDisplayTitle(item), JXEpisodeLabel(item)] : item.name;
	UILabel *meta = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textBody];
	meta.text = JXMetaLine(item, YES);

	UIButton *play = [JXTheme primaryButton:item.resumable ? [@"繼續播放 " stringByAppendingString:JXFormatTicks(item.positionTicks)] : @"播放"
	                                   icon:[JXIcons playOfSize:18]];
	[play addTarget:self action:@selector(heroPlay) forControlEvents:UIControlEventTouchUpInside];
	UIButton *details = [JXTheme outlineButton:@"詳細資訊"];
	[details addTarget:self action:@selector(heroDetails) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *buttons = [[UIStackView alloc] initWithArrangedSubviews:@[play, details]];
	buttons.spacing = 12;

	NSMutableArray *col = [@[eyebrow, title, meta] mutableCopy];
	if (item.resumable) {
		UIProgressView *progress = [[UIProgressView alloc] initWithProgressViewStyle:UIProgressViewStyleDefault];
		progress.progressTintColor = JXTheme.accent;
		progress.trackTintColor = JXTheme.progressTrack;
		progress.progress = JXProgress(item);
		[progress.widthAnchor constraintEqualToConstant:260].active = YES;
		UILabel *remaining = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		remaining.text = JXFormatRemaining(item);
		UIStackView *pr = [[UIStackView alloc] initWithArrangedSubviews:@[progress, remaining]];
		pr.spacing = 12;
		pr.alignment = UIStackViewAlignmentCenter;
		[col addObject:pr];
	}
	[col addObject:buttons];
	UIStackView *text = [[UIStackView alloc] initWithArrangedSubviews:col];
	text.translatesAutoresizingMaskIntoConstraints = NO;
	text.axis = UILayoutConstraintAxisVertical;
	text.alignment = UIStackViewAlignmentLeading;
	text.spacing = 10;
	[text setCustomSpacing:16 afterView:col[col.count - 2]];

	[hero addSubview:image];
	[hero addSubview:host];
	[hero addSubview:text];
	[NSLayoutConstraint activateConstraints:@[
		[image.leadingAnchor constraintEqualToAnchor:hero.leadingAnchor],
		[image.trailingAnchor constraintEqualToAnchor:hero.trailingAnchor],
		[image.topAnchor constraintEqualToAnchor:hero.topAnchor],
		[image.bottomAnchor constraintEqualToAnchor:hero.bottomAnchor],
		[host.leadingAnchor constraintEqualToAnchor:hero.leadingAnchor],
		[host.trailingAnchor constraintEqualToAnchor:hero.trailingAnchor],
		[host.topAnchor constraintEqualToAnchor:hero.topAnchor],
		[host.bottomAnchor constraintEqualToAnchor:hero.bottomAnchor],
		[text.leadingAnchor constraintEqualToAnchor:hero.leadingAnchor constant:40],
		[text.bottomAnchor constraintEqualToAnchor:hero.bottomAnchor constant:-24],
		[text.widthAnchor constraintLessThanOrEqualToConstant:600],
	]];
	_heroItem = item;
	return hero;
}

- (void)heroPlay {
	JXItem *item = _heroItem;
	if (item) [JXRoot() playItem:item startTicks:item.resumable ? item.positionTicks : 0 audio:JXAudioDefault subtitle:JXSubtitleAuto];
}

- (void)heroDetails {
	JXItem *item = _heroItem;
	if (!item) return;
	// 集數的詳細資訊是所屬影集
	if ([item.type isEqualToString:@"Episode"] && item.seriesId) [JXRoot() push:[[JXSeriesViewController alloc] initWithSeries:item.seriesId season:item.seasonId]];
	else [JXRoot() openItem:item];
}

@end
