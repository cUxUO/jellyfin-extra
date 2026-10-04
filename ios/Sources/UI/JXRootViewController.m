#import "JXRootViewController.h"
#import "JXEndpoints.h"
#import "JXFolderViewController.h"
#import "JXHomeViewController.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXLibraryViewController.h"
#import "JXLoginViewController.h"
#import "JXMovieViewController.h"
#import "JXPlayerViewController.h"
#import "JXSeriesViewController.h"
#import "JXSettings.h"
#import "JXSettingsViewController.h"
#import "JXTheme.h"

NSString *const JXPlaybackDidFinishNotification = @"JXPlaybackDidFinish";

JXRootViewController *JXRoot(void) {
	return (JXRootViewController *)UIApplication.sharedApplication.keyWindow.rootViewController;
}

/// 導覽列的一項：圖示加文字，選取時琥珀色並加底色。
@interface JXRailButton : UIControl
@property (nonatomic, strong) UIImageView *icon;
@property (nonatomic, strong) UILabel *label;
@end

@implementation JXRailButton
- (instancetype)initWithTitle:(NSString *)title icon:(UIImage *)icon {
	if ((self = [super initWithFrame:CGRectZero])) {
		self.translatesAutoresizingMaskIntoConstraints = NO;
		self.layer.cornerRadius = 14;
		_icon = [[UIImageView alloc] initWithImage:icon];
		_icon.translatesAutoresizingMaskIntoConstraints = NO;
		_label = [JXTheme labelWithSize:12 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		_label.text = title;
		_label.textAlignment = NSTextAlignmentCenter;
		[self addSubview:_icon];
		[self addSubview:_label];
		self.accessibilityLabel = title;
		self.accessibilityTraits = UIAccessibilityTraitButton;
		self.isAccessibilityElement = YES;
		[NSLayoutConstraint activateConstraints:@[
			[self.widthAnchor constraintEqualToConstant:72],
			[self.heightAnchor constraintEqualToConstant:62],
			[_icon.centerXAnchor constraintEqualToAnchor:self.centerXAnchor],
			[_icon.topAnchor constraintEqualToAnchor:self.topAnchor constant:9],
			[_label.topAnchor constraintEqualToAnchor:_icon.bottomAnchor constant:4],
			[_label.leadingAnchor constraintEqualToAnchor:self.leadingAnchor],
			[_label.trailingAnchor constraintEqualToAnchor:self.trailingAnchor],
		]];
		self.selected = NO;
	}
	return self;
}

- (void)setSelected:(BOOL)selected {
	[super setSelected:selected];
	UIColor *c = selected ? JXTheme.accent : JXTheme.textSecondary;
	self.icon.tintColor = c;
	self.label.textColor = c;
	self.label.font = [JXTheme fontOfSize:12 weight:selected ? UIFontWeightSemibold : UIFontWeightRegular];
	self.backgroundColor = selected ? JXTheme.surfaceHigh : UIColor.clearColor;
	self.accessibilityTraits = selected ? (UIAccessibilityTraitButton | UIAccessibilityTraitSelected) : UIAccessibilityTraitButton;
}
@end

@interface JXRootViewController ()
@property (nonatomic, strong) UIStackView *railStack;
@property (nonatomic, strong) UINavigationController *nav;
@property (nonatomic, strong) UIActivityIndicatorView *spinner;
@property (nonatomic, copy) NSArray<JXItem *> *views;
@property (nonatomic, strong) NSMutableArray<JXRailButton *> *buttons;
@property (nonatomic, copy) NSArray<NSString *> *rootKeys;
@property (nonatomic) BOOL started;
@end

@implementation JXRootViewController

- (UIStatusBarStyle)preferredStatusBarStyle { return UIStatusBarStyleLightContent; }

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;

	UIView *rail = [[UIView alloc] init];
	rail.translatesAutoresizingMaskIntoConstraints = NO;
	UIView *divider = [[UIView alloc] init];
	divider.translatesAutoresizingMaskIntoConstraints = NO;
	divider.backgroundColor = JXColor(0x1E222A);

	UIView *logo = [[UIView alloc] init];
	logo.translatesAutoresizingMaskIntoConstraints = NO;
	logo.backgroundColor = JXTheme.accent;
	logo.layer.cornerRadius = 12;
	UIImageView *logoIcon = [[UIImageView alloc] initWithImage:[JXIcons playOfSize:20]];
	logoIcon.translatesAutoresizingMaskIntoConstraints = NO;
	logoIcon.tintColor = JXTheme.onAccent;
	[logo addSubview:logoIcon];

	self.railStack = [[UIStackView alloc] init];
	self.railStack.translatesAutoresizingMaskIntoConstraints = NO;
	self.railStack.axis = UILayoutConstraintAxisVertical;
	self.railStack.spacing = 4;
	self.railStack.alignment = UIStackViewAlignmentCenter;

	self.nav = [[UINavigationController alloc] init];
	self.nav.navigationBarHidden = YES;
	self.nav.interactivePopGestureRecognizer.delegate = nil; // 隱藏導覽列時仍可從左緣滑動返回
	[self addChildViewController:self.nav];
	self.nav.view.translatesAutoresizingMaskIntoConstraints = NO;

	self.spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	self.spinner.translatesAutoresizingMaskIntoConstraints = NO;
	self.spinner.color = JXTheme.accent;

	[self.view addSubview:rail];
	[self.view addSubview:divider];
	[self.view addSubview:self.nav.view];
	[self.view addSubview:self.spinner];
	[rail addSubview:logo];
	[rail addSubview:self.railStack];
	[self.nav didMoveToParentViewController:self];

	UILayoutGuide *safe = self.view.safeAreaLayoutGuide;
	[NSLayoutConstraint activateConstraints:@[
		[rail.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[rail.topAnchor constraintEqualToAnchor:safe.topAnchor],
		[rail.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[rail.widthAnchor constraintEqualToConstant:88],
		[divider.leadingAnchor constraintEqualToAnchor:rail.trailingAnchor],
		[divider.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[divider.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[divider.widthAnchor constraintEqualToConstant:1],
		[self.nav.view.leadingAnchor constraintEqualToAnchor:divider.trailingAnchor],
		[self.nav.view.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[self.nav.view.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[self.nav.view.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[logo.topAnchor constraintEqualToAnchor:rail.topAnchor constant:16],
		[logo.centerXAnchor constraintEqualToAnchor:rail.centerXAnchor],
		[logo.widthAnchor constraintEqualToConstant:44],
		[logo.heightAnchor constraintEqualToConstant:44],
		[logoIcon.centerXAnchor constraintEqualToAnchor:logo.centerXAnchor],
		[logoIcon.centerYAnchor constraintEqualToAnchor:logo.centerYAnchor],
		[self.railStack.topAnchor constraintEqualToAnchor:logo.bottomAnchor constant:20],
		[self.railStack.centerXAnchor constraintEqualToAnchor:rail.centerXAnchor],
		[self.spinner.centerXAnchor constraintEqualToAnchor:self.nav.view.centerXAnchor],
		[self.spinner.centerYAnchor constraintEqualToAnchor:self.nav.view.centerYAnchor],
	]];
	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(foreground) name:UIApplicationWillEnterForegroundNotification object:nil];
}

- (void)viewDidAppear:(BOOL)animated {
	[super viewDidAppear:animated];
	if (self.started) return;
	self.started = YES;
	if (JXSettings.shared.loggedIn) [self setup]; else [self presentLogin];
}

/// 從背景回來時位址可能變了（例如換網路），下次請求前重新探測。
- (void)foreground {
	if (JXSettings.shared.loggedIn) [JXEndpoints resolveJellyfin:^(NSURL *base) {}];
}

- (void)presentLogin {
	JXLoginViewController *login = [[JXLoginViewController alloc] init];
	__weak typeof(self) weakSelf = self;
	login.onLoggedIn = ^{ [weakSelf dismissViewControllerAnimated:YES completion:^{ [weakSelf setup]; }]; };
	if (JXSettings.shared.loggedIn && self.views) login.onCancel = ^{ [weakSelf dismissViewControllerAnimated:YES completion:nil]; };
	login.modalPresentationStyle = UIModalPresentationFullScreen;
	[self presentViewController:login animated:YES completion:nil];
}

- (void)setup {
	[self.spinner startAnimating];
	__weak typeof(self) weakSelf = self;
	[JXEndpoints resolveJellyfin:^(NSURL *base) {
		if (!base) {
			[weakSelf.spinner stopAnimating];
			[weakSelf alert:@"連不到 Jellyfin（內網與外部網址都失敗）" retry:YES];
			return;
		}
		[[[JXJellyfin alloc] initWithBase:base] userViews:^(NSArray<JXItem *> *items, NSError *error) {
			[weakSelf.spinner stopAnimating];
			if (error.code == 401) { [weakSelf logout]; return; }
			if (error) { [weakSelf alert:error.localizedDescription retry:YES]; return; }
			weakSelf.views = items;
			[weakSelf buildRail];
			[weakSelf selectKey:@"home"];
		}];
	}];
}

- (void)alert:(NSString *)message retry:(BOOL)retry {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:nil message:message preferredStyle:UIAlertControllerStyleAlert];
	__weak typeof(self) weakSelf = self;
	if (retry) [a addAction:[UIAlertAction actionWithTitle:@"重試" style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) { [weakSelf setup]; }]];
	[a addAction:[UIAlertAction actionWithTitle:@"連線設定" style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) { [weakSelf presentLogin]; }]];
	[self presentViewController:a animated:YES completion:nil];
}

- (void)buildRail {
	for (UIView *v in self.railStack.arrangedSubviews) [v removeFromSuperview];
	self.buttons = [NSMutableArray array];
	NSMutableArray *keys = [NSMutableArray array];
	void (^add)(NSString *, NSString *, UIImage *) = ^(NSString *key, NSString *title, UIImage *icon) {
		JXRailButton *b = [[JXRailButton alloc] initWithTitle:title icon:icon];
		b.tag = keys.count;
		[b addTarget:self action:@selector(railTapped:) forControlEvents:UIControlEventTouchUpInside];
		[self.railStack addArrangedSubview:b];
		[self.buttons addObject:b];
		[keys addObject:key];
	};
	add(@"home", @"首頁", JXIcons.home);
	NSInteger i = 0;
	for (JXItem *v in self.views) {
		if (i >= 4) break; // 導覽列放不下太多媒體庫
		UIImage *icon = [v.collectionType isEqualToString:@"movies"] ? JXIcons.film
			: [v.collectionType isEqualToString:@"tvshows"] ? JXIcons.tv
			: [v.collectionType isEqualToString:@"playlists"] ? JXIcons.list : JXIcons.folder;
		// 播放清單庫在 Jellyfin 預設叫「Playlists」，改用中文
		NSString *title = [v.collectionType isEqualToString:@"playlists"] ? @"清單" : v.name;
		add([NSString stringWithFormat:@"view:%ld", (long)i], title, icon);
		i++;
	}
	add(@"search", @"搜尋", JXIcons.search);
	add(@"settings", @"設定", JXIcons.settings);
	self.rootKeys = keys;
}

- (void)railTapped:(JXRailButton *)sender {
	[self selectKey:self.rootKeys[sender.tag]];
}

- (void)selectKey:(NSString *)key {
	NSInteger idx = [self.rootKeys indexOfObject:key];
	[self.buttons enumerateObjectsUsingBlock:^(JXRailButton *b, NSUInteger i, BOOL *stop) { b.selected = (i == idx); }];
	UIViewController *root;
	if ([key isEqualToString:@"home"]) root = [[JXHomeViewController alloc] init];
	else if ([key isEqualToString:@"search"]) root = [JXFolderViewController search];
	else if ([key isEqualToString:@"settings"]) root = [[JXSettingsViewController alloc] init];
	else root = [[JXLibraryViewController alloc] initWithView:self.views[[[key substringFromIndex:5] integerValue]]];
	[self.nav setViewControllers:@[root] animated:NO];
}

- (void)showView:(JXItem *)view {
	NSUInteger i = [self.views indexOfObjectPassingTest:^BOOL(JXItem *v, NSUInteger idx, BOOL *stop) { return [v.itemId isEqualToString:view.itemId]; }];
	if (i != NSNotFound && i < 4) [self selectKey:[NSString stringWithFormat:@"view:%ld", (long)i]];
	else [self push:[[JXLibraryViewController alloc] initWithView:view]];
}

- (void)push:(UIViewController *)vc {
	[self.nav pushViewController:vc animated:YES];
}

- (void)openItem:(JXItem *)item {
	NSString *t = item.type;
	if ([@[@"Movie", @"Video", @"MusicVideo"] containsObject:t]) [self push:[[JXMovieViewController alloc] initWithItemId:item.itemId]];
	else if ([t isEqualToString:@"Series"]) [self push:[[JXSeriesViewController alloc] initWithSeries:item.itemId season:nil]];
	else if ([t isEqualToString:@"Season"]) { if (item.seriesId) [self push:[[JXSeriesViewController alloc] initWithSeries:item.seriesId season:item.itemId]]; }
	else if ([t isEqualToString:@"Episode"]) [self playItem:item startTicks:item.resumable ? item.positionTicks : 0 audio:JXAudioDefault subtitle:JXSubtitleAuto];
	else if ([t isEqualToString:@"CollectionFolder"] || [t isEqualToString:@"UserView"]) [self showView:item];
	else [self push:[JXFolderViewController folder:item]];
}

- (void)playItem:(JXItem *)item startTicks:(long long)ticks audio:(NSInteger)audio subtitle:(NSInteger)subtitle {
	JXPlayerViewController *p = [[JXPlayerViewController alloc] initWithItem:item startTicks:ticks audio:audio subtitle:subtitle];
	p.modalPresentationStyle = UIModalPresentationFullScreen;
	[self presentViewController:p animated:YES completion:nil];
}

- (void)logout {
	[JXSettings.shared logout];
	self.started = YES;
	[self presentLogin];
}

@end
