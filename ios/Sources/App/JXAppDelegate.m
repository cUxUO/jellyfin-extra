#import "JXAppDelegate.h"
#import <AVFoundation/AVFoundation.h>
#import "JXRootViewController.h"
#import "JXTheme.h"

@implementation JXAppDelegate

- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {
	// 海報與背景圖的磁碟快取；iPad Air 1 只有 1GB 記憶體，記憶體快取壓小
	NSURLCache.sharedURLCache = [[NSURLCache alloc] initWithMemoryCapacity:8 * 1024 * 1024
	                                                          diskCapacity:150 * 1024 * 1024
	                                                              diskPath:@"images"];
	[AVAudioSession.sharedInstance setCategory:AVAudioSessionCategoryPlayback error:nil];

	self.window = [[UIWindow alloc] initWithFrame:UIScreen.mainScreen.bounds];
	self.window.backgroundColor = JXTheme.bg;
	self.window.tintColor = JXTheme.accent;
	self.window.rootViewController = [[JXRootViewController alloc] init];
	[self.window makeKeyAndVisible];
	return YES;
}

@end
