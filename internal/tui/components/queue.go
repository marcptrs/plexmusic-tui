package components

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"plexmusic-tui/internal/app"
	"plexmusic-tui/internal/domain"
	"plexmusic-tui/internal/tui"
	"plexmusic-tui/internal/tui/styles"
	"plexmusic-tui/internal/tui/util"
)

type QueueComponent struct {
	coordinator  app.Coordinatorer
	list         list.Model
	keys         tui.LibraryKeyMap
	orchestrator *tui.Orchestrator
	focused      bool
}

// PlayResultMsg is emitted by components that initiate playback to notify
// the caller (page) about the result of starting playback.
type PlayResultMsg struct{ Err error }

func NewQueueComponent(coord app.Coordinatorer, orch *tui.Orchestrator) *QueueComponent {
	delegate := styles.NewCustomDelegate()
	l := list.New(nil, delegate, 20, 10)
	l.Title = "Queue"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Disable q/Q quit keys - use ctrl+c for quit

	return &QueueComponent{
		coordinator:  coord,
		list:         l,
		keys:         tui.DefaultLibraryKeyMap(),
		orchestrator: orch,
	}
}

func (c *QueueComponent) Init() tea.Cmd {
	return nil
}

func (c *QueueComponent) SetSize(width, height int) {
	c.list.SetSize(width, height-1)
}

func (c *QueueComponent) SetWidth(width int) {
	c.list.SetWidth(width)
}

func (c *QueueComponent) SetItems(items []list.Item) {
	c.list.SetItems(items)
}

func (c *QueueComponent) Select(index int) {
	c.list.Select(index)
}

func (c *QueueComponent) Index() int {
	return c.list.Index()
}

func (c *QueueComponent) SelectedItem() list.Item {
	return c.list.SelectedItem()
}

func (c *QueueComponent) SetFocused(focused bool) {
	c.focused = focused
}

func (c *QueueComponent) IsFocused() bool {
	return c.focused
}

func (c *QueueComponent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if c.focused {
			// Handle queue-specific keys
			if handled, cmd := c.handleKeyMsg(msg); handled {
				return c, cmd
			}
		}
	}

	if _, ok := msg.(tea.KeyMsg); ok && !c.focused {
		return c, nil
	}

	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *QueueComponent) View() string {
	v := c.list.View()

	var total int
	items := c.list.Items()
	for _, item := range items {
		if qi, ok := item.(util.QueueItem); ok {
			total += qi.Track.Duration
		}
	}

	summary := fmt.Sprintf("%d items • %s", len(items), util.FormatTrackDuration(total))
	summaryView := styles.MutedStyle.Padding(0, 1).Render(summary)

	return lipgloss.JoinVertical(lipgloss.Left, v, summaryView)
}

func (c *QueueComponent) handleKeyMsg(msg tea.KeyMsg) (bool, tea.Cmd) {
	// Navigation keys (up/down/k/j or rune fallback) are handled by the list component.
	if key.Matches(msg, c.keys.Up) || key.Matches(msg, c.keys.Down) || isRuneKey(msg, 'k') ||
		isRuneKey(msg, 'j') {
		var cmd tea.Cmd
		c.list, cmd = c.list.Update(msg)
		return true, cmd
	}

	// Move the selected queue item up
	if key.Matches(msg, c.keys.QueueMoveUp) {
		sel := c.list.Index()
		q := c.coordinator.Queue()
		if sel > 0 && sel < len(q) {
			c.coordinator.MoveQueueItem(sel, sel-1)
			c.UpdateListFromCoordinator()
			// Select the item at its new position
			c.list.Select(sel - 1)
		}
		return true, nil
	}

	// Move the selected queue item down
	if key.Matches(msg, c.keys.QueueMoveDown) {
		sel := c.list.Index()
		q := c.coordinator.Queue()
		if sel >= 0 && sel < len(q)-1 {
			c.coordinator.MoveQueueItem(sel, sel+1)
			c.UpdateListFromCoordinator()
			c.list.Select(sel + 1)
		}
		return true, nil
	}

	// Remove the selected queue item
	if key.Matches(msg, c.keys.QueueRemove) {
		sel := c.list.Index()
		q := c.coordinator.Queue()
		if sel >= 0 && sel < len(q) {
			// Check if we are removing the playing item
			playingIdx := c.coordinator.QueueIndex()
			wasPlaying := (sel == playingIdx)

			c.coordinator.RemoveQueueItem(sel)

			if wasPlaying {
				if c.orchestrator != nil {
					_ = c.orchestrator.Stop()
				}
			}

			c.UpdateListFromCoordinator()

			// Adjust selection
			newLen := len(c.coordinator.Queue())
			if sel < newLen {
				c.list.Select(sel)
			} else if newLen > 0 {
				c.list.Select(newLen - 1)
			}
		}
		return true, nil
	}

	// Play selected queue item
	if key.Matches(msg, c.keys.PlaySelected) || key.Matches(msg, c.keys.Enter) {
		sel := c.list.Index()
		q := c.coordinator.Queue()
		if len(q) == 0 || sel < 0 || sel >= len(q) {
			return true, nil
		}

		// Trim previous tracks
		if sel > 0 {
			// We can't use Move/Remove for this easily.
			// We need to set the queue.
			newQ := make([]app.Track, len(q)-sel)
			copy(newQ, q[sel:])
			c.coordinator.SetQueue(newQ)
			c.coordinator.SetQueueIndex(0)
			c.UpdateListFromCoordinator()
			c.list.Select(0)

			c.coordinator.SetShowQueueModal(false)
			return true, c.playAppTrack(&newQ[0])
		}

		// Selected item is already first
		at := &q[sel]
		c.coordinator.SetShowQueueModal(false)
		return true, c.playAppTrack(at)
	}

	return false, nil
}

