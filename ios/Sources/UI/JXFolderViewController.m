#import "JXFolderViewController.h"
#import "JXCells.h"
#import "JXIcons.h"
#import "JXJellyfin.h"
#import "JXRootViewController.h"
#import "JXTheme.h"

@interface JXFolderViewController () <UICollectionViewDataSource, UICollectionViewDelegate, UITextFieldDelegate>
@end

@implementation JXFolderViewController {
	JXItem *_folder; // nil 表示搜尋
	JXJellyfin *_api;
	NSArray<JXItem *> *_items;
	UITextField *_field;
	UILabel *_message;
	UICollectionView *_grid;
	UICollectionViewFlowLayout *_layout;
	UIActivityIndicatorView *_spinner;
	NSTimer *_debounce;
	NSInteger _generation;
}

+ (instancetype)folder:(JXItem *)item {
	JXFolderViewController *vc = [[self alloc] init];
	vc->_folder = item;
	return vc;
}

+ (instancetype)search { return [[self alloc] init]; }

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	_api = JXJellyfin.current;

	UIView *header;
	if (_folder) {
		UIButton *back = [JXTheme roundIconButton:JXIcons.back size:44 label:@"返回"];
		[back addTarget:self action:@selector(back) forControlEvents:UIControlEventTouchUpInside];
		UILabel *title = [JXTheme labelWithSize:32 weight:UIFontWeightBold color:JXTheme.text];
		title.text = _folder.name;
		UIStackView *h = [[UIStackView alloc] initWithArrangedSubviews:@[back, title]];
		h.spacing = 14;
		h.alignment = UIStackViewAlignmentCenter;
		header = h;
	} else {
		_field = [[UITextField alloc] init];
		_field.attributedPlaceholder = [[NSAttributedString alloc] initWithString:@"搜尋電影、影集、集數"
		                                                               attributes:@{NSForegroundColorAttributeName: JXTheme.textTertiary}];
		_field.textColor = JXTheme.text;
		_field.font = [JXTheme fontOfSize:18 weight:UIFontWeightRegular];
		_field.backgroundColor = JXTheme.surface;
		_field.layer.cornerRadius = 24;
		_field.layer.borderWidth = 1;
		_field.layer.borderColor = JXTheme.line.CGColor;
		UIImageView *icon = [[UIImageView alloc] initWithImage:JXIcons.search];
		icon.tintColor = JXTheme.textTertiary;
		icon.contentMode = UIViewContentModeCenter;
		icon.frame = CGRectMake(0, 0, 52, 48);
		_field.leftView = icon;
		_field.leftViewMode = UITextFieldViewModeAlways;
		_field.clearButtonMode = UITextFieldViewModeWhileEditing;
		_field.returnKeyType = UIReturnKeySearch;
		_field.keyboardAppearance = UIKeyboardAppearanceDark;
		_field.autocorrectionType = UITextAutocorrectionTypeNo;
		_field.delegate = self;
		[_field addTarget:self action:@selector(textChanged) forControlEvents:UIControlEventEditingChanged];
		[_field.heightAnchor constraintEqualToConstant:48].active = YES;
		header = _field;
	}
	header.translatesAutoresizingMaskIntoConstraints = NO;

	_layout = JXGridLayout();
	_grid = [[UICollectionView alloc] initWithFrame:CGRectZero collectionViewLayout:_layout];
	_grid.translatesAutoresizingMaskIntoConstraints = NO;
	_grid.backgroundColor = UIColor.clearColor;
	_grid.keyboardDismissMode = UIScrollViewKeyboardDismissModeOnDrag;
	_grid.dataSource = self;
	_grid.delegate = self;
	[_grid registerClass:JXPosterCell.class forCellWithReuseIdentifier:@"c"];

	_message = [JXTheme labelWithSize:16 weight:UIFontWeightRegular color:JXTheme.textTertiary];
	_message.textAlignment = NSTextAlignmentCenter;
	_message.numberOfLines = 0;
	_spinner = [[UIActivityIndicatorView alloc] initWithActivityIndicatorStyle:UIActivityIndicatorViewStyleWhiteLarge];
	_spinner.translatesAutoresizingMaskIntoConstraints = NO;
	_spinner.color = JXTheme.accent;

	for (UIView *v in @[header, _grid, _message, _spinner]) [self.view addSubview:v];
	[NSLayoutConstraint activateConstraints:@[
		[header.topAnchor constraintEqualToAnchor:self.view.safeAreaLayoutGuide.topAnchor constant:16],
		[header.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor constant:_folder ? 28 : 32],
		[header.trailingAnchor constraintLessThanOrEqualToAnchor:self.view.trailingAnchor constant:-32],
		[_grid.topAnchor constraintEqualToAnchor:header.bottomAnchor constant:4],
		[_grid.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_grid.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_grid.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[_message.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_message.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
		[_spinner.centerXAnchor constraintEqualToAnchor:self.view.centerXAnchor],
		[_spinner.centerYAnchor constraintEqualToAnchor:self.view.centerYAnchor],
	]];
	if (!_folder) [_field.widthAnchor constraintEqualToConstant:560].active = YES;

	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(reload) name:JXPlaybackDidFinishNotification object:nil];
	if (_folder) [self reload];
}

- (void)viewDidAppear:(BOOL)animated {
	[super viewDidAppear:animated];
	if (!_folder && !_items.count && !_field.text.length) [_field becomeFirstResponder];
}

- (void)viewDidLayoutSubviews {
	[super viewDidLayoutSubviews];
	JXUpdateGridLayout(_layout, _grid.bounds.size.width);
}

- (void)back { [self.navigationController popViewControllerAnimated:YES]; }

- (void)textChanged {
	[_debounce invalidate];
	__weak typeof(self) weakSelf = self;
	_debounce = [NSTimer scheduledTimerWithTimeInterval:0.4 repeats:NO block:^(NSTimer *t) { [weakSelf reload]; }];
}

- (BOOL)textFieldShouldReturn:(UITextField *)f {
	[_debounce invalidate];
	[f resignFirstResponder];
	[self reload];
	return YES;
}

- (void)reload {
	NSInteger gen = ++_generation;
	__weak typeof(self) weakSelf = self;
	JXItemsCompletion done = ^(NSArray<JXItem *> *items, NSError *error) {
		typeof(self) s = weakSelf;
		if (!s || gen != s->_generation) return;
		[s->_spinner stopAnimating];
		s->_items = items ?: @[];
		[s->_grid reloadData];
		if (error) s->_message.text = error.localizedDescription;
		else s->_message.text = s->_items.count ? nil : (s->_folder ? @"這裡是空的" : @"找不到符合的項目");
	};
	if (_folder) {
		if (!_items.count) [_spinner startAnimating];
		[_api children:_folder completion:done];
		return;
	}
	NSString *term = [_field.text stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
	if (!term.length) {
		_items = @[];
		[_grid reloadData];
		_message.text = nil;
		return;
	}
	[_spinner startAnimating];
	[_api search:term completion:done];
}

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
