#import "daemon.h"
#import "ipc.h"
#import "SPMediaKeyTap.h"

@interface PlexMusicDaemon ()
@property (nonatomic, strong) SPMediaKeyTap *mediaKeyTap;
@property (nonatomic, strong) IPCServer *ipcServer;
@property (nonatomic, strong) NSMutableDictionary *nowPlayingInfo;
@end

@implementation PlexMusicDaemon

- (instancetype)init {
    self = [super init];
    if (self) {
        _nowPlayingInfo = [[NSMutableDictionary alloc] init];
    }
    return self;
}

- (BOOL)start {
    NSLog(@"[Daemon] Initializing daemon components");

    // Create NSApplication instance (required for media key handling)
    [NSApplication sharedApplication];
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    NSLog(@"[Daemon] NSApplication created with Accessory policy");

    // Start IPC server to receive commands from TUI
    self.ipcServer = [[IPCServer alloc] init];
    __weak typeof(self) weakSelf = self;
    self.ipcServer.messageHandler = ^(NSDictionary *message) {
        [weakSelf handleIPCMessage:message];
    };

    if (![self.ipcServer start]) {
        NSLog(@"[Daemon] ERROR: Failed to start IPC server");
        return NO;
    }
    NSLog(@"[Daemon] IPC server started");

    // Initialize SPMediaKeyTap for global media key capture
    self.mediaKeyTap = [[SPMediaKeyTap alloc] initWithDelegate:self];
    if (self.mediaKeyTap == nil) {
        NSLog(@"[Daemon] ERROR: Failed to create SPMediaKeyTap");
        return NO;
    }
    [self.mediaKeyTap startWatchingMediaKeys];
    NSLog(@"[Daemon] SPMediaKeyTap initialized and watching");

    // Enable remote command center (for completeness, though media keys are via SPMediaKeyTap)
    [self setupRemoteCommandCenter];

    return YES;
}

- (void)stop {
    NSLog(@"[Daemon] Stopping daemon");

    if (self.mediaKeyTap) {
        [self.mediaKeyTap stopWatchingMediaKeys];
    }

    if (self.ipcServer) {
        [self.ipcServer stop];
    }

    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nil];
}

