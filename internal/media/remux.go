package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MediaAudioTrack describes an audio stream inside an MKV/MP4 container.
type MediaAudioTrack struct {
	Ordinal     int    `json:"ordinal"`
	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec"`
	Language    string `json:"language,omitempty"`
	Title       string `json:"title,omitempty"`
	Label       string `json:"label"`
	Channels    int    `json:"channels,omitempty"`
}

// MediaSubtitleTrack describes a text-convertible subtitle stream inside an MKV container.
type MediaSubtitleTrack struct {
	Ordinal     int    `json:"ordinal"`
	StreamIndex int    `json:"streamIndex"`
	Codec       string `json:"codec"`
	Language    string `json:"language,omitempty"`
	Title       string `json:"title,omitempty"`
	Label       string `json:"label"`
}

// ProbeResult holds normalized metadata, audio tracks, and subtitles for a media file or stream.
type ProbeResult struct {
	Duration    float64              `json:"duration"`
	VideoCodec  string               `json:"videoCodec,omitempty"`
	PixFmt      string               `json:"pixFmt,omitempty"`
	Profile     string               `json:"profile,omitempty"`
	Width       int                  `json:"width,omitempty"`
	Height      int                  `json:"height,omitempty"`
	AudioTracks []MediaAudioTrack    `json:"audioTracks"`
	Subtitles   []MediaSubtitleTrack `json:"subtitles"`
}

var (
	probeCacheMu sync.RWMutex
	probeCache   = make(map[string]*ProbeResult)

	subVTTCacheMu sync.RWMutex
	subVTTCache   = make(map[string]string)
)

// ProbeMedia runs ffprobe on a local path or HTTP range stream URL and caches track metadata.
func ProbeMedia(ctx context.Context, inputURL string) (*ProbeResult, error) {
	probeCacheMu.RLock()
	cached, ok := probeCache[inputURL]
	probeCacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	_, fpPath, err := EnsureFFmpeg(ctx)
	if err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		fpPath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputURL,
	)
	configureHiddenProcess(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	res, err := parseFFprobeJSON(out)
	if err != nil {
		return nil, err
	}

	probeCacheMu.Lock()
	probeCache[inputURL] = res
	probeCacheMu.Unlock()
	return res, nil
}

