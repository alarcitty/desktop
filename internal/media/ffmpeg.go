package media

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	primaryFFmpegZipURL  = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	fallbackFFmpegZipURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
)

// FFmpegStatus describes the availability or active download progress of portable ffmpeg/ffprobe.
type FFmpegStatus struct {
	Available   bool   `json:"available"`
	Downloading bool   `json:"downloading"`
	ProgressPct int    `json:"progressPct"`
	Stage       string `json:"stage"`
	FFmpegPath  string `json:"ffmpegPath,omitempty"`
	FFprobePath string `json:"ffprobePath,omitempty"`
	Error       string `json:"error,omitempty"`
}

var (
	ffmpegMu     sync.Mutex
	ffmpegState  = FFmpegStatus{Stage: "idle"}
	ffmpegHTTP   = &http.Client{Timeout: 15 * time.Minute}
	ensureOnceMu sync.Mutex
)

// BinDir returns `%AppData%/mila/bin` (or config dir fallback) where portable tools are stored.
func BinDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(cfgDir) == "" {
		cfgDir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgDir = filepath.Join(cfgDir, ".config")
	}
	return filepath.Join(cfgDir, "mila", "bin"), nil
}

// GetFFmpegStatus reports whether ffmpeg/ffprobe are already usable or currently downloading.
func GetFFmpegStatus() FFmpegStatus {
	if ff, fp, ok := findExistingBinaries(); ok {
		ffmpegMu.Lock()
		ffmpegState = FFmpegStatus{
			Available:   true,
			Downloading: false,
			ProgressPct: 100,
			Stage:       "ready",
			FFmpegPath:  ff,
			FFprobePath: fp,
		}
		out := ffmpegState
		ffmpegMu.Unlock()
		return out
	}
	ffmpegMu.Lock()
	out := ffmpegState
	ffmpegMu.Unlock()
	return out
}

func findExistingBinaries() (string, string, bool) {
	ffName := "ffmpeg"
	fpName := "ffprobe"
	if runtime.GOOS == "windows" {
		ffName = "ffmpeg.exe"
		fpName = "ffprobe.exe"
	}

	if dir, err := BinDir(); err == nil {
		ff := filepath.Join(dir, ffName)
		fp := filepath.Join(dir, fpName)
		if fileExists(ff) && fileExists(fp) {
			return ff, fp, true
		}
	}

	if ffSys, err1 := exec.LookPath(ffName); err1 == nil {
		if fpSys, err2 := exec.LookPath(fpName); err2 == nil {
			return ffSys, fpSys, true
		}
	}
	return "", "", false
}

// EnsureFFmpeg locates local/PATH binaries or lazily downloads portable Windows ffmpeg+ffprobe on first use.
func EnsureFFmpeg(ctx context.Context) (string, string, error) {
	if ff, fp, ok := findExistingBinaries(); ok {
		return ff, fp, nil
	}
	if runtime.GOOS != "windows" {
		return "", "", errors.New("ffmpeg and ffprobe must be installed on PATH")
	}

	ensureOnceMu.Lock()
	defer ensureOnceMu.Unlock()

	if ff, fp, ok := findExistingBinaries(); ok {
		return ff, fp, nil
	}

	binDir, err := BinDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", "", err
	}

	updateStatus(FFmpegStatus{
		Available:   false,
		Downloading: true,
		ProgressPct: 1,
		Stage:       "downloading ffmpeg bundle",
	})

	tmpZip := filepath.Join(binDir, "ffmpeg-bundle.zip.tmp")
	defer os.Remove(tmpZip)

	var dlErr error
	for _, srcURL := range []string{primaryFFmpegZipURL, fallbackFFmpegZipURL} {
		dlErr = downloadWithProgress(ctx, srcURL, tmpZip)
		if dlErr == nil {
			break
		}
	}
	if dlErr != nil {
		updateStatus(FFmpegStatus{
			Available:   false,
			Downloading: false,
			ProgressPct: 0,
			Stage:       "error",
			Error:       dlErr.Error(),
		})
		return "", "", dlErr
	}

	updateStatus(FFmpegStatus{
		Available:   false,
		Downloading: true,
		ProgressPct: 95,
		Stage:       "extracting binaries",
	})

	if err := extractFFmpegBinariesFromZip(tmpZip, binDir); err != nil {
		updateStatus(FFmpegStatus{
			Available:   false,
			Downloading: false,
			ProgressPct: 0,
			Stage:       "error",
			Error:       err.Error(),
		})
		return "", "", err
	}

	ff := filepath.Join(binDir, "ffmpeg.exe")
	fp := filepath.Join(binDir, "ffprobe.exe")
	updateStatus(FFmpegStatus{
		Available:   true,
		Downloading: false,
		ProgressPct: 100,
		Stage:       "ready",
		FFmpegPath:  ff,
		FFprobePath: fp,
	})
	return ff, fp, nil
}

func downloadWithProgress(ctx context.Context, srcURL string, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}
	resp, err := ffmpegHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ffmpeg download HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return wErr
			}
			written += int64(n)
			if total > 0 {
				pct := int((written * 92) / total)
				if pct < 1 {
					pct = 1
				}
				updateStatus(FFmpegStatus{
					Available:   false,
					Downloading: true,
					ProgressPct: pct,
					Stage:       "downloading ffmpeg bundle",
				})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return out.Close()
}

func extractFFmpegBinariesFromZip(zipPath string, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var foundFFmpeg, foundFFprobe bool
	for _, f := range r.File {
		base := strings.ToLower(filepath.Base(f.Name))
		if base != "ffmpeg.exe" && base != "ffprobe.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, base)
		tmpDest := dest + ".tmp"
		out, createErr := os.Create(tmpDest)
		if createErr != nil {
			rc.Close()
			return createErr
		}
		_, copyErr := io.Copy(out, rc)
		closeOutErr := out.Close()
		rc.Close()
		if copyErr != nil {
			os.Remove(tmpDest)
			return copyErr
		}
		if closeOutErr != nil {
			os.Remove(tmpDest)
			return closeOutErr
		}
		if err := os.Rename(tmpDest, dest); err != nil {
			os.Remove(tmpDest)
			return err
		}
		if base == "ffmpeg.exe" {
			foundFFmpeg = true
		} else {
			foundFFprobe = true
		}
		if foundFFmpeg && foundFFprobe {
			break
		}
	}
	if !foundFFmpeg || !foundFFprobe {
		return errors.New("downloaded archive did not contain ffmpeg.exe and ffprobe.exe")
	}
	return nil
}

func updateStatus(next FFmpegStatus) {
	ffmpegMu.Lock()
	ffmpegState = next
	ffmpegMu.Unlock()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
