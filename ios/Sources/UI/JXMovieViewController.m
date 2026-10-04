#import "JXMovieViewController.h"
#import "JXFormat.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXPlayerViewController.h"
#import "JXRootViewController.h"
#import "JXSettings.h"
#import "JXTheme.h"
#import "JXTrackPicker.h"
#import "UIImageView+JX.h"

/// 音軌／字幕選擇卡片：小標加目前選擇。
@interface JXPickCard : UIControl
@property (nonatomic, strong) UILabel *caption;
@property (nonatomic, strong) UILabel *value;
@end

@implementation JXPickCard
- (instancetype)initWithCaption:(NSString *)caption {
	if ((self = [super initWithFrame:CGRectZero])) {
		self.translatesAutoresizingMaskIntoConstraints = NO;
		self.backgroundColor = JXColorA(0x171A20, 0.85);
		self.layer.cornerRadius = 12;
		self.layer.borderWidth = 1;
		self.layer.borderColor = JXTheme.line.CGColor;
		_caption = [JXTheme labelWithSize:12 weight:UIFontWeightRegular color:JXTheme.textTertiary];
		_caption.text = caption;
		_value = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.text];
		[self addSubview:_caption];
		[self addSubview:_value];
		self.isAccessibilityElement = YES;
		self.accessibilityTraits = UIAccessibilityTraitButton;
		[NSLayoutConstraint activateConstraints:@[
			[self.heightAnchor constraintEqualToConstant:56],
			[self.widthAnchor constraintGreaterThanOrEqualToConstant:250],
			[_caption.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:16],
			[_caption.topAnchor constraintEqualToAnchor:self.topAnchor constant:9],
			[_value.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:16],
			[_value.trailingAnchor constraintEqualToAnchor:self.trailingAnchor constant:-16],
			[_value.topAnchor constraintEqualToAnchor:_caption.bottomAnchor constant:2],
		]];
	}
	return self;
}
- (void)setHighlighted:(BOOL)h { [super setHighlighted:h]; self.alpha = h ? 0.6 : 1; }
- (void)setText:(NSString *)text {
	self.value.text = text;
	self.accessibilityLabel = [NSString stringWithFormat:@"%@：%@", self.caption.text, text];
}
@end

@implementation JXMovieViewController {
	NSString *_itemId;
	JXJellyfin *_api;
	JXItem *_item;
	JXMediaInfo *_info;
	NSString *_subtitlePref;
	NSInteger _audio;     // JXAudioDefault 或 Jellyfin Index
	NSInteger _subtitle;  // JXSubtitleAuto、JXSubtitleOff 或 Index

	UIImageView *_backdrop;
	CAGradientLayer *_left, *_bottom;
	UIScrollView *_scroll;
	UIStackView *_column;
	UIActivityIndicatorView *_spinner;
	JXPickCard *_audioCard, *_subtitleCard;
}

