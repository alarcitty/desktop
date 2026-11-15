package screen

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	minPersistWindowWidth  = 320
	minPersistWindowHeight = 180
)

// WindowGeometry captures persisted desktop window placement without a database.
type WindowGeometry struct {
	X         int   `json:"x"`
	Y         int   `json:"y"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Maximized bool  `json:"maximized"`
	UpdatedAt int64 `json:"updatedAt"`
}

// DefaultWindowStatePath returns the user config JSON path used to persist window geometry.
func DefaultWindowStatePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return filepath.Join(os.TempDir(), "mila", "window-state.json"), nil
	}
	return filepath.Join(base, "mila", "window-state.json"), nil
}

// NormalizeWindowGeometry enforces safe minimum dimensions and populates UpdatedAt.
func NormalizeWindowGeometry(g WindowGeometry) (WindowGeometry, bool) {
	if g.Width < minPersistWindowWidth || g.Height < minPersistWindowHeight {
		return WindowGeometry{}, false
	}
	if g.UpdatedAt <= 0 {
		g.UpdatedAt = time.Now().UTC().UnixMilli()
	}
	return g, true
}

// SaveWindowGeometry persists window bounds atomically as JSON (no database).
func SaveWindowGeometry(path string, g WindowGeometry) error {
	normalized, ok := NormalizeWindowGeometry(g)
	if !ok {
		return errors.New("window dimensions below minimum persistable bounds")
	}

	target := strings.TrimSpace(path)
	if target == "" {
		resolved, err := DefaultWindowStatePath()
		if err != nil {
			return err
		}
		target = resolved
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := target + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// LoadWindowGeometry reads persisted window bounds from JSON if present and valid.
func LoadWindowGeometry(path string) (WindowGeometry, bool, error) {
	target := strings.TrimSpace(path)
	if target == "" {
		resolved, err := DefaultWindowStatePath()
		if err != nil {
			return WindowGeometry{}, false, err
		}
		target = resolved
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WindowGeometry{}, false, nil
		}
		return WindowGeometry{}, false, err
	}

	var g WindowGeometry
	if err := json.Unmarshal(raw, &g); err != nil {
		return WindowGeometry{}, false, err
	}
	normalized, ok := NormalizeWindowGeometry(g)
	return normalized, ok, nil
}
