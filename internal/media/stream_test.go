package media

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestBuildRemuxArgsWithOptions(t *testing.T) {
	hevcArgs := BuildRemuxArgsWithOptions("http://127.0.0.1:9999/torrent/stream/abc/0", 1, 12.5, RemuxOptions{
		VideoCodec: "hevc",
	})
	for _, required := range []string{"-fflags", "+genpts+discardcorrupt", "-analyzeduration", "3M", "-probesize", "3M", "-ss", "12.500", "-avoid_negative_ts", "make_zero", "-max_muxing_queue_size", "1024", "-tag:v", "hvc1"} {
		if !slices.Contains(hevcArgs, required) {
			t.Fatalf("expected %q in HEVC remux args: %v", required, hevcArgs)
		}
	}

	transcodeArgs := BuildRemuxArgsWithOptions("http://127.0.0.1:9999/torrent/stream/abc/0", 0, 0, RemuxOptions{
		VideoCodec: "h264",
		Transcode:  true,
	})
	for _, required := range []string{"libx264", "veryfast", "zerolatency", "yuv420p"} {
		if !slices.Contains(transcodeArgs, required) {
			t.Fatalf("expected %q in transcode remux args: %v", required, transcodeArgs)
		}
	}
	if slices.Contains(transcodeArgs, "-ss") {
		t.Fatalf("did not expect -ss for startSec=0: %v", transcodeArgs)
	}
}

func TestStreamFlushedSendsBytesBeforeEOF(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setRemuxStreamHeaders(w)
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		_ = StreamFlushed(w, pr)
	}))
	defer srv.Close()

	reqDone := make(chan error, 1)
	go func() {
		_, err := pw.Write([]byte("ftypisom-first-chunk"))
		if err != nil {
			reqDone <- err
			return
		}
		// Keep pipe open until reader verifies progressive chunk delivery.
		time.Sleep(120 * time.Millisecond)
		_ = pw.Close()
		reqDone <- nil
	}()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "none" {
		t.Fatalf("Accept-Ranges = %q, want %q", got, "none")
	}

	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("first Read failed before EOF: %v", err)
	}
	if string(buf[:n]) != "ftypisom-first-chunk" {
		t.Fatalf("unexpected chunk %q", string(buf[:n]))
	}
	if err := <-reqDone; err != nil {
		t.Fatalf("writer error: %v", err)
	}
}

func TestStreamRemuxedMP4HeadDoesNotRunFFmpeg(t *testing.T) {
	req := httptest.NewRequest(http.MethodHead, "/media/stream.mp4?hash=abc&file=0", nil)
	rec := httptest.NewRecorder()
	StreamRemuxedMP4WithOptions(rec, req, "http://127.0.0.1:1/nonexistent.mkv", 0, 0, RemuxOptions{})

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q, want video/mp4", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "none" {
		t.Fatalf("Accept-Ranges = %q, want none", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD body len = %d, want 0", rec.Body.Len())
	}
}

func TestParseFFprobeJSONCapturesPixFmtAndProfile(t *testing.T) {
	raw := []byte(`{
		"format": {"duration": "1425.024000"},
		"streams": [
			{"index": 0, "codec_type": "video", "codec_name": "h264", "pix_fmt": "yuv420p10le", "profile": "High 10", "width": 1920, "height": 1080},
			{"index": 1, "codec_type": "audio", "codec_name": "aac", "channels": 2, "tags": {"language": "jpn"}}
		]
	}`)
	res, err := parseFFprobeJSON(raw)
	if err != nil {
		t.Fatalf("parseFFprobeJSON failed: %v", err)
	}
	if res.PixFmt != "yuv420p10le" || res.Profile != "High 10" {
		t.Fatalf("expected PixFmt=yuv420p10le and Profile=High 10, got %+v", res)
	}
}