func parseFFprobeJSON(raw []byte) (*ProbeResult, error) {
	var data struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			CodecName string            `json:"codec_name"`
			PixFmt    string            `json:"pix_fmt"`
			Profile   string            `json:"profile"`
			Width     int               `json:"width"`
			Height    int               `json:"height"`
			Channels  int               `json:"channels"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	dur, _ := strconv.ParseFloat(strings.TrimSpace(data.Format.Duration), 64)
	res := &ProbeResult{
		Duration:    dur,
		AudioTracks: make([]MediaAudioTrack, 0),
		Subtitles:   make([]MediaSubtitleTrack, 0),
	}

	audioOrd := 0
	subOrd := 0
	for _, s := range data.Streams {
		lang := strings.TrimSpace(tagLookup(s.Tags, "language"))
		title := strings.TrimSpace(tagLookup(s.Tags, "title"))
		switch s.CodecType {
		case "video":
			if res.VideoCodec == "" && s.Width > 0 && s.Height > 0 {
				res.VideoCodec = s.CodecName
				res.PixFmt = s.PixFmt
				res.Profile = s.Profile
				res.Width = s.Width
				res.Height = s.Height
			}
		case "audio":
			res.AudioTracks = append(res.AudioTracks, MediaAudioTrack{
				Ordinal:     audioOrd,
				StreamIndex: s.Index,
				Codec:       s.CodecName,
				Language:    lang,
				Title:       title,
				Label:       formatTrackLabel(lang, title, audioOrd, "Audio"),
				Channels:    s.Channels,
			})
			audioOrd++
		case "subtitle":
			codecLower := strings.ToLower(s.CodecName)
			// Only expose text subtitle formats convertible to WebVTT without OCR.
			if isTextSubtitleCodec(codecLower) {
				res.Subtitles = append(res.Subtitles, MediaSubtitleTrack{
					Ordinal:     subOrd,
					StreamIndex: s.Index,
					Codec:       s.CodecName,
					Language:    lang,
					Title:       title,
					Label:       formatTrackLabel(lang, title, subOrd, "Sub"),
				})
			}
			subOrd++
		}
	}

	return res, nil
}

func tagLookup(tags map[string]string, targetKey string) string {
	for k, v := range tags {
		if strings.EqualFold(k, targetKey) {
			return v
		}
	}
	return ""
}

func formatTrackLabel(lang string, title string, ordinal int, fallbackPrefix string) string {
	cleanLang := strings.ToUpper(strings.TrimSpace(lang))
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle != "" && cleanLang != "" && !strings.Contains(strings.ToUpper(cleanTitle), cleanLang) {
		return fmt.Sprintf("%s · %s", cleanLang, cleanTitle)
	}
	if cleanTitle != "" {
		return cleanTitle
	}
	if cleanLang != "" {
		return fmt.Sprintf("%s Track %d", cleanLang, ordinal+1)
	}
	return fmt.Sprintf("%s %d", fallbackPrefix, ordinal+1)
}

func isTextSubtitleCodec(codec string) bool {
	switch codec {
	case "ass", "ssa", "subrip", "srt", "webvtt", "mov_text", "text":
		return true
	default:
		return false
	}
}

// RemuxOptions controls optional codec tagging or software transcode fallback during live fMP4 remuxing.
type RemuxOptions struct {
	VideoCodec string
	Transcode  bool
}

// BuildRemuxArgs builds zero-video-reencode FFmpeg arguments for fragmented MP4 live streaming.
func BuildRemuxArgs(inputURL string, audioOrdinal int, startSec float64) []string {
	return BuildRemuxArgsWithOptions(inputURL, audioOrdinal, startSec, RemuxOptions{})
}

// BuildRemuxArgsWithOptions builds FFmpeg arguments for fragmented MP4 live streaming with optional codec handling.
func BuildRemuxArgsWithOptions(inputURL string, audioOrdinal int, startSec float64, opts RemuxOptions) []string {
	args := []string{
		"-v", "error",
		"-fflags", "+genpts+discardcorrupt",
		"-analyzeduration", "3M",
		"-probesize", "3M",
	}
	if startSec > 0.25 {
		args = append(args, "-ss", strconv.FormatFloat(startSec, 'f', 3, 64))
	}
	args = append(
		args,
		"-i", inputURL,
		"-map", "0:v:0",
		"-map", fmt.Sprintf("0:a:%d?", max(0, audioOrdinal)),
		"-sn",
	)
	if opts.Transcode {
		args = append(
			args,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-pix_fmt", "yuv420p",
			"-crf", "21",
		)
	} else {
		args = append(args, "-c:v", "copy")
		vcodec := strings.ToLower(strings.TrimSpace(opts.VideoCodec))
		if vcodec == "hevc" || vcodec == "h265" {
			args = append(args, "-tag:v", "hvc1")
		}
	}
	args = append(
		args,
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
		"-avoid_negative_ts", "make_zero",
		"-max_muxing_queue_size", "1024",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)
	return args
}

// BuildSubtitleVTTArgs builds FFmpeg arguments extracting a subtitle stream directly as WebVTT.
func BuildSubtitleVTTArgs(inputURL string, subOrdinal int) []string {
	return []string{
		"-v", "error",
		"-i", inputURL,
		"-map", fmt.Sprintf("0:s:%d", max(0, subOrdinal)),
		"-f", "webvtt",
		"pipe:1",
	}
}

// StreamFlushed copies bytes from src to w in 32 KiB chunks, flushing immediately after each write.
func StreamFlushed(w http.ResponseWriter, src io.Reader) error {
	buf := make([]byte, 32*1024)
	flusher, hasFlusher := w.(http.Flusher)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if hasFlusher {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

func setRemuxStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// StreamRemuxedMP4 executes FFmpeg remux and streams fragmented MP4 bytes directly to the HTTP client.
func StreamRemuxedMP4(w http.ResponseWriter, r *http.Request, inputURL string, audioOrdinal int, startSec float64) {
	StreamRemuxedMP4WithOptions(w, r, inputURL, audioOrdinal, startSec, RemuxOptions{})
}

// StreamRemuxedMP4WithOptions executes FFmpeg remux with codec options and flushes fMP4 chunks progressively.
func StreamRemuxedMP4WithOptions(w http.ResponseWriter, r *http.Request, inputURL string, audioOrdinal int, startSec float64, opts RemuxOptions) {
	if r.Method == http.MethodHead {
		setRemuxStreamHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}

	ffPath, _, err := EnsureFFmpeg(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	args := BuildRemuxArgsWithOptions(inputURL, audioOrdinal, startSec, opts)
	cmd := exec.CommandContext(r.Context(), ffPath, args...)
	configureHiddenProcess(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	setRemuxStreamHeaders(w)
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	_ = StreamFlushed(w, stdout)
	_ = cmd.Wait()
}

// ExtractSubtitleVTT converts an embedded subtitle stream to WebVTT and caches it per file/track.
func ExtractSubtitleVTT(ctx context.Context, inputURL string, subOrdinal int) (string, error) {
	cacheKey := fmt.Sprintf("%s#%d", inputURL, subOrdinal)
	subVTTCacheMu.RLock()
	cached, ok := subVTTCache[cacheKey]
	subVTTCacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	ffPath, _, err := EnsureFFmpeg(ctx)
	if err != nil {
		return "", err
	}

	extractCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(extractCtx, ffPath, BuildSubtitleVTTArgs(inputURL, subOrdinal)...)
	configureHiddenProcess(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("subtitle extract failed: %w", err)
	}

	vtt := string(out)
	subVTTCacheMu.Lock()
	subVTTCache[cacheKey] = vtt
	subVTTCacheMu.Unlock()
	return vtt, nil
}