- (void)setupRemoteCommandCenter {
    MPRemoteCommandCenter *commandCenter = [MPRemoteCommandCenter sharedCommandCenter];
    __weak typeof(self) weakSelf = self;

    // Play command
    [[commandCenter playCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        NSLog(@"[Daemon] Remote command: play");
        [weakSelf.ipcServer sendCommand:@"play"];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Pause command
    [[commandCenter pauseCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        NSLog(@"[Daemon] Remote command: pause");
        [weakSelf.ipcServer sendCommand:@"pause"];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Toggle play/pause command
    [[commandCenter togglePlayPauseCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        NSLog(@"[Daemon] Remote command: toggle_play_pause");
        [weakSelf.ipcServer sendCommand:@"toggle_play_pause"];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Next track command
    [[commandCenter nextTrackCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        NSLog(@"[Daemon] Remote command: next");
        [weakSelf.ipcServer sendCommand:@"next"];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Previous track command
    [[commandCenter previousTrackCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        NSLog(@"[Daemon] Remote command: previous");
        [weakSelf.ipcServer sendCommand:@"previous"];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Change playback position command (for scrubbing)
    [[commandCenter changePlaybackPositionCommand] addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
        MPChangePlaybackPositionCommandEvent *seekEvent = (MPChangePlaybackPositionCommandEvent *)event;
        NSLog(@"[Daemon] Remote command: seek to %.2f seconds", seekEvent.positionTime);
        NSDictionary *message = @{
            @"type": @"seek",
            @"data": @{
                @"position": @(seekEvent.positionTime)
            }
        };
        [weakSelf.ipcServer sendMessage:message];
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Enable all commands
    [[commandCenter playCommand] setEnabled:YES];
    [[commandCenter pauseCommand] setEnabled:YES];
    [[commandCenter togglePlayPauseCommand] setEnabled:YES];
    [[commandCenter nextTrackCommand] setEnabled:YES];
    [[commandCenter previousTrackCommand] setEnabled:YES];
    [[commandCenter changePlaybackPositionCommand] setEnabled:YES];

    NSLog(@"[Daemon] Remote command center configured with handlers");
}

#pragma mark - IPC Message Handling

- (void)handleIPCMessage:(NSDictionary *)message {
    NSString *type = message[@"type"];
    if (![type isEqualToString:@"playback.position"]) {
        NSLog(@"[Daemon] Received IPC message: %@", type);
    }

    if ([type isEqualToString:@"playback.started"]) {
        [self handlePlaybackStarted:message[@"data"]];
    } else if ([type isEqualToString:@"playback.paused"]) {
        [self handlePlaybackPaused:message[@"data"]];
    } else if ([type isEqualToString:@"playback.resumed"]) {
        [self handlePlaybackResumed:message[@"data"]];
    } else if ([type isEqualToString:@"playback.stopped"]) {
        [self handlePlaybackStopped];
    } else if ([type isEqualToString:@"playback.position"]) {
        [self handlePlaybackPosition:message[@"data"]];
    } else if ([type isEqualToString:@"playback.artwork"]) {
        [self handleArtworkUpdate:message[@"data"]];
    }
}

- (void)handlePlaybackStarted:(NSDictionary *)data {
    NSLog(@"[Daemon] Handling playback.started");

    NSString *title = data[@"title"];
    NSString *artist = data[@"artist"];
    NSString *album = data[@"album"];
    NSNumber *duration = data[@"duration"];

    [self.nowPlayingInfo removeAllObjects];

    if (title) {
        self.nowPlayingInfo[MPMediaItemPropertyTitle] = title;
    }
    if (artist) {
        self.nowPlayingInfo[MPMediaItemPropertyArtist] = artist;
    }
    if (album) {
        self.nowPlayingInfo[MPMediaItemPropertyAlbumTitle] = album;
    }
    if (duration) {
        self.nowPlayingInfo[MPMediaItemPropertyPlaybackDuration] = @([duration doubleValue] / 1000.0);
    }

    // Set playback rate to playing
    self.nowPlayingInfo[MPNowPlayingInfoPropertyPlaybackRate] = @1.0;
    self.nowPlayingInfo[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @0.0;

    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:self.nowPlayingInfo];
    NSLog(@"[Daemon] Now playing info updated: %@", self.nowPlayingInfo);
}

- (void)handlePlaybackPaused:(NSDictionary *)data {
    NSLog(@"[Daemon] Handling playback.paused");

    if (data) {
        NSNumber *position = data[@"position"];
        NSNumber *sampleRate = data[@"sampleRate"];
        NSLog(@"[Daemon] Pause data - pos: %@, rate: %@", position, sampleRate);
        
        if (position && sampleRate && [sampleRate intValue] > 0) {
             double posSeconds = [position doubleValue] / [sampleRate doubleValue];
             NSLog(@"[Daemon] Setting elapsed time to: %f", posSeconds);
             self.nowPlayingInfo[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(posSeconds);
        } else {
             NSLog(@"[Daemon] Invalid pause data, not updating elapsed time");
        }
    } else {
        NSLog(@"[Daemon] No data for pause");
    }

    self.nowPlayingInfo[MPNowPlayingInfoPropertyPlaybackRate] = @0.0;
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:self.nowPlayingInfo];
}

- (void)handlePlaybackResumed:(NSDictionary *)data {
    NSLog(@"[Daemon] Handling playback.resumed");

    if (data) {
        NSNumber *position = data[@"position"];
        NSNumber *sampleRate = data[@"sampleRate"];
        if (position && sampleRate && [sampleRate intValue] > 0) {
             double posSeconds = [position doubleValue] / [sampleRate doubleValue];
             self.nowPlayingInfo[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(posSeconds);
        }
    }

    self.nowPlayingInfo[MPNowPlayingInfoPropertyPlaybackRate] = @1.0;
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:self.nowPlayingInfo];
}

- (void)handlePlaybackStopped {
    NSLog(@"[Daemon] Handling playback.stopped");

    [self.nowPlayingInfo removeAllObjects];
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:nil];
}

- (void)handlePlaybackPosition:(NSDictionary *)data {
    NSNumber *position = data[@"position"];
    NSNumber *sampleRate = data[@"sampleRate"];

    if (!position || !sampleRate || [sampleRate intValue] == 0) {
        return;
    }

    double posSeconds = [position doubleValue] / [sampleRate doubleValue];

    // Update position, keeping existing duration from playback.started
    self.nowPlayingInfo[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(posSeconds);
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:self.nowPlayingInfo];
}

- (void)handleArtworkUpdate:(NSDictionary *)data {
    NSString *base64Data = data[@"png_base64"];
    if (!base64Data || [base64Data length] == 0) {
        NSLog(@"[Daemon] No artwork data provided");
        return;
    }

    NSData *pngData = [[NSData alloc] initWithBase64EncodedString:base64Data options:0];
    if (!pngData || [pngData length] == 0) {
        NSLog(@"[Daemon] Failed to decode base64 artwork data");
        return;
    }

    NSImage *image = [[NSImage alloc] initWithData:pngData];
    if (!image) {
        NSLog(@"[Daemon] Failed to create image from artwork data");
        return;
    }

    MPMediaItemArtwork *artwork = [[MPMediaItemArtwork alloc] initWithBoundsSize:image.size
        requestHandler:^NSImage * _Nonnull(CGSize size) {
            return image;
        }];

    self.nowPlayingInfo[MPMediaItemPropertyArtwork] = artwork;
    [[MPNowPlayingInfoCenter defaultCenter] setNowPlayingInfo:self.nowPlayingInfo];
    NSLog(@"[Daemon] Artwork updated (%.0fx%.0f)", image.size.width, image.size.height);
}

#pragma mark - SPMediaKeyTap Delegate

- (void)mediaKeyTap:(SPMediaKeyTap *)keyTap receivedMediaKeyEvent:(NSEvent *)event {
    NSAssert([event type] == NSSystemDefined && [event subtype] == SPSystemDefinedEventMediaKeys, @"Unexpected NSEvent in mediaKeyTap:receivedMediaKeyEvent:");

    int keyCode = (([event data1] & 0xFFFF0000) >> 16);
    int keyFlags = ([event data1] & 0x0000FFFF);
    BOOL keyIsPressed = (((keyFlags & 0xFF00) >> 8)) == 0xA;

    if (keyIsPressed) {
        NSString *command = nil;

        switch (keyCode) {
            case NX_KEYTYPE_PLAY:
                NSLog(@"[Daemon] Media key: Play/Pause");
                command = @"toggle_play_pause";
                break;
            case NX_KEYTYPE_FAST:
            case NX_KEYTYPE_NEXT:
                NSLog(@"[Daemon] Media key: Next");
                command = @"next";
                break;
            case NX_KEYTYPE_REWIND:
            case NX_KEYTYPE_PREVIOUS:
                NSLog(@"[Daemon] Media key: Previous");
                command = @"previous";
                break;
        }

        if (command) {
            // Send command back to TUI via IPC
            [self.ipcServer sendCommand:command];
        }
    }
}

@end
