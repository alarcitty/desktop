package logger

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var colorsSupported = false

const (
	// DefaultMaxFileBytes triggers size-based log rotation at 5 MiB per file.
	DefaultMaxFileBytes int64 = 5 << 20
	// DefaultMaxFiles caps retained rotated log files inside the log directory.
	DefaultMaxFiles = 14
)

// palette
const (
	colorReset   = "\033[0m"
	colorGreen   = "\033[38;2;166;227;161m" // #a6e3a1
	colorTeal    = "\033[38;2;148;226;213m" // #94e2d5
	colorYellow  = "\033[38;2;249;226;175m" // #f9e2af
	colorPeach   = "\033[38;2;250;179;135m" // #fab387
	colorRed     = "\033[38;2;243;139;168m" // #f38ba8
	colorSubtext = "\033[38;2;166;173;200m" // #a6adc8
	colorWhite   = "\033[38;2;205;214;244m" // #cdd6f4
	colorKey     = "\033[38;2;137;220;235m" // #89dceb
	colorVal     = "\033[38;2;166;227;161m" // #a6e3a1
)

// FileConfig configures size- and date-based rotating file logging and ZIP diagnostics.
type FileConfig struct {
	Dir          string
	MaxFileBytes int64
	MaxFiles     int
	Now          func() time.Time
}

// DiagnosticsReport describes an exported sanitized diagnostic ZIP archive.
type DiagnosticsReport struct {
	ZipPath    string `json:"path"`
	LogFiles   int    `json:"files"`
	Bytes      int64  `json:"bytes"`
	ExportedAt string `json:"exportedAt"`
	Redacted   bool   `json:"redacted"`
}

// FileSink writes structured log lines to date- and size-rotated log files without secrets.
type FileSink struct {
	mu           sync.Mutex
	dir          string
	maxFileBytes int64
	maxFiles     int
	now          func() time.Time

	curDate string
	curFile *os.File
	curSize int64
	closed  bool
}

var (
	defaultSinkMu sync.RWMutex
	defaultSink   *FileSink

	reBearer       = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-~+/]+=*`)
	reURLCreds     = regexp.MustCompile(`(https?://)([^/@\s:]+):([^/@\s]+)@`)
	reKVSecret     = regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:token|secret|password|passwd|apikey|api_key|authorization|cookie|session|jwt)[a-z0-9_]*)\s*([=:])\s*([^\s"'&,;]+)`)
	reQuotedSecret = regexp.MustCompile(`(?i)"([a-z0-9_]*(?:token|secret|password|passwd|apikey|api_key|authorization|cookie|session|jwt)[a-z0-9_]*)"\s*:\s*"([^"]*)"`)
	reANSIEscape    = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

// DefaultLogDir resolves the platform cache directory for persisted Mila log files.
func DefaultLogDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		return filepath.Join(os.TempDir(), "mila", "logs"), nil
	}
	return filepath.Join(base, "mila", "logs"), nil
}

// NewFileSink creates a thread-safe rotating disk log sink.
func NewFileSink(cfg FileConfig) (*FileSink, error) {
	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		resolved, err := DefaultLogDir()
		if err != nil {
			return nil, err
		}
		dir = resolved
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	maxBytes := cfg.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileBytes
	}
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = DefaultMaxFiles
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &FileSink{
		dir:          dir,
		maxFileBytes: maxBytes,
		maxFiles:     maxFiles,
		now:          nowFn,
	}, nil
}

// EnableDefaultFileLogging initializes package-wide rotating disk logs once using DefaultLogDir.
func EnableDefaultFileLogging() error {
	defaultSinkMu.Lock()
	defer defaultSinkMu.Unlock()
	if defaultSink != nil && !defaultSink.isClosed() {
		return nil
	}
	sink, err := NewFileSink(FileConfig{})
	if err != nil {
		return err
	}
	defaultSink = sink
	return nil
}

// ConfigureDefaultFileSink replaces the package-wide rotating log sink.
func ConfigureDefaultFileSink(cfg FileConfig) (*FileSink, error) {
	sink, err := NewFileSink(cfg)
	if err != nil {
		return nil, err
	}
	defaultSinkMu.Lock()
	old := defaultSink
	defaultSink = sink
	defaultSinkMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return sink, nil
}

