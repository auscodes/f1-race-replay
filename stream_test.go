package main

import (
	"context"
	"net/http/httptest"
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
	server := httptest.NewServer(replayRoutes(clip, time.Now().Add(-90*time.Minute)))
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
