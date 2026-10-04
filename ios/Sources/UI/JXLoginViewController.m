#import "JXLoginViewController.h"
#import "JXEndpoints.h"
#import "JXJellyfin.h"
#import "JXSettings.h"
#import "JXTheme.h"

@interface JXLoginViewController () <UITextFieldDelegate>
@end

@implementation JXLoginViewController {
	UITextField *_lanJellyfin, *_lanXcode, *_external, *_user, *_password;
	UISegmentedControl *_profile;
	UIButton *_login;
	UILabel *_status;
	UIScrollView *_scroll;
}

- (UIStatusBarStyle)preferredStatusBarStyle { return UIStatusBarStyleLightContent; }

- (UITextField *)field:(NSString *)placeholder secure:(BOOL)secure {
	UITextField *f = [[UITextField alloc] init];
	f.translatesAutoresizingMaskIntoConstraints = NO;
	f.attributedPlaceholder = [[NSAttributedString alloc] initWithString:placeholder attributes:@{NSForegroundColorAttributeName: JXTheme.textTertiary}];
	f.textColor = JXTheme.text;
	f.font = [JXTheme fontOfSize:17 weight:UIFontWeightRegular];
	f.backgroundColor = JXTheme.surface;
	f.layer.cornerRadius = 10;
	f.layer.borderWidth = 1;
	f.layer.borderColor = JXTheme.line.CGColor;
	f.leftView = [[UIView alloc] initWithFrame:CGRectMake(0, 0, 14, 1)];
	f.leftViewMode = UITextFieldViewModeAlways;
	f.autocapitalizationType = UITextAutocapitalizationTypeNone;
	f.autocorrectionType = UITextAutocorrectionTypeNo;
	f.secureTextEntry = secure;
	f.keyboardType = secure ? UIKeyboardTypeDefault : UIKeyboardTypeURL;
	f.keyboardAppearance = UIKeyboardAppearanceDark;
	f.delegate = self;
	[f.heightAnchor constraintEqualToConstant:48].active = YES;
	return f;
}

- (UIStackView *)labeled:(NSString *)label view:(UIView *)v {
	UILabel *l = [JXTheme labelWithSize:13 weight:UIFontWeightRegular color:JXTheme.textTertiary];
	l.text = label;
	UIStackView *s = [[UIStackView alloc] initWithArrangedSubviews:@[l, v]];
	s.axis = UILayoutConstraintAxisVertical;
	s.spacing = 6;
	return s;
}