- (instancetype)initWithItemId:(NSString *)itemId {
	if ((self = [super init])) {
		_itemId = itemId;
		_audio = JXAudioDefault;
		_subtitle = JXSubtitleAuto;
	}
	return self;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	_api = JXJellyfin.current;

	_backdrop = [[UIImageView alloc] init];
	_backdrop.translatesAutoresizingMaskIntoConstraints = NO;
	_backdrop.contentMode = UIViewContentModeScaleAspectFill;
	_backdrop.clipsToBounds = YES;
	UIView *scrims = [[UIView alloc] init];
	scrims.translatesAutoresizingMaskIntoConstraints = NO;
	_left = JXTheme.leftScrim;
	_bottom = JXTheme.bottomScrim;
	[scrims.layer addSublayer:_left];
	[scrims.layer addSublayer:_bottom];

	_scroll = [[UIScrollView alloc] init];
	_scroll.translatesAutoresizingMaskIntoConstraints = NO;
	_column = [[UIStackView alloc] init];
	_column.translatesAutoresizingMaskIntoConstraints = NO;
	_column.axis = UILayoutConstraintAxisVertical;
	_column.alignment = UIStackViewAlignmentLeading;
	_column.spacing = 14;

	UIButton *back = [JXTheme roundIconButton:JXIcons.back size:44 label:@"返回"];
	[back addTarget:self action:@selector(back) forControlEvents:UIControlEventTouchUpInside];
	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;

	for (UIView *v in @[_backdrop, scrims, _scroll, back, _spinner]) [self.view addSubview:v];
	[_scroll addSubview:_column];
	UILayoutGuide *safe = self.view.safeAreaLayoutGuide;
	[NSLayoutConstraint activateConstraints:@[
		[_backdrop.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_backdrop.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_backdrop.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_backdrop.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[scrims.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[scrims.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[scrims.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[scrims.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_scroll.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_scroll.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_scroll.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_scroll.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_column.leadingAnchor constraintEqualToAnchor:_scroll.leadingAnchor constant:48],
		[_column.topAnchor constraintEqualToAnchor:_scroll.topAnchor constant:96],
		[_column.bottomAnchor constraintEqualToAnchor:_scroll.bottomAnchor constant:-40],
		[_column.widthAnchor constraintEqualToConstant:600],
		[back.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor constant:32],
		[back.topAnchor constraintEqualToAnchor:safe.topAnchor constant:16],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
	]];

	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(reloadItem) name:JXPlaybackDidFinishNotification object:nil];
	[_spinner startAnimating];
	[self load];
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	_left.frame = self.view.bounds;
	_bottom.frame = self.view.bounds;
}

- (void)back { [self.navigationController popViewControllerAnimated:YES]; }

- (void)load {
	dispatch_group_t g = dispatch_group_create();
	__block NSError *err = nil;
	__weak typeof(self) weakSelf = self;
	dispatch_group_enter(g);
	[_api item:_itemId completion:^(JXItem *item, NSError *error) {
		typeof(self) s = weakSelf;
		if (s) s->_item = item;
		err = error;
		dispatch_group_leave(g);
	}];
	// 音軌、字幕清單與偏好拿不到時照樣能播，只是不能選
	dispatch_group_enter(g);
	[_api mediaInfo:_itemId completion:^(JXMediaInfo *info, NSError *error) {
		typeof(self) s = weakSelf;
		if (s) s->_info = info;
		dispatch_group_leave(g);
	}];
	dispatch_group_enter(g);
	[_api subtitleLanguagePreference:^(NSString *pref) {
		typeof(self) s = weakSelf;
		if (s) s->_subtitlePref = pref;
		dispatch_group_leave(g);
	}];
	dispatch_group_notify(g, dispatch_get_main_queue(), ^{
		typeof(self) s = weakSelf;
		if (!s) return;
		[s->_spinner stopAnimating];
		if (!s->_item) {
			UILabel *l = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.textSecondary];
			l.text = err.localizedDescription ?: @"載入失敗";
			[s->_column addArrangedSubview:l];
			return;
		}
		[s bind];
	});
}

/// 播放回來後只更新進度與已看狀態。
- (void)reloadItem {
	__weak typeof(self) weakSelf = self;
	[_api item:_itemId completion:^(JXItem *item, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s || !item) return;
		s->_item = item;
		[s bind];
	}];
}

- (void)bind {
	JXItem *it = _item;
	for (UIView *v in _column.arrangedSubviews) [v removeFromSuperview];
	if (!_backdrop.image) [_backdrop jx_setImageURL:[_api imageURL:it.backdrop maxWidth:1600]];

	UILabel *title = [JXTheme labelWithSize:46 weight:UIFontWeightBold color:JXTheme.text];
	title.numberOfLines = 2;
	title.text = it.name;
	UIStackView *titleBlock = [[UIStackView alloc] initWithArrangedSubviews:@[title]];
	titleBlock.axis = UILayoutConstraintAxisVertical;
	titleBlock.spacing = 4;
	if (it.originalTitle.length && ![it.originalTitle isEqualToString:it.name]) {
		UILabel *orig = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		orig.text = it.originalTitle;
		[titleBlock addArrangedSubview:orig];
	}
	[_column addArrangedSubview:titleBlock];

	UILabel *meta = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textBody];
	meta.numberOfLines = 2;
	meta.text = JXMetaLine(it, YES);
	[_column addArrangedSubview:meta];

	if (it.overview.length) {
		UILabel *overview = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.textBody];
		overview.numberOfLines = 6;
		NSMutableParagraphStyle *ps = [[NSMutableParagraphStyle alloc] init];
		ps.lineSpacing = 6;
		overview.attributedText = [[NSAttributedString alloc] initWithString:it.overview attributes:@{NSParagraphStyleAttributeName: ps}];
		[_column addArrangedSubview:overview];
		[overview.widthAnchor constraintEqualToAnchor:_column.widthAnchor].active = YES;
	}

	UIButton *play = [JXTheme primaryButton:it.resumable ? [@"繼續播放 " stringByAppendingString:JXFormatTicks(it.positionTicks)] : @"播放"
	                                   icon:[JXIcons playOfSize:18]];
	[play addTarget:self action:@selector(playResume) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *buttons = [[UIStackView alloc] initWithArrangedSubviews:@[play]];
	buttons.spacing = 12;
	if (it.resumable) {
		UIButton *restart = [JXTheme outlineButton:@"從頭播放"];
		[restart addTarget:self action:@selector(playFromStart) forControlEvents:UIControlEventTouchUpInside];
		[buttons addArrangedSubview:restart];
	}
	UIButton *played = [UIButton buttonWithType:UIButtonTypeSystem];
	played.translatesAutoresizingMaskIntoConstraints = NO;
	[played setImage:JXIcons.check forState:UIControlStateNormal];
	played.layer.cornerRadius = 24;
	played.layer.borderWidth = 1;
	played.layer.borderColor = (it.played ? JXTheme.accent : JXTheme.outline).CGColor;
	played.tintColor = it.played ? JXTheme.accent : JXTheme.text;
	played.accessibilityLabel = it.played ? @"標記為未看" : @"標記為已看";
	[played.widthAnchor constraintEqualToConstant:48].active = YES;
	[played.heightAnchor constraintEqualToConstant:48].active = YES;
	[played addTarget:self action:@selector(togglePlayed) forControlEvents:UIControlEventTouchUpInside];
	[buttons addArrangedSubview:played];
	[_column addArrangedSubview:buttons];
	[_column setCustomSpacing:8 afterView:buttons];

	if (it.resumable) {
		UIProgressView *progress = [[UIProgressView alloc] initWithProgressViewStyle:UIProgressViewStyleDefault];
		progress.progressTintColor = JXTheme.accent;
		progress.trackTintColor = JXTheme.progressTrack;
		progress.progress = JXProgress(it);
		[progress.widthAnchor constraintEqualToConstant:300].active = YES;
		UILabel *remaining = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		remaining.text = JXFormatRemaining(it);
		UIStackView *row = [[UIStackView alloc] initWithArrangedSubviews:@[progress, remaining]];
		row.spacing = 12;
		row.alignment = UIStackViewAlignmentCenter;
		[_column addArrangedSubview:row];
	}

	NSMutableArray *cards = [NSMutableArray array];
	if (_info.audio.count > 1) {
		_audioCard = [[JXPickCard alloc] initWithCaption:@"音軌"];
		[_audioCard addTarget:self action:@selector(pickAudio) forControlEvents:UIControlEventTouchUpInside];
		[cards addObject:_audioCard];
	}
	if ([_info.subtitles indexOfObjectPassingTest:^BOOL(JXSubtitleTrack *t, NSUInteger i, BOOL *stop) { return t.playable; }] != NSNotFound) {
		_subtitleCard = [[JXPickCard alloc] initWithCaption:@"字幕"];
		[_subtitleCard addTarget:self action:@selector(pickSubtitle) forControlEvents:UIControlEventTouchUpInside];
		[cards addObject:_subtitleCard];
	}
	if (cards.count) {
		UIStackView *row = [[UIStackView alloc] initWithArrangedSubviews:cards];
		row.spacing = 12;
		[_column addArrangedSubview:row];
		[self updateCards];
	}

	if (_info.videoDescription.length) {
		NSDictionary *heights = @{@"ipad-air1": @"1080p", @"zenpad10": @"800p"};
		NSString *target = heights[JXSettings.shared.profile] ?: JXSettings.shared.profile;
		UILabel *src = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textTertiary];
		src.text = [NSString stringWithFormat:@"片源 %@ · 在這台裝置轉成 %@ H.264 播放", _info.videoDescription, target];
		[_column addArrangedSubview:src];
	}
}

