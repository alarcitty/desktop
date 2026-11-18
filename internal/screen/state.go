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
	minAppWindowWidth      = 858
	minAppWindowHeight     = 651
)

// MonitorBounds describes a connected display's usable work area in virtual desktop coordinates.
type MonitorBounds struct {
	X       int
	Y       int
	Width   int
	Height  int
	Primary bool
}

// ClampWindowGeometry ensures window dimensions and coordinates sit cleanly inside one of the active monitors.
func ClampWindowGeometry(g WindowGeometry, monitors []MonitorBounds) WindowGeometry {
	if len(monitors) == 0 {
		w, h := ThreeQuarterSize()
		if g.Width < minAppWindowWidth {
			g.Width = w
		}
		if g.Height < minAppWindowHeight {
			g.Height = h
		}
		if g.X < 0 {
			g.X = 0
		}
		if g.Y < 0 {
			g.Y = 0
		}
		return g
	}

	target := monitors[0]
	bestOverlap := int64(0)
	for _, m := range monitors {
		if m.Primary {
			target = m
		}
	}

	gRight := g.X + maxInt(g.Width, 1)
	gBottom := g.Y + maxInt(g.Height, 1)
	for _, m := range monitors {
		if m.Width <= 0 || m.Height <= 0 {
			continue
		}
		ox := maxInt(0, minInt(gRight, m.X+m.Width)-maxInt(g.X, m.X))
		oy := maxInt(0, minInt(gBottom, m.Y+m.Height)-maxInt(g.Y, m.Y))
		area := int64(ox) * int64(oy)
		if area > bestOverlap {
			bestOverlap = area
			target = m
		}
	}

	// If window lies completely offscreen (0 overlap with any active display), snap to nearest monitor center.
	if bestOverlap == 0 {
		cx := g.X + g.Width/2
		cy := g.Y + g.Height/2
		bestDist := int64(-1)
		for _, m := range monitors {
			if m.Width <= 0 || m.Height <= 0 {
				continue
			}
			mx := m.X + m.Width/2
			my := m.Y + m.Height/2
			dx := int64(cx - mx)
			dy := int64(cy - my)
			dist := dx*dx + dy*dy
			if bestDist < 0 || dist < bestDist {
				bestDist = dist
				target = m
			}
		}
	}

	if target.Width > 0 {
		maxW := target.Width
		minW := minInt(minAppWindowWidth, maxW)
		if g.Width < minW {
			g.Width = minW
		}
		if g.Width > maxW {
			g.Width = maxW
		}
		if g.X < target.X {
			g.X = target.X
		}
		if g.X+g.Width > target.X+target.Width {
			g.X = target.X + target.Width - g.Width
		}
	}
	if target.Height > 0 {
		maxH := target.Height
		minH := minInt(minAppWindowHeight, maxH)
		if g.Height < minH {
			g.Height = minH
		}
		if g.Height > maxH {
			g.Height = maxH
		}
		if g.Y < target.Y {
			g.Y = target.Y
		}
		if g.Y+g.Height > target.Y+target.Height {
			g.Y = target.Y + target.Height - g.Height
		}
	}
	return g
}

// LoadClampedWindowGeometry reads persisted geometry and clamps it against active display monitors.
func LoadClampedWindowGeometry(path string) (WindowGeometry, bool) {
	g, ok, err := LoadWindowGeometry(path)
	if err != nil || !ok {
		return WindowGeometry{}, false
	}
	monitors := EnumMonitorBounds()
	return ClampWindowGeometry(g, monitors), true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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