// DefaultFileSink returns the currently active package-wide FileSink, or nil if disabled.
func DefaultFileSink() *FileSink {
	defaultSinkMu.RLock()
	defer defaultSinkMu.RUnlock()
	return defaultSink
}

// FlushDefaultSink flushes active log file buffers to OS storage.
func FlushDefaultSink() error {
	sink := DefaultFileSink()
	if sink == nil {
		return nil
	}
	return sink.Flush()
}

// CloseDefaultSink flushes and closes the package-wide log sink.
func CloseDefaultSink() error {
	defaultSinkMu.Lock()
	sink := defaultSink
	defaultSink = nil
	defaultSinkMu.Unlock()
	if sink == nil {
		return nil
	}
	return sink.Close()
}

func (s *FileSink) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Dir returns the directory managed by this FileSink.
func (s *FileSink) Dir() string {
	return s.dir
}

// WriteLine writes one sanitized log line, rotating by calendar day or MaxFileBytes as needed.
func (s *FileSink) WriteLine(line string) error {
	clean := SanitizeLogText(reANSIEscape.ReplaceAllString(line, ""))
	if !strings.HasSuffix(clean, "\n") {
		clean += "\n"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("log sink is closed")
	}

	today := s.now().UTC().Format("2006-01-02")
	if err := s.ensureOpenForDateLocked(today, int64(len(clean))); err != nil {
		return err
	}

	n, err := s.curFile.WriteString(clean)
	s.curSize += int64(n)
	return err
}

// Flush syncs any buffered OS writes for the active log file.
func (s *FileSink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.curFile == nil {
		return nil
	}
	return s.curFile.Sync()
}

// Close flushes and closes the active log file.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.curFile == nil {
		return nil
	}
	_ = s.curFile.Sync()
	err := s.curFile.Close()
	s.curFile = nil
	s.curSize = 0
	return err
}

func (s *FileSink) activePath(date string) string {
	return filepath.Join(s.dir, fmt.Sprintf("mila-%s.log", date))
}

func (s *FileSink) ensureOpenForDateLocked(today string, incomingBytes int64) error {
	if s.curFile != nil && s.curDate != today {
		_ = s.curFile.Sync()
		_ = s.curFile.Close()
		s.curFile = nil
		s.curSize = 0
	}

	if s.curFile != nil && s.curSize > 0 && s.curSize+incomingBytes > s.maxFileBytes {
		if err := s.rotateSizeLocked(); err != nil {
			return err
		}
	}

	if s.curFile == nil {
		path := s.activePath(today)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		info, statErr := f.Stat()
		var size int64
		if statErr == nil {
			size = info.Size()
		}
		// If pre-existing file for today is already over limit, rotate it right away.
		if size > 0 && size+incomingBytes > s.maxFileBytes {
			_ = f.Close()
			s.curDate = today
			s.curSize = size
			if err := s.rotatePathLocked(path, today); err != nil {
				return err
			}
			f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err != nil {
				return err
			}
			size = 0
		}
		s.curDate = today
		s.curFile = f
		s.curSize = size
		s.pruneOldFilesLocked()
	}

	return nil
}

func (s *FileSink) rotateSizeLocked() error {
	if s.curFile != nil {
		_ = s.curFile.Sync()
		_ = s.curFile.Close()
		s.curFile = nil
	}
	err := s.rotatePathLocked(s.activePath(s.curDate), s.curDate)
	s.curSize = 0
	s.pruneOldFilesLocked()
	return err
}

func (s *FileSink) rotatePathLocked(activePath, date string) error {
	if _, err := os.Stat(activePath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	seq := 1
	for {
		archived := filepath.Join(s.dir, fmt.Sprintf("mila-%s.%d.log", date, seq))
		if _, err := os.Stat(archived); errors.Is(err, os.ErrNotExist) {
			return os.Rename(activePath, archived)
		}
		seq++
	}
}

func (s *FileSink) listLogFilesLocked() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var logs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "mila-") && strings.HasSuffix(name, ".log") {
			logs = append(logs, filepath.Join(s.dir, name))
		}
	}
	sort.Slice(logs, func(i, j int) bool {
		ii, errI := os.Stat(logs[i])
		jj, errJ := os.Stat(logs[j])
		if errI == nil && errJ == nil && !ii.ModTime().Equal(jj.ModTime()) {
			return ii.ModTime().Before(jj.ModTime())
		}
		return filepath.Base(logs[i]) < filepath.Base(logs[j])
	})
	return logs, nil
}

