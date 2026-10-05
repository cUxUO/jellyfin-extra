#import "JXPlaybackLog.h"
#import "JXPlayerViewController.h"
#import <AVFoundation/AVFoundation.h>
#import "JXEndpoints.h"
#import "JXFormat.h"
#import "JXHTTP.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXRootViewController.h"
#import "JXSettings.h"
#import "JXSubtitles.h"
#import "JXTheme.h"
#import "JXXcode.h"

static void *kStatusContext = &kStatusContext;
static void *kTimeControlContext = &kTimeControlContext;
static const double kSeekStep = 10;
/// 片長以 Jellyfin 記錄為準，實際檔案可能短一點點，最後一段失敗時當作播完。
static const double kEndTolerance = 10;

@interface JXVideoView : UIView
@property (nonatomic, readonly) AVPlayerLayer *playerLayer;
@end

@implementation JXVideoView
+ (Class)layerClass { return AVPlayerLayer.class; }
- (AVPlayerLayer *)playerLayer { return (AVPlayerLayer *)self.layer; }
@end

/// 面板的一列：分組標題或選項。
@interface JXPanelRow : NSObject
@property (nonatomic, copy) NSString *label;
@property (nonatomic, copy) NSString *note;
@property (nonatomic) BOOL header;
@property (nonatomic) BOOL selected;
@property (nonatomic, copy) void (^onPick)(void);
@end

@implementation JXPanelRow
+ (instancetype)header:(NSString *)label {
	JXPanelRow *r = [[self alloc] init];
	r.label = label;
	r.header = YES;
	return r;
}
+ (instancetype)option:(NSString *)label note:(NSString *)note selected:(BOOL)selected onPick:(void (^)(void))onPick {
	JXPanelRow *r = [[self alloc] init];
	r.label = label;
	r.note = note;
	r.selected = selected;
	r.onPick = onPick;
	return r;
}
@end

@interface JXPlayerViewController () <UITableViewDataSource, UITableViewDelegate, UIGestureRecognizerDelegate>
@end

@implementation JXPlayerViewController {
	JXItem *_item;
	long long _startTicks;
	NSInteger _requestedAudio, _requestedSubtitle;

	AVPlayer *_player;
	AVPlayerItem *_observedItem;
	id _timeObserver;
	JXJellyfin *_jf;
	JXXcode *_xcode;
	JXXcodeSession *_session;
	JXMediaInfo *_info;
	JXSubtitleTrack *_currentSubtitle;
	NSInteger _currentAudio; // -1 表示未知或沒有
	NSURLSessionDataTask *_subtitleTask;
	NSArray<JXCue *> *_cues;

	NSString *_playMethod;
	NSString *_playSessionId;
	BOOL _reportedStart, _finishing, _failed, _scrubbing, _controlsVisible;
	NSTimer *_progressTimer, *_hideTimer;
	double _pendingSeek; // 新項目就緒後要確認的位置，-1 表示沒有
	double _suspendedAt; // 進背景時關掉轉碼 session 的位置，-1 表示沒有；回前景時從這裡重建
	NSInteger _suspendedBurn; // 關掉的 session 燒錄的字幕，重建時沿用
	BOOL _restoring;

	JXVideoView *_video;
	UIView *_controls;
	CAGradientLayer *_shade;
	UILabel *_title, *_statusLine;
	UIButton *_audioButton, *_subtitleButton, *_playPause;
	UISlider *_slider;
	UIProgressView *_buffer;
	UILabel *_elapsed, *_remaining;
	UILabel *_status;
	UIActivityIndicatorView *_spinner;
	UILabel *_subtitleLabel, *_subtitleOutline, *_subtitleHint;
	NSLayoutConstraint *_subtitleBottom;
	UIView *_panelScrim, *_panel;
	NSLayoutConstraint *_panelTrailing;
	UILabel *_panelTitle;
	UITableView *_panelTable;
	NSArray<JXPanelRow *> *_rows;
}

- (instancetype)initWithItem:(JXItem *)item startTicks:(long long)ticks audio:(NSInteger)audio subtitle:(NSInteger)subtitle {
	if ((self = [super init])) {
		_item = item;
		_startTicks = ticks;
		_requestedAudio = audio;
		_requestedSubtitle = subtitle;
		_currentAudio = -1;
		_playMethod = @"Transcode";
		_playSessionId = [NSUUID.UUID.UUIDString stringByReplacingOccurrencesOfString:@"-" withString:@""].lowercaseString;
		_pendingSeek = -1;
		_suspendedAt = -1;
	}
	return self;
}

- (BOOL)prefersStatusBarHidden { return YES; }
- (BOOL)prefersHomeIndicatorAutoHidden { return YES; }

#pragma mark - 畫面

- (UIButton *)iconButton:(UIImage *)icon label:(NSString *)label {
	UIButton *b = [UIButton buttonWithType:UIButtonTypeSystem];
	b.translatesAutoresizingMaskIntoConstraints = NO;
	[b setImage:icon forState:UIControlStateNormal];
	b.tintColor = JXTheme.text;
	b.layer.cornerRadius = 24;
	b.accessibilityLabel = label;
	[b.widthAnchor constraintEqualToConstant:48].active = YES;
	[b.heightAnchor constraintEqualToConstant:48].active = YES;
	return b;
}

