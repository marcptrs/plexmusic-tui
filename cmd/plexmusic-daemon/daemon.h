#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>
#import <MediaPlayer/MediaPlayer.h>

@interface PlexMusicDaemon : NSObject

- (BOOL)start;
- (void)stop;

@end
