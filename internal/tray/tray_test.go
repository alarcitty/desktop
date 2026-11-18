package tray

import (
	"sync/atomic"
	"testing"
)

func TestTrayMenuLabelsAndDispatch(t *testing.T) {
	var playToggles, showToggles, updateChecks, quits atomic.Int32
	m := &Manager{
		playing: false,
		visible: true,
		cb: Callbacks{
			OnTogglePlay:  func() { playToggles.Add(1) },
			OnToggleShow:  func() { showToggles.Add(1) },
			OnCheckUpdate: func() { updateChecks.Add(1) },
			OnQuit:        func() { quits.Add(1) },
		},
	}

	playLbl, visLbl := m.MenuLabels()
	if playLbl != "Resume" || visLbl != "Hide to Tray" {
		t.Fatalf("unexpected initial labels: play=%q vis=%q", playLbl, visLbl)
	}

	m.SetPlaying(true)
	m.SetVisible(false)
	playLbl, visLbl = m.MenuLabels()
	if playLbl != "Pause" || visLbl != "Show Mila" {
		t.Fatalf("unexpected updated labels: play=%q vis=%q", playLbl, visLbl)
	}

	m.Dispatch(MenuTogglePlay)
	m.Dispatch(MenuToggleShow)
	m.Dispatch(MenuCheckUpd)
	m.Dispatch(MenuQuit)

	if playToggles.Load() != 1 || showToggles.Load() != 1 || updateChecks.Load() != 1 || quits.Load() != 1 {
		t.Fatalf("callbacks not invoked cleanly: play=%d show=%d upd=%d quit=%d",
			playToggles.Load(), showToggles.Load(), updateChecks.Load(), quits.Load())
	}

	// Close is idempotent.
	m.Close()
	m.Close()
}
