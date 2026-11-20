# Redis shared playback clock

New to Redis or WebSockets? Start with [how a race frame gets to the browser](STACK_GUIDE.md#how-a-race-frame-gets-to-the-browser).

## Clock ownership

Two Go processes can load the same recording and still show different frames if they start at different times. Redis gives replicas a common start time. The key is scoped to a recording hash, so replicas only share a clock when they agree on what to play. Each server reads the Redis start time and the current Redis time together, then maps that elapsed time onto its own monotonic clock. That gives it a cheap local calculation for every frame.

The Redis key holds the authoritative epoch. Pub/Sub tells running replicas when it changes, such as after an operator resets the replay. On a notification, a replica reads the key again. Since Pub/Sub delivers messages only to connected subscribers, the server also rechecks the key periodically to repair a missed update. Redis carries clock changes. Go still streams the actual telemetry frames over WebSockets.

## Data and control flow

```mermaid
flowchart LR
    File["Same prepared recording bytes"] --A["Go replica A"]
    File --B["Go replica B"]
    A <-->|"Read epoch + Redis time"| R["Redis key · authoritative start time"]
    B <-->|"Read epoch + Redis time"| R
    Reset["Operator reset command"] -->|"Update key and publish atomically"| R
    R --P["Pub/Sub · clock changed"]
    P --A
    P --B
    A -->|"WebSocket frames"| X["Viewers of A"]
    B -->|"WebSocket frames"| Y["Viewers of B"]
```

| Decision | Reason and trade-off |
| --- | --- |
| **Atomic initialization and reset** | A Redis Lua script initializes a missing epoch or replaces it during an explicit reset. The script also publishes the notification. Concurrent starting replicas read the same value, and a subscriber can read the updated key after receiving the notification. |
| **Redis time as a reference** | Sampling Redis time with the epoch avoids depending on the Go hosts having identical wall clocks at startup. The midpoint of the round trip estimates when Redis took the sample. Network asymmetry and clock drift leave some residual skew. |
| **Monotonic local clock per replica** | Each connection reads the same local start time through an atomic pointer. A normal frame needs a timer and an array lookup. The clock is re-anchored only when the Redis epoch changes. |
| **Key plus Pub/Sub** | The key gives late joiners and new replicas a source of truth. Notifications let already-running replicas react promptly. A two-second reconciliation interval bounds how long a missed notification normally takes to catch up after Redis is available. |
| **Keep the last clock during a Redis outage** | Running replicas can continue playing from the last known epoch. A configured replica that cannot read Redis at startup fails startup. If Redis loses the epoch key, the next successful read creates a new timeline. Redis persistence determines whether the epoch survives a Redis restart. |

[Implementation](../replay_clock.go) · [WebSocket loop](../stream.go) · [Redis delivery semantics](https://redis.io/docs/latest/develop/pubsub/#delivery-semantics)

## Verify locally

With a Redis instance running, start two Go servers with the same `REPLAY_REDIS_URL` and the same file or bundled demo. Start the second a few seconds later and open shared playback on both. A reset through `-reset-shared-clock` should move both existing connections back to the beginning.

The Redis integration tests start two HTTP servers with distinct client connections. They compare WebSocket frame indices on late join, use an operator reset to prove Pub/Sub propagation to existing connections, and simulate a lost notification to verify reconciliation. Run the same tests with:

```sh
REPLAY_TEST_REDIS_URL=redis://127.0.0.1:6379/0 go test -race ./...
```

[Run instructions](../README.md#shared-playback-across-servers) · [Redis integration tests](../replay_clock_test.go) · [All milestones](ROADMAP.md)
