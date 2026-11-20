package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func testRedisClock(t *testing.T, id string) *redisReplayClock {
	t.Helper()
	redisURL := os.Getenv("REPLAY_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set REPLAY_TEST_REDIS_URL to run Redis integration tests")
	}
	clock, err := newRedisReplayClock(redisURL, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clock.client.Close() })
	return clock
}

func waitForClock(t *testing.T, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-timer.C:
			t.Fatal("timed out waiting for replay clock")
		case <-ticker.C:
		}
	}
}

func watchTestClock(t *testing.T, shared *redisReplayClock, clock *playbackClock, repair time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		shared.watch(ctx, clock, repair)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func TestRedisPlaybackAcrossServers(t *testing.T) {
	id := fmt.Sprintf("test-stream-%d", time.Now().UnixNano())
	a, b := testRedisClock(t, id), testRedisClock(t, id)
	clip, err := loadRecording(demo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	origin, err := a.client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.client.Set(ctx, a.key, origin.Add(-5*time.Second).UnixMilli(), 0).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.client.Del(context.Background(), a.key) })

	firstClock := newPlaybackClock(time.Now())
	if err := a.refresh(ctx, firstClock, "read"); err != nil {
		t.Fatal(err)
	}
	firstServer := httptest.NewServer(replayRoutes(clip, firstClock))
	defer firstServer.Close()

	secondClock := newPlaybackClock(time.Now())
	if err := b.refresh(ctx, secondClock, "read"); err != nil {
		t.Fatal(err)
	}
	secondServer := httptest.NewServer(replayRoutes(clip, secondClock))
	defer secondServer.Close()
	watchTestClock(t, a, firstClock, time.Hour)
	watchTestClock(t, b, secondClock, time.Hour)
	waitForClock(t, func() bool {
		count, err := a.client.PubSubNumSub(ctx, a.topic).Result()
		return err == nil && count[a.topic] == 2
	})

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	connect := func(endpoint string) *websocket.Conn {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(endpoint, "http")+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.CloseNow() })
		return conn
	}
	read := func(conn *websocket.Conn) frame {
		var result frame
		if err := wsjson.Read(ctx, conn, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	viewerA, viewerB := connect(firstServer.URL), connect(secondServer.URL)
	left, right := read(viewerA), read(viewerB)
	if left.Sequence < 25 || right.Sequence < 25 || max(left.Sequence, right.Sequence)-min(left.Sequence, right.Sequence) > 1 {
		t.Fatalf("viewers did not join the same ongoing replay: %d and %d", left.Sequence, right.Sequence)
	}

	// Use a third client as an operator. A one-hour repair interval ensures
	// the existing connections only see this update via the subscription.
	admin := testRedisClock(t, id)
	if err := admin.refresh(ctx, newPlaybackClock(time.Now()), "reset"); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []*websocket.Conn{viewerA, viewerB} {
		for {
			if current := read(viewer); current.Sequence < 10 {
				break
			}
		}
	}
}

func TestRedisClockRepairsMissedChange(t *testing.T) {
	shared := testRedisClock(t, fmt.Sprintf("test-repair-%d", time.Now().UnixNano()))
	clock := newPlaybackClock(time.Now())
	if err := shared.refresh(t.Context(), clock, "read"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.client.Del(context.Background(), shared.key) })
	original := clock.startedAt()
	watchTestClock(t, shared, clock, 25*time.Millisecond)
	waitForClock(t, func() bool {
		count, err := shared.client.PubSubNumSub(t.Context(), shared.topic).Result()
		return err == nil && count[shared.topic] == 1
	})

	// Change the authoritative value without publishing, as if the watcher
	// had missed an update while disconnected.
	if err := shared.client.Set(t.Context(), shared.key, original.Add(-10*time.Second).UnixMilli(), 0).Err(); err != nil {
		t.Fatal(err)
	}
	waitForClock(t, func() bool { return original.Sub(clock.startedAt()) > 9*time.Second })
}
