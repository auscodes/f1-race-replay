package main

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/prometheus/client_golang/prometheus"
)

type frameStream struct {
	clip        recording
	clock       *playbackClock
	connections prometheus.Gauge
}

func (s frameStream) serve(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, nil)
	if err != nil {
		return // Accept sends an HTTP response when the handshake fails.
	}
	s.connections.Inc()
	defer s.connections.Dec()
	defer conn.CloseNow()
	ctx := conn.CloseRead(req.Context())

	timer := time.NewTimer(0) // Send the current frame as soon as a viewer joins.
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		startedAt := s.clock.startedAt()
		current := s.clip.frameAt(max(0, time.Since(startedAt)))
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := wsjson.Write(writeCtx, conn, current)
		cancel()
		if err != nil {
			return
		}

		// Align each connection with the shared clock. If a write falls behind,
		// the next send picks the current frame without queuing older ones.
		next := startedAt.Add(time.Duration((current.Sequence+1)*s.clip.IntervalMS) * time.Millisecond)
		timer.Reset(time.Until(next))
	}
}
