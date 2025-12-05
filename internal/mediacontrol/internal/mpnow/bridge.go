//go:build darwin

package mpnow

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework MediaPlayer
#include "bridge.h"
#include <stdlib.h>

// Forward declarations for Go callbacks
extern void goPlayCallback();
extern void goPauseCallback();
extern void goTogglePlayPauseCallback();
extern void goStopCallback();
extern void goNextCallback();
extern void goPreviousCallback();
extern void goSeekCallback(double position);
*/
import "C"

import (
	"bytes"
	"image"
	"image/png"
	"time"
	"unsafe"
)

// Bridge wraps the Objective-C media control bridge
type Bridge struct {
	handle C.MediaControlHandle
}

// CommandHandler defines the interface for handling media commands
type CommandHandler interface {
	HandlePlay()
	HandlePause()
	HandleTogglePlayPause()
	HandleStop()
	HandleNext()
	HandlePrevious()
	HandleSeek(position time.Duration)
}

var globalHandler CommandHandler

// Export callback functions for CGo

//export goPlayCallback
func goPlayCallback() {
	if globalHandler != nil {
		globalHandler.HandlePlay()
	}
}

//export goPauseCallback
func goPauseCallback() {
	if globalHandler != nil {
		globalHandler.HandlePause()
	}
}

//export goTogglePlayPauseCallback
func goTogglePlayPauseCallback() {
	if globalHandler != nil {
		globalHandler.HandleTogglePlayPause()
	}
}

//export goStopCallback
func goStopCallback() {
	if globalHandler != nil {
		globalHandler.HandleStop()
	}
}

//export goNextCallback
func goNextCallback() {
	if globalHandler != nil {
		globalHandler.HandleNext()
	}
}

//export goPreviousCallback
func goPreviousCallback() {
	if globalHandler != nil {
		globalHandler.HandlePrevious()
	}
}

//export goSeekCallback
func goSeekCallback(position C.double) {
	if globalHandler != nil {
		globalHandler.HandleSeek(time.Duration(float64(position) * float64(time.Second)))
	}
}

// NewBridge creates a new Objective-C media control bridge
func NewBridge() *Bridge {
	println("[Bridge] Calling C.MediaControlCreate()")
	handle := C.MediaControlCreate()
	if handle == nil {
		println("[Bridge] ERROR: C.MediaControlCreate() returned nil")
		return nil
	}

	println("[Bridge] Bridge created successfully with handle:", handle)
	return &Bridge{
		handle: handle,
	}
}

// Destroy releases the bridge resources
func (b *Bridge) Destroy() {
	if b.handle != nil {
		C.MediaControlDestroy(b.handle)
		b.handle = nil
	}
	globalHandler = nil
}

// SetCommandHandler sets the handler for media commands
func (b *Bridge) SetCommandHandler(handler CommandHandler) {
	if b.handle == nil {
		println("[Bridge] ERROR: handle is nil in SetCommandHandler")
		return
	}

	println("[Bridge] SetCommandHandler called")
	globalHandler = handler

	var callbacks C.MediaControlCallbacks
	callbacks.play = C.PlayCallback(C.goPlayCallback)
	callbacks.pause = C.PauseCallback(C.goPauseCallback)
	callbacks.togglePlayPause = C.TogglePlayPauseCallback(C.goTogglePlayPauseCallback)
	callbacks.stop = C.StopCallback(C.goStopCallback)
	callbacks.next = C.NextCallback(C.goNextCallback)
	callbacks.previous = C.PreviousCallback(C.goPreviousCallback)
	callbacks.seek = C.SeekCallback(C.goSeekCallback)

	println("[Bridge] Calling C.MediaControlSetCallbacks")
	C.MediaControlSetCallbacks(b.handle, callbacks)
	println("[Bridge] SetCommandHandler complete")
}

// UpdateMetadata updates the now playing metadata
func (b *Bridge) UpdateMetadata(title, artist, album string, duration time.Duration) {
	if b.handle == nil {
		println("[Bridge] ERROR: handle is nil in UpdateMetadata")
		return
	}

	println("[Bridge] UpdateMetadata called - title:", title, "artist:", artist, "album:", album)

	cTitle := C.CString(title)
	cArtist := C.CString(artist)
	cAlbum := C.CString(album)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cArtist))
	defer C.free(unsafe.Pointer(cAlbum))

	durationSeconds := duration.Seconds()
	println("[Bridge] Calling C.MediaControlUpdateMetadata with handle:", b.handle)
	C.MediaControlUpdateMetadata(b.handle, cTitle, cArtist, cAlbum, C.double(durationSeconds))
	println("[Bridge] C.MediaControlUpdateMetadata returned")
}

// UpdatePlaybackState updates the playback state
// 0 = stopped, 1 = playing, 2 = paused
func (b *Bridge) UpdatePlaybackState(state int) {
	if b.handle == nil {
		println("[Bridge] ERROR: handle is nil in UpdatePlaybackState")
		return
	}
	println("[Bridge] UpdatePlaybackState called - state:", state)
	C.MediaControlUpdatePlaybackState(b.handle, C.int(state))
	println("[Bridge] UpdatePlaybackState returned")
}

// UpdatePosition updates the playback position
func (b *Bridge) UpdatePosition(position, duration time.Duration) {
	posSeconds := position.Seconds()
	durSeconds := duration.Seconds()
	C.MediaControlUpdatePosition(b.handle, C.double(posSeconds), C.double(durSeconds))
}

// SetArtwork sets the album artwork from an image
func (b *Bridge) SetArtwork(img image.Image) error {
	if img == nil {
		return nil
	}

	// Encode image to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}

	pngData := buf.Bytes()
	if len(pngData) == 0 {
		return nil
	}

	// Pass PNG data to Objective-C
	result := C.MediaControlSetArtwork(
		b.handle,
		unsafe.Pointer(&pngData[0]),
		C.uint64_t(len(pngData)),
	)

	if result == 0 {
		return ErrArtworkFailed
	}

	return nil
}

// ErrArtworkFailed is returned when setting artwork fails
var ErrArtworkFailed = &artworkError{}

type artworkError struct{}

func (e *artworkError) Error() string {
	return "failed to set artwork"
}
