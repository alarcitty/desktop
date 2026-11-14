package torrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bt "github.com/anacrolix/torrent"
)

const (
	maxCacheBytes = int64(6) << 30 // 6 GiB automatic disk cache budget
)

// TorrentFileItem represents a playable video file mapped to a sequence index inside a torrent.
type TorrentFileItem struct {
	FileIndex   int    `json:"fileIndex"`
	Ordinal     int    `json:"ordinal"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Length      int64  `json:"length"`
	NeedsRemux  bool   `json:"needsRemux"`
	ContentType string `json:"contentType"`
}

// TorrentReleaseInfo describes an opened magnet/torrent release and its sorted episode files.
type TorrentReleaseInfo struct {
	InfoHash string            `json:"infoHash"`
	Name     string            `json:"name"`
	Files    []TorrentFileItem `json:"files"`
}

// Manager coordinates anacrolix/torrent client lifecycle, sequential readers, and local HTTP range streaming.
type Manager struct {
	mu           sync.Mutex
	client       *bt.Client
	torrents     map[string]*bt.Torrent
	dataDir      string
	loopbackAddr string
	loopbackSrv  *http.Server
}

var (
	globalMgrMu sync.Mutex
	globalMgr   *Manager

	reNoiseTags    = regexp.MustCompile(`(?i)\[[^\]]*\]|\([^\)]*\)|(?:1080|720|480|2160)[pi]|(?:x|h)\.?26[45]|hevc|avc|10bit|aac|flac|opus|ddp?5\.1|web-?dl|webrip|bdrip|blu-?ray`)
	reExtraSkip    = regexp.MustCompile(`(?i)(?:^|[\s._/-])(ncop|nced|op\d*|ed\d*|pv\d*|cm\d*|menu|creditless|trailer|preview|bonus|ova\s*op)(?:$|[\s._/-])`)
	reSeasonEp     = regexp.MustCompile(`(?i)s\d{1,2}\s*[eex]\s*(\d{1,4})`)
	reDashEp       = regexp.MustCompile(`(?i)(?:^|\s[-–—]\s*|ep\.?\s*|episode\s*|серия\s*|e)(\d{1,4})(?:v\d+)?(?:\s|$|\.)`)
	reTrailingNum  = regexp.MustCompile(`(\d{1,4})(?:v\d+)?$`)
)

// DefaultManager returns a shared singleton torrent Manager.
func DefaultManager() *Manager {
	globalMgrMu.Lock()
	defer globalMgrMu.Unlock()
	if globalMgr == nil {
		globalMgr = &Manager{
			torrents: make(map[string]*bt.Torrent),
		}
	}
	return globalMgr
}

func (m *Manager) ensureInitialized() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		return nil
	}

	dir, err := torrentCacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pruneCacheDir(dir, maxCacheBytes)

	cfg := bt.NewDefaultClientConfig()
	cfg.DataDir = dir
	cfg.NoUpload = false
	cfg.Seed = false
	cfg.DisableUTP = false
	cfg.EstablishedConnsPerTorrent = 55
	cfg.HalfOpenConnsPerTorrent = 25

	cl, err := bt.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to start torrent engine: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cl.Close()
		return fmt.Errorf("failed to bind local torrent stream port: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/torrent/stream/", m.ServeHTTPStream)
	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(ln)
	}()

	m.dataDir = dir
	m.client = cl
	m.loopbackAddr = "http://" + ln.Addr().String()
	m.loopbackSrv = srv
	return nil
}

// LocalStreamURL returns the fast loopback HTTP Range URL that ffmpeg/ffprobe read from sequentially.
func (m *Manager) LocalStreamURL(infoHash string, fileIndex int) (string, error) {
	if err := m.ensureInitialized(); err != nil {
		return "", err
	}
	m.mu.Lock()
	base := m.loopbackAddr
	m.mu.Unlock()
	return fmt.Sprintf("%s/torrent/stream/%s/%d", base, strings.ToLower(strings.TrimSpace(infoHash)), fileIndex), nil
}

// OpenMagnet adds a magnet link, waits for torrent metadata, and maps sequential video episodes.
func (m *Manager) OpenMagnet(ctx context.Context, magnet string) (*TorrentReleaseInfo, error) {
	if strings.TrimSpace(magnet) == "" {
		return nil, errors.New("empty magnet URI")
	}
	if err := m.ensureInitialized(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	cl := m.client
	m.mu.Unlock()

	t, err := cl.AddMagnet(magnet)
	if err != nil {
		return nil, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	select {
	case <-t.GotInfo():
	case <-waitCtx.Done():
		return nil, errors.New("timed out fetching torrent metadata from DHT/trackers")
	}

	hashHex := strings.ToLower(t.InfoHash().HexString())
	m.mu.Lock()
	m.torrents[hashHex] = t
	m.mu.Unlock()

	files := MapTorrentVideoFiles(t.Files())
	if len(files) == 0 {
		return nil, errors.New("torrent contains no supported video files (.mkv/.mp4)")
	}

	return &TorrentReleaseInfo{
		InfoHash: hashHex,
		Name:     t.Name(),
		Files:    files,
	}, nil
}

// MapTorrentVideoFiles filters video files from a torrent and assigns sequential episode ordinals.
func MapTorrentVideoFiles(rawFiles []*bt.File) []TorrentFileItem {
	items := make([]TorrentFileItem, 0, len(rawFiles))
	for idx, f := range rawFiles {
		if f == nil {
			continue
		}
		path := f.DisplayPath()
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".mkv" && ext != ".mp4" && ext != ".webm" {
			continue
		}
		if reExtraSkip.MatchString(path) {
			continue
		}
		base := filepath.Base(path)
		ct := "video/x-matroska"
		if ext == ".mp4" {
			ct = "video/mp4"
		} else if ext == ".webm" {
			ct = "video/webm"
		}
		items = append(items, TorrentFileItem{
			FileIndex:   idx,
			Ordinal:     ParseEpisodeOrdinal(base),
			Name:        base,
			Path:        path,
			Length:      f.Length(),
			NeedsRemux:  ext != ".mp4",
			ContentType: ct,
		})
	}

	// Fallback if all files matched extra filter (e.g. an OVA or movie literally named "Special").
	if len(items) == 0 {
		for idx, f := range rawFiles {
			if f == nil {
				continue
			}
			path := f.DisplayPath()
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".mkv" && ext != ".mp4" && ext != ".webm" {
				continue
			}
			ct := "video/x-matroska"
			if ext == ".mp4" {
				ct = "video/mp4"
			}
			items = append(items, TorrentFileItem{
				FileIndex:   idx,
				Ordinal:     len(items) + 1,
				Name:        filepath.Base(path),
				Path:        path,
				Length:      f.Length(),
				NeedsRemux:  ext != ".mp4",
				ContentType: ct,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Ordinal > 0 && items[j].Ordinal > 0 && items[i].Ordinal != items[j].Ordinal {
			return items[i].Ordinal < items[j].Ordinal
		}
		return items[i].Name < items[j].Name
	})

	for i := range items {
		if items[i].Ordinal <= 0 {
			items[i].Ordinal = i + 1
		}
	}
	return items
}

// ParseEpisodeOrdinal extracts a positive episode number from typical anime torrent filenames.
func ParseEpisodeOrdinal(filename string) int {
	noExt := strings.TrimSuffix(filename, filepath.Ext(filename))
	if m := reSeasonEp.FindStringSubmatch(noExt); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	cleaned := strings.TrimSpace(reNoiseTags.ReplaceAllString(noExt, " "))
	cleaned = strings.Join(strings.Fields(strings.ReplaceAll(cleaned, "_", " ")), " ")
	if m := reDashEp.FindAllStringSubmatch(cleaned, -1); len(m) > 0 {
		last := m[len(m)-1]
		if len(last) > 1 {
			if n, err := strconv.Atoi(last[1]); err == nil && n > 0 && n < 2000 {
				return n
			}
		}
	}
	if m := reTrailingNum.FindStringSubmatch(cleaned); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 2000 {
			return n
		}
	}
	return 0
}

// ServeHTTPStream streams any torrent file sequentially with HTTP Range support.
func (m *Manager) ServeHTTPStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/torrent/stream/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid torrent stream path", http.StatusBadRequest)
		return
	}
	hash := strings.ToLower(strings.TrimSpace(parts[0]))
	fileIdx, err := strconv.Atoi(parts[1])
	if err != nil || fileIdx < 0 {
		http.Error(w, "invalid file index", http.StatusBadRequest)
		return
	}

	t := m.getTorrent(hash)
	if t == nil {
		http.Error(w, "torrent session not active", http.StatusNotFound)
		return
	}
	files := t.Files()
	if fileIdx >= len(files) {
		http.Error(w, "file index out of range", http.StatusNotFound)
		return
	}
	file := files[fileIdx]

	reader := file.NewReader()
	defer reader.Close()
	reader.SetResponsive()
	reader.SetReadahead(16 << 20)

	// Prioritize this file ahead of remaining batch episodes for smooth streaming.
	file.SetPriority(bt.PiecePriorityNormal)

	ext := strings.ToLower(filepath.Ext(file.DisplayPath()))
	switch ext {
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".webm":
		w.Header().Set("Content-Type", "video/webm")
	default:
		w.Header().Set("Content-Type", "video/x-matroska")
	}

	http.ServeContent(w, r, filepath.Base(file.DisplayPath()), time.Time{}, &contextReadSeeker{
		ctx: r.Context(),
		rs:  reader,
	})
}

// Close shuts down the torrent client and loopback HTTP server gracefully.
func (m *Manager) Close() {
	m.mu.Lock()
	srv := m.loopbackSrv
	cl := m.client
	m.loopbackSrv = nil
	m.client = nil
	m.mu.Unlock()

	if srv != nil {
		_ = srv.Close()
	}
	if cl != nil {
		_ = cl.Close()
	}
}

func (m *Manager) getTorrent(hash string) *bt.Torrent {
	m.mu.Lock()
	t := m.torrents[hash]
	cl := m.client
	m.mu.Unlock()
	if t != nil || cl == nil {
		return t
	}
	for _, item := range cl.Torrents() {
		if strings.EqualFold(item.InfoHash().HexString(), hash) {
			m.mu.Lock()
			m.torrents[hash] = item
			m.mu.Unlock()
			return item
		}
	}
	return nil
}

type contextReadSeeker struct {
	ctx context.Context
	rs  bt.Reader
}

func (c *contextReadSeeker) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, io.EOF
	}
	return c.rs.Read(p)
}

func (c *contextReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return c.rs.Seek(offset, whence)
}

func torrentCacheDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(cfgDir) == "" {
		cfgDir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cfgDir = filepath.Join(cfgDir, ".config")
	}
	return filepath.Join(cfgDir, "mila", "torrents"), nil
}

func pruneCacheDir(root string, budgetBytes int64) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type dirStat struct {
		path    string
		size    int64
		modTime time.Time
	}
	var stats []dirStat
	var total int64
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		full := filepath.Join(root, e.Name())
		sz := entrySize(full)
		total += sz
		stats = append(stats, dirStat{path: full, size: sz, modTime: info.ModTime()})
	}
	if total <= budgetBytes {
		return
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].modTime.Before(stats[j].modTime)
	})
	for _, st := range stats {
		if total <= budgetBytes {
			break
		}
		if rmErr := os.RemoveAll(st.path); rmErr == nil {
			total -= st.size
		}
	}
}

func entrySize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
