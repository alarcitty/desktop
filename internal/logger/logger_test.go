package logger

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileSinkRotationBySizeAndDate(t *testing.T) {
	dir := t.TempDir()
	nowVal := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	sink, err := NewFileSink(FileConfig{
		Dir:          dir,
		MaxFileBytes: 220,
		MaxFiles:     4,
		Now:          func() time.Time { return nowVal },
	})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer sink.Close()

	log := MakeWithSink("test", sink)
	for i := 0; i < 8; i++ {
		log.Info("request.completed", "step", i, "payload", strings.Repeat("x", 45))
	}

	// Verify size-based rotation occurred on the same date while enforcing MaxFiles retention.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	var logNames []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") {
			logNames = append(logNames, entry.Name())
		}
	}
	if len(logNames) < 2 || len(logNames) > 4 {
		t.Fatalf("expected 2..4 rotated log files under MaxFiles=4, got %v", logNames)
	}

	// Advance calendar day -> triggers date rotation into a new mila-2026-11-16.log file.
	nowVal = nowVal.Add(24 * time.Hour)
	log.Ok("day.rolled", "marker", "day2")
	_ = sink.Flush()

	day2Path := filepath.Join(dir, "mila-2026-11-16.log")
	day2Bytes, err := os.ReadFile(day2Path)
	if err != nil {
		t.Fatalf("expected date-rotated file %s: %v", day2Path, err)
	}
	if !strings.Contains(string(day2Bytes), "day.rolled") {
		t.Fatalf("day2 log missing expected record: %s", string(day2Bytes))
	}
}

func TestSecretRedactionAndDiagnosticsZipExport(t *testing.T) {
	dir := t.TempDir()
	nowVal := time.Date(2026, 11, 15, 12, 30, 0, 0, time.UTC)

	sink, err := NewFileSink(FileConfig{
		Dir:          dir,
		MaxFileBytes: 4096,
		MaxFiles:     5,
		Now:          func() time.Time { return nowVal },
	})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer sink.Close()

	log := MakeWithSink("auth", sink)
	log.Info("upstream.call",
		"token", "shiki-super-secret-999",
		"Authorization", "Bearer top-secret-jwt-value",
		"url", "https://user:hunter2@api.example.com/v1?access_token=xyz123&page=1",
		"ok", "yes",
	)
	_ = sink.Flush()

	exportDir := t.TempDir()
	report, err := sink.ExportDiagnosticsZip(exportDir)
	if err != nil {
		t.Fatalf("ExportDiagnosticsZip failed: %v", err)
	}
	if !report.Redacted || report.LogFiles == 0 || report.Bytes == 0 || report.ZipPath == "" {
		t.Fatalf("unexpected diagnostics report: %+v", report)
	}

	zipRaw, err := os.ReadFile(report.ZipPath)
	if err != nil {
		t.Fatalf("failed reading zip archive: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipRaw), int64(len(zipRaw)))
	if err != nil {
		t.Fatalf("invalid zip archive: %v", err)
	}

	var hasSystemJSON, hasLogFile bool
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s failed: %v", f.Name, err)
		}
		content, _ := io.ReadAll(rc)
		_ = rc.Close()

		txt := string(content)
		if strings.Contains(txt, "shiki-super-secret-999") ||
			strings.Contains(txt, "top-secret-jwt-value") ||
			strings.Contains(txt, "hunter2") ||
			strings.Contains(txt, "xyz123") {
			t.Fatalf("secret leaked in zip entry %s: %s", f.Name, txt)
		}

		if f.Name == "system.json" {
			hasSystemJSON = true
			var parsed map[string]any
			if err := json.Unmarshal(content, &parsed); err != nil || parsed["redacted"] != true {
				t.Fatalf("unexpected system.json metadata: %s", txt)
			}
		}
		if strings.HasPrefix(f.Name, "logs/mila-") && strings.HasSuffix(f.Name, ".log") {
			hasLogFile = true
			if !strings.Contains(txt, "[REDACTED]") {
				t.Fatalf("expected [REDACTED] markers in %s: %s", f.Name, txt)
			}
		}
	}

	if !hasSystemJSON || !hasLogFile {
		t.Fatalf("zip missing required entries: systemJSON=%v logFile=%v", hasSystemJSON, hasLogFile)
	}
}

func TestFileSinkConcurrentWritesAndDefaultExport(t *testing.T) {
	dir := t.TempDir()
	sink, err := ConfigureDefaultFileSink(FileConfig{
		Dir:          dir,
		MaxFileBytes: 1024,
		MaxFiles:     10,
	})
	if err != nil {
		t.Fatalf("ConfigureDefaultFileSink failed: %v", err)
	}
	t.Cleanup(func() {
		_ = CloseDefaultSink()
	})

	log := Make("concurrent")
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				log.Info("worker.tick", "worker", id, "iter", i, "secret", "should-not-leak")
			}
		}(g)
	}
	wg.Wait()
	_ = FlushDefaultSink()

	payload, report, err := ExportDiagnosticsBytes()
	if err != nil {
		t.Fatalf("ExportDiagnosticsBytes failed: %v", err)
	}
	if len(payload) == 0 || report.LogFiles == 0 {
		t.Fatalf("expected non-empty diagnostics bytes, got len=%d report=%+v", len(payload), report)
	}
	if bytes.Contains(payload, []byte("should-not-leak")) {
		t.Fatalf("secret leaked inside compressed zip stream")
	}
	if sink.Dir() != dir {
		t.Fatalf("expected dir %s, got %s", dir, sink.Dir())
	}
}
