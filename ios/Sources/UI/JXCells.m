#import "JXCells.h"
#import "JXItem.h"
#import "JXJellyfin.h"
#import "JXFormat.h"
#import "JXTheme.h"
#import "UIImageView+JX.h"

static UIImageView *JXImageView(void) {
	UIImageView *iv = [[UIImageView alloc] init];
	iv.contentMode = UIViewContentModeScaleAspectFill;
	iv.clipsToBounds = YES;
	iv.layer.cornerRadius = 8;
	iv.backgroundColor = JXTheme.placeholder;
	return iv;
}

static UIProgressView *JXThinProgress(void) {
	UIProgressView *p = [[UIProgressView alloc] initWithProgressViewStyle:UIProgressViewStyleBar];
	p.progressTintColor = JXTheme.accent;
	p.trackTintColor = JXColorA(0x000000, 0.5);
	return p;
}

static UILabel *JXBadge(void) {
	UILabel *l = [[UILabel alloc] init];
	l.font = [JXTheme fontOfSize:11 weight:UIFontWeightBold];
	l.textColor = JXTheme.text;
	l.backgroundColor = JXColorA(0x0E1014, 0.8);
	l.textAlignment = NSTextAlignmentCenter;
	l.layer.cornerRadius = 4;
	l.clipsToBounds = YES;
	return l;
}

#pragma mark - Poster

@implementation JXPosterCell {
	UIImageView *_image;
	UIProgressView *_progress;
	UILabel *_badge, *_title, *_subtitle;
}

+ (CGFloat)textHeight { return 46; }

- (instancetype)initWithFrame:(CGRect)frame {
	if ((self = [super initWithFrame:frame])) {
		_image = JXImageView();
		_progress = JXThinProgress();
		_badge = JXBadge();
		_title = [JXTheme labelWithSize:14 weight:UIFontWeightRegular color:JXTheme.text];
		_subtitle = [JXTheme labelWithSize:12 weight:UIFontWeightRegular color:JXTheme.textTertiary];
		for (UIView *v in @[_image, _progress, _badge, _title, _subtitle]) {
			v.translatesAutoresizingMaskIntoConstraints = YES;
			[self.contentView addSubview:v];
		}
	}
	return self;
}

- (void)layoutSubviews {
	[super layoutSubviews];
	CGFloat w = self.contentView.bounds.size.width;
	CGFloat h = w * 1.5;
	_image.frame = CGRectMake(0, 0, w, h);
	_progress.frame = CGRectMake(4, h - 6, w - 8, 3);
	CGSize bs = [_badge.text sizeWithAttributes:@{NSFontAttributeName: _badge.font}];
	_badge.frame = CGRectMake(w - bs.width - 18, 8, bs.width + 10, 18);
	_title.frame = CGRectMake(0, h + 8, w, 18);
	_subtitle.frame = CGRectMake(0, h + 27, w, 16);
}

- (void)setHighlighted:(BOOL)highlighted {
	[super setHighlighted:highlighted];
	self.contentView.alpha = highlighted ? 0.6 : 1;
}

- (void)configure:(JXItem *)item api:(JXJellyfin *)api {
	[_image jx_setImageURL:[api imageURL:item.poster maxWidth:260]];
	_title.text = JXDisplayTitle(item);
	if ([item.type isEqualToString:@"Episode"]) _subtitle.text = JXEpisodeLabel(item);
	else if ([@[@"Playlist", @"Folder", @"BoxSet"] containsObject:item.type]) _subtitle.text = item.childCount ? [NSString stringWithFormat:@"%ld 項", (long)item.childCount] : @"";
	else _subtitle.text = item.productionYear ? @(item.productionYear).stringValue : @"";
	_progress.hidden = !item.resumable;
	_progress.progress = JXProgress(item);
	if (item.played) _badge.text = @"已看";
	else if ([item.type isEqualToString:@"Series"] && item.unplayedCount > 0) _badge.text = [NSString stringWithFormat:@"%ld 集未看", (long)item.unplayedCount];
	else _badge.text = nil;
	_badge.hidden = _badge.text == nil;
	self.accessibilityLabel = JXDisplayTitle(item);
	self.isAccessibilityElement = YES;
	[self setNeedsLayout];
}

@end

#pragma mark - Wide

@implementation JXWideCell {
	UIImageView *_image;
	UIProgressView *_progress;
	UILabel *_title, *_subtitle;
}

+ (CGFloat)textHeight { return 48; }

- (instancetype)initWithFrame:(CGRect)frame {
	if ((self = [super initWithFrame:frame])) {
		_image = JXImageView();
		_progress = JXThinProgress();
		_title = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.text];
		_subtitle = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textSecondary];
		for (UIView *v in @[_image, _progress, _title, _subtitle]) {
			v.translatesAutoresizingMaskIntoConstraints = YES;
			[self.contentView addSubview:v];
		}
	}
	return self;
}

- (void)layoutSubviews {
	[super layoutSubviews];
	CGFloat w = self.contentView.bounds.size.width;
	CGFloat h = w * 9 / 16;
	_image.frame = CGRectMake(0, 0, w, h);
	_progress.frame = CGRectMake(4, h - 6, w - 8, 3);
	_title.frame = CGRectMake(0, h + 8, w, 19);
	_subtitle.frame = CGRectMake(0, h + 29, w, 17);
}

- (void)setHighlighted:(BOOL)highlighted {
	[super setHighlighted:highlighted];
	self.contentView.alpha = highlighted ? 0.6 : 1;
}