- (UIButton *)seekButton:(NSString *)text label:(NSString *)label {
	UIButton *b = [UIButton buttonWithType:UIButtonTypeSystem];
	b.translatesAutoresizingMaskIntoConstraints = NO;
	[b setTitle:text forState:UIControlStateNormal];
	b.titleLabel.font = [JXTheme fontOfSize:15 weight:UIFontWeightBold];
	b.tintColor = JXTheme.text;
	b.backgroundColor = JXColorA(0x0E1014, 0.45);
	b.layer.cornerRadius = 32;
	b.accessibilityLabel = label;
	[b.widthAnchor constraintEqualToConstant:64].active = YES;
	[b.heightAnchor constraintEqualToConstant:64].active = YES;
	return b;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = UIColor.blackColor;

	_video = [[JXVideoView alloc] init];
	_video.translatesAutoresizingMaskIntoConstraints = NO;
	_video.playerLayer.videoGravity = AVLayerVideoGravityResizeAspect;
	UITapGestureRecognizer *tap = [[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(toggleControls)];
	tap.delegate = self;
	[self.view addGestureRecognizer:tap];

	// 字幕：白字黑邊，底部置中
	_subtitleLabel = [[UILabel alloc] init];
	_subtitleLabel.translatesAutoresizingMaskIntoConstraints = NO;
	_subtitleLabel.numberOfLines = 0;
	_subtitleLabel.textAlignment = NSTextAlignmentCenter;
	_subtitleLabel.userInteractionEnabled = NO;
	// 黑邊另外畫在底下一層：iOS 的描邊以字形輪廓為中心，直接描在白字上會吃掉一半筆畫，細的中文字看起來發灰
	_subtitleOutline = [[UILabel alloc] init];
	_subtitleOutline.translatesAutoresizingMaskIntoConstraints = NO;
	_subtitleOutline.numberOfLines = 0;
	_subtitleOutline.textAlignment = NSTextAlignmentCenter;
	_subtitleOutline.userInteractionEnabled = NO;

	_subtitleHint = [JXTheme labelWithSize:14 weight:UIFontWeightRegular color:JXTheme.text];
	_subtitleHint.numberOfLines = 0;
	_subtitleHint.textAlignment = NSTextAlignmentCenter;
	_subtitleHint.backgroundColor = JXColorA(0x0E1014, 0.75);
	_subtitleHint.layer.cornerRadius = 8;
	_subtitleHint.clipsToBounds = YES;
	_subtitleHint.hidden = YES;

	_status = [JXTheme labelWithSize:17 weight:UIFontWeightMedium color:JXTheme.text];
	_status.numberOfLines = 0;
	_status.textAlignment = NSTextAlignmentCenter;
	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;
	_spinner.hidesWhenStopped = YES;

	[self buildControls];
	[self buildPanel];

	for (UIView *v in @[_video, _subtitleOutline, _subtitleLabel, _controls, _spinner, _status, _subtitleHint, _panelScrim, _panel]) [self.view addSubview:v];
	_subtitleBottom = [_subtitleLabel.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor constant:-40];
	[NSLayoutConstraint activateConstraints:@[
		[_video.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_video.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_video.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_video.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_controls.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_controls.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_controls.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_controls.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		_subtitleBottom,
		[_subtitleLabel.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_subtitleLabel.widthAnchor constraintLessThanOrEqualToAnchor:self.view.widthAnchor multiplier:0.86],
		[_subtitleOutline.centerXAnchor constraintEqualToAnchor:_subtitleLabel.centerXAnchor],
		[_subtitleOutline.bottomAnchor constraintEqualToAnchor:_subtitleLabel.bottomAnchor],
		[_subtitleOutline.widthAnchor constraintEqualToAnchor:_subtitleLabel.widthAnchor],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
		[_status.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_status.topAnchor constraintEqualToAnchor:_spinner.bottomAnchor constant:20],
		[_status.widthAnchor constraintLessThanOrEqualToConstant:600],
		[_subtitleHint.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_subtitleHint.topAnchor constraintEqualToAnchor:self.view.topAnchor constant:96],
		[_subtitleHint.widthAnchor constraintLessThanOrEqualToConstant:620],
		[_panelScrim.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_panelScrim.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_panelScrim.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_panelScrim.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
	]];

	_title.text = [_item.type isEqualToString:@"Episode"]
		? [NSString stringWithFormat:@"%@　%@", JXDisplayTitle(_item), JXEpisodeLabel(_item)] : _item.name;
	_statusLine.text = nil;
	_audioButton.hidden = YES;
	_subtitleButton.hidden = YES;
	[self setStatus:@"準備轉碼中…" spinning:YES];
	[self setControlsVisible:YES animated:NO];

	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserver:self selector:@selector(didEnd:) name:AVPlayerItemDidPlayToEndTimeNotification object:nil];
	[nc addObserver:self selector:@selector(failedToEnd:) name:AVPlayerItemFailedToPlayToEndTimeNotification object:nil];
	[nc addObserver:self selector:@selector(willResignActive) name:UIApplicationWillResignActiveNotification object:nil];
	[nc addObserver:self selector:@selector(didEnterBackground) name:UIApplicationDidEnterBackgroundNotification object:nil];
	[nc addObserver:self selector:@selector(willEnterForeground) name:UIApplicationWillEnterForegroundNotification object:nil];

	JXPlaybackLogReset();
	JXPlaybackLog(@"open item=%@ start=%.1fs", _item.itemId, (double)_startTicks / JX_TICKS_PER_SECOND);
	[self prepare];
}

- (void)buildControls {
	_controls = [[UIView alloc] init];
	_controls.translatesAutoresizingMaskIntoConstraints = NO;
	_shade = [CAGradientLayer layer];
	_shade.colors = @[(id)JXColorA(0, 0.75).CGColor, (id)JXColorA(0, 0).CGColor, (id)JXColorA(0, 0).CGColor, (id)JXColorA(0, 0.8).CGColor];
	_shade.locations = @[@0, @0.22, @0.7, @1];
	[_controls.layer addSublayer:_shade];

	UIButton *back = [self iconButton:JXIcons.back label:@"返回"];
	[back addTarget:self action:@selector(finish) forControlEvents:UIControlEventTouchUpInside];
	_title = [JXTheme labelWithSize:20 weight:UIFontWeightBold color:JXTheme.text];
	_statusLine = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textSecondary];
	UIStackView *titles = [[UIStackView alloc] initWithArrangedSubviews:@[_title, _statusLine]];
	titles.axis = UILayoutConstraintAxisVertical;
	_audioButton = [self iconButton:JXIcons.audio label:@"音軌"];
	[_audioButton addTarget:self action:@selector(showAudioPanel) forControlEvents:UIControlEventTouchUpInside];
	_subtitleButton = [self iconButton:JXIcons.subtitles label:@"字幕"];
	[_subtitleButton addTarget:self action:@selector(showSubtitlePanel) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *top = [[UIStackView alloc] initWithArrangedSubviews:@[back, titles, _audioButton, _subtitleButton]];
	top.translatesAutoresizingMaskIntoConstraints = NO;
	top.spacing = 16;
	top.alignment = UIStackViewAlignmentCenter;

	UIButton *rewind = [self seekButton:@"−10" label:@"倒退 10 秒"];
	[rewind addTarget:self action:@selector(rewind) forControlEvents:UIControlEventTouchUpInside];
	UIButton *forward = [self seekButton:@"+10" label:@"快轉 10 秒"];
	[forward addTarget:self action:@selector(forward) forControlEvents:UIControlEventTouchUpInside];
	_playPause = [UIButton buttonWithType:UIButtonTypeSystem];
	_playPause.translatesAutoresizingMaskIntoConstraints = NO;
	_playPause.backgroundColor = JXTheme.accent;
	_playPause.tintColor = JXTheme.onAccent;
	_playPause.layer.cornerRadius = 42;
	[_playPause.widthAnchor constraintEqualToConstant:84].active = YES;
	[_playPause.heightAnchor constraintEqualToConstant:84].active = YES;
	[_playPause addTarget:self action:@selector(togglePlay) forControlEvents:UIControlEventTouchUpInside];
	UIStackView *center = [[UIStackView alloc] initWithArrangedSubviews:@[rewind, _playPause, forward]];
	center.translatesAutoresizingMaskIntoConstraints = NO;
	center.spacing = 48;
	center.alignment = UIStackViewAlignmentCenter;

	_buffer = [[UIProgressView alloc] initWithProgressViewStyle:UIProgressViewStyleDefault];
	_buffer.translatesAutoresizingMaskIntoConstraints = NO;
	_buffer.progressTintColor = JXColorA(0xFFFFFF, 0.45);
	_buffer.trackTintColor = JXColorA(0xFFFFFF, 0.25);
	_slider = [[UISlider alloc] init];
	_slider.translatesAutoresizingMaskIntoConstraints = NO;
	_slider.minimumTrackTintColor = JXTheme.accent;
	_slider.maximumTrackTintColor = UIColor.clearColor;
	_slider.thumbTintColor = JXTheme.accent;
	[_slider setThumbImage:[self thumbImage] forState:UIControlStateNormal];
	_slider.accessibilityLabel = @"播放位置";
	[_slider addTarget:self action:@selector(scrubBegan) forControlEvents:UIControlEventTouchDown];
	[_slider addTarget:self action:@selector(scrubMoved) forControlEvents:UIControlEventValueChanged];
	[_slider addTarget:self action:@selector(scrubEnded) forControlEvents:UIControlEventTouchUpInside | UIControlEventTouchUpOutside | UIControlEventTouchCancel];
	_elapsed = [JXTheme labelWithSize:14 weight:UIFontWeightRegular color:JXTheme.textBody];
	_elapsed.font = [UIFont monospacedDigitSystemFontOfSize:14 weight:UIFontWeightRegular];
	_remaining = [JXTheme labelWithSize:14 weight:UIFontWeightRegular color:JXTheme.textBody];
	_remaining.font = [UIFont monospacedDigitSystemFontOfSize:14 weight:UIFontWeightRegular];

	for (UIView *v in @[top, center, _buffer, _slider, _elapsed, _remaining]) [_controls addSubview:v];
	[NSLayoutConstraint activateConstraints:@[
		[top.leadingAnchor constraintEqualToAnchor:_controls.leadingAnchor constant:24],
		[top.trailingAnchor constraintEqualToAnchor:_controls.trailingAnchor constant:-24],
		[top.topAnchor constraintEqualToAnchor:_controls.topAnchor constant:16],
		[center.centerXAnchor constraintEqualToAnchor:_controls.centerXAnchor],
		[center.centerYAnchor constraintEqualToAnchor:_controls.centerYAnchor],
		[_slider.leadingAnchor constraintEqualToAnchor:_controls.leadingAnchor constant:32],
		[_slider.trailingAnchor constraintEqualToAnchor:_controls.trailingAnchor constant:-32],
		[_slider.bottomAnchor constraintEqualToAnchor:_elapsed.topAnchor constant:-4],
		[_buffer.leadingAnchor constraintEqualToAnchor:_slider.leadingAnchor constant:2],
		[_buffer.trailingAnchor constraintEqualToAnchor:_slider.trailingAnchor constant:-2],
		[_buffer.centerYAnchor constraintEqualToAnchor:_slider.centerYAnchor],
		[_elapsed.leadingAnchor constraintEqualToAnchor:_slider.leadingAnchor],
		[_elapsed.bottomAnchor constraintEqualToAnchor:_controls.bottomAnchor constant:-24],
		[_remaining.trailingAnchor constraintEqualToAnchor:_slider.trailingAnchor],
		[_remaining.centerYAnchor constraintEqualToAnchor:_elapsed.centerYAnchor],
	]];
	[self updatePlayButton];
}

- (UIImage *)thumbImage {
	UIGraphicsImageRenderer *r = [[UIGraphicsImageRenderer alloc] initWithSize:CGSizeMake(18, 18)];
	return [r imageWithActions:^(UIGraphicsImageRendererContext *ctx) {
		[JXTheme.accent setFill];
		[[UIBezierPath bezierPathWithOvalInRect:CGRectMake(1, 1, 16, 16)] fill];
	}];
}

- (void)buildPanel {
	_panelScrim = [[UIView alloc] init];
	_panelScrim.translatesAutoresizingMaskIntoConstraints = NO;
	_panelScrim.backgroundColor = JXColorA(0, 0.35);
	_panelScrim.hidden = YES;
	[_panelScrim addGestureRecognizer:[[UITapGestureRecognizer alloc] initWithTarget:self action:@selector(hidePanel)]];

	_panel = [[UIView alloc] init];
	_panel.translatesAutoresizingMaskIntoConstraints = NO;
	_panel.backgroundColor = JXTheme.surface;
	_panel.hidden = YES;
	UIView *border = [[UIView alloc] init];
	border.translatesAutoresizingMaskIntoConstraints = NO;
	border.backgroundColor = JXTheme.line;
	_panelTitle = [JXTheme labelWithSize:20 weight:UIFontWeightBold color:JXTheme.text];
	UIButton *close = [UIButton buttonWithType:UIButtonTypeSystem];
	close.translatesAutoresizingMaskIntoConstraints = NO;
	[close setImage:JXIcons.close forState:UIControlStateNormal];
	close.tintColor = JXTheme.textSecondary;
	close.accessibilityLabel = @"關閉";
	[close addTarget:self action:@selector(hidePanel) forControlEvents:UIControlEventTouchUpInside];
	UIView *line = [[UIView alloc] init];
	line.translatesAutoresizingMaskIntoConstraints = NO;
	line.backgroundColor = JXTheme.line;
	_panelTable = [[UITableView alloc] initWithFrame:CGRectZero style:UITableViewStylePlain];
	_panelTable.translatesAutoresizingMaskIntoConstraints = NO;
	_panelTable.backgroundColor = UIColor.clearColor;
	_panelTable.separatorStyle = UITableViewCellSeparatorStyleNone;
	_panelTable.dataSource = self;
	_panelTable.delegate = self;
	_panelTable.contentInset = UIEdgeInsetsMake(8, 0, 8, 0);

	for (UIView *v in @[border, _panelTitle, close, line, _panelTable]) [_panel addSubview:v];
	[self.view addSubview:_panel]; // 先加入以便設定與 self.view 的約束，viewDidLoad 會再調整順序
	_panelTrailing = [_panel.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor constant:360];
	[NSLayoutConstraint activateConstraints:@[
		_panelTrailing,
		[_panel.topAnchor constraintEqualToAnchor:self.view.topAnchor],
		[_panel.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_panel.widthAnchor constraintEqualToConstant:360],
		[border.leadingAnchor constraintEqualToAnchor:_panel.leadingAnchor],
		[border.topAnchor constraintEqualToAnchor:_panel.topAnchor],
		[border.bottomAnchor constraintEqualToAnchor:_panel.bottomAnchor],
		[border.widthAnchor constraintEqualToConstant:1],
		[_panelTitle.leadingAnchor constraintEqualToAnchor:_panel.leadingAnchor constant:24],
		[_panelTitle.centerYAnchor constraintEqualToAnchor:_panel.topAnchor constant:40],
		[close.trailingAnchor constraintEqualToAnchor:_panel.trailingAnchor constant:-14],
		[close.centerYAnchor constraintEqualToAnchor:_panelTitle.centerYAnchor],
		[close.widthAnchor constraintEqualToConstant:44],
		[close.heightAnchor constraintEqualToConstant:44],
		[line.leadingAnchor constraintEqualToAnchor:_panel.leadingAnchor],
		[line.trailingAnchor constraintEqualToAnchor:_panel.trailingAnchor],
		[line.topAnchor constraintEqualToAnchor:_panel.topAnchor constant:80],
		[line.heightAnchor constraintEqualToConstant:1],
		[_panelTable.topAnchor constraintEqualToAnchor:line.bottomAnchor],
		[_panelTable.leadingAnchor constraintEqualToAnchor:_panel.leadingAnchor constant:1],
		[_panelTable.trailingAnchor constraintEqualToAnchor:_panel.trailingAnchor],
		[_panelTable.bottomAnchor constraintEqualToAnchor:_panel.bottomAnchor],
	]];
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	_shade.frame = _controls.bounds;
}

- (void)viewDidAppear:(BOOL)animated {
	[super viewDidAppear:animated];
	UIApplication.sharedApplication.idleTimerDisabled = YES;
}

- (BOOL)gestureRecognizer:(UIGestureRecognizer *)g shouldReceiveTouch:(UITouch *)touch {
	// 只有點在影片空白處才切換控制列；按鈕、進度條、面板照常運作
	UIView *v = touch.view;
	return v == self.view || v == _video || v == _controls;
}

- (void)setStatus:(NSString *)text spinning:(BOOL)spinning {
	_status.text = text;
	_status.hidden = text == nil;
	if (spinning) [_spinner startAnimating]; else [_spinner stopAnimating];
}

#pragma mark - 準備播放

- (void)prepare {
	_jf = JXJellyfin.current;
	if (!_jf) { [self fail:@"連不到 Jellyfin"]; return; }
	__weak typeof(self) weakSelf = self;
	// 音軌、字幕清單或偏好拿不到時照樣播放，只是不能切換
	[_jf mediaInfo:_item.itemId completion:^(JXMediaInfo *info, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s) return;
		s->_info = info;
		if (s->_requestedSubtitle == JXSubtitleAuto && info) {
			[s->_jf subtitleLanguagePreference:^(NSString *pref) {
				[weakSelf startWithSubtitle:[JXSubtitleChooser choose:info.subtitles preferredLanguage:pref locale:NSLocale.currentLocale]];
			}];
		} else {
			JXSubtitleTrack *sub = nil;
			for (JXSubtitleTrack *t in info.subtitles) if (t.index == s->_requestedSubtitle) sub = t;
			[s startWithSubtitle:sub];
		}
	}];
}

- (void)startWithSubtitle:(JXSubtitleTrack *)subtitle {
	__weak typeof(self) weakSelf = self;
	double start = (double)_startTicks / JX_TICKS_PER_SECOND;
	[JXEndpoints resolveXcode:^(NSURL *base) {
		typeof(self) s = weakSelf;
		if (!s || s->_finishing) return;
		if (base) {
			s->_xcode = [[JXXcode alloc] initWithBase:base];
			[s->_xcode createSession:s->_item.itemId profile:JXSettings.shared.profile startTicks:s->_startTicks
			            burnSubtitle:subtitle.isImage ? subtitle.index : -1 audioIndex:s->_requestedAudio
			              completion:^(JXXcodeSession *session, NSError *error) {
				typeof(self) s2 = weakSelf;
				if (!s2) return;
				JXPlaybackLog(@"create session: %@ error=%@", session.sessionId, error);
				if (error) { [s2 fail:error.localizedDescription]; return; }
				if (s2->_finishing) { [s2->_xcode deleteSession:session.sessionId]; return; }
				s2->_session = session;
				s2->_currentAudio = session.audioIndex;
				[s2 playURL:session.playlist headers:nil at:start];
				[s2 didStartWithSubtitle:subtitle];
			}];
		} else {
			s->_playMethod = @"DirectPlay";
			[s setStatus:@"轉碼伺服器離線，改為直接播放" spinning:YES];
			// 原始檔要帶 Jellyfin 的驗證標頭
			NSDictionary *headers = @{@"Authorization": [JXHTTP authHeaderWithToken:JXSettings.shared.token]};
			[s playURL:[s->_jf directStreamURL:s->_item.itemId] headers:headers at:start];
			// 直接播放無法燒錄圖形字幕，也無法換音軌（播放器播原始檔的預設音軌）
			[s didStartWithSubtitle:subtitle.isImage ? nil : subtitle];
		}
	}];
}

- (void)didStartWithSubtitle:(JXSubtitleTrack *)subtitle {
	[self updateStatusLine];
	[self updateTrackButtons];
	_currentSubtitle = subtitle;
	if (subtitle && !subtitle.isImage) [self loadTextSubtitle:subtitle];
}

/// 換上新的播放來源並從 position 秒開始。轉碼與直接播放的時間軸都是整部片。
- (void)playURL:(NSURL *)url headers:(NSDictionary *)headers at:(double)position {
	[self playURL:url headers:headers at:position autoplay:YES];
}

- (void)playURL:(NSURL *)url headers:(NSDictionary *)headers at:(double)position autoplay:(BOOL)autoplay {
	NSDictionary *opts = headers ? @{@"AVURLAssetHTTPHeaderFieldsKey": headers} : nil;
	AVURLAsset *asset = [AVURLAsset URLAssetWithURL:url options:opts];
	AVPlayerItem *item = [AVPlayerItem playerItemWithAsset:asset];
	// 在項目載入前就定位，避免先向轉碼伺服器要第 0 段（會讓伺服器從片頭重轉）
	if (position > 0) [item seekToTime:CMTimeMakeWithSeconds(position, 600) toleranceBefore:kCMTimeZero toleranceAfter:kCMTimeZero completionHandler:nil];
	_pendingSeek = position;
	[self observeItem:item];

	if (!_player) {
		_player = [AVPlayer playerWithPlayerItem:item];
		_player.automaticallyWaitsToMinimizeStalling = YES;
		[_player addObserver:self forKeyPath:@"timeControlStatus" options:NSKeyValueObservingOptionNew context:kTimeControlContext];
		_video.playerLayer.player = _player;
		__weak typeof(self) weakSelf = self;
		_timeObserver = [_player addPeriodicTimeObserverForInterval:CMTimeMake(1, 10) queue:dispatch_get_main_queue() usingBlock:^(CMTime t) {
			[weakSelf tick];
		}];
	} else {
		[_player replaceCurrentItemWithPlayerItem:item];
	}
	if (autoplay) [_player play];
}

- (void)observeItem:(AVPlayerItem *)item {
	if (_observedItem) [_observedItem removeObserver:self forKeyPath:@"status" context:kStatusContext];
	_observedItem = item;
	[item addObserver:self forKeyPath:@"status" options:NSKeyValueObservingOptionNew context:kStatusContext];
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context {
	if (context == kStatusContext) {
		dispatch_async(dispatch_get_main_queue(), ^{ [self itemStatusChanged]; });
	} else if (context == kTimeControlContext) {
		dispatch_async(dispatch_get_main_queue(), ^{ [self timeControlChanged]; });
	} else {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
	}
}

- (void)itemStatusChanged {
	AVPlayerItem *item = _player.currentItem;
	if (item != _observedItem || _finishing) return;
	JXPlaybackLog(@"item status=%ld error=%@", (long)item.status, item.error);
	if (item.status == AVPlayerItemStatusReadyToPlay) {
		// 預先定位若沒生效（例如某些來源載入後才接受定位），這裡補一次
		if (_pendingSeek > 0 && fabs(CMTimeGetSeconds(item.currentTime) - _pendingSeek) > 3) {
			[item seekToTime:CMTimeMakeWithSeconds(_pendingSeek, 600) completionHandler:nil];
		}
		_pendingSeek = -1;
	} else if (item.status == AVPlayerItemStatusFailed) {
		[self playbackError:item.error];
	}
}

- (void)timeControlChanged {
	if (_finishing) return;
	AVPlayerTimeControlStatus st = _player.timeControlStatus;
	JXPlaybackLog(@"timeControl=%ld pos=%.1f", (long)st, self.position);
	BOOL waiting = st == AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate;
	if (st == AVPlayerTimeControlStatusPlaying) {
		if (!_failed) [self setStatus:nil spinning:NO];
		if (!_reportedStart) {
			_reportedStart = YES;
			[self report:@"Start"];
			__weak typeof(self) weakSelf = self;
			_progressTimer = [NSTimer scheduledTimerWithTimeInterval:10 repeats:YES block:^(NSTimer *t) { [weakSelf report:@"Progress"]; }];
		} else {
			[self report:@"Progress"];
		}
		[self scheduleHide];
	} else if (waiting) {
		if (!_failed) { [_spinner startAnimating]; }
	} else {
		[_spinner stopAnimating];
		if (_reportedStart) [self report:@"Progress"];
		[self setControlsVisible:YES animated:YES];
	}
	[self updatePlayButton];
}

- (void)didEnd:(NSNotification *)n {
	if (n.object == _player.currentItem) [self finish];
}

- (void)failedToEnd:(NSNotification *)n {
	if (n.object != _player.currentItem) return;
	[self playbackError:n.userInfo[AVPlayerItemFailedToPlayToEndTimeErrorKey]];
}

- (void)playbackError:(NSError *)error {
	double pos = self.position, dur = self.duration;
	if (dur > 0 && pos > dur - kEndTolerance) { [self finish]; return; }
	[self fail:[NSString stringWithFormat:@"播放錯誤：%@", error.localizedDescription ?: @"未知"]];
}

- (void)fail:(NSString *)message {
	JXPlaybackLog(@"fail: %@", message);
	_failed = YES;
	[_player pause];
	[self setStatus:message spinning:NO];
	[self setControlsVisible:YES animated:YES];
}

- (void)willResignActive {
	JXPlaybackLog(@"willResignActive pos=%.1f", self.position);
	[_player pause];
}

/// 進背景（按 Home、鎖定）時關掉轉碼 session，不在背景佔用 GPU 名額；回到前景再從同一位置建立新的。
/// 直接播放沒有 session，只暫停。
- (void)didEnterBackground {
	JXPlaybackLog(@"didEnterBackground finishing=%d session=%@ xcode=%d suspendedAt=%.1f",
	              _finishing, _session.sessionId, _xcode != nil, _suspendedAt);
	if (_finishing || !_session || !_xcode || _suspendedAt >= 0) return;
	double position = self.position;
	[_player pause];
	_suspendedAt = position; // 之後 self.position 回傳這個值，回報與控制列不會變成 0
	_suspendedBurn = _session.burnedSubtitle;
	NSString *sessionId = _session.sessionId;
	_session = nil;
	_pendingSeek = -1;
	[self observeItem:nil];
	[_player replaceCurrentItemWithPlayerItem:nil]; // 不讓播放器再向已關掉的 session 要片段

	// 回報與刪除要在 app 被暫停前送出
	UIApplication *app = UIApplication.sharedApplication;
	__block UIBackgroundTaskIdentifier task = UIBackgroundTaskInvalid;
	__block NSInteger pending = 2;
	void (^done)(void) = ^{
		if (--pending > 0 || task == UIBackgroundTaskInvalid) return;
		[app endBackgroundTask:task];
		task = UIBackgroundTaskInvalid;
	};
	task = [app beginBackgroundTaskWithExpirationHandler:^{
		[app endBackgroundTask:task];
		task = UIBackgroundTaskInvalid;
	}];
	if (_reportedStart) [self report:@"Progress" completion:done]; else done();
	[_xcode deleteSession:sessionId completion:^{
		JXPlaybackLog(@"deleted %@", sessionId);
		done();
	}];
}

- (void)willEnterForeground {
	JXPlaybackLog(@"willEnterForeground finishing=%d suspendedAt=%.1f restoring=%d", _finishing, _suspendedAt, _restoring);
	if (_finishing || _suspendedAt < 0 || _restoring) return;
	_restoring = YES;
	double position = _suspendedAt;
	[self setStatus:@"準備轉碼中…" spinning:YES];
	[self setControlsVisible:YES animated:NO];
	__weak typeof(self) weakSelf = self;
	JXXcode *xcode = _xcode;
	[xcode createSession:_item.itemId profile:JXSettings.shared.profile startTicks:(long long)(position * JX_TICKS_PER_SECOND)
	        burnSubtitle:_suspendedBurn audioIndex:_currentAudio completion:^(JXXcodeSession *session, NSError *error) {
		typeof(self) s = weakSelf;
		JXPlaybackLog(@"restore session: %@ error=%@", session.sessionId, error);
		if (!s) { if (session) [xcode deleteSession:session.sessionId]; return; } // 播放畫面已關閉
		s->_restoring = NO;
		if (error) { [s fail:error.localizedDescription]; return; }
		if (s->_finishing) { [s->_xcode deleteSession:session.sessionId]; return; }
		s->_session = session;
		s->_currentAudio = session.audioIndex;
		s->_suspendedAt = -1;
		[s updateStatusLine];
		[s setStatus:nil spinning:NO];
		// 停在離開時的畫面，由使用者按播放
		[s playURL:session.playlist headers:nil at:position autoplay:NO];
	}];
}

#pragma mark - 時間與控制列

- (double)position {
	if (_suspendedAt >= 0) return _suspendedAt;
	// 換上新項目（開播、回前景、換音軌字幕）到就緒前 currentTime 還是 0，回報這個值會蓋掉 Jellyfin 的進度
	if (_pendingSeek >= 0) return _pendingSeek;
	CMTime t = _player.currentTime;
	return CMTIME_IS_NUMERIC(t) ? CMTimeGetSeconds(t) : 0;
}

- (double)duration {
	CMTime d = _player.currentItem.duration;
	if (CMTIME_IS_NUMERIC(d) && CMTimeGetSeconds(d) > 0) return CMTimeGetSeconds(d);
	return (double)_item.runTimeTicks / JX_TICKS_PER_SECOND;
}

- (void)tick {
	double pos = self.position, dur = self.duration;
	if (_cues.count) [self renderSubtitleAt:pos];
	if (!_controlsVisible) return;
	if (!_scrubbing && dur > 0) {
		_slider.value = (float)(pos / dur);
		_elapsed.text = JXFormatTime(pos);
		_remaining.text = [@"−" stringByAppendingString:JXFormatTime(dur - pos)];
	}
	NSValue *range = _player.currentItem.loadedTimeRanges.firstObject;
	if (range && dur > 0) {
		CMTimeRange r = range.CMTimeRangeValue;
		_buffer.progress = (float)(CMTimeGetSeconds(CMTimeRangeGetEnd(r)) / dur);
	}
}

- (void)renderSubtitleAt:(double)t {
	NSString *text = [JXSubtitles textAt:t cues:_cues];
	if ((text == nil && _subtitleLabel.attributedText.length == 0) || [text isEqualToString:_subtitleLabel.attributedText.string]) return;
	[self showSubtitleText:text];
}

- (void)showSubtitleText:(NSString *)text {
	if (!text) {
		_subtitleLabel.attributedText = nil;
		_subtitleOutline.attributedText = nil;
		return;
	}
	UIFont *font = [UIFont systemFontOfSize:30 weight:UIFontWeightSemibold];
	NSMutableParagraphStyle *ps = [[NSMutableParagraphStyle alloc] init];
	ps.alignment = NSTextAlignmentCenter;
	ps.lineSpacing = 2;
	_subtitleLabel.attributedText = [[NSAttributedString alloc] initWithString:text attributes:@{
		NSFontAttributeName: font,
		NSForegroundColorAttributeName: UIColor.whiteColor,
		NSParagraphStyleAttributeName: ps,
	}];
	// 正的 stroke 寬度只畫外框（字級的 %），一半露在白字外面
	NSShadow *shadow = [[NSShadow alloc] init];
	shadow.shadowColor = JXColorA(0, 0.6);
	shadow.shadowBlurRadius = 3;
	shadow.shadowOffset = CGSizeMake(0, 1);
	_subtitleOutline.attributedText = [[NSAttributedString alloc] initWithString:text attributes:@{
		NSFontAttributeName: font,
		NSForegroundColorAttributeName: UIColor.blackColor,
		NSStrokeColorAttributeName: UIColor.blackColor,
		NSStrokeWidthAttributeName: @(16),
		NSShadowAttributeName: shadow,
		NSParagraphStyleAttributeName: ps,
	}];
}

- (void)toggleControls {
	[self setControlsVisible:!_controlsVisible animated:YES];
}

- (void)setControlsVisible:(BOOL)visible animated:(BOOL)animated {
	_controlsVisible = visible;
	[_hideTimer invalidate];
	// 控制列出現時把字幕往上移，免得蓋住進度條
	_subtitleBottom.constant = visible ? -140 : -40;
	if (visible) [self tick];
	[UIView animateWithDuration:animated ? 0.2 : 0 animations:^{
		self->_controls.alpha = visible ? 1 : 0;
		[self.view layoutIfNeeded];
	}];
	_controls.userInteractionEnabled = visible;
	if (visible) [self scheduleHide];
}

- (void)scheduleHide {
	[_hideTimer invalidate];
	if (!_controlsVisible || _player.timeControlStatus != AVPlayerTimeControlStatusPlaying || _scrubbing) return;
	__weak typeof(self) weakSelf = self;
	_hideTimer = [NSTimer scheduledTimerWithTimeInterval:4 repeats:NO block:^(NSTimer *t) { [weakSelf setControlsVisible:NO animated:YES]; }];
}

- (void)updatePlayButton {
	BOOL playing = _player.rate > 0 || _player.timeControlStatus == AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate;
	[_playPause setImage:playing ? [JXIcons pauseOfSize:32] : [JXIcons playOfSize:32] forState:UIControlStateNormal];
	_playPause.accessibilityLabel = playing ? @"暫停" : @"播放";
}

- (void)togglePlay {
	if (!_player || _failed || _suspendedAt >= 0) return;
	if (_player.rate > 0 || _player.timeControlStatus == AVPlayerTimeControlStatusWaitingToPlayAtSpecifiedRate) [_player pause];
	else [_player play];
	[self scheduleHide];
}

- (void)seekTo:(double)seconds {
	if (_suspendedAt >= 0) return;
	double dur = self.duration;
	seconds = MAX(0, dur > 0 ? MIN(seconds, dur - 1) : seconds);
	[_player seekToTime:CMTimeMakeWithSeconds(seconds, 600) toleranceBefore:kCMTimeZero toleranceAfter:kCMTimeZero];
	if (_cues.count) [self renderSubtitleAt:seconds];
	[self scheduleHide];
}

- (void)rewind { [self seekTo:self.position - kSeekStep]; }
- (void)forward { [self seekTo:self.position + kSeekStep]; }

- (void)scrubBegan {
	_scrubbing = YES;
	[_hideTimer invalidate];
}

- (void)scrubMoved {
	double dur = self.duration;
	double t = _slider.value * dur;
	_elapsed.text = JXFormatTime(t);
	_remaining.text = [@"−" stringByAppendingString:JXFormatTime(dur - t)];
}

- (void)scrubEnded {
	if (!_scrubbing) return;
	_scrubbing = NO;
	[self seekTo:_slider.value * self.duration];
}

#pragma mark - 狀態列與軌道按鈕

/// 標題下的狀態，例如「1080p · GPU 轉碼」。
- (void)updateStatusLine {
	if (_session) _statusLine.text = [NSString stringWithFormat:@"%ldp · %@ 轉碼", (long)_session.height, _session.hwDecode ? @"GPU" : @"CPU"];
	else _statusLine.text = @"直接播放";
}

- (void)updateTrackButtons {
	BOOL anySub = [_info.subtitles indexOfObjectPassingTest:^BOOL(JXSubtitleTrack *t, NSUInteger i, BOOL *stop) { return t.playable; }] != NSNotFound;
	_subtitleButton.hidden = !anySub;
	// 直接播放時播原始檔，換音軌要靠轉碼伺服器
	_audioButton.hidden = !(_info.audio.count > 1 && _xcode);
	BOOL on = _currentSubtitle != nil;
	_subtitleButton.tintColor = on ? JXTheme.accent : JXTheme.text;
	_subtitleButton.backgroundColor = on ? JXColorA(0xF2A93B, 0.18) : UIColor.clearColor;
}

#pragma mark - 音軌／字幕面板

- (NSString *)subtitleNote:(JXSubtitleTrack *)t {
	// Jellyfin 的 DisplayTitle 通常已含格式，這裡只補標題沒有的資訊
	NSMutableArray *p = [NSMutableArray array];
	NSString *codec = t.codec.uppercaseString;
	if (codec.length && [t.title rangeOfString:codec options:NSCaseInsensitiveSearch].location == NSNotFound) [p addObject:codec];
	if (t.isExternal && [t.title rangeOfString:@"外部"].location == NSNotFound) [p addObject:@"外掛"];
	if (t.isForced) [p addObject:@"強制"];
	return [p componentsJoinedByString:@" · "];
}

- (void)showSubtitlePanel {
	NSMutableArray<JXPanelRow *> *rows = [NSMutableArray array];
	__weak typeof(self) weakSelf = self;
	[rows addObject:[JXPanelRow option:@"關閉" note:nil selected:_currentSubtitle == nil onPick:^{ [weakSelf applySubtitle:nil]; }]];
	NSMutableArray *text = [NSMutableArray array], *image = [NSMutableArray array];
	for (JXSubtitleTrack *t in _info.subtitles) {
		if (!t.playable) continue;
		[t.isImage ? image : text addObject:t];
	}
	void (^add)(NSArray *) = ^(NSArray *tracks) {
		for (JXSubtitleTrack *t in tracks) {
			[rows addObject:[JXPanelRow option:t.title note:[self subtitleNote:t] selected:t.index == self->_currentSubtitle.index && self->_currentSubtitle
			                            onPick:^{ [weakSelf applySubtitle:t]; }]];
		}
	};
	if (text.count) { [rows addObject:[JXPanelRow header:@"文字字幕"]]; add(text); }
	if (image.count) { [rows addObject:[JXPanelRow header:@"圖形字幕（燒進畫面，切換時會重新緩衝）"]]; add(image); }
	[self showPanel:@"字幕" rows:rows];
}

- (void)showAudioPanel {
	NSMutableArray<JXPanelRow *> *rows = [NSMutableArray arrayWithObject:[JXPanelRow header:@"切換音軌會重新緩衝"]];
	__weak typeof(self) weakSelf = self;
	for (JXAudioTrack *t in _info.audio) {
		[rows addObject:[JXPanelRow option:t.title note:t.isDefault ? @"預設" : nil selected:t.index == _currentAudio
		                            onPick:^{ [weakSelf applyAudio:t]; }]];
	}
	[self showPanel:@"音軌" rows:rows];
}

- (void)showPanel:(NSString *)title rows:(NSArray<JXPanelRow *> *)rows {
	_panelTitle.text = title;
	_rows = rows;
	[_panelTable reloadData];
	[self setControlsVisible:NO animated:YES];
	_panelScrim.hidden = NO;
	_panel.hidden = NO;
	[self.view layoutIfNeeded];
	_panelTrailing.constant = 0;
	[UIView animateWithDuration:0.18 animations:^{ [self.view layoutIfNeeded]; }];
}

- (void)hidePanel {
	_panelTrailing.constant = 360;
	[UIView animateWithDuration:0.15 animations:^{ [self.view layoutIfNeeded]; } completion:^(BOOL done) {
		self->_panelScrim.hidden = YES;
		self->_panel.hidden = YES;
	}];
}

- (NSInteger)tableView:(UITableView *)tv numberOfRowsInSection:(NSInteger)section { return _rows.count; }

- (CGFloat)tableView:(UITableView *)tv heightForRowAtIndexPath:(NSIndexPath *)ip {
	JXPanelRow *r = _rows[ip.row];
	return r.header ? 44 : (r.note.length ? 60 : 52);
}

- (UITableViewCell *)tableView:(UITableView *)tv cellForRowAtIndexPath:(NSIndexPath *)ip {
	JXPanelRow *r = _rows[ip.row];
	UITableViewCell *c = [tv dequeueReusableCellWithIdentifier:r.header ? @"h" : @"o"];
	if (!c) {
		c = [[UITableViewCell alloc] initWithStyle:r.header ? UITableViewCellStyleDefault : UITableViewCellStyleSubtitle reuseIdentifier:r.header ? @"h" : @"o"];
		c.backgroundColor = UIColor.clearColor;
		c.separatorInset = UIEdgeInsetsZero;
		UIView *sel = [[UIView alloc] init];
		sel.backgroundColor = JXTheme.surfaceHigh;
		c.selectedBackgroundView = sel;
		c.indentationWidth = 0;
		c.detailTextLabel.font = [JXTheme fontOfSize:12 weight:UIFontWeightRegular];
		c.detailTextLabel.textColor = JXTheme.textTertiary;
	}
	if (r.header) {
		c.textLabel.text = r.label;
		c.textLabel.font = [JXTheme fontOfSize:12 weight:UIFontWeightMedium];
		c.textLabel.textColor = JXTheme.textTertiary;
		c.textLabel.numberOfLines = 2;
		c.selectionStyle = UITableViewCellSelectionStyleNone;
		c.backgroundColor = UIColor.clearColor;
		c.accessoryView = nil;
		c.accessibilityTraits = UIAccessibilityTraitHeader;
	} else {
		c.textLabel.text = r.label;
		c.textLabel.font = [JXTheme fontOfSize:15 weight:UIFontWeightRegular];
		c.textLabel.textColor = r.selected ? JXTheme.accent : JXTheme.text;
		c.detailTextLabel.text = r.note.length ? r.note : nil;
		c.selectionStyle = UITableViewCellSelectionStyleDefault;
		c.backgroundColor = r.selected ? JXColorA(0xF2A93B, 0.12) : UIColor.clearColor;
		UIImageView *check = [[UIImageView alloc] initWithImage:JXIcons.check];
		check.tintColor = JXTheme.accent;
		c.accessoryView = r.selected ? check : nil;
		c.accessibilityTraits = r.selected ? (UIAccessibilityTraitButton | UIAccessibilityTraitSelected) : UIAccessibilityTraitButton;
	}
	return c;
}

- (BOOL)tableView:(UITableView *)tv shouldHighlightRowAtIndexPath:(NSIndexPath *)ip { return !_rows[ip.row].header; }

- (void)tableView:(UITableView *)tv didSelectRowAtIndexPath:(NSIndexPath *)ip {
	JXPanelRow *r = _rows[ip.row];
	[tv deselectRowAtIndexPath:ip animated:YES];
	if (r.header) return;
	[self hidePanel];
	if (r.onPick) r.onPick();
}

#pragma mark - 字幕與音軌切換

- (void)applySubtitle:(JXSubtitleTrack *)track {
	if (track.isImage && !_xcode) {
		[self toast:@"圖形字幕需要轉碼伺服器"];
		return;
	}
	_currentSubtitle = track;
	[_subtitleTask cancel];
	_subtitleTask = nil;
	_subtitleHint.hidden = YES;
	_cues = nil;
	[self showSubtitleText:nil];
	[self updateTrackButtons];

	// 燒錄狀態改變（換成圖形字幕、換另一條圖形字幕、或離開圖形字幕）才需要重建轉碼
	NSInteger burnNow = _session ? _session.burnedSubtitle : -1;
	NSInteger burnWanted = track.isImage ? track.index : -1;
	if (_xcode && burnWanted != burnNow) [self restartSessionBurn:burnWanted audio:_currentAudio];
	if (track && !track.isImage) [self loadTextSubtitle:track];
}

- (void)applyAudio:(JXAudioTrack *)track {
	if (track.index == _currentAudio) return;
	_currentAudio = track.index;
	[self restartSessionBurn:_session ? _session.burnedSubtitle : -1 audio:track.index];
}

- (void)loadTextSubtitle:(JXSubtitleTrack *)track {
	if (!_jf || !_info) return;
	// 外掛字幕通常不到一秒；內嵌字幕可能要等 Jellyfin 讀完整個檔案，提示一直顯示到載入完成
	NSString *hint = track.isExternal ? @"字幕載入中…" : @"字幕載入中…（內嵌字幕第一次要讓 Jellyfin 讀完整個檔案，大檔可能要幾分鐘）";
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.8 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		if (self->_currentSubtitle != track || self->_cues || self->_finishing) return;
		self->_subtitleHint.text = [NSString stringWithFormat:@"  %@  ", hint];
		self->_subtitleHint.hidden = NO;
	});
	__weak typeof(self) weakSelf = self;
	_subtitleTask = [_jf subtitleVTT:_item.itemId source:_info.mediaSourceId index:track.index completion:^(NSData *data, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s || s->_currentSubtitle != track) return;
		s->_subtitleHint.hidden = YES;
		s->_subtitleTask = nil;
		if (error) {
			if (error.code != NSURLErrorCancelled) [s toast:[@"字幕載入失敗：" stringByAppendingString:error.localizedDescription]];
			return;
		}
		s->_cues = [JXSubtitles parseVTT:data];
		[s renderSubtitleAt:s.position];
	}];
}

