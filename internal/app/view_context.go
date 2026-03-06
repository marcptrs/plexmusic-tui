package app

import (
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"

	"plexmusic-tui/internal/image"
)

// ViewContext holds pure UI state that pages and components use for rendering.
// It's separate from domain data and services to reduce coupling and make
// testing easier. Inspired by gh-dash's ProgramContext pattern.
type ViewContext struct {
	// Dimensions
	width  int
	height int

	// Navigation and view state
	activeTab      TabType
	currentContent ContentViewType
	sessionState   SessionState

	// Selection indices for different views
	selectedHome     int
	selectedAlbum    int
	selectedPlaylist int
	selectedTrack    int
	contentScroll    int

	// UI state flags
	showQueueModal bool

	// Color palette derived from current album art
	currentPalette *image.Palette

	// Login form inputs
	usernameInput textinput.Model
	passwordInput textinput.Model
	focusIndex    int

	// Notifications
	notifMsg      string
	notifSeverity string
	notifExpiry   time.Time

	// Debug/troubleshooting
	dumpView bool

	// Task tracking
	tasks *TaskManager

	mu sync.RWMutex
}

// NewViewContext creates a new view context with default values
func NewViewContext() *ViewContext {
	usernameInput := textinput.New()
	usernameInput.Placeholder = "Email or username"
	usernameInput.Focus()
	usernameInput.CharLimit = 100
	usernameInput.SetWidth(40)

	passwordInput := textinput.New()
	passwordInput.Placeholder = "Password"
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.EchoCharacter = '•'
	passwordInput.CharLimit = 100
	passwordInput.SetWidth(40)

	return &ViewContext{
		sessionState:  LoginView,
		usernameInput: usernameInput,
		passwordInput: passwordInput,
		tasks:         NewTaskManager(),
	}
}

// Dimensions

func (v *ViewContext) Width() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.width
}

func (v *ViewContext) SetWidth(w int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.width = w
}

func (v *ViewContext) Height() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.height
}

func (v *ViewContext) SetHeight(h int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.height = h
}

// Navigation

func (v *ViewContext) ActiveTab() TabType {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.activeTab
}

func (v *ViewContext) SetActiveTab(tab TabType) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.activeTab = tab
}

func (v *ViewContext) CurrentContent() ContentViewType {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.currentContent
}

func (v *ViewContext) SetCurrentContent(content ContentViewType) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.currentContent = content
}

func (v *ViewContext) SessionState() SessionState {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.sessionState
}

func (v *ViewContext) SetSessionState(state SessionState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sessionState = state
}

// Selection indices

func (v *ViewContext) SelectedHome() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.selectedHome
}

func (v *ViewContext) SetSelectedHome(idx int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.selectedHome = idx
}

func (v *ViewContext) SelectedAlbum() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.selectedAlbum
}

func (v *ViewContext) SetSelectedAlbum(idx int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.selectedAlbum = idx
}

func (v *ViewContext) SelectedPlaylist() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.selectedPlaylist
}

func (v *ViewContext) SetSelectedPlaylist(idx int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.selectedPlaylist = idx
}

func (v *ViewContext) SelectedTrack() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.selectedTrack
}

func (v *ViewContext) SetSelectedTrack(idx int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.selectedTrack = idx
}

func (v *ViewContext) ContentScroll() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.contentScroll
}

func (v *ViewContext) SetContentScroll(scroll int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contentScroll = scroll
}

// UI state

func (v *ViewContext) ShowQueueModal() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.showQueueModal
}

func (v *ViewContext) SetShowQueueModal(show bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.showQueueModal = show
}

// Login inputs

func (v *ViewContext) UsernameInput() textinput.Model {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.usernameInput
}

func (v *ViewContext) SetUsernameInput(input textinput.Model) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.usernameInput = input
}

func (v *ViewContext) PasswordInput() textinput.Model {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.passwordInput
}

func (v *ViewContext) SetPasswordInput(input textinput.Model) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.passwordInput = input
}

func (v *ViewContext) FocusIndex() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.focusIndex
}

func (v *ViewContext) SetFocusIndex(idx int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.focusIndex = idx
}

func (v *ViewContext) GetInput(index int) textinput.Model {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if index == 0 {
		return v.usernameInput
	}
	return v.passwordInput
}

func (v *ViewContext) UpdateInput(index int, input textinput.Model) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if index == 0 {
		v.usernameInput = input
	} else {
		v.passwordInput = input
	}
}

// Notifications

func (v *ViewContext) SetNotification(msg, severity string, duration time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.notifMsg = msg
	v.notifSeverity = severity
	v.notifExpiry = time.Now().Add(duration)
}

func (v *ViewContext) Notification() (string, string, time.Time) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.notifMsg, v.notifSeverity, v.notifExpiry
}

func (v *ViewContext) ClearNotification() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.notifMsg = ""
	v.notifSeverity = ""
	v.notifExpiry = time.Time{}
}

func (v *ViewContext) NotificationActive() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.notifMsg != "" && time.Now().Before(v.notifExpiry)
}

// Debug

func (v *ViewContext) DumpView() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.dumpView
}

func (v *ViewContext) SetDumpView(dump bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.dumpView = dump
}

// Tasks

func (v *ViewContext) Tasks() *TaskManager {
	return v.tasks
}

// Palette

func (v *ViewContext) CurrentPalette() *image.Palette {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.currentPalette
}

func (v *ViewContext) SetCurrentPalette(palette *image.Palette) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.currentPalette = palette
}