- (void)configure:(JXItem *)item api:(JXJellyfin *)api {
	[_image jx_setImageURL:[api imageURL:item.wide maxWidth:480]];
	_title.text = JXDisplayTitle(item);
	NSMutableArray *parts = [NSMutableArray array];
	if ([item.type isEqualToString:@"Episode"]) [parts addObject:JXEpisodeLabel(item)];
	if (item.resumable) [parts addObject:JXFormatRemaining(item)];
	else if (item.runTimeTicks > 0) [parts addObject:JXFormatDuration(item.runTimeTicks)];
	_subtitle.text = [parts componentsJoinedByString:@" · "];
	_progress.hidden = !item.resumable;
	_progress.progress = JXProgress(item);
	self.accessibilityLabel = JXDisplayTitle(item);
	self.isAccessibilityElement = YES;
}

@end

#pragma mark - Row

@implementation JXRowView {
	UILabel *_title;
	UIButton *_action;
	UICollectionView *_list;
	NSArray<JXItem *> *_items;
	JXJellyfin *_api;
	BOOL _wide;
}

- (instancetype)initWithTitle:(NSString *)title wide:(BOOL)wide api:(JXJellyfin *)api items:(NSArray<JXItem *> *)items {
	if ((self = [super initWithFrame:CGRectZero])) {
		self.translatesAutoresizingMaskIntoConstraints = NO;
		_items = items;
		_api = api;
		_wide = wide;
		_title = [JXTheme labelWithSize:20 weight:UIFontWeightBold color:JXTheme.text];
		_title.text = title;
		_action = [UIButton buttonWithType:UIButtonTypeSystem];
		_action.translatesAutoresizingMaskIntoConstraints = NO;
		_action.titleLabel.font = [JXTheme fontOfSize:14 weight:UIFontWeightRegular];
		_action.hidden = YES;
		[_action addTarget:self action:@selector(actionTapped) forControlEvents:UIControlEventTouchUpInside];

		CGSize cell = wide ? CGSizeMake(240, 240 * 9 / 16 + JXWideCell.textHeight + 8) : CGSizeMake(124, 124 * 1.5 + JXPosterCell.textHeight + 8);
		UICollectionViewFlowLayout *layout = [[UICollectionViewFlowLayout alloc] init];
		layout.scrollDirection = UICollectionViewScrollDirectionHorizontal;
		layout.itemSize = cell;
		layout.minimumLineSpacing = 18;
		layout.sectionInset = UIEdgeInsetsMake(0, 40, 0, 32);
		_list = [[UICollectionView alloc] initWithFrame:CGRectZero collectionViewLayout:layout];
		_list.translatesAutoresizingMaskIntoConstraints = NO;
		_list.backgroundColor = UIColor.clearColor;
		_list.showsHorizontalScrollIndicator = NO;
		_list.dataSource = self;
		_list.delegate = self;
		[_list registerClass:wide ? JXWideCell.class : JXPosterCell.class forCellWithReuseIdentifier:@"c"];

		[self addSubview:_title];
		[self addSubview:_action];
		[self addSubview:_list];
		[NSLayoutConstraint activateConstraints:@[
			[_title.topAnchor constraintEqualToAnchor:self.topAnchor constant:16],
			[_title.leadingAnchor constraintEqualToAnchor:self.leadingAnchor constant:40],
			[_action.centerYAnchor constraintEqualToAnchor:_title.centerYAnchor],
			[_action.trailingAnchor constraintEqualToAnchor:self.trailingAnchor constant:-28],
			[_action.heightAnchor constraintEqualToConstant:44],
			[_list.topAnchor constraintEqualToAnchor:_title.bottomAnchor constant:12],
			[_list.leadingAnchor constraintEqualToAnchor:self.leadingAnchor],
			[_list.trailingAnchor constraintEqualToAnchor:self.trailingAnchor],
			[_list.heightAnchor constraintEqualToConstant:cell.height],
			[_list.bottomAnchor constraintEqualToAnchor:self.bottomAnchor],
		]];
	}
	return self;
}

- (void)setActionTitle:(NSString *)title {
	[_action setTitle:title forState:UIControlStateNormal];
	_action.hidden = title == nil;
}

- (void)actionTapped { if (self.onAction) self.onAction(); }

- (NSInteger)collectionView:(UICollectionView *)cv numberOfItemsInSection:(NSInteger)section { return _items.count; }

- (UICollectionViewCell *)collectionView:(UICollectionView *)cv cellForItemAtIndexPath:(NSIndexPath *)ip {
	id cell = [cv dequeueReusableCellWithReuseIdentifier:@"c" forIndexPath:ip];
	[cell configure:_items[ip.item] api:_api];
	return cell;
}

- (void)collectionView:(UICollectionView *)cv didSelectItemAtIndexPath:(NSIndexPath *)ip {
	if (self.onSelect) self.onSelect(_items[ip.item]);
}

@end

UICollectionViewFlowLayout *JXGridLayout(void) {
	UICollectionViewFlowLayout *l = [[UICollectionViewFlowLayout alloc] init];
	l.minimumInteritemSpacing = 16;
	l.minimumLineSpacing = 22;
	l.sectionInset = UIEdgeInsetsMake(18, 32, 28, 32);
	return l;
}

void JXUpdateGridLayout(UICollectionViewFlowLayout *l, CGFloat width) {
	if (width <= 0) return;
	CGFloat usable = width - l.sectionInset.left - l.sectionInset.right;
	NSInteger columns = MAX(3, (NSInteger)((usable + l.minimumInteritemSpacing) / (130 + l.minimumInteritemSpacing)));
	CGFloat w = floor((usable - l.minimumInteritemSpacing * (columns - 1)) / columns);
	CGSize size = CGSizeMake(w, w * 1.5 + JXPosterCell.textHeight);
	if (!CGSizeEqualToSize(l.itemSize, size)) l.itemSize = size;
}
