#import "JXSettingsViewController.h"
#import "JXEndpoints.h"
#import "JXRootViewController.h"
#import "JXSettings.h"
#import "JXTheme.h"

@implementation JXSettingsViewController {
	UILabel *_xcodeStatus;
	UILabel *_subtitleSize;
}

- (UIView *)section:(NSString *)title rows:(NSArray<NSArray<NSString *> *> *)rows {
	UILabel *head = [JXTheme labelWithSize:13 weight:UIFontWeightMedium color:JXTheme.textTertiary];
	head.text = title;
	UIStackView *card = [[UIStackView alloc] init];
	card.axis = UILayoutConstraintAxisVertical;
	card.backgroundColor = JXTheme.surface;
	for (NSArray<NSString *> *r in rows) {
		UILabel *k = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.text];
		k.text = r[0];
		UILabel *v = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		v.text = r[1];
		v.textAlignment = NSTextAlignmentRight;
		v.lineBreakMode = NSLineBreakByTruncatingMiddle;
		if ([r[0] isEqualToString:@"轉碼伺服器"]) _xcodeStatus = v;
		if ([r[0] isEqualToString:@"字幕大小"]) _subtitleSize = v;
		UIStackView *row = [[UIStackView alloc] initWithArrangedSubviews:@[k, v]];
		row.layoutMarginsRelativeArrangement = YES;
		row.layoutMargins = UIEdgeInsetsMake(0, 18, 0, 18);
		row.spacing = 16;
		[k setContentHuggingPriority:UILayoutPriorityRequired forAxis:UILayoutConstraintAxisHorizontal];
		[row.heightAnchor constraintEqualToConstant:52].active = YES;
		if (v == _subtitleSize) {
			v.textColor = JXTheme.accent;
			[row addGestureRecognizer:[[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(pickSubtitleSize:)]];
		}
		[card addArrangedSubview:row];
	}
	// UIStackView 在 iOS 12 不畫背景，包一層
	UIView *bg = [[UIView alloc] init];
	bg.backgroundColor = JXTheme.surface;
	bg.layer.cornerRadius = 12;
	bg.clipsToBounds = YES;
	card.translatesAutoresizingMaskIntoConstraints = NO;
	[bg addSubview:card];
	[NSLayoutConstraint activateConstraints:@[
		[card.leadingAnchor constraintEqualToAnchor:bg.leadingAnchor],
		[card.trailingAnchor constraintEqualToAnchor:bg.trailingAnchor],
		[card.topAnchor constraintEqualToAnchor:bg.topAnchor],
		[card.bottomAnchor constraintEqualToAnchor:bg.bottomAnchor],
	]];
	UIStackView *s = [[UIStackView alloc] initWithArrangedSubviews:@[head, bg]];
	s.axis = UILayoutConstraintAxisVertical;
	s.spacing = 8;
	return s;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	JXSettings *st = JXSettings.shared;
	UILabel *title = [JXTheme labelWithSize:32 weight:UIFontWeightBold color:JXTheme.text];
	title.text = @"設定";
	NSString *version = [NSBundle.mainBundle objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"";

	UIButton *connection = [JXTheme outlineButton:@"變更連線設定"];
	[connection addTarget:self action:@selector(changeConnection) forControlEvents:UIControlEventTouchUpInside];
	UIButton *logout = [JXTheme outlineButton:@"登出"];
	[logout setTitleColor:JXColor(0xE57373) forState:UIControlStateNormal];
	[logout addTarget:self action:@selector(logout) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *buttons = [[UIStackView alloc] initWithArrangedSubviews:@[connection, logout]];
	buttons.spacing = 12;

	UIStackView *col = [[UIStackView alloc] initWithArrangedSubviews:@[
		title,
		[self section:@"帳號" rows:@[@[@"使用者", st.userName]]],
		[self section:@"連線" rows:@[
			@[@"Jellyfin（使用中）", JXEndpoints.jellyfinBase.absoluteString ?: @"—"],
			@[@"轉碼伺服器", @"檢查中…"],
			@[@"外部網址", st.external.length ? st.external : @"未設定"],
		]],
		[self section:@"播放" rows:@[
			@[@"裝置規格", st.profile],
			@[@"字幕大小", JXSettings.subtitleSizeLabels[st.subtitleSize]],
		]],
		buttons,
		[self section:@"關於" rows:@[@[@"版本", [@"Jellyfin Extra " stringByAppendingString:version]]]],
	]];
	col.translatesAutoresizingMaskIntoConstraints = NO;
	col.axis = UILayoutConstraintAxisVertical;
	col.spacing = 24;
	[col setCustomSpacing:28 afterView:title];
	UIScrollView *scroll = [[UIScrollView alloc] init];
	scroll.translatesAutoresizingMaskIntoConstraints = NO;
	[self.view addSubview:scroll];
	[scroll addSubview:col];
	[NSLayoutConstraint activateConstraints:@[
		[scroll.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[scroll.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[scroll.topAnchor constraintEqualToAnchor:self.view.safeAreaLayoutGuide.topAnchor],
		[scroll.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[col.topAnchor constraintEqualToAnchor:scroll.topAnchor constant:16],
		[col.bottomAnchor constraintEqualToAnchor:scroll.bottomAnchor constant:-32],
		[col.leadingAnchor constraintEqualToAnchor:scroll.leadingAnchor constant:32],
		[col.widthAnchor constraintEqualToConstant:600],
	]];

	__weak typeof(self) weakSelf = self;
	[JXEndpoints resolveXcode:^(NSURL *base) {
		typeof(self) s = weakSelf;
		if (s) s->_xcodeStatus.text = base ? base.absoluteString : @"離線（改為直接播放）";
	}];
}

/// 文字字幕大小；播放中也可在字幕面板調整，兩邊共用同一個值。
- (void)pickSubtitleSize:(UITapGestureRecognizer *)g {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:@"字幕大小" message:@"只影響文字字幕；播放中也可在字幕面板調整"
	                                                    preferredStyle:UIAlertControllerStyleActionSheet];
	NSArray<NSString *> *labels = JXSettings.subtitleSizeLabels;
	NSInteger current = JXSettings.shared.subtitleSize;
	__weak typeof(self) weakSelf = self;
	for (NSInteger i = 0; i < (NSInteger)labels.count; i++) {
		NSString *title = i == current ? [labels[i] stringByAppendingString:@" ✓"] : labels[i];
		[a addAction:[UIAlertAction actionWithTitle:title style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) {
			JXSettings.shared.subtitleSize = i;
			typeof(self) s = weakSelf;
			if (s) s->_subtitleSize.text = labels[i];
		}]];
	}
	[a addAction:[UIAlertAction actionWithTitle:@"取消" style:UIAlertActionStyleCancel handler:nil]];
	// iPad 的 action sheet 要指定來源位置
	a.popoverPresentationController.sourceView = g.view;
	a.popoverPresentationController.sourceRect = _subtitleSize.frame;
	[self presentViewController:a animated:YES completion:nil];
}

- (void)changeConnection { [JXRoot() presentLogin]; }

- (void)logout {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:@"登出？" message:nil preferredStyle:UIAlertControllerStyleAlert];
	[a addAction:[UIAlertAction actionWithTitle:@"取消" style:UIAlertActionStyleCancel handler:nil]];
	[a addAction:[UIAlertAction actionWithTitle:@"登出" style:UIAlertActionStyleDestructive handler:^(UIAlertAction *x) { [JXRoot() logout]; }]];
	[self presentViewController:a animated:YES completion:nil];
}

@end