- (JXAudioTrack *)defaultAudio {
	for (JXAudioTrack *t in _info.audio) if (t.isDefault) return t;
	return _info.audio.firstObject;
}

- (JXSubtitleTrack *)effectiveSubtitle {
	if (_subtitle == JXSubtitleOff) return nil;
	if (_subtitle == JXSubtitleAuto) return [JXSubtitleChooser choose:_info.subtitles preferredLanguage:_subtitlePref locale:NSLocale.currentLocale];
	for (JXSubtitleTrack *t in _info.subtitles) if (t.index == _subtitle) return t;
	return nil;
}

- (void)updateCards {
	if (_audioCard) {
		JXAudioTrack *cur = nil;
		for (JXAudioTrack *t in _info.audio) if (t.index == _audio) cur = t;
		[_audioCard setText:(cur ?: self.defaultAudio).title ?: @"預設"];
	}
	if (_subtitleCard) {
		JXSubtitleTrack *t = self.effectiveSubtitle;
		NSString *label = t ? [JXTrackPicker subtitleLabel:t] : @"關閉";
		if (_subtitle == JXSubtitleAuto) label = [label stringByAppendingString:@"（自動選擇）"];
		[_subtitleCard setText:label];
	}
}

- (void)pickAudio {
	NSInteger cur = _audio == JXAudioDefault ? self.defaultAudio.index : _audio;
	__weak typeof(self) weakSelf = self;
	[JXTrackPicker pickAudioFrom:self source:_audioCard tracks:_info.audio current:cur onPick:^(JXAudioTrack *t) {
		typeof(self) s = weakSelf;
		if (!s) return;
		s->_audio = t.index;
		[s updateCards];
	}];
}

- (void)pickSubtitle {
	__weak typeof(self) weakSelf = self;
	[JXTrackPicker pickSubtitleFrom:self source:_subtitleCard tracks:_info.subtitles current:self.effectiveSubtitle onPick:^(JXSubtitleTrack *t) {
		typeof(self) s = weakSelf;
		if (!s) return;
		s->_subtitle = t ? t.index : JXSubtitleOff;
		[s updateCards];
	}];
}

- (void)playResume {
	[JXRoot() playItem:_item startTicks:_item.resumable ? _item.positionTicks : 0 audio:_audio subtitle:_subtitle];
}

- (void)playFromStart {
	[JXRoot() playItem:_item startTicks:0 audio:_audio subtitle:_subtitle];
}

- (void)togglePlayed {
	BOOL target = !_item.played;
	__weak typeof(self) weakSelf = self;
	[_api setPlayed:_itemId played:target completion:^(NSError *error) { [weakSelf reloadItem]; }];
}

@end