func (s *FileSink) pruneOldFilesLocked() {
	if s.maxFiles <= 0 {
		return
	}
	files, err := s.listLogFilesLocked()
	if err != nil || len(files) <= s.maxFiles {
		return
	}
	excess := len(files) - s.maxFiles
	active := ""
	if s.curDate != "" {
		active = s.activePath(s.curDate)
	}
	removed := 0
	for _, path := range files {
		if removed >= excess {
			break
		}
		if path == active {
			continue
		}
		if os.Remove(path) == nil {
			removed++
		}
	}
}

// ExportDiagnosticsBytes builds an in-memory redacted ZIP diagnostics bundle from all retained log files.
func (s *FileSink) ExportDiagnosticsBytes() ([]byte, DiagnosticsReport, error) {
	s.mu.Lock()
	if s.curFile != nil {
		_ = s.curFile.Sync()
	}
	files, err := s.listLogFilesLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, DiagnosticsReport{}, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	var totalRawBytes int64
	writtenFiles := 0
	var archivedNames []string

	for _, logPath := range files {
		raw, readErr := os.ReadFile(logPath)
		if readErr != nil {
			continue
		}
		sanitized := SanitizeLogText(string(raw))
		name := filepath.Base(logPath)
		w, createErr := zw.Create("logs/" + name)
		if createErr != nil {
			_ = zw.Close()
			return nil, DiagnosticsReport{}, createErr
		}
		n, writeErr := io.WriteString(w, sanitized)
		if writeErr != nil {
			_ = zw.Close()
			return nil, DiagnosticsReport{}, writeErr
		}
		totalRawBytes += int64(n)
		writtenFiles++
		archivedNames = append(archivedNames, name)
	}

	exportedAt := s.now().UTC().Format(time.RFC3339)
	meta := map[string]any{
		"app":        "mila",
		"goVersion":  runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"exportedAt": exportedAt,
		"redacted":   true,
		"logFiles":   archivedNames,
		"totalBytes": totalRawBytes,
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = zw.Close()
		return nil, DiagnosticsReport{}, err
	}
	mw, err := zw.Create("system.json")
	if err != nil {
		_ = zw.Close()
		return nil, DiagnosticsReport{}, err
	}
	if _, err := mw.Write(metaJSON); err != nil {
		_ = zw.Close()
		return nil, DiagnosticsReport{}, err
	}

	if err := zw.Close(); err != nil {
		return nil, DiagnosticsReport{}, err
	}

	report := DiagnosticsReport{
		LogFiles:   writtenFiles,
		Bytes:      int64(buf.Len()),
		ExportedAt: exportedAt,
		Redacted:   true,
	}
	return buf.Bytes(), report, nil
}

// ExportDiagnosticsZip writes a sanitized diagnostic ZIP archive atomically to destPath (or a generated file inside destPath if it is a directory).
func (s *FileSink) ExportDiagnosticsZip(destPath string) (DiagnosticsReport, error) {
	payload, report, err := s.ExportDiagnosticsBytes()
	if err != nil {
		return DiagnosticsReport{}, err
	}

	target := strings.TrimSpace(destPath)
	if target == "" {
		target = filepath.Join(s.dir, fmt.Sprintf("mila-diagnostics-%s.zip", s.now().UTC().Format("20060102-150405")))
	} else {
		info, statErr := os.Stat(target)
		if (statErr == nil && info.IsDir()) || strings.HasSuffix(target, string(os.PathSeparator)) || !strings.HasSuffix(strings.ToLower(target), ".zip") {
			target = filepath.Join(target, fmt.Sprintf("mila-diagnostics-%s.zip", s.now().UTC().Format("20060102-150405")))
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return DiagnosticsReport{}, err
	}

	tmpPath := target + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return DiagnosticsReport{}, err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return DiagnosticsReport{}, err
	}

	report.ZipPath = target
	return report, nil
}

