#import "JXSettings.h"

@implementation JXSettings

+ (instancetype)shared {
	static JXSettings *s;
	static dispatch_once_t once;
	dispatch_once(&once, ^{ s = [[JXSettings alloc] init]; });
	return s;
}

- (NSUserDefaults *)d { return NSUserDefaults.standardUserDefaults; }
- (NSString *)str:(NSString *)key { return [self.d stringForKey:key] ?: @""; }
- (void)set:(NSString *)key value:(NSString *)v {
	[self.d setObject:[v stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] forKey:key];
}

- (NSString *)lanJellyfin { return [self str:@"lan_jellyfin"]; }
- (void)setLanJellyfin:(NSString *)v { [self set:@"lan_jellyfin" value:v]; }
- (NSString *)lanXcode { return [self str:@"lan_xcode"]; }
- (void)setLanXcode:(NSString *)v { [self set:@"lan_xcode" value:v]; }
- (NSString *)external { return [self str:@"external"]; }
- (void)setExternal:(NSString *)v { [self set:@"external" value:v]; }
- (NSString *)profile { return [self.d stringForKey:@"profile"] ?: @"ipad-air1"; }
- (void)setProfile:(NSString *)v { [self.d setObject:v forKey:@"profile"]; }
- (NSString *)token { return [self str:@"token"]; }
- (NSString *)userId { return [self str:@"user_id"]; }
- (NSString *)userName { return [self str:@"user_name"]; }
- (BOOL)loggedIn { return self.token.length > 0 && self.userId.length > 0; }

- (NSString *)deviceId {
	NSString *v = [self.d stringForKey:@"device_id"];
	if (!v) {
		v = NSUUID.UUID.UUIDString;
		[self.d setObject:v forKey:@"device_id"];
	}
	return v;
}

- (void)saveToken:(NSString *)token userId:(NSString *)userId userName:(NSString *)userName {
	[self.d setObject:token forKey:@"token"];
	[self.d setObject:userId forKey:@"user_id"];
	[self.d setObject:userName ?: @"" forKey:@"user_name"];
}

- (void)logout {
	[self.d removeObjectForKey:@"token"];
	[self.d removeObjectForKey:@"user_id"];
	[self.d removeObjectForKey:@"user_name"];
}

@end
