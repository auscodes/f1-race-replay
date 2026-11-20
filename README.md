# F1 Race Replay

F1 Race Replay renders recorded car positions on a circuit map. Viewers connected to the same server share a playback timeline and can join a replay in progress. The bundled demo uses a fictional circuit and a 12-second synthetic recording.

## Quick start

Requires Go 1.26 or newer. Start the server from the repository root.

```sh
go run .
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). The replay starts automatically and loops. Use **Inspect a frame** to freeze the map and view the corresponding position data. Close the inspector to resume playback.

The server listens on `127.0.0.1:8080` by default. Pass `-addr` to use a different local port.

```sh
go run . -addr 127.0.0.1:8090
```

## Interfaces

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Browser viewer |
| `GET /api/session` | Circuit geometry, driver IDs, and clip duration in JSON |
| `GET /ws` | WebSocket stream of timestamped position snapshots |

## Development

```sh
go vet ./...
go test -race ./...
```

GitHub Actions also checks Go formatting. See the [project guide](PROJECT_GUIDE.md) for the architecture and a tour of the code.