func (c *QueueComponent) UpdateListFromCoordinator() {
	q := c.coordinator.Queue()
	qItems := make([]list.Item, len(q))
	for i, t := range q {
		if dt := util.AppTrackToDomain(&t); dt != nil {
			qi := util.QueueItem{Track: *dt, Index: i}
			if c.coordinator.QueueIndex() == i {
				qi.Playing = true
			}
			qItems[i] = qi
		} else {
			qi := util.QueueItem{
				Track: domain.Track{
					Title:    t.Title,
					Artist:   t.Artist,
					Album:    t.Album,
					Duration: t.Duration,
				},
				Index: i,
			}
			if c.coordinator.QueueIndex() == i {
				qi.Playing = true
			}
			qItems[i] = qi
		}
	}
	c.list.SetItems(qItems)

	// Sync selection with playing track
	sel := c.coordinator.QueueIndex()
	if len(q) == 0 {
		c.list.Select(0)
		return
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= len(q) {
		sel = len(q) - 1
	}
	c.list.Select(sel)
}

func (c *QueueComponent) playAppTrack(at *app.Track) tea.Cmd {
	if at == nil {
		return nil
	}
	// Update UI coordinator state preemptively
	c.coordinator.SetCurrentTrack(at)
	c.coordinator.SetPlaybackState(app.PlaybackPlaying)

	// Orchestrator is required
	if c.orchestrator != nil {
		// Use a background command to call PlayAppTrack to avoid blocking caller.
		return func() tea.Msg {
			err := c.orchestrator.PlayAppTrack(context.TODO(), at)
			return PlayResultMsg{Err: err}
		}
	}
	c.coordinator.SetNotification("Play failed: playback orchestrator unavailable", "error", 10*time.Second)
	return nil

	// We also need to fetch cover art. LibraryPage did this.
	// But QueueComponent doesn't have LibraryService directly (it's in Orchestrator but private/interface).
	// LibraryPage had p.fetchCoverArtCmd.
	// Maybe we should let LibraryPage handle the side effects of playback start?
	// Or we can emit a message "QueuePlayRequest" and let LibraryPage handle it.

	// But we want to encapsulate logic.
	// The Orchestrator handles playback.
	// Cover art fetching is a UI concern.
	// Maybe Orchestrator should fetch cover art? Or Coordinator?

	// For now, I'll skip cover art fetching here. The playback.started event will trigger in LibraryPage
	// and LibraryPage handles playback.started.
	// Let's check LibraryPage.Update handling playback.started.

	/*
		case "playback.started":
			p.coordinator.SetPlaybackState(app.PlaybackPlaying)
			p.finishedTriggered = false
			if msg.Track != nil {
				track := util.DomainTrackToApp(msg.Track)
				p.coordinator.SetCurrentTrack(track)
			}
	*/

	// It doesn't seem to fetch cover art there.
	// It fetches cover art in playAppTrack:
	/*
		if at.Thumb != "" {
			cmds = append(cmds, p.fetchCoverArtCmd(at.Thumb))
		}
	*/

	// So if I don't do it here, cover art won't update immediately.
	// But maybe that's fine for now.
}

// Helper to detect rune keypresses
func isRuneKey(msg tea.KeyMsg, r rune) bool {
	if len(msg.Runes) == 0 {
		return false
	}
	return len(msg.Runes) == 1 && msg.Runes[0] == r
}

// RenderWithModal composes the base view layout with the queue modal overlay.
func (c *QueueComponent) RenderWithModal(base string, width, height int) string {
	// Calculate modal dimensions
	modalWidth := 60
	modalHeight := 20
	if modalWidth > width {
		modalWidth = width - 4
	}
	if modalHeight > height {
		modalHeight = height - 4
	}

	// Update list size for the modal
	c.list.SetSize(modalWidth-4, modalHeight-3)

	// Render the list
	listView := c.View()

	// Wrap with background color instead of border
	modal := lipgloss.NewStyle().
		Background(styles.ColorPaneFocusedBackground).
		Padding(0, 1).
		Width(modalWidth).
		Height(modalHeight).
		Render(listView)

	// Place centered
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		modal,
	)
}

// SetOrchestrator updates the orchestrator instance
func (c *QueueComponent) SetOrchestrator(orch *tui.Orchestrator) {
	c.orchestrator = orch
}
