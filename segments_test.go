package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionedSegmentCompression(t *testing.T) {
	clip := recording{id: "abc", Session: session{SegmentSize: 2}, Frames: [][]position{
		{{Driver: "44", point: point{X: 1}}},
	}}
	handler := replayRoutes(clip, newPlaybackClock(time.Now()))
	get := func(path, encoding string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept-Encoding", encoding)
		handler.ServeHTTP(response, request)
		return response
	}
	var metadata session
	if err := json.Unmarshal(get("/api/session", "").Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RecordingVersion != "v1-abc" {
		t.Fatalf("unexpected recording version: %q", metadata.RecordingVersion)
	}
	path := "/api/recordings/" + metadata.RecordingVersion + "/segments/0"
	plain := get(path, "gzip;q=0")
	zipped := get(path, "gzip")
	if plain.Code != 200 || zipped.Code != 200 || plain.Header().Get("Content-Encoding") != "" ||
		zipped.Header().Get("Content-Encoding") != "gzip" || zipped.Header().Get("Vary") != "Accept-Encoding" ||
		zipped.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected plain or gzip response: %d %v, %d %v", plain.Code, plain.Header(), zipped.Code, zipped.Header())
	}
	reader, err := gzip.NewReader(zipped.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != plain.Body.String() {
		t.Fatalf("decompressed segment differs from plain response: %v", err)
	}
	stale := get("/api/recordings/v1-stale/segments/0", "")
	if stale.Code != http.StatusNotFound || stale.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response to stale version: %d %v", stale.Code, stale.Header())
	}
}

func TestRecordingSegments(t *testing.T) {
	clip := recording{Session: session{SegmentSize: 2}, Frames: [][]position{
		{{Driver: "44", point: point{X: 1}}}, {}, {{Driver: "63", point: point{X: 2}}},
	}}
	handler := replayRoutes(clip, newPlaybackClock(time.Now()))
	for _, tc := range []struct {
		index string
		code  int
		start int64
		count int
	}{
		{"0", 200, 0, 2}, {"1", 200, 2, 1}, {"2", 404, 0, 0},
		{"-1", 400, 0, 0}, {"oops", 400, 0, 0}, {"9223372036854775807", 404, 0, 0},
	} {
		t.Run(tc.index, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/segments/"+tc.index, nil))
			if response.Code != tc.code {
				t.Fatalf("status = %d, want %d", response.Code, tc.code)
			}
			if tc.code == http.StatusOK {
				var result struct {
					StartFrame int64
					Frames     [][]position
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.StartFrame != tc.start || len(result.Frames) != tc.count ||
					result.Frames[0][0] != clip.Frames[tc.start][0] {
					t.Fatalf("unexpected segment: %+v", result)
				}
			}
		})
	}
}
