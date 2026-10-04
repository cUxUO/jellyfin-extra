#import "JXChips.h"
#import "JXTheme.h"

@implementation JXChips {
	UIStackView *_stack;
}

- (instancetype)initWithFrame:(CGRect)frame {
	if ((self = [super initWithFrame:frame])) {
		self.translatesAutoresizingMaskIntoConstraints = NO;
		self.showsHorizontalScrollIndicator = NO;
		_stack = [[UIStackView alloc] init];
		_stack.translatesAutoresizingMaskIntoConstraints = NO;
		_stack.spacing = 8;
		[self addSubview:_stack];
		[NSLayoutConstraint activateConstraints:@[
			[_stack.leadingAnchor constraintEqualToAnchor:self.leadingAnchor],
			[_stack.trailingAnchor constraintEqualToAnchor:self.trailingAnchor],
			[_stack.topAnchor constraintEqualToAnchor:self.topAnchor],
			[_stack.bottomAnchor constraintEqualToAnchor:self.bottomAnchor],
			[_stack.heightAnchor constraintEqualToAnchor:self.heightAnchor],
			[self.heightAnchor constraintEqualToConstant:40],
		]];
	}
	return self;
}

- (void)setTitles:(NSArray<NSString *> *)titles {
	for (UIView *v in _stack.arrangedSubviews) [v removeFromSuperview];
	[titles enumerateObjectsUsingBlock:^(NSString *t, NSUInteger i, BOOL *stop) {
		UIButton *b = [UIButton buttonWithType:UIButtonTypeCustom];
		[b setTitle:t forState:UIControlStateNormal];
		b.tag = i;
		b.layer.cornerRadius = 20;
		b.layer.borderWidth = 1;
		b.contentEdgeInsets = UIEdgeInsetsMake(0, 18, 0, 18);
		[b addTarget:self action:@selector(tapped:) forControlEvents:UIControlEventTouchUpInside];
		[self->_stack addArrangedSubview:b];
	}];
	[self restyle];
}

- (void)setSelectedIndex:(NSInteger)selectedIndex {
	_selectedIndex = selectedIndex;
	[self restyle];
}

- (void)restyle {
	for (UIButton *b in _stack.arrangedSubviews) {
		BOOL on = b.tag == _selectedIndex;
		b.backgroundColor = on ? JXTheme.accent : UIColor.clearColor;
		b.layer.borderColor = (on ? JXTheme.accent : JXTheme.outline).CGColor;
		b.titleLabel.font = [JXTheme fontOfSize:14 weight:on ? UIFontWeightBold : UIFontWeightRegular];
		[b setTitleColor:on ? JXTheme.onAccent : JXTheme.text forState:UIControlStateNormal];
		b.accessibilityTraits = on ? (UIAccessibilityTraitButton | UIAccessibilityTraitSelected) : UIAccessibilityTraitButton;
	}
}

- (void)tapped:(UIButton *)b {
	if (b.tag == _selectedIndex) return;
	self.selectedIndex = b.tag;
	if (self.onSelect) self.onSelect(b.tag);
}

@end