/// 在目前位置建立新的轉碼 session（燒錄字幕或音軌改變時），播放器接著播，舊 session 隨後刪除。
- (void)restartSessionBurn:(NSInteger)burn audio:(NSInteger)audio {
	if (!_xcode || !_player) return;
	JXXcodeSession *old = _session;
	double position = self.position;
	[_player pause];
	[self setStatus:@"準備轉碼中…" spinning:YES];
	__weak typeof(self) weakSelf = self;
	[_xcode createSession:_item.itemId profile:JXSettings.shared.profile startTicks:(long long)(position * JX_TICKS_PER_SECOND)
	         burnSubtitle:burn audioIndex:audio completion:^(JXXcodeSession *session, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s) return;
		if (error) { [s fail:error.localizedDescription]; return; }
		if (s->_finishing) { [s->_xcode deleteSession:session.sessionId]; return; }
		s->_session = session;
		s->_currentAudio = session.audioIndex;
		[s updateStatusLine];
		[s playURL:session.playlist headers:nil at:position];
		if (old) [s->_xcode deleteSession:old.sessionId];
	}];
}

- (void)toast:(NSString *)message {
	UILabel *l = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.text];
	l.text = [NSString stringWithFormat:@"   %@   ", message];
	l.numberOfLines = 0;
	l.backgroundColor = JXColorA(0x20242C, 0.95);
	l.layer.cornerRadius = 10;
	l.clipsToBounds = YES;
	[self.view addSubview:l];
	[NSLayoutConstraint activateConstraints:@[
		[l.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[l.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor constant:-120],
		[l.heightAnchor constraintGreaterThanOrEqualToConstant:44],
		[l.widthAnchor constraintLessThanOrEqualToConstant:640],
	]];
	[UIView animateWithDuration:0.3 delay:3 options:0 animations:^{ l.alpha = 0; } completion:^(BOOL f) { [l removeFromSuperview]; }];
}

