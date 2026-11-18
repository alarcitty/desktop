//go:build !windows

package tray

func (m *Manager) start(_ Config) {
	// Native tray integration is implemented for Windows desktop builds.
}
