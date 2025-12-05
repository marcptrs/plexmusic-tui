#ifndef MPNOW_BRIDGE_H
#define MPNOW_BRIDGE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// MediaControlHandle is an opaque handle to the Objective-C media control object
typedef void* MediaControlHandle;

// Callback function types for media commands
typedef void (*PlayCallback)(void);
typedef void (*PauseCallback)(void);
typedef void (*TogglePlayPauseCallback)(void);
typedef void (*StopCallback)(void);
typedef void (*NextCallback)(void);
typedef void (*PreviousCallback)(void);
typedef void (*SeekCallback)(double position);

// MediaControlCallbacks holds all the callback functions
typedef struct {
    PlayCallback play;
    PauseCallback pause;
    TogglePlayPauseCallback togglePlayPause;
    StopCallback stop;
    NextCallback next;
    PreviousCallback previous;
    SeekCallback seek;
} MediaControlCallbacks;

// MediaControlCreate creates and initializes the media control object
MediaControlHandle MediaControlCreate(void);

// MediaControlDestroy releases the media control object
void MediaControlDestroy(MediaControlHandle handle);

// MediaControlSetCallbacks sets the command handler callbacks
void MediaControlSetCallbacks(MediaControlHandle handle, MediaControlCallbacks callbacks);

// MediaControlUpdateMetadata updates the now playing metadata
void MediaControlUpdateMetadata(
    MediaControlHandle handle,
    const char* title,
    const char* artist,
    const char* album,
    double duration
);

// MediaControlUpdatePlaybackState updates the playback state
// state: 0 = stopped, 1 = playing, 2 = paused
void MediaControlUpdatePlaybackState(MediaControlHandle handle, int state);

// MediaControlUpdatePosition updates the playback position
void MediaControlUpdatePosition(MediaControlHandle handle, double position, double duration);

// MediaControlSetArtwork sets the album artwork from PNG data
// Returns 1 on success, 0 on failure
int MediaControlSetArtwork(
    MediaControlHandle handle,
    const void* pngData,
    uint64_t pngDataLength
);

#ifdef __cplusplus
}
#endif

#endif // MPNOW_BRIDGE_H
