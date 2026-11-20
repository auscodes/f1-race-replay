package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// A local monotonic time serves every connection, whether the clock was
// started by this process or initialized from Redis.
type playbackClock struct {
	start atomic.Pointer[time.Time]
}

func newPlaybackClock(start time.Time) *playbackClock {
	clock := &playbackClock{}
	clock.start.Store(&start)
	return clock
}

func (c *playbackClock) startedAt() time.Time {
	return *c.start.Load()
}

const clockRefreshInterval = 2 * time.Second

// Initialize a missing clock and publish changes in the same Redis operation.
// TIME gives replicas a common reference regardless of their wall clock.
var redisClockScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local start = redis.call('GET', KEYS[1])
if not start or ARGV[1] == 'reset' then
    start = string.format('%.0f', now)
    redis.call('SET', KEYS[1], start)
    redis.call('PUBLISH', KEYS[2], start)
end
return {tonumber(start), now}
`)

type redisReplayClock struct {
	client *redis.Client
	key    string
	topic  string
	epoch  int64
}

func newRedisReplayClock(redisURL, recordingID string) (*redisReplayClock, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse replay Redis URL: %w", err)
	}
	options.ContextTimeoutEnabled = true
	key := "f1-replay:" + recordingID + ":clock"
	topic := fmt.Sprintf("%s:db%d:changed", key, options.DB)
	return &redisReplayClock{client: redis.NewClient(options), key: key, topic: topic}, nil
}

// refresh is called by startup and then by a single watcher. Each new Redis
// epoch is converted to local monotonic time at the midpoint of the request.
func (c *redisReplayClock) refresh(ctx context.Context, clock *playbackClock, operation string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	sent := time.Now()
	values, err := redisClockScript.Run(ctx, c.client, []string{c.key, c.topic}, operation).Slice()
	received := time.Now()
	if err != nil {
		return fmt.Errorf("read shared replay clock: %w", err)
	}
	if len(values) != 2 {
		return fmt.Errorf("invalid shared replay clock response")
	}
	epoch, epochOK := values[0].(int64)
	now, nowOK := values[1].(int64)
	if !epochOK || !nowOK || epoch <= 0 || now < epoch || now-epoch > int64((1<<63-1)/time.Millisecond) {
		return fmt.Errorf("invalid shared replay clock timestamps")
	}
	if epoch != c.epoch {
		midpoint := sent.Add(received.Sub(sent) / 2)
		start := midpoint.Add(-time.Duration(now-epoch) * time.Millisecond)
		clock.start.Store(&start)
		c.epoch = epoch
	}
	return nil
}

func (c *redisReplayClock) watch(ctx context.Context, clock *playbackClock, repairInterval time.Duration) {
	subscription := c.client.Subscribe(ctx, c.topic)
	defer subscription.Close()
	changes := subscription.ChannelWithSubscriptions()
	ticker := time.NewTicker(repairInterval)
	defer ticker.Stop()

	failed := false
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-changes:
			if !ok {
				changes = nil
			}
		case <-ticker.C:
			// Recover changes missed during a Pub/Sub disconnect.
		}

		err := c.refresh(ctx, clock, "read")
		if err != nil && !failed && ctx.Err() == nil {
			log.Printf("replay clock: keeping last Redis epoch: %v", err)
		}
		if err == nil && failed {
			log.Print("replay clock: Redis synchronization restored")
		}
		failed = err != nil
	}
}
