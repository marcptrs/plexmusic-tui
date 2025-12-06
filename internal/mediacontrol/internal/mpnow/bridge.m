#import <Foundation/Foundation.h>
#import <MediaPlayer/MediaPlayer.h>
#import <AppKit/AppKit.h>
#import "bridge.h"

@interface MediaControlBridge : NSObject {
    @public
    MediaControlCallbacks callbacks;
}
@end

@implementation MediaControlBridge

- (instancetype)init {
    self = [super init];
    if (self) {
        NSLog(@"[MediaControl] Initializing media control bridge");

        // Initialize callbacks to NULL
        memset(&callbacks, 0, sizeof(MediaControlCallbacks));

        // CRITICAL: Create NSApplication to make macOS treat us as a GUI app
        // This allows us to receive media key events
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        NSLog(@"[MediaControl] Created NSApplication with Accessory policy");

        // Set up remote command center handlers
        MPRemoteCommandCenter *commandCenter = [MPRemoteCommandCenter sharedCommandCenter];
        NSLog(@"[MediaControl] Got shared command center: %@", commandCenter);

        // Play command
        [[commandCenter playCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            NSLog(@"[MediaControl] Play command received");
            if (self->callbacks.play != NULL) {
                self->callbacks.play();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            NSLog(@"[MediaControl] Play callback is NULL");
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Pause command
        [[commandCenter pauseCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            if (self->callbacks.pause != NULL) {
                self->callbacks.pause();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Toggle play/pause command
        [[commandCenter togglePlayPauseCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            NSLog(@"[MediaControl] Toggle play/pause command received");
            if (self->callbacks.togglePlayPause != NULL) {
                self->callbacks.togglePlayPause();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            NSLog(@"[MediaControl] Toggle play/pause callback is NULL");
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Stop command
        [[commandCenter stopCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            if (self->callbacks.stop != NULL) {
                self->callbacks.stop();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Next track command
        [[commandCenter nextTrackCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            if (self->callbacks.next != NULL) {
                self->callbacks.next();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Previous track command
        [[commandCenter previousTrackCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            if (self->callbacks.previous != NULL) {
                self->callbacks.previous();
                return MPRemoteCommandHandlerStatusSuccess;
            }
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Change playback position command (for scrubbing)
        [[commandCenter changePlaybackPositionCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
            if (self->callbacks.seek != NULL) {
                MPChangePlaybackPositionCommandEvent *seekEvent = (MPChangePlaybackPositionCommandEvent *)event;
                self->callbacks.seek(seekEvent.positionTime);
                return MPRemoteCommandHandlerStatusSuccess;
            }
            return MPRemoteCommandHandlerStatusCommandFailed;
        }];

        // Enable the commands
        [[commandCenter playCommand] setEnabled:YES];
        [[commandCenter pauseCommand] setEnabled:YES];
        [[commandCenter togglePlayPauseCommand] setEnabled:YES];
        [[commandCenter stopCommand] setEnabled:YES];
        [[commandCenter nextTrackCommand] setEnabled:YES];
        [[commandCenter previousTrackCommand] setEnabled:YES];
        [[commandCenter changePlaybackPositionCommand] setEnabled:YES];

        NSLog(@"[MediaControl] All remote commands enabled");
    }
    return self;
}

- (void)dealloc {
    // Disable all commands
    MPRemoteCommandCenter *commandCenter = [MPRemoteCommandCenter sharedCommandCenter];
    [[commandCenter playCommand] setEnabled:NO];
    [[commandCenter pauseCommand] setEnabled:NO];
    [[commandCenter togglePlayPauseCommand] setEnabled:NO];
    [[commandCenter stopCommand] setEnabled:NO];
    [[commandCenter nextTrackCommand] setEnabled:NO];
    [[commandCenter previousTrackCommand] setEnabled:NO];
    [[commandCenter changePlaybackPositionCommand] setEnabled:NO];
    [super dealloc];
}

- (void)updateMetadata:(NSString *)title artist:(NSString *)artist album:(NSString *)album duration:(double)duration {
    NSLog(@"[MediaControl] updateMetadata called - title: %@, artist: %@, album: %@", title, artist, album);
    NSMutableDictionary *nowPlayingInfo = [[NSMutableDictionary alloc] init];

    if (title != nil) {
        [nowPlayingInfo setObject:title forKey:MPMediaItemPropertyTitle];
    }
    if (artist != nil) {
        [nowPlayingInfo setObject:artist forKey:MPMediaItemPropertyArtist];
    }
    if (album != nil) {
        [nowPlayingInfo setObject:album forKey:MPMediaItemPropertyAlbumTitle];
    }
    if (duration > 0) {
        [nowPlayingInfo setObject:[NSNumber numberWithDouble:duration] forKey:MPMediaItemPropertyPlaybackDuration];
    }

    // Set playback rate and position - critical for macOS to recognize as active player
    [nowPlayingInfo setObject:[NSNumber numberWithDouble:1.0] forKey:MPNowPlayingInfoPropertyPlaybackRate];
    [nowPlayingInfo setObject:[NSNumber numberWithDouble:0.0] forKey:MPNowPlayingInfoPropertyElapsedPlaybackTime];

    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
    NSLog(@"[MediaControl] Now playing info updated: %@", nowPlayingInfo);
}

- (void)updatePlaybackState:(int)state {
    NSMutableDictionary *nowPlayingInfo = [[[MPNowPlayingInfoCenter defaultCenter] nowPlayingInfo] mutableCopy];
    if (nowPlayingInfo == nil) {
        nowPlayingInfo = [[NSMutableDictionary alloc] init];
    }

    // Update playback rate based on state
    // 0 = stopped, 1 = playing, 2 = paused
    double playbackRate = 0.0;
    if (state == 1) {
        playbackRate = 1.0; // Playing
    } else {
        playbackRate = 0.0; // Stopped or paused
    }

    [nowPlayingInfo setObject:[NSNumber numberWithDouble:playbackRate] forKey:MPNowPlayingInfoPropertyPlaybackRate];
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
}

- (void)updatePosition:(double)position duration:(double)duration {
    NSMutableDictionary *nowPlayingInfo = [[[MPNowPlayingInfoCenter defaultCenter] nowPlayingInfo] mutableCopy];
    if (nowPlayingInfo == nil) {
        nowPlayingInfo = [[NSMutableDictionary alloc] init];
    }

    [nowPlayingInfo setObject:[NSNumber numberWithDouble:position] forKey:MPNowPlayingInfoPropertyElapsedPlaybackTime];
    if (duration > 0) {
        [nowPlayingInfo setObject:[NSNumber numberWithDouble:duration] forKey:MPMediaItemPropertyPlaybackDuration];
    }

    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
}

- (BOOL)setArtwork:(NSData *)pngData {
    if (pngData == nil || [pngData length] == 0) {
        return NO;
    }

    NSImage *image = [[NSImage alloc] initWithData:pngData];
    if (image == nil) {
        return NO;
    }

    MPMediaItemArtwork *artwork = [[MPMediaItemArtwork alloc] initWithBoundsSize:image.size requestHandler:^NSImage * _Nonnull(CGSize size) {
        return image;
    }];

    NSMutableDictionary *nowPlayingInfo = [[[MPNowPlayingInfoCenter defaultCenter] nowPlayingInfo] mutableCopy];
    if (nowPlayingInfo == nil) {
        nowPlayingInfo = [[NSMutableDictionary alloc] init];
    }

    [nowPlayingInfo setObject:artwork forKey:MPMediaItemPropertyArtwork];
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nowPlayingInfo];
    return YES;
}

@end

// C API Implementation
// Note: This code is compiled without ARC, so we use manual retain/release

MediaControlHandle MediaControlCreate(void) {
    MediaControlBridge *bridge = [[MediaControlBridge alloc] init];
    // Manual retain to transfer ownership to the C handle
    return (void *)bridge;
}

void MediaControlDestroy(MediaControlHandle handle) {
    if (handle == NULL) return;
    MediaControlBridge *bridge = (MediaControlBridge *)handle;
    [bridge release];
}

void MediaControlSetCallbacks(MediaControlHandle handle, MediaControlCallbacks callbacks) {
    if (handle == NULL) return;
    MediaControlBridge *bridge = (MediaControlBridge *)handle;
    bridge->callbacks = callbacks;
}

void MediaControlUpdateMetadata(
    MediaControlHandle handle,
    const char* title,
    const char* artist,
    const char* album,
    double duration
) {
    if (handle == NULL) return;
    MediaControlBridge *bridge = (MediaControlBridge *)handle;

    NSString *nsTitle = title != NULL ? [NSString stringWithUTF8String:title] : nil;
    NSString *nsArtist = artist != NULL ? [NSString stringWithUTF8String:artist] : nil;
    NSString *nsAlbum = album != NULL ? [NSString stringWithUTF8String:album] : nil;

    [bridge updateMetadata:nsTitle artist:nsArtist album:nsAlbum duration:duration];
}

void MediaControlUpdatePlaybackState(MediaControlHandle handle, int state) {
    if (handle == NULL) return;
    MediaControlBridge *bridge = (MediaControlBridge *)handle;
    [bridge updatePlaybackState:state];
}

void MediaControlUpdatePosition(MediaControlHandle handle, double position, double duration) {
    if (handle == NULL) return;
    MediaControlBridge *bridge = (MediaControlBridge *)handle;
    [bridge updatePosition:position duration:duration];
}

int MediaControlSetArtwork(
    MediaControlHandle handle,
    const void* pngData,
    uint64_t pngDataLength
) {
    if (handle == NULL || pngData == NULL || pngDataLength == 0) {
        return 0;
    }

    MediaControlBridge *bridge = (MediaControlBridge *)handle;
    NSData *data = [NSData dataWithBytes:pngData length:(NSUInteger)pngDataLength];
    BOOL success = [bridge setArtwork:data];
    return success ? 1 : 0;
}