#pragma mark - 播放回報與結束

/// 回報失敗不影響播放，只是 Jellyfin 的紀錄不會更新。
- (void)report:(NSString *)what {
	[self report:what completion:nil];
}

- (void)report:(NSString *)what completion:(void (^)(void))completion {
	if (!_jf) { if (completion) completion(); return; }
	long long ticks = (long long)(self.position * JX_TICKS_PER_SECOND);
	BOOL paused = _player.timeControlStatus == AVPlayerTimeControlStatusPaused;
	[_jf reportPlayback:what item:_item.itemId session:_playSessionId position:ticks paused:paused method:_playMethod
	         completion:^(NSError *error) { if (completion) completion(); }];
}

- (void)finish {
	JXPlaybackLog(@"finish pos=%.1f", self.position);
	if (_finishing) return;
	_finishing = YES;
	[_player pause];
	[_progressTimer invalidate];
	[_hideTimer invalidate];
	[_subtitleTask cancel];
	UIApplication.sharedApplication.idleTimerDisabled = NO;

	// 先回報停止再通知列表重新載入，Jellyfin 才會有最新進度；停掉轉碼釋出 GPU 名額
	JXXcode *xcode = _xcode;
	JXXcodeSession *session = _session;
	void (^done)(void) = ^{
		[NSNotificationCenter.defaultCenter postNotificationName:JXPlaybackDidFinishNotification object:nil];
	};
	if (_reportedStart) [self report:@"Stopped" completion:done]; else done();
	if (session) [xcode deleteSession:session.sessionId];
	[self teardownPlayer];
	[self dismissViewControllerAnimated:YES completion:nil];
}

- (void)teardownPlayer {
	if (_timeObserver) [_player removeTimeObserver:_timeObserver];
	_timeObserver = nil;
	if (_observedItem) [_observedItem removeObserver:self forKeyPath:@"status" context:kStatusContext];
	_observedItem = nil;
	if (_player) [_player removeObserver:self forKeyPath:@"timeControlStatus" context:kTimeControlContext];
	[_player replaceCurrentItemWithPlayerItem:nil];
	_player = nil;
	[NSNotificationCenter.defaultCenter removeObserver:self];
}

- (void)dealloc {
	if (!_finishing) [self teardownPlayer];
}

@end
