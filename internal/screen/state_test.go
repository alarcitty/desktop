package screen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadWindowGeometry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "window-state.json")

	want := WindowGeometry{
		X:         120,
		Y:         80,
		Width:     1440,
		Height:    900,
		Maximized: true,
		UpdatedAt: 1700000000000,
	}
	if err := SaveWindowGeometry(path, want); err != nil {
		t.Fatalf("SaveWindowGeometry failed: %v", err)
	}

	got, ok, err := LoadWindowGeometry(path)
	if err != nil || !ok {
		t.Fatalf("LoadWindowGeometry failed: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Fatalf("geometry mismatch: got %+v want %+v", got, want)
	}

	// Missing file returns ok=false with nil error.
	_, ok, err = LoadWindowGeometry(filepath.Join(dir, "missing.json"))
	if err != nil || ok {
		t.Fatalf("expected (false, nil) for missing file, got ok=%v err=%v", ok, err)
	}

	// Degenerate zero-sized window must be rejected rather than corrupting saved state.
	if err := SaveWindowGeometry(path, WindowGeometry{Width: 100, Height: 100}); err == nil {
		t.Fatalf("expected error saving tiny window dimensions")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected no lingering temp file")
	}
}

func TestClampWindowGeometryMultiMonitor(t *testing.T) {
	monitors := []MonitorBounds{
		{X: 0, Y: 0, Width: 1920, Height: 1040, Primary: true},
		{X: 1920, Y: 0, Width: 2560, Height: 1400, Primary: false},
	}

	// Valid window on second monitor stays intact.
	onSecond := ClampWindowGeometry(WindowGeometry{X: 2100, Y: 100, Width: 1600, Height: 900}, monitors)
	if onSecond.X != 2100 || onSecond.Y != 100 || onSecond.Width != 1600 || onSecond.Height != 900 {
		t.Fatalf("unexpected change to valid second-monitor geometry: %+v", onSecond)
	}

	// Window left on a disconnected third monitor (x=-1920) snaps back onto nearest active monitor.
	singleOnly := monitors[:1]
	disconnected := ClampWindowGeometry(WindowGeometry{X: -1920, Y: -200, Width: 1200, Height: 800}, singleOnly)
	if disconnected.X < 0 || disconnected.Y < 0 || disconnected.X+disconnected.Width > 1920 || disconnected.Y+disconnected.Height > 1040 {
		t.Fatalf("expected disconnected monitor window clamped inside primary bounds, got %+v", disconnected)
	}

	// Window hanging off right/bottom edge and oversized is shrunk & clamped inside target work area.
	oversized := ClampWindowGeometry(WindowGeometry{X: 1700, Y: 900, Width: 2400, Height: 1500}, singleOnly)
	if oversized.Width != 1920 || oversized.Height != 1040 || oversized.X != 0 || oversized.Y != 0 {
		t.Fatalf("expected oversized window clamped to (0,0,1920,1040), got %+v", oversized)
	}

	// Tiny saved dimensions are lifted to safe minimum.
	tiny := ClampWindowGeometry(WindowGeometry{X: 50, Y: 50, Width: 400, Height: 300}, singleOnly)
	if tiny.Width < minAppWindowWidth || tiny.Height < minAppWindowHeight {
		t.Fatalf("expected tiny dimensions bumped to minimum (%dx%d), got %+v", minAppWindowWidth, minAppWindowHeight, tiny)
	}
}
