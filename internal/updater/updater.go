package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mila/internal/version"
)

const (
	DefaultLatestReleaseURL = "https://api.github.com/repos/mila-desktop/desktop/releases/latest"
	DefaultMinAssetBytes    = 1024 * 1024 // 1 MiB safety guard against HTML error responses
)

// ReleaseAsset describes a downloadable file attached to a GitHub release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GitHubRelease models the subset of GitHub's /releases/latest payload needed by the updater.
type GitHubRelease struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Body    string         `json:"body"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

// UpdateStatus captures the current updater lifecycle state for the UI.
type UpdateStatus struct {
	CurrentVersion string  `json:"currentVersion"`
	LatestVersion  string  `json:"latestVersion"`
	HasUpdate      bool    `json:"hasUpdate"`
	State          string  `json:"state"` // idle | checking | up_to_date | downloading | installing | error
	Progress       float64 `json:"progress"`
	ReleaseURL     string  `json:"releaseUrl,omitempty"`
	Error          string  `json:"error,omitempty"`
}

// Config configures updater network endpoints and validation limits (overridable in tests).
type Config struct {
	LatestReleaseURL string
	HTTPClient       *http.Client
	CurrentVersion   func() string
	ExecutablePath   func() (string, error)
	CanReplaceExe    func(exePath string) bool
	ApplyUpdate      func(stagedPath, currentExePath string, pid int, isInstaller bool) error
	MinAssetBytes    int64
}

// Manager coordinates checking for GitHub Releases, downloading updates, and relaunching cleanly.
type Manager struct {
	cfg    Config
	mu     sync.Mutex
	active bool
	status UpdateStatus
}

// NewManager constructs a Manager using production defaults unless overridden in Config.
func NewManager(cfg Config) *Manager {
	if strings.TrimSpace(cfg.LatestReleaseURL) == "" {
		cfg.LatestReleaseURL = DefaultLatestReleaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.CurrentVersion == nil {
		cfg.CurrentVersion = version.Current
	}
	if cfg.ExecutablePath == nil {
		cfg.ExecutablePath = os.Executable
	}
	if cfg.CanReplaceExe == nil {
		cfg.CanReplaceExe = CanReplaceExecutable
	}
	if cfg.ApplyUpdate == nil {
		cfg.ApplyUpdate = ApplyAndRelaunch
	}
	if cfg.MinAssetBytes <= 0 {
		cfg.MinAssetBytes = DefaultMinAssetBytes
	}
	return &Manager{
		cfg: cfg,
		status: UpdateStatus{
			CurrentVersion: cfg.CurrentVersion(),
			State:          "idle",
		},
	}
}

// Status returns a snapshot of the most recent updater status.
func (m *Manager) Status() UpdateStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) setStatus(next UpdateStatus, onProgress func(UpdateStatus)) {
	m.mu.Lock()
	m.status = next
	m.mu.Unlock()
	if onProgress != nil {
		onProgress(next)
	}
}

// SelectAsset picks the best Windows executable or NSIS installer asset from a release.
func SelectAsset(assets []ReleaseAsset, canWriteExeDir bool) (*ReleaseAsset, bool) {
	var portable *ReleaseAsset
	var installer *ReleaseAsset

	for i := range assets {
		a := &assets[i]
		lower := strings.ToLower(strings.TrimSpace(a.Name))
		if a.BrowserDownloadURL == "" || !strings.HasSuffix(lower, ".exe") {
			continue
		}
		if strings.Contains(lower, "installer") || strings.Contains(lower, "setup") {
			if installer == nil {
				installer = a
			}
			continue
		}
		if strings.HasPrefix(lower, "mila") {
			if portable == nil {
				portable = a
			}
		}
	}

	if canWriteExeDir {
		if portable != nil {
			return portable, false
		}
		if installer != nil {
			return installer, true
		}
		return nil, false
	}
	if installer != nil {
		return installer, true
	}
	if portable != nil {
		return portable, false
	}
	return nil, false
}

// FetchLatestRelease queries GitHub for the latest published release metadata.
func (m *Manager) FetchLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.LatestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Mila-Desktop/"+m.cfg.CurrentVersion())

	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github releases returned HTTP %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return nil, errors.New("latest release missing tag_name")
	}
	return &rel, nil
}

// CheckAndInstall checks GitHub Releases; if an update is found, it downloads and applies it before triggering quitFn.
func (m *Manager) CheckAndInstall(
	ctx context.Context,
	onProgress func(UpdateStatus),
	quitFn func(),
) UpdateStatus {
	m.mu.Lock()
	if m.active {
		current := m.status
		m.mu.Unlock()
		return current
	}
	m.active = true
	curVer := m.cfg.CurrentVersion()
	checking := UpdateStatus{
		CurrentVersion: curVer,
		State:          "checking",
		Progress:       0,
	}
	m.status = checking
	m.mu.Unlock()

	if onProgress != nil {
		onProgress(checking)
	}

	releaseActive := func() {
		m.mu.Lock()
		m.active = false
		m.mu.Unlock()
	}

	rel, err := m.FetchLatestRelease(ctx)
	if err != nil {
		releaseActive()
		errStatus := UpdateStatus{
			CurrentVersion: curVer,
			State:          "error",
			Error:          err.Error(),
		}
		m.setStatus(errStatus, onProgress)
		return errStatus
	}

	if version.Compare(rel.TagName, curVer) <= 0 {
		releaseActive()
		upToDate := UpdateStatus{
			CurrentVersion: curVer,
			LatestVersion:  rel.TagName,
			HasUpdate:      false,
			State:          "up_to_date",
			ReleaseURL:     rel.HTMLURL,
		}
		m.setStatus(upToDate, onProgress)
		return upToDate
	}

	exePath, exeErr := m.cfg.ExecutablePath()
	canReplace := exeErr == nil && exePath != "" && m.cfg.CanReplaceExe(exePath)
	asset, isInstaller := SelectAsset(rel.Assets, canReplace)
	if asset == nil {
		releaseActive()
		errStatus := UpdateStatus{
			CurrentVersion: curVer,
			LatestVersion:  rel.TagName,
			HasUpdate:      true,
			State:          "error",
			ReleaseURL:     rel.HTMLURL,
			Error:          "no compatible Windows update asset found",
		}
		m.setStatus(errStatus, onProgress)
		return errStatus
	}

	initialDownload := UpdateStatus{
		CurrentVersion: curVer,
		LatestVersion:  rel.TagName,
		HasUpdate:      true,
		State:          "downloading",
		Progress:       0,
		ReleaseURL:     rel.HTMLURL,
	}
	m.setStatus(initialDownload, onProgress)

	go func() {
		defer releaseActive()

		dlCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		stagedPath, dlErr := m.downloadAsset(dlCtx, *asset, rel.TagName, func(pct float64) {
			m.setStatus(UpdateStatus{
				CurrentVersion: curVer,
				LatestVersion:  rel.TagName,
				HasUpdate:      true,
				State:          "downloading",
				Progress:       pct,
				ReleaseURL:     rel.HTMLURL,
			}, onProgress)
		})
		if dlErr != nil {
			m.setStatus(UpdateStatus{
				CurrentVersion: curVer,
				LatestVersion:  rel.TagName,
				HasUpdate:      true,
				State:          "error",
				ReleaseURL:     rel.HTMLURL,
				Error:          dlErr.Error(),
			}, onProgress)
			return
		}

		m.setStatus(UpdateStatus{
			CurrentVersion: curVer,
			LatestVersion:  rel.TagName,
			HasUpdate:      true,
			State:          "installing",
			Progress:       100,
			ReleaseURL:     rel.HTMLURL,
		}, onProgress)

		if applyErr := m.cfg.ApplyUpdate(stagedPath, exePath, os.Getpid(), isInstaller); applyErr != nil {
			_ = os.Remove(stagedPath)
			m.setStatus(UpdateStatus{
				CurrentVersion: curVer,
				LatestVersion:  rel.TagName,
				HasUpdate:      true,
				State:          "error",
				ReleaseURL:     rel.HTMLURL,
				Error:          applyErr.Error(),
			}, onProgress)
			return
		}

		if quitFn != nil {
			time.Sleep(180 * time.Millisecond)
			quitFn()
		}
	}()

	return initialDownload
}

func (m *Manager) downloadAsset(
	ctx context.Context,
	asset ReleaseAsset,
	tag string,
	onPct func(float64),
) (string, error) {
	// Use a client without a short request timeout for large binary downloads.
	dlClient := &http.Client{
		Transport: m.cfg.HTTPClient.Transport,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mila-Desktop/"+m.cfg.CurrentVersion())

	resp, err := dlClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	totalBytes := resp.ContentLength
	if totalBytes <= 0 && asset.Size > 0 {
		totalBytes = asset.Size
	}

	safeTag := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(tag)
	stagedName := fmt.Sprintf("mila-update-%s-%d.exe", safeTag, time.Now().UnixNano())
	stagedPath := filepath.Join(os.TempDir(), stagedName)

	out, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}

	cleanupOnErr := true
	defer func() {
		_ = out.Close()
		if cleanupOnErr {
			_ = os.Remove(stagedPath)
		}
	}()

	buf := make([]byte, 64*1024)
	var written int64
	lastEmit := time.Now().Add(-time.Second)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return "", writeErr
			}
			written += int64(n)
			if onPct != nil && time.Since(lastEmit) >= 60*time.Millisecond {
				lastEmit = time.Now()
				pct := computeProgress(written, totalBytes)
				onPct(pct)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}

	if err := out.Sync(); err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}

	if written < m.cfg.MinAssetBytes {
		return "", fmt.Errorf("downloaded update too small (%d bytes)", written)
	}
	if totalBytes > 0 && written != totalBytes {
		return "", fmt.Errorf("incomplete download (%d of %d bytes)", written, totalBytes)
	}

	if err := verifyPEExecutableHeader(stagedPath); err != nil {
		return "", err
	}

	if onPct != nil {
		onPct(100)
	}
	cleanupOnErr = false
	return stagedPath, nil
}

func computeProgress(written, total int64) float64 {
	if total > 0 {
		pct := (float64(written) / float64(total)) * 100
		if pct > 100 {
			return 100
		}
		if pct < 0 {
			return 0
		}
		return pct
	}
	// Smooth asymptotic curve up to 90% when server omits Content-Length.
	mb := float64(written) / (1024 * 1024)
	pct := 90 * (mb / (mb + 12))
	return pct
}

func verifyPEExecutableHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return err
	}
	if magic[0] != 'M' || magic[1] != 'Z' {
		return errors.New("downloaded file is not a valid Windows executable")
	}
	return nil
}
