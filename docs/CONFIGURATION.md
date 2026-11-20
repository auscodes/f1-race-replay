# Replay server configuration

## Hamilton–Russell recordings

Generate the 50-second Virtual Safety Car clip or the full two-driver recording. The importer prints the output path. Pass that path to `go run . -recording`.

```sh
python3 tools/import_barcelona.py
python3 tools/import_barcelona.py --full
```

## Load a recording from S3

For a recording stored in S3, pass its object URI. Configure the bucket's region and AWS credentials with permission to read that object. [AWS SDK configuration](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)

```sh
go run . -recording s3://your-bucket/clips/barcelona-2026.json
```

## Shared playback across servers

Set `REPLAY_REDIS_URL` on each server and use identical recording bytes. Start Redis in one terminal.

```sh
docker run --rm -p 127.0.0.1:6379:6379 redis:7.4.12-alpine
```

Run each Go command in its own terminal.

```sh
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -addr 127.0.0.1:8080
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -addr 127.0.0.1:8081
```

Those commands use the bundled demo. For Barcelona, add `-recording .recordings/barcelona-2026-grid.json` to both, then choose **Join shared playback** in each viewer. To reset the shared clock, run the command below with the same recording selection. It changes the timeline for every connected shared viewer.

```sh
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -reset-shared-clock
```

The default startup uses a process-local clock. The [Redis design notes](learning/redis-clock.md) explain clock ownership and how to verify cross-server playback.

## Interfaces

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Browser viewer |
| `GET /api/session` | Circuit geometry, driver IDs, and clip duration in JSON |
| `GET /api/segments/{index}` | Position frames for local playback and seeking. Full-grid segments cover 20 seconds. |
| `GET /api/timing` | Full-grid timing, tyre stints, and race outcomes, loaded after the first map frame |
| `GET /ws` | WebSocket stream of timestamped position snapshots |
| `GET /readyz` | Readiness after the recording and clock are initialized |
| `GET /metrics` | Prometheus gauge for open replay WebSockets |
