package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mila/internal/aniliberty"
	"mila/internal/anime"
	"mila/internal/logger"
)

type serverOptions struct {
	imageProxyConfig *ImageProxyConfig
}

// Option customizes Server initialization.
type Option func(*serverOptions)

// WithImageProxyConfig overrides the default persistent disk image proxy configuration.
func WithImageProxyConfig(cfg ImageProxyConfig) Option {
	return func(o *serverOptions) {
		copyCfg := cfg
		o.imageProxyConfig = &copyCfg
	}
}

// Server exposes the anime backend over HTTP JSON endpoints.
type Server struct {
	anime  *anime.Service
	images *ImageProxy
	log    *logger.Logger
	mux    *http.ServeMux
}

// New builds an HTTP handler for the anime backend API.
func New(animeSvc *anime.Service, opts ...Option) *Server {
	var options serverOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	log := logger.Make("http")
	imgCfg := ImageProxyConfig{
		QuotaBytes: DefaultImageCacheQuotaBytes,
	}
	if options.imageProxyConfig != nil {
		imgCfg = *options.imageProxyConfig
		if imgCfg.QuotaBytes <= 0 {
			imgCfg.QuotaBytes = DefaultImageCacheQuotaBytes
		}
	}

	imgProxy, err := NewImageProxy(imgCfg)
	if err != nil {
		fallbackCfg := imgCfg
		fallbackCfg.CacheDir = filepath.Join(os.TempDir(), "mila-image-cache")
		if log != nil {
			log.Warn("image proxy init failed in %s (%v); falling back to %s", imgCfg.CacheDir, err, fallbackCfg.CacheDir)
		}
		imgProxy, err = NewImageProxy(fallbackCfg)
		if err != nil && log != nil {
			log.Warn("image proxy disabled: %v", err)
		}
	}

	s := &Server{
		anime:  animeSvc,
		images: imgProxy,
		log:    log,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

// ImageProxy returns the backing disk-cached image proxy, or nil if initialization failed.
func (s *Server) ImageProxy() *ImageProxy {
	return s.images
}

// Handler returns the root HTTP handler wrapped with recovery, CORS, and logging middleware.
func (s *Server) Handler() http.Handler {
	return s.recoverMiddleware(s.corsMiddleware(s.logMiddleware(s.mux)))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /api/v1/image", s.handleImageProxy)
	s.mux.HandleFunc("HEAD /api/v1/image", s.handleImageProxy)

	s.mux.HandleFunc("POST /api/v1/diagnostics/logs", s.handleDiagnosticsExport)
	s.mux.HandleFunc("GET /api/v1/diagnostics/logs.zip", s.handleDiagnosticsZipDownload)

	s.mux.HandleFunc("GET /api/v1/anime/latest", s.handleLatest)
	s.mux.HandleFunc("GET /api/v1/anime/catalog", s.handleCatalog)
	s.mux.HandleFunc("GET /api/v1/anime/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/v1/anime/releases/{id}", s.handleRelease)
	s.mux.HandleFunc("GET /api/v1/anime/releases/{id}/episodes/{ordinal}/master.m3u8", s.handleEpisodeMasterPlaylist)
	s.mux.HandleFunc("GET /api/v1/anime/schedule/now", s.handleScheduleNow)
	s.mux.HandleFunc("GET /api/v1/anime/schedule/week", s.handleScheduleWeek)
	s.mux.HandleFunc("GET /api/v1/anime/genres", s.handleGenres)
	s.mux.HandleFunc("GET /api/v1/anime/references", s.handleReferences)

	s.mux.HandleFunc("GET /api/v1/shikimori/search", s.handleShikimoriSearch)
	s.mux.HandleFunc("GET /api/v1/shikimori/anime/{id}", s.handleShikimoriByID)
}

func (s *Server) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	if s.images == nil {
		writeError(w, http.StatusServiceUnavailable, "image proxy is unavailable")
		return
	}
	s.images.ServeHTTP(w, r)
}

func (s *Server) handleDiagnosticsExport(w http.ResponseWriter, r *http.Request) {
	_ = logger.FlushDefaultSink()
	dest := strings.TrimSpace(r.URL.Query().Get("dir"))
	report, err := logger.ExportDiagnosticsZip(dest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export diagnostic logs")
		return
	}
	writeData(w, http.StatusOK, report)
}

func (s *Server) handleDiagnosticsZipDownload(w http.ResponseWriter, r *http.Request) {
	_ = logger.FlushDefaultSink()
	payload, _, err := logger.ExportDiagnosticsBytes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build diagnostic archive")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="mila-diagnostics.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"time":         time.Now().UTC().Format(time.RFC3339),
		"mode":         "http",
		"apiBase":      "/api/v1",
		"extBase":      "/ext",
		"capabilities": []string{"anime", "shikimori", "image", "diagnostics"},
	})
}

