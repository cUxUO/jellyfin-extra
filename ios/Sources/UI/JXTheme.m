#import "JXTheme.h"

UIColor *JXColorA(uint32_t rgb, CGFloat alpha) {
	return [UIColor colorWithRed:((rgb >> 16) & 0xFF) / 255.0 green:((rgb >> 8) & 0xFF) / 255.0 blue:(rgb & 0xFF) / 255.0 alpha:alpha];
}

UIColor *JXColor(uint32_t rgb) {
	return JXColorA(rgb, 1);
}

@implementation JXTheme

+ (UIColor *)bg { return JXColor(0x0E1014); }
+ (UIColor *)surface { return JXColor(0x171A20); }
+ (UIColor *)surfaceHigh { return JXColor(0x20242C); }
+ (UIColor *)line { return JXColor(0x2A2F38); }
+ (UIColor *)outline { return JXColor(0x4A505B); }
+ (UIColor *)text { return JXColor(0xF1EFEA); }
+ (UIColor *)textSecondary { return JXColor(0xA9AEB7); }
+ (UIColor *)textTertiary { return JXColor(0x8A909A); }
+ (UIColor *)textBody { return JXColor(0xC9CDD4); }
+ (UIColor *)accent { return JXColor(0xF2A93B); }
+ (UIColor *)onAccent { return JXColor(0x1A1206); }
+ (UIColor *)progressTrack { return JXColor(0x3A3F49); }
+ (UIColor *)placeholder { return JXColor(0x232832); }

+ (UIFont *)fontOfSize:(CGFloat)size weight:(UIFontWeight)weight {
	return [UIFont systemFontOfSize:size weight:weight];
}

+ (UILabel *)labelWithSize:(CGFloat)size weight:(UIFontWeight)weight color:(UIColor *)color {
	UILabel *l = [[UILabel alloc] init];
	l.font = [self fontOfSize:size weight:weight];
	l.textColor = color;
	l.translatesAutoresizingMaskIntoConstraints = NO;
	return l;
}

+ (UIButton *)pill:(NSString *)title {
	UIButton *b = [UIButton buttonWithType:UIButtonTypeCustom];
	b.translatesAutoresizingMaskIntoConstraints = NO;
	[b setTitle:title forState:UIControlStateNormal];
	b.titleLabel.font = [self fontOfSize:16 weight:UIFontWeightSemibold];
	b.layer.cornerRadius = 24;
	b.contentEdgeInsets = UIEdgeInsetsMake(0, 22, 0, 22);
	[b.heightAnchor constraintEqualToConstant:48].active = YES;
	return b;
}

+ (UIButton *)primaryButton:(NSString *)title icon:(UIImage *)icon {
	UIButton *b = [self pill:title];
	b.backgroundColor = self.accent;
	[b setTitleColor:self.onAccent forState:UIControlStateNormal];
	[b setTitleColor:JXColorA(0x1A1206, 0.5) forState:UIControlStateHighlighted];
	if (icon) {
		[b setImage:icon forState:UIControlStateNormal];
		b.tintColor = self.onAccent;
		b.imageEdgeInsets = UIEdgeInsetsMake(0, -6, 0, 6);
		b.contentEdgeInsets = UIEdgeInsetsMake(0, 26, 0, 22);
	}
	return b;
}

+ (UIButton *)outlineButton:(NSString *)title {
	UIButton *b = [self pill:title];
	b.layer.borderWidth = 1;
	b.layer.borderColor = self.outline.CGColor;
	[b setTitleColor:self.text forState:UIControlStateNormal];
	[b setTitleColor:self.textTertiary forState:UIControlStateHighlighted];
	return b;
}

+ (UIButton *)roundIconButton:(UIImage *)icon size:(CGFloat)size label:(NSString *)label {
	UIButton *b = [UIButton buttonWithType:UIButtonTypeSystem];
	b.translatesAutoresizingMaskIntoConstraints = NO;
	[b setImage:icon forState:UIControlStateNormal];
	b.tintColor = self.text;
	b.backgroundColor = JXColorA(0x0E1014, 0.55);
	b.layer.cornerRadius = size / 2;
	b.accessibilityLabel = label;
	[b.widthAnchor constraintEqualToConstant:size].active = YES;
	[b.heightAnchor constraintEqualToConstant:size].active = YES;
	return b;
}

+ (CAGradientLayer *)leftScrim {
	CAGradientLayer *g = [CAGradientLayer layer];
	g.colors = @[(id)JXColorA(0x0E1014, 1).CGColor, (id)JXColorA(0x0E1014, 0.88).CGColor, (id)JXColorA(0x0E1014, 0).CGColor];
	g.locations = @[@0, @0.42, @0.78];
	g.startPoint = CGPointMake(0, 0.5);
	g.endPoint = CGPointMake(1, 0.5);
	return g;
}

+ (CAGradientLayer *)bottomScrim {
	CAGradientLayer *g = [CAGradientLayer layer];
	g.colors = @[(id)JXColorA(0x0E1014, 0).CGColor, (id)JXColorA(0x0E1014, 0).CGColor, (id)JXColorA(0x0E1014, 1).CGColor];
	g.locations = @[@0, @0.62, @1];
	return g;
}

@end
