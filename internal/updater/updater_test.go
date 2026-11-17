package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestSelectAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "Mila-amd64-installer.exe", BrowserDownloadURL: "https://example.com/installer.exe"},
		{Name: "Mila-windows-amd64-v0.1.2.zip", BrowserDownloadURL: "https://example.com/app.zip"},
		{Name: "Mila.exe", BrowserDownloadURL: "https://example.com/Mila.exe"},
	}

	portable, isInst := SelectAsset(assets, true)
	if portable == nil || portable.Name != "Mila.exe" || isInst {
		t.Fatalf("expected Mila.exe non-installer when writable, got %+v isInst=%v", portable, isInst)
	}

	installer, isInst := SelectAsset(assets, false)
	if installer == nil || installer.Name != "Mila-amd64-installer.exe" || !isInst {
		t.Fatalf("expected installer when not writable, got %+v isInst=%v", installer, isInst)
	}
}

func TestCheckAndInstall_UpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(GitHubRelease{
			TagName: "v0.1.1",
			HTMLURL: "https://github.com/mila-desktop/desktop/releases/tag/v0.1.1",
		})
	}))
	defer srv.Close()

	mgr := NewManager(Config{
		LatestReleaseURL: srv.URL,
		CurrentVersion:   func() string { return "v0.1.1" },
	})

	status := mgr.CheckAndInstall(context.Background(), nil, nil)
	if status.State != "up_to_date" || status.HasUpdate {
		t.Fatalf("expected up_to_date with HasUpdate=false, got %+v", status)
	}
}

func TestCheckAndInstall_DownloadsAndAppliesUpdate(t *testing.T) {
	fakePE := append([]byte("MZ"), make([]byte, 2046)...)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Mila.exe" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(fakePE)
			return
		}
		_ = json.NewEncoder(w).Encode(GitHubRelease{
			TagName: "v0.2.0",
			HTMLURL: "https://github.com/mila-desktop/desktop/releases/tag/v0.2.0",
			Assets: []ReleaseAsset{
				{
					Name:               "Mila.exe",
					Size:               int64(len(fakePE)),
					BrowserDownloadURL: srv.URL + "/Mila.exe",
				},
			},
		})
	}))
	defer srv.Close()

	var mu sync.Mutex
	var events []UpdateStatus
	applied := make(chan string, 1)
	quitCalled := make(chan struct{}, 1)

	mgr := NewManager(Config{
		LatestReleaseURL: srv.URL,
		CurrentVersion:   func() string { return "v0.1.1" },
		ExecutablePath:   func() (string, error) { return "C:\\temp\\Mila.exe", nil },
		CanReplaceExe:    func(string) bool { return true },
		MinAssetBytes:    512,
		ApplyUpdate: func(stagedPath, currentExePath string, pid int, isInstaller bool) error {
			_ = os.Remove(stagedPath)
			applied <- currentExePath
			return nil
		},
	})

	initial := mgr.CheckAndInstall(
		context.Background(),
		func(st UpdateStatus) {
			mu.Lock()
			events = append(events, st)
			mu.Unlock()
		},
		func() {
			quitCalled <- struct{}{}
		},
	)

	if initial.State != "downloading" || !initial.HasUpdate {
		t.Fatalf("expected initial downloading state, got %+v", initial)
	}

	select {
	case gotExe := <-applied:
		if gotExe != "C:\\temp\\Mila.exe" {
			t.Fatalf("unexpected target exe: %s", gotExe)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for ApplyUpdate")
	}

	select {
	case <-quitCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for quitFn")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 || events[len(events)-1].State != "installing" || events[len(events)-1].Progress != 100 {
		t.Fatalf("expected final installing event with progress 100, got %+v", events)
	}
}