func (s *Server) handleLatest(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 14)
	items, err := s.anime.Latest(r.Context(), limit)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	params := aniliberty.CatalogParams{
		Page:     parseIntQuery(r, "page", 1),
		Limit:    parseIntQuery(r, "limit", 20),
		Search:   strings.TrimSpace(r.URL.Query().Get("search")),
		Sorting:  strings.TrimSpace(r.URL.Query().Get("sorting")),
		Genres:   parseCSVInts(r.URL.Query().Get("genres")),
		Types:    parseCSVStrings(r.URL.Query().Get("types")),
		Seasons:  parseCSVStrings(r.URL.Query().Get("seasons")),
		FromYear: parseIntQuery(r, "from_year", 0),
		ToYear:   parseIntQuery(r, "to_year", 0),
	}
	page, err := s.anime.Catalog(r.Context(), params)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, page)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter 'q'")
		return
	}
	items, err := s.anime.Search(r.Context(), q)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing release id or alias")
		return
	}
	detail, err := s.anime.GetRelease(r.Context(), id)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, detail)
}

func (s *Server) handleEpisodeMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	rawOrd := strings.TrimSpace(r.PathValue("ordinal"))
	if id == "" || rawOrd == "" {
		writeError(w, http.StatusBadRequest, "missing release id or episode ordinal")
		return
	}
	ordinal, err := strconv.ParseFloat(rawOrd, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid episode ordinal")
		return
	}

	detail, err := s.anime.GetRelease(r.Context(), id)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}

	var matched *anime.Episode
	for i := range detail.Episodes {
		if detail.Episodes[i].Ordinal == ordinal {
			matched = &detail.Episodes[i]
			break
		}
	}
	if matched == nil {
		writeError(w, http.StatusNotFound, "episode not found")
		return
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	if matched.HLS1080 != "" {
		b.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=8000000,RESOLUTION=1920x1080,NAME=\"1080p\"\n")
		b.WriteString(matched.HLS1080)
		b.WriteByte('\n')
	}
	if matched.HLS720 != "" {
		b.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=3500000,RESOLUTION=1280x720,NAME=\"720p\"\n")
		b.WriteString(matched.HLS720)
		b.WriteByte('\n')
	}
	if matched.HLS480 != "" {
		b.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=1500000,RESOLUTION=854x480,NAME=\"480p\"\n")
		b.WriteString(matched.HLS480)
		b.WriteByte('\n')
	}
	if b.Len() <= len("#EXTM3U\n#EXT-X-VERSION:3\n") {
		writeError(w, http.StatusNotFound, "no HLS streams available for episode")
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=120")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleScheduleNow(w http.ResponseWriter, r *http.Request) {
	sched, err := s.anime.ScheduleNow(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, sched)
}

func (s *Server) handleScheduleWeek(w http.ResponseWriter, r *http.Request) {
	items, err := s.anime.ScheduleWeek(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request) {
	genres, err := s.anime.Genres(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, genres)
}

func (s *Server) handleReferences(w http.ResponseWriter, r *http.Request) {
	refs, err := s.anime.References(r.Context())
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, refs)
}

func (s *Server) handleShikimoriSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter 'q'")
		return
	}
	limit := parseIntQuery(r, "limit", 10)
	items, err := s.anime.SearchShikimori(r.Context(), q, limit)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) handleShikimoriByID(w http.ResponseWriter, r *http.Request) {
	rawID := strings.TrimSpace(r.PathValue("id"))
	id, err := strconv.Atoi(rawID)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid shikimori anime id")
		return
	}
	item, err := s.anime.GetShikimoriByID(r.Context(), id)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "anime not found on shikimori")
		return
	}
	writeData(w, http.StatusOK, item)
}

func (s *Server) writeUpstreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, aniliberty.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "upstream timeout")
	case errors.Is(err, context.Canceled):
		writeError(w, http.StatusRequestTimeout, "request canceled")
	default:
		s.log.Error("request.upstream_failed", "err", err.Error())
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, If-None-Match")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("http.panic", "path", r.URL.Path, "panic", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

func parseIntQuery(r *http.Request, name string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func parseCSVStrings(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseCSVInts(raw string) []int {
	parts := parseCSVStrings(raw)
	if len(parts) == 0 {
		return nil
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	return out
}
