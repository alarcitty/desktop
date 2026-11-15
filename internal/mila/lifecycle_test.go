package mila

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mila/internal/logger"
	"mila/internal/screen"
)

func TestMilaBeforeCloseFlushAndGeometry(t *testing.T) {
	logDir := t.TempDir()
	if _, err := logger.ConfigureDefaultFileSink(logger.FileConfig{
		Dir:          logDir,
		MaxFileBytes: 4096,
		MaxFiles:     4,
	}); err != nil {
		t.Fatalf("ConfigureDefaultFileSink failed: %v", err)
	}
	t.Cleanup(func() {
		_ = logger.CloseDefaultSink()
	})

	statePath := filepath.Join(t.TempDir(), "window-state.json")
	m := New()
	m.windowStatePath = statePath

	var emittedEvents []string
	m.emitCloseEvent = func(_ context.Context, eventName string) {
		emittedEvents = append(emittedEvents, eventName)
	}
	m.captureGeometry = func(_ context.Context) (screen.WindowGeometry, bool) {
		return screen.WindowGeometry{
			X:         64,
			Y:         48,
			Width:     1366,
			Height:    768,
			Maximized: false,
		}, true
	}

	prevent := m.BeforeClose(context.Background())
	if prevent {
		t.Fatalf("BeforeClose must not prevent window close")
	}
	if len(emittedEvents) != 1 || emittedEvents[0] != "mila:before-close" {
		t.Fatalf("expected mila:before-close event, got %v", emittedEvents)
	}

	geom, ok, err := screen.LoadWindowGeometry(statePath)
	if err != nil || !ok {
		t.Fatalf("expected persisted window geometry on BeforeClose, ok=%v err=%v", ok, err)
	}
	if geom.Width != 1366 || geom.Height != 768 || geom.X != 64 || geom.Y != 48 {
		t.Fatalf("unexpected persisted window geometry: %+v", geom)
	}

	// Explicit SaveWindowState binding updates geometry consistently.
	if err := m.SaveWindowState(10, 20, 1920, 1080, true); err != nil {
		t.Fatalf("SaveWindowState failed: %v", err)
	}
	geom, ok, err = screen.LoadWindowGeometry(statePath)
	if err != nil || !ok || !geom.Maximized || geom.Width != 1920 {
		t.Fatalf("unexpected updated geometry: %+v ok=%v err=%v", geom, ok, err)
	}

	m.Shutdown(context.Background())
	// Verify runtime.shutdown record flushed to log file on Shutdown.
	entries, _ := os.ReadDir(logDir)
	var combined strings.Builder
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			raw, _ := os.ReadFile(filepath.Join(logDir, entry.Name()))
			combined.Write(raw)
		}
	}
	if !strings.Contains(combined.String(), "runtime.shutdown") {
		t.Fatalf("expected runtime.shutdown flushed to disk log, got: %s", combined.String())
	}
}

func TestMilaUnifiedContractAndDiagnostics(t *testing.T) {
	logDir := t.TempDir()
	if _, err := logger.ConfigureDefaultFileSink(logger.FileConfig{
		Dir:          logDir,
		MaxFileBytes: 4096,
		MaxFiles:     4,
	}); err != nil {
		t.Fatalf("ConfigureDefaultFileSink failed: %v", err)
	}
	t.Cleanup(func() {
		_ = logger.CloseDefaultSink()
	})

	log := logger.Make("contract")
	log.Info("auth.probe", "token", "wails-secret-token-value")

	m := New()
	rawContract, err := m.GetContractInfo()
	if err != nil {
		t.Fatalf("GetContractInfo failed: %v", err)
	}
	var env struct {
		Data struct {
			Status       string   `json:"status"`
			Mode         string   `json:"mode"`
			APIBase      string   `json:"apiBase"`
			ExtBase      string   `json:"extBase"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawContract), &env); err != nil {
		t.Fatalf("invalid GetContractInfo JSON: %v", err)
	}
	if env.Data.Status != "ok" || env.Data.Mode != "wails" || env.Data.APIBase != "/api/v1" || env.Data.ExtBase != "/ext" {
		t.Fatalf("unexpected GetContractInfo envelope: %+v", env.Data)
	}

	exportDir := t.TempDir()
	rawExport, err := m.ExportDiagnosticsLogs(exportDir)
	if err != nil {
		t.Fatalf("ExportDiagnosticsLogs failed: %v", err)
	}
	var exportEnv struct {
		Data logger.DiagnosticsReport `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawExport), &exportEnv); err != nil {
		t.Fatalf("invalid ExportDiagnosticsLogs JSON: %v", err)
	}
	if !exportEnv.Data.Redacted || exportEnv.Data.ZipPath == "" || exportEnv.Data.LogFiles == 0 {
		t.Fatalf("unexpected diagnostics report: %+v", exportEnv.Data)
	}

	zipRaw, err := os.ReadFile(exportEnv.Data.ZipPath)
	if err != nil {
		t.Fatalf("reading exported zip failed: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipRaw), int64(len(zipRaw)))
	if err != nil {
		t.Fatalf("invalid zip from Wails binding: %v", err)
	}
	for _, f := range zr.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		if bytes.Contains(body, []byte("wails-secret-token-value")) {
			t.Fatalf("secret leaked in exported zip entry %s", f.Name)
		}
	}

	// Verify Wails AssetServer handler exposes unified /health and /api/v1/* endpoints.
	handler := NewProviderHandler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"apiBase":"/api/v1"`) {
		t.Fatalf("expected unified /health inside Wails handler, got %d %s", rec.Code, rec.Body.String())
	}
}
