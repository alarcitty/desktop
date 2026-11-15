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