- (void)viewDidLoad {
	[super viewDidLoad];
	self.view.backgroundColor = JXTheme.bg;
	JXSettings *st = JXSettings.shared;

	_lanJellyfin = [self field:@"http://192.168.x.x:8096" secure:NO];
	_lanXcode = [self field:@"http://192.168.x.x:8097" secure:NO];
	_external = [self field:@"https://example.com" secure:NO];
	_user = [self field:@"" secure:NO];
	_user.keyboardType = UIKeyboardTypeDefault;
	_password = [self field:@"" secure:YES];
	_lanJellyfin.text = st.lanJellyfin;
	_lanXcode.text = st.lanXcode;
	_external.text = st.external;
	_user.text = st.userName;

	_profile = [[UISegmentedControl alloc] initWithItems:@[@"ipad-air1", @"zenpad10"]];
	_profile.selectedSegmentIndex = [st.profile isEqualToString:@"zenpad10"] ? 1 : 0;
	_profile.tintColor = JXTheme.accent;

	_login = [JXTheme primaryButton:@"登入" icon:nil];
	[_login addTarget:self action:@selector(loginTapped) forControlEvents:UIControlEventTouchUpInside];
	_status = [JXTheme labelWithSize:15 weight:UIFontWeightRegular color:JXTheme.textSecondary];
	_status.numberOfLines = 0;

	UILabel *title = [JXTheme labelWithSize:32 weight:UIFontWeightBold color:JXTheme.text];
	title.text = @"連線設定";

	UIStackView *form = [[UIStackView alloc] initWithArrangedSubviews:@[
		title,
		[self labeled:@"內網 Jellyfin 位址" view:_lanJellyfin],
		[self labeled:@"內網轉碼伺服器位址" view:_lanXcode],
		[self labeled:@"外部網址（選填）" view:_external],
		[self labeled:@"裝置規格" view:_profile],
		[self labeled:@"帳號" view:_user],
		[self labeled:@"密碼" view:_password],
		_login, _status,
	]];
	form.translatesAutoresizingMaskIntoConstraints = NO;
	form.axis = UILayoutConstraintAxisVertical;
	form.spacing = 16;
	[form setCustomSpacing:28 afterView:title];
	[form setCustomSpacing:28 afterView:form.arrangedSubviews[6]];

	_scroll = [[UIScrollView alloc] init];
	_scroll.translatesAutoresizingMaskIntoConstraints = NO;
	_scroll.keyboardDismissMode = UIScrollViewKeyboardDismissModeInteractive;
	[self.view addSubview:_scroll];
	[_scroll addSubview:form];
	[NSLayoutConstraint activateConstraints:@[
		[_scroll.leadingAnchor constraintEqualToAnchor:self.view.leadingAnchor],
		[_scroll.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor],
		[_scroll.topAnchor constraintEqualToAnchor:self.view.safeAreaLayoutGuide.topAnchor],
		[_scroll.bottomAnchor constraintEqualToAnchor:self.view.bottomAnchor],
		[form.topAnchor constraintEqualToAnchor:_scroll.topAnchor constant:40],
		[form.bottomAnchor constraintEqualToAnchor:_scroll.bottomAnchor constant:-40],
		[form.centerXAnchor constraintEqualToAnchor:_scroll.centerXAnchor],
		[form.widthAnchor constraintEqualToConstant:520],
	]];
	if (self.onCancel) {
		UIButton *cancel = [UIButton buttonWithType:UIButtonTypeSystem];
		cancel.translatesAutoresizingMaskIntoConstraints = NO;
		[cancel setTitle:@"取消" forState:UIControlStateNormal];
		cancel.titleLabel.font = [JXTheme fontOfSize:17 weight:UIFontWeightRegular];
		[cancel addTarget:self action:@selector(cancelTapped) forControlEvents:UIControlEventTouchUpInside];
		[self.view addSubview:cancel];
		[NSLayoutConstraint activateConstraints:@[
			[cancel.topAnchor constraintEqualToAnchor:self.view.safeAreaLayoutGuide.topAnchor constant:12],
			[cancel.trailingAnchor constraintEqualToAnchor:self.view.trailingAnchor constant:-28],
			[cancel.heightAnchor constraintEqualToConstant:44],
		]];
	}
	[[NSNotificationCenter defaultCenter] addObserver:self selector:@selector(keyboard:) name:UIKeyboardWillChangeFrameNotification object:nil];
}

- (void)keyboard:(NSNotification *)n {
	CGRect end = [self.view convertRect:[n.userInfo[UIKeyboardFrameEndUserInfoKey] CGRectValue] fromView:nil];
	CGFloat overlap = MAX(0, CGRectGetMaxY(self.view.bounds) - end.origin.y);
	_scroll.contentInset = UIEdgeInsetsMake(0, 0, overlap, 0);
}

- (void)cancelTapped {
	[self.view endEditing:YES];
	self.onCancel();
}

- (BOOL)textFieldShouldReturn:(UITextField *)f {
	if (f == _password) [self loginTapped]; else [f resignFirstResponder];
	return YES;
}

- (void)loginTapped {
	JXSettings *st = JXSettings.shared;
	st.lanJellyfin = _lanJellyfin.text;
	st.lanXcode = _lanXcode.text;
	st.external = _external.text;
	st.profile = _profile.selectedSegmentIndex == 1 ? @"zenpad10" : @"ipad-air1";
	NSString *user = [_user.text stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
	NSString *pw = _password.text ?: @"";
	[self.view endEditing:YES];
	_login.enabled = NO;
	_login.alpha = 0.5;
	_status.text = @"連線中…";
	__weak typeof(self) weakSelf = self;
	[JXEndpoints resolveJellyfinWithLAN:st.lanJellyfin external:st.external completion:^(NSURL *base) {
		if (!base) { [weakSelf fail:@"連不到 Jellyfin，請檢查位址"]; return; }
		[[[JXJellyfin alloc] initWithBase:base] authenticateUser:user password:pw
		                                              completion:^(NSString *token, NSString *userId, NSString *userName, NSError *error) {
			if (error) { [weakSelf fail:error.code == 401 ? @"帳號或密碼錯誤" : [@"登入失敗：" stringByAppendingString:error.localizedDescription]]; return; }
			[JXSettings.shared saveToken:token userId:userId userName:userName];
			if (weakSelf.onLoggedIn) weakSelf.onLoggedIn();
		}];
	}];
}

- (void)fail:(NSString *)message {
	_login.enabled = YES;
	_login.alpha = 1;
	_status.text = message;
}

@end
