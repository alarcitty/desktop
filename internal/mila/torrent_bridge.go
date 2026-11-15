package mila

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mila/internal/media"
	"mila/internal/torrent"
)

func init() {
	torrent.DefaultManager().MountHandler("/media/", http.HandlerFunc(serveMediaRoute))
}

// GetTorrentMediaBase returns the direct TCP loopback HTTP origin for unbuffered remux/media streaming.
func (m *Mila) GetTorrentMediaBase() string {
	base, err := torrent.DefaultManager().MediaBaseURL()
	if err != nil {
		return ""
	}
	return base
}

// TorrentSearch queries Nyaa and AniLibria torrents for a release or query string.
func (m *Mila) TorrentSearch(query string, aniLibriaID int) (string, error) {
	items, err := torrent.SearchTorrents(context.Background(), query, aniLibriaID)
	if err != nil {
		return "[]", err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "[]", err
	}
	return string(raw), nil
}

// TorrentOpen opens a magnet link and returns its mapped episode video files.
func (m *Mila) TorrentOpen(magnet string) (string, error) {
	info, err := torrent.DefaultManager().OpenMagnet(context.Background(), magnet)
	if err != nil {
		return `{"infoHash":"","name":"","files":[]}`, err
	}
	raw, err := json.Marshal(info)
	if err != nil {
		return `{"infoHash":"","name":"","files":[]}`, err
	}
	return string(raw), nil
}

// TorrentProbeFile probes embedded audio/subtitle tracks of a torrent video file via ffprobe.
func (m *Mila) TorrentProbeFile(infoHash string, fileIndex int) (string, error) {
	localURL, err := torrent.DefaultManager().LocalStreamURL(infoHash, fileIndex)
	if err != nil {
		return `{"duration":0,"audioTracks":[],"subtitles":[]}`, err
	}
	res, err := media.ProbeMedia(context.Background(), localURL)
	if err != nil {
		return `{"duration":0,"audioTracks":[],"subtitles":[]}`, err
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return `{"duration":0,"audioTracks":[],"subtitles":[]}`, err
	}
	return string(raw), nil
}

// GetFFmpegStatus reports whether portable ffmpeg/ffprobe are ready or downloading.
func (m *Mila) GetFFmpegStatus() (string, error) {
	raw, err := json.Marshal(media.GetFFmpegStatus())
	if err != nil {
		return `{"available":false,"downloading":false,"progressPct":0,"stage":"error"}`, err
	}
	return string(raw), nil
}

func serveTorrentRoute(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch {
	case r.URL.Path == "/torrent/search":
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		libID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("lib")))
		items, err := torrent.SearchTorrents(r.Context(), q, libID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(items)
	case r.URL.Path == "/torrent/open":
		magnet := strings.TrimSpace(r.URL.Query().Get("magnet"))
		info, err := torrent.DefaultManager().OpenMagnet(r.Context(), magnet)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(info)
	case strings.HasPrefix(r.URL.Path, "/torrent/stream/"):
		torrent.DefaultManager().ServeHTTPStream(w, r)
	default:
		http.NotFound(w, r)
	}
}

func serveMediaRoute(w http.ResponseWriter, r *http.Request) {
	setProxyCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch r.URL.Path {
	case "/media/ffmpeg-status":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(media.GetFFmpegStatus())
	case "/media/probe":
		hash := strings.TrimSpace(r.URL.Query().Get("hash"))
		fileIdx, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("file")))
		if hash == "" || err != nil || fileIdx < 0 {
			http.Error(w, "invalid hash or file", http.StatusBadRequest)
			return
		}
		localURL, err := torrent.DefaultManager().LocalStreamURL(hash, fileIdx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		res, err := media.ProbeMedia(r.Context(), localURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(res)
	case "/media/stream.mp4":
		hash := strings.TrimSpace(r.URL.Query().Get("hash"))
		fileIdx, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("file")))
		if hash == "" || err != nil || fileIdx < 0 {
			http.Error(w, "invalid hash or file", http.StatusBadRequest)
			return
		}
		audioOrd, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("a")))
		startSec, _ := strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get("t")), 64)
		vcodec := strings.TrimSpace(r.URL.Query().Get("vcodec"))
		transcode := strings.TrimSpace(r.URL.Query().Get("transcode")) == "1"
		localURL, err := torrent.DefaultManager().LocalStreamURL(hash, fileIdx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		media.StreamRemuxedMP4WithOptions(w, r, localURL, audioOrd, startSec, media.RemuxOptions{
			VideoCodec: vcodec,
			Transcode:  transcode,
		})
	case "/media/subtitles.vtt":
		hash := strings.TrimSpace(r.URL.Query().Get("hash"))
		fileIdx, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("file")))
		if hash == "" || err != nil || fileIdx < 0 {
			http.Error(w, "invalid hash or file", http.StatusBadRequest)
			return
		}
		subOrd, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("s")))
		localURL, err := torrent.DefaultManager().LocalStreamURL(hash, fileIdx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		vtt, err := media.ExtractSubtitleVTT(r.Context(), localURL, subOrd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(vtt))
	default:
		http.NotFound(w, r)
	}
}
