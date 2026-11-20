package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
