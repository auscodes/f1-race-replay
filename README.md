# F1 Race Replay

Watch two cars move around a fictional circuit using recorded positions. Everyone connected to the Go server shares the same playback clock, including viewers who join halfway through the clip.

## Run locally

From the repository root, with Go 1.26 or later installed, run `go run .` and open [localhost:8080](http://127.0.0.1:8080). The recording loops every 12 seconds. To use another port, run `go run . -addr 127.0.0.1:8090`.

## Follow one frame

1. [`testdata/demo.json`](testdata/demo.json) stores the track and one list of car positions every 200 milliseconds. Each position contains a driver ID and `x` and `y` map coordinates.
2. [`replay.go`](replay.go) turns time since the replay started into a frame sequence. At 2.4 seconds, the sequence is 12. At 12.0 seconds, the sequence reaches 60 and the recording wraps back to the first frame.
3. [`stream.go`](stream.go) sends the frame over `/ws`. Every message contains the current positions of both cars, so the first message is enough to draw the current state. The same server clock is used for every connection.
4. [`web/viewer.js`](web/viewer.js) reads each message and draws the cars using their coordinates. It gets the static circuit outline once from `/api/session`.

## Try it

1. Open **Inspect a frame** to freeze the map and find `positions` for car `01`. A larger `x` moves the marker right and a larger `y` moves it down. Use **Capture latest frame** to take another snapshot, or close the inspector to return to shared playback.
2. While the clip is playing, open a second tab. Predict its first frame before you look. Both tabs should follow the same replay, even though they opened at different times.
3. Close the Go server while a tab is open. The browser should report the lost connection. Start the server again and watch the tab reconnect to the new playback session.

An interview question to practice is “What happens if a viewer misses an update?” Each update contains a full snapshot of the current positions. The next frame gives the browser enough information to draw the cars again, including after a reconnect.

## Development

```sh
go vet ./...
go test -race ./...
```

The tests cover frame selection at interval boundaries, looping, the synthetic fixture, and two WebSocket clients joining a replay in progress. GitHub Actions also checks Go formatting.

This milestone runs one Go process with a synthetic recording. The next milestone imports a historical clip and loads the prepared recording from S3.
