package tray

import "sync"

// MenuID enumerates native tray menu actions.
type MenuID uint32

const (
	MenuTogglePlay MenuID = 1001
	MenuToggleShow MenuID = 1002
	MenuCheckUpd   MenuID = 1003
	MenuQuit       MenuID = 1004
)

// Callbacks connects native tray actions to the host application lifecycle.
type Callbacks struct {
	OnTogglePlay  func()
	OnToggleShow  func()
	OnCheckUpdate func()
	OnQuit        func()
}

// Config configures the system tray icon, tooltip, initial state, and callbacks.
type Config struct {
	Tooltip   string
	IconICO   []byte
	Playing   bool
	Visible   bool
	Callbacks Callbacks
}

// Manager coordinates tray state and dispatches OS tray lifecycle hooks safely across threads.
type Manager struct {
	mu      sync.RWMutex
	playing bool
	visible bool
	closed  bool
	cb      Callbacks
	stopFn  func()
}

// NewManager validates config and starts the platform tray integration.
func NewManager(cfg Config) *Manager {
	m := &Manager{
		playing: cfg.Playing,
		visible: cfg.Visible,
		cb:      cfg.Callbacks,
	}
	m.start(cfg)
	return m
}

// SetPlaying updates the cached playback state used to label the Pause/Resume menu item.
func (m *Manager) SetPlaying(playing bool) {
	m.mu.Lock()
	m.playing = playing
	m.mu.Unlock()
}

// SetVisible updates the cached window visibility state used to label Show/Hide in the tray menu.
func (m *Manager) SetVisible(visible bool) {
	m.mu.Lock()
	m.visible = visible
	m.mu.Unlock()
}

// State returns the current playback and window visibility snapshot.
func (m *Manager) State() (playing bool, visible bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.playing, m.visible
}

// MenuLabels returns localized/human-readable labels reflecting current playback and window state.
func (m *Manager) MenuLabels() (playLabel, visibilityLabel string) {
	playing, visible := m.State()
	if playing {
		playLabel = "Pause"
	} else {
		playLabel = "Resume"
	}
	if visible {
		visibilityLabel = "Hide to Tray"
	} else {
		visibilityLabel = "Show Mila"
	}
	return playLabel, visibilityLabel
}

// Dispatch invokes the registered callback for the given tray menu ID safely.
func (m *Manager) Dispatch(id MenuID) {
	m.mu.RLock()
	cb := m.cb
	m.mu.RUnlock()

	switch id {
	case MenuTogglePlay:
		if cb.OnTogglePlay != nil {
			cb.OnTogglePlay()
		}
	case MenuToggleShow:
		if cb.OnToggleShow != nil {
			cb.OnToggleShow()
		}
	case MenuCheckUpd:
		if cb.OnCheckUpdate != nil {
			cb.OnCheckUpdate()
		}
	case MenuQuit:
		if cb.OnQuit != nil {
			cb.OnQuit()
		}
	}
}

// Close tears down the system tray icon cleanly (idempotent).
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	stop := m.stopFn
	m.stopFn = nil
	m.mu.Unlock()

	if stop != nil {
		stop()
	}
}
