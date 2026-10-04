#import "JXLibraryViewController.h"
#import "JXCells.h"
#import "JXChips.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXRootViewController.h"
#import "JXTheme.h"

@interface JXLibraryViewController () <UICollectionViewDataSource, UICollectionViewDelegate>
@end

@implementation JXLibraryViewController {
	JXItem *_view;
	JXJellyfin *_api;
	NSArray<JXItem *> *_items;
	NSArray<NSString *> *_genres;
	JXSort _sort;
	NSString *_genre;
	UILabel *_count;
	UISegmentedControl *_sortControl;
	JXChips *_chips;
	UICollectionView *_grid;
	UICollectionViewFlowLayout *_layout;
	UIActivityIndicatorView *_spinner;
	NSInteger _generation;
}

- (instancetype)initWithView:(JXItem *)view {
	if ((self = [super init])) _view = view;
	return self;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	_api = JXJellyfin.current;

	BOOL root = self.navigationController.viewControllers.firstObject == self;
	UIButton *back = [JXTheme roundIconButton:JXIcons.back size:44 label:@"返回"];
	[back addTarget:self action:@selector(back) forControlEvents:UIControlEventTouchUpInside];
	back.hidden = root;
	UILabel *title = [JXTheme labelWithSize:32 weight:UIFontWeightBold color:JXTheme.text];
	title.text = [_view.collectionType isEqualToString:@"playlists"] ? @"播放清單" : _view.name;
	_count = [JXTheme labelWithSize:14 weight:UIFontWeightRegular color:JXTheme.textTertiary];

	_sortControl = [[UISegmentedControl alloc] initWithItems:@[@"最新加入", @"名稱", @"年份", @"評分"]];
	_sortControl.translatesAutoresizingMaskIntoConstraints = NO;
	_sortControl.selectedSegmentIndex = 0;
	_sortControl.tintColor = JXTheme.accent;
	[_sortControl addTarget:self action:@selector(sortChanged) forControlEvents:UIControlEventValueChanged];
	BOOL sortable = [_view.collectionType isEqualToString:@"movies"] || [_view.collectionType isEqualToString:@"tvshows"];
	_sortControl.hidden = !sortable;
	if (!sortable) _sort = JXSortName;

	_chips = [[JXChips alloc] init];
	_chips.hidden = YES;
	__weak typeof(self) weakSelf = self;
	_chips.onSelect = ^(NSInteger i) { [weakSelf genreSelected:i]; };

	_layout = JXGridLayout();
	_grid = [[UICollectionView alloc] initWithFrame:CGRectZero collectionViewLayout:_layout];
	_grid.translatesAutoresizingMaskIntoConstraints = NO;
	_grid.backgroundColor = UIColor.clearColor;
	_grid.dataSource = self;
	_grid.delegate = self;
	[_grid registerClass:JXPosterCell.class forCellWithReuseIdentifier:@"c"];

	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;

	for (UIView *v in @[back, title, _count, _sortControl, _chips, _grid, _spinner]) [self.view addSubview:v];
	UILayoutGuide *safe = self.view.safeAreaLayoutGuide;
	NSLayoutXAxisAnchor *titleLeading = root ? self.view.leadingAnchor : back.trailingAnchor;
	[NSLayoutConstraint activateConstraints:@[
		[back.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor constant:28],
		[back.centerYAnchor constraintEqualToAnchor:title.centerYAnchor],
		[title.topAnchor constraintEqualToAnchor:safe.topAnchor constant:16],
		[title.leadingAnchor constraintEqualToAnchor:titleLeading constant:root ? 32 : 14],
		[_count.firstBaselineAnchor constraintEqualToAnchor:title.firstBaselineAnchor],
		[_count.leadingAnchor constraintEqualToAnchor:title.trailingAnchor constant:14],
		[_sortControl.centerYAnchor constraintEqualToAnchor:title.centerYAnchor],
		[_sortControl.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor constant:-32],
		[_sortControl.widthAnchor constraintEqualToConstant:320],
		[_chips.topAnchor constraintEqualToAnchor:title.bottomAnchor constant:14],
		[_chips.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_chips.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_grid.topAnchor constraintEqualToAnchor:title.bottomAnchor constant:8],
		[_grid.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_grid.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_grid.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
	]];
	_chips.contentInset = UIEdgeInsetsMake(0, 32, 0, 32);

	if (sortable) {
		[_api genresInParent:_view.itemId completion:^(NSArray<NSString *> *names, NSError *error) {
			[weakSelf showGenres:names];
		}];
	}
	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(load) name:JXPlaybackDidFinishNotification object:nil];
	[self load];
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	JXUpdateGridLayout(_layout, _grid.bounds.size.width);
}

- (void)showGenres:(NSArray<NSString *> *)names {
	if (!names.count) return;
	_genres = names;
	[_chips setTitles:[@[@"全部"] arrayByAddingObjectsFromArray:names]];
	_chips.hidden = NO;
	_layout.sectionInset = UIEdgeInsetsMake(64, 32, 28, 32);
	[_layout invalidateLayout];
	[self.view bringSubviewToFront:_chips];
	_grid.scrollIndicatorInsets = UIEdgeInsetsMake(56, 0, 0, 0);
}

- (void)genreSelected:(NSInteger)i {
	_genre = i == 0 ? nil : _genres[i - 1];
	[self load];
}

- (void)sortChanged {
	_sort = (JXSort)_sortControl.selectedSegmentIndex;
	[self load];
}

- (void)load {
	NSInteger gen = ++_generation;
	if (!_items.count) [_spinner startAnimating];
	__weak typeof(self) weakSelf = self;
	[_api library:_view sort:_sort genre:_genre completion:^(NSArray<JXItem *> *items, NSInteger total, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s || gen != s->_generation) return; // 已換排序或篩選
		[s->_spinner stopAnimating];
		if (error) { s->_count.text = error.localizedDescription; return; }
		BOOL reset = s->_items.count == 0 || ![[s->_items valueForKey:@"itemId"] isEqualToArray:[items valueForKey:@"itemId"]];
		s->_items = items;
		s->_count.text = [NSString stringWithFormat:@"%ld 部", (long)total];
		CGPoint offset = s->_grid.contentOffset;
		[s->_grid reloadData];
		// 只是更新進度時保留捲動位置；換排序或篩選才回到最上面
		if (reset) [s->_grid setContentOffset:CGPointMake(0, -s->_grid.adjustedContentInset.top) animated:NO];
		else s->_grid.contentOffset = offset;
	}];
}

- (void)back { [self.navigationController popViewControllerAnimated:YES]; }

- (NSInteger)collectionView:(UICollectionView *)cv numberOfItemsInSection:(NSInteger)section { return _items.count; }

- (UICollectionViewCell *)collectionView:(UICollectionView *)cv cellForItemAtIndexPath:(NSIndexPath *)ip {
	JXPosterCell *cell = [cv dequeueReusableCellWithReuseIdentifier:@"c" forIndexPath:ip];
	[cell configure:_items[ip.item] api:_api];
	return cell;
}

- (void)collectionView:(UICollectionView *)cv didSelectItemAtIndexPath:(NSIndexPath *)ip {
	[JXRoot() openItem:_items[ip.item]];
}

@end
