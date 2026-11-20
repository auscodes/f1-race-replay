package main

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSessionRouteReturnsOnlySessionMetadata(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	replayRoutes(clip, newPlaybackClock(time.Now())).ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("session status = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("session content type = %q, want application/json", got)
	}
	var got session
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode session metadata: %v", err)
	}
	if !reflect.DeepEqual(got, clip.Session) {
		t.Errorf("response metadata differs from the recording's session metadata")
	}
}

func TestWebSocketRouteRequiresUpgrade(t *testing.T) {
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	replayRoutes(recording{}, newPlaybackClock(time.Now())).ServeHTTP(response, req)
	if response.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain GET /ws status = %d, want 426", response.Code)
	}
}

func TestGridTimingLoadsSeparately(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	driver := clip.Session.Drivers[0]
	clip.Session.Grid = map[string]gridDriver{driver: {Code: "ONE"}}
	clip.Session.Timeline = &raceTimeline{
		Events:     []raceEvent{{ElapsedMS: 200, Label: "Chequered flag"}},
		RaceStates: []timedValue{{Value: "green"}, {ElapsedMS: 200, Value: "chequered"}},
		RaceLaps:   []timedValue{{Value: "1"}},
		TotalLaps:  66,
		Drivers: map[string]driverTiming{driver: {
			Laps:      []timedValue{{Value: "1"}},
			Positions: []timedValue{{Value: "1"}},
			Outcome:   &raceOutcome{ElapsedMS: 250, Status: "finished", Position: 1},
		}},
	}
	mux := replayRoutes(clip, newPlaybackClock(time.Now()))

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	var metadata session
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Timeline == nil || !reflect.DeepEqual(metadata.Timeline.Events, clip.Session.Timeline.Events) {
		t.Fatal("session metadata must include race events")
	}
	if metadata.Timeline.TotalLaps != 66 ||
		!reflect.DeepEqual(metadata.Timeline.RaceStates, clip.Session.Timeline.RaceStates) ||
		!reflect.DeepEqual(metadata.Timeline.RaceLaps, clip.Session.Timeline.RaceLaps) {
		t.Fatal("session metadata must include race states, lap starts, and lap count")
	}
	timing := metadata.Timeline.Drivers[driver]
	if len(timing.Laps) != 1 || len(timing.Positions) != 0 || timing.Outcome != nil {
		t.Fatalf("session must include lap jumps and defer detailed timing, got %+v", timing)
	}

	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/timing", nil))
	var timeline raceTimeline
	if err := json.NewDecoder(response.Body).Decode(&timeline); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(timeline, *clip.Session.Timeline) {
		t.Error("timing endpoint differs from the prepared full timeline")
	}

	for _, encoding := range []string{"gzip;q=0", "br, gzip;q=0.5"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/timing", nil)
		request.Header.Set("Accept-Encoding", encoding)
		mux.ServeHTTP(response, request)
		if got := response.Header().Get("Content-Encoding"); encoding == "gzip;q=0" {
			if got != "" {
				t.Fatalf("gzip was rejected, got content encoding %q", got)
			}
		} else {
			if got != "gzip" {
				t.Fatalf("expected gzip timing, got content encoding %q", got)
			}
			reader, err := gzip.NewReader(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var got raceTimeline
			if err := json.NewDecoder(reader).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, timeline) {
				t.Error("compressed and uncompressed timing differ")
			}
		}
	}
}
