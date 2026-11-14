package media

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFFprobeJSONAndTrackLabels(t *testing.T) {
	sample := []byte(`{
		"format": {"duration": "1420.55"},
		"streams": [
			{"index": 0, "codec_type": "video", "codec_name": "hevc", "width": 1920, "height": 1080},
			{"index": 1, "codec_type": "audio", "codec_name": "eac3", "channels": 2, "tags": {"language": "rus", "title": "Студийная Банда"}},
			{"index": 2, "codec_type": "audio", "codec_name": "flac", "channels": 2, "tags": {"language": "jpn", "title": "Original"}},
			{"index": 3, "codec_type": "subtitle", "codec_name": "ass", "tags": {"language": "rus", "title": "Надписи + Субтитры"}},
			{"index": 4, "codec_type": "subtitle", "codec_name": "hdmv_pgs_subtitle", "tags": {"language": "eng", "title": "PGS"}}
		]
	}`)

	res, err := parseFFprobeJSON(sample)
	if err != nil {
		t.Fatalf("parseFFprobeJSON failed: %v", err)
	}
	if res.Duration != 1420.55 || res.VideoCodec != "hevc" || res.Height != 1080 {
		t.Fatalf("unexpected probe header: %+v", res)
	}
	if len(res.AudioTracks) != 2 {
		t.Fatalf("expected 2 audio tracks, got %+v", res.AudioTracks)
	}
	if res.AudioTracks[0].Label != "RUS · Студийная Банда" || res.AudioTracks[1].Label != "JPN · Original" {
		t.Fatalf("unexpected audio labels: %+v", res.AudioTracks)
	}
	if len(res.Subtitles) != 1 || res.Subtitles[0].Codec != "ass" {
		t.Fatalf("expected only convertible ASS subtitle, got %+v", res.Subtitles)
	}
}

func TestBuildRemuxAndSubtitleArgs(t *testing.T) {
	args := BuildRemuxArgs("http://127.0.0.1:8080/torrent/stream/abc/0", 2, 125.5)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ss 125.500 -i http://127.0.0.1:8080/torrent/stream/abc/0") {
		t.Fatalf("expected seek flag before -i, got %s", joined)
	}
	if !strings.Contains(joined, "-map 0:v:0 -map 0:a:2? -sn -c:v copy -c:a aac") {
		t.Fatalf("expected zero-reencode copy args, got %s", joined)
	}

	subArgs := BuildSubtitleVTTArgs("input.mkv", 1)
	if strings.Join(subArgs, " ") != "-v error -i input.mkv -map 0:s:1 -f webvtt pipe:1" {
		t.Fatalf("unexpected sub args: %v", subArgs)
	}
}

func TestExtractFFmpegBinariesFromZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bundle.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, entry := range []string{"ffmpeg-build/bin/ffmpeg.exe", "ffmpeg-build/bin/ffprobe.exe", "ffmpeg-build/README.txt"} {
		w, _ := zw.Create(entry)
		_, _ = w.Write([]byte("BINARY:" + entry))
	}
	_ = zw.Close()
	_ = f.Close()

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractFFmpegBinariesFromZip(zipPath, outDir); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	for _, exe := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		raw, rErr := os.ReadFile(filepath.Join(outDir, exe))
		if rErr != nil || !strings.Contains(string(raw), exe) {
			t.Fatalf("expected extracted %s, got err=%v", exe, rErr)
		}
	}
}