// ExportDiagnosticsZip exports diagnostics using the active package-wide FileSink (initializing one if needed).
func ExportDiagnosticsZip(destPath string) (DiagnosticsReport, error) {
	if DefaultFileSink() == nil {
		if err := EnableDefaultFileLogging(); err != nil {
			return DiagnosticsReport{}, err
		}
	}
	return DefaultFileSink().ExportDiagnosticsZip(destPath)
}

// ExportDiagnosticsBytes builds a sanitized diagnostic ZIP payload in memory using the active FileSink.
func ExportDiagnosticsBytes() ([]byte, DiagnosticsReport, error) {
	if DefaultFileSink() == nil {
		if err := EnableDefaultFileLogging(); err != nil {
			return nil, DiagnosticsReport{}, err
		}
	}
	return DefaultFileSink().ExportDiagnosticsBytes()
}

type Logger struct {
	scope string
	sink  *FileSink
}

// make_unique logger factory
func Make(scope string) *Logger {
	return &Logger{scope: scope}
}

// MakeWithSink creates a Logger bound to an explicit rotating FileSink (primarily for tests).
func MakeWithSink(scope string, sink *FileSink) *Logger {
	return &Logger{scope: scope, sink: sink}
}

func isSensitiveFieldKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "" {
		return false
	}
	sensitive := []string{
		"token", "secret", "password", "passwd", "authorization",
		"auth", "cookie", "apikey", "api_key", "bearer", "session", "jwt", "credential",
	}
	for _, needle := range sensitive {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// SanitizeLogText scrubs bearer tokens, embedded URL credentials, and common token/secret fields.
func SanitizeLogText(raw string) string {
	if raw == "" {
		return ""
	}
	out := reBearer.ReplaceAllString(raw, "Bearer [REDACTED]")
	out = reURLCreds.ReplaceAllString(out, "${1}[REDACTED]@")
	out = reQuotedSecret.ReplaceAllString(out, `"${1}":"[REDACTED]"`)
	out = reKVSecret.ReplaceAllString(out, "${1}${2}[REDACTED]")
	return out
}

// parameters key=val without extra air
func formatFields(colorized bool, fields ...any) string {
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(fields); i += 2 {
		key := fmt.Sprintf("%v", fields[i])
		val := "nil"
		if i+1 < len(fields) {
			if isSensitiveFieldKey(key) {
				val = "[REDACTED]"
			} else {
				val = SanitizeLogText(fmt.Sprintf("%v", fields[i+1]))
			}
		}
		if colorized {
			b.WriteString(fmt.Sprintf(" %s%s%s=%s%s%s", colorKey, key, colorReset, colorVal, val, colorReset))
		} else {
			b.WriteString(fmt.Sprintf(" %s=%s", key, val))
		}
	}
	return b.String()
}

func (l *Logger) emit(level, levelColor, action string, fields ...any) {
	now := time.Now()
	nowClock := now.Format("15:04:05")
	cleanAction := SanitizeLogText(action)
	plainParams := formatFields(false, fields...)

	if colorsSupported {
		colorParams := formatFields(true, fields...)
		fmt.Printf("%s%s%s %s[%s]%s %s%s%s %s%s%s%s\n",
			colorSubtext, nowClock, colorReset,
			colorGreen, l.scope, colorReset,
			levelColor, level, colorReset,
			colorWhite, cleanAction, colorReset,
			colorParams,
		)
	} else {
		fmt.Printf("%s [%s] %s: %s%s\n", nowClock, l.scope, level, cleanAction, plainParams)
	}

	sink := l.sink
	if sink == nil {
		sink = DefaultFileSink()
	}
	if sink != nil {
		line := fmt.Sprintf("%s [%s] %s: %s%s\n",
			now.UTC().Format(time.RFC3339),
			l.scope,
			level,
			cleanAction,
			plainParams,
		)
		_ = sink.WriteLine(line)
	}
}

func (l *Logger) Info(action string, fields ...any)  { l.emit("info", colorTeal, action, fields...) }
func (l *Logger) Ok(action string, fields ...any)    { l.emit("ok", colorGreen, action, fields...) }
func (l *Logger) Warn(action string, fields ...any)  { l.emit("warn", colorPeach, action, fields...) }
func (l *Logger) Error(action string, fields ...any) { l.emit("err", colorRed, action, fields...) }
