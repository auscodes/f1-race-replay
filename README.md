# F1 Race Replay

F1 Race Replay is a Go project for watching recorded car positions on a track. Viewers share a playback clock, so a viewer who joins halfway through sees the race in progress.

The first milestone is a local replay using a short, synthetic recording. It establishes the flow from recorded positions through a WebSocket to the browser. Historical telemetry, shared playback across servers, and a load test follow once that flow is working.
