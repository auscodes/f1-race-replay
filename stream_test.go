package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestJoiningStreamUsesCurrentFrame(t *testing.T) {
	// Long frames make the expected first message stable without any sleeps.
	clip := recording{
		IntervalMS: time.Hour.Milliseconds(),
		Frames: [][]position{
			{{Driver: "01", point: point{X: 100, Y: 120}}},
			{{Driver: "01", point: point{X: 300, Y: 120}}},
		},
	}
	server := httptest.NewServer(replayRoutes(clip, newPlaybackClock(time.Now().Add(-90*time.Minute))))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	for i := range 2 {
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.CloseNow()
		var got frame
		if err := wsjson.Read(ctx, conn, &got); err != nil {
			t.Fatal(err)
		}
		if got.Sequence != 1 || got.ElapsedMS != clip.IntervalMS || len(got.Positions) != 1 || got.Positions[0].X != 300 {
			t.Fatalf("viewer %d joined on the wrong frame: %+v", i+1, got)
		}
	}
}

func TestStreamAdvancesPastInitialFrame(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(replayRoutes(clip, newPlaybackClock(time.Now())))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var initial, next frame
	if err := wsjson.Read(ctx, conn, &initial); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, conn, &next); err != nil {
		t.Fatal(err)
	}
	// The stream can skip overdue frames under load. Any newer snapshot is valid.
	if next.Sequence <= initial.Sequence {
		t.Fatalf("next sequence = %d, want a sequence newer than %d", next.Sequence, initial.Sequence)
	}
	index := next.Sequence % int64(len(clip.Frames))
	if next.ElapsedMS != index*clip.IntervalMS || !slices.Equal(next.Positions, clip.Frames[index]) {
		t.Fatalf("next frame = %+v, want the full snapshot at index %d", next, index)
	}
}

func TestConnectionMetricTracksOpenSockets(t *testing.T) {
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(replayRoutes(clip, newPlaybackClock(time.Now())))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	reported := func(expected string) bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/metrics", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(string(body), "\nf1_replay_websocket_connections "+expected+"\n")
	}
	if !reported("0") {
		t.Fatal("connection gauge should start at zero")
	}
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var got frame
	if err := wsjson.Read(ctx, conn, &got); err != nil {
		t.Fatal(err)
	}
	if !reported("1") {
		t.Fatal("connection gauge should include an established stream")
	}
	if err := conn.CloseNow(); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !reported("0") {
		select {
		case <-ctx.Done():
			t.Fatal("gauge did not return to zero after disconnect")
		case <-ticker.C:
		}
	}
}
