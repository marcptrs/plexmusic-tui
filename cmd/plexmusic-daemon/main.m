#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>
#import "daemon.h"

int main(int argc, const char * argv[]) {
    @autoreleasepool {
        NSLog(@"[PlexMusicDaemon] Starting daemon");

        // Create and start the daemon
        PlexMusicDaemon *daemon = [[PlexMusicDaemon alloc] init];
        if (![daemon start]) {
            NSLog(@"[PlexMusicDaemon] Failed to start daemon");
            return 1;
        }

        NSLog(@"[PlexMusicDaemon] Daemon started successfully");

        // Run the application event loop
        [NSApp run];

        // Cleanup
        [daemon stop];
    }
    return 0;
}
