#import "JXPlaybackLog.h"

#ifdef DEBUG
static NSFileHandle *gHandle;
static NSDateFormatter *gTime;

static NSString *LogPath(void) {
	NSString *caches = NSSearchPathForDirectoriesInDomains(NSCachesDirectory, NSUserDomainMask, YES).firstObject;
	NSString *dir = [caches stringByAppendingPathComponent:NSBundle.mainBundle.bundleIdentifier ?: @"JellyfinExtra"];
	[NSFileManager.defaultManager createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:nil];
	return [dir stringByAppendingPathComponent:@"playback.log"];
}
#endif

void JXPlaybackLogReset(void) {
#ifdef DEBUG
	NSString *path = LogPath();
	[gHandle closeFile];
	[NSData.data writeToFile:path atomically:NO];
	gHandle = [NSFileHandle fileHandleForWritingAtPath:path];
	if (!gTime) {
		gTime = [[NSDateFormatter alloc] init];
		gTime.dateFormat = @"HH:mm:ss.SSS";
	}
#endif
}

void JXPlaybackLog(NSString *format, ...) {
#ifdef DEBUG
	if (!gHandle) return;
	va_list args;
	va_start(args, format);
	NSString *msg = [[NSString alloc] initWithFormat:format arguments:args];
	va_end(args);
	NSString *line = [NSString stringWithFormat:@"%@ %@\n", [gTime stringFromDate:NSDate.date], msg];
	[gHandle seekToEndOfFile];
	[gHandle writeData:[line dataUsingEncoding:NSUTF8StringEncoding]];
#endif
}
