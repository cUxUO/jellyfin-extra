#import "JXTrackPicker.h"

@implementation JXTrackPicker

+ (UIAlertController *)sheet:(NSString *)title source:(UIView *)source {
	UIAlertController *a = [UIAlertController alertControllerWithTitle:title message:nil preferredStyle:UIAlertControllerStyleActionSheet];
	a.popoverPresentationController.sourceView = source;
	a.popoverPresentationController.sourceRect = source.bounds;
	a.popoverPresentationController.permittedArrowDirections = UIPopoverArrowDirectionUp | UIPopoverArrowDirectionDown;
	return a;
}

+ (NSString *)mark:(NSString *)label selected:(BOOL)selected {
	return selected ? [@"✓ " stringByAppendingString:label] : label;
}

+ (void)pickAudioFrom:(UIViewController *)vc source:(UIView *)source tracks:(NSArray<JXAudioTrack *> *)tracks
              current:(NSInteger)current onPick:(void (^)(JXAudioTrack *))onPick {
	UIAlertController *a = [self sheet:@"音軌" source:source];
	for (JXAudioTrack *t in tracks) {
		[a addAction:[UIAlertAction actionWithTitle:[self mark:t.title selected:t.index == current] style:UIAlertActionStyleDefault
		                                    handler:^(UIAlertAction *x) { onPick(t); }]];
	}
	[vc presentViewController:a animated:YES completion:nil];
}

+ (void)pickSubtitleFrom:(UIViewController *)vc source:(UIView *)source tracks:(NSArray<JXSubtitleTrack *> *)tracks
                 current:(JXSubtitleTrack *)current onPick:(void (^)(JXSubtitleTrack *))onPick {
	UIAlertController *a = [self sheet:@"字幕" source:source];
	[a addAction:[UIAlertAction actionWithTitle:[self mark:@"關閉" selected:current == nil] style:UIAlertActionStyleDefault
	                                    handler:^(UIAlertAction *x) { onPick(nil); }]];
	for (JXSubtitleTrack *t in tracks) {
		if (!t.playable) continue;
		[a addAction:[UIAlertAction actionWithTitle:[self mark:[self subtitleLabel:t] selected:t.index == current.index && current]
		                                      style:UIAlertActionStyleDefault handler:^(UIAlertAction *x) { onPick(t); }]];
	}
	[vc presentViewController:a animated:YES completion:nil];
}

+ (NSString *)subtitleLabel:(JXSubtitleTrack *)t {
	return t.isImage ? [t.title stringByAppendingString:@"（圖形，燒進畫面）"] : t.title;
}

@end
