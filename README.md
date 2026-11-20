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

## Historical replay

The Barcelona clip follows Lewis Hamilton and George Russell around Hamilton's final stop under the Virtual Safety Car at the 2026 Barcelona-Catalunya Grand Prix. Generate it with Python 3.9 or newer and access to the [OpenF1 archive](https://openf1.org/docs/#location).

```sh
python3 tools/import_barcelona.py
go run . -recording .recordings/barcelona-2026.json
```

Import the full Hamilton–Russell race to use the timeline, lap and race-event jumps, and driver timing.

```sh
python3 tools/import_barcelona.py --full
go run . -recording .recordings/barcelona-2026-full.json
```

Full-race recordings open in local analysis mode. Review the gap-to-leader chart, compare completed lap times, or jump from the event list to a pit stop or Virtual Safety Car message. The pit-stop list shows stationary time and total pit-lane time when available. Use **Join shared playback** to follow the server's clock over WebSocket.

For a recording already stored in S3, pass its object URI. Configure the bucket's region and AWS credentials with permission to read that object. [AWS SDK configuration](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)

```sh
go run . -recording s3://your-bucket/clips/barcelona-2026.json
```

## Interfaces

| Endpoint | Purpose |
| --- | --- |
| `GET /` | Browser viewer |
| `GET /api/session` | Circuit geometry, driver IDs, and clip duration in JSON |
| `GET /api/segments/{index}` | About one minute of positions for local playback and seeking |
| `GET /ws` | WebSocket stream of timestamped position snapshots |

## Development

```sh
go vet ./...
go test -race ./...
python3 -m unittest discover -s tools
```

The browser tests use Node.js 24 and run the app in Chromium.

```sh
npm ci
npx playwright install chromium
npm run test:browser
```

The Go, importer, and browser suites run in GitHub Actions. The Go job also checks formatting. For the architecture and a tour of the code, see the [project guide](PROJECT_GUIDE.md).
