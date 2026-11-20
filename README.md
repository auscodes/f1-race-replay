# F1 Race Replay

F1 Race Replay renders recorded car positions on a circuit map. Viewers share a playback timeline and can join a replay in progress. Configure Redis to share the clock across Go replicas. The bundled demo uses a fictional circuit and a 12-second synthetic recording.

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

Replay the 2026 Barcelona-Catalunya Grand Prix from lights out through the last classified finisher. The leaderboard follows all 22 drivers with running position, gap to the leader or interval to the car ahead, tyre compound and age, and last completed lap. Select a driver's code to highlight their car on the map. A checkered flag appears when each classified finisher crosses the line.

Generate the recording with Python 3.9 or newer and access to the [OpenF1 archive](https://openf1.org/docs/). The importer caches downloaded data in `.recordings/openf1-grid` so interrupted imports can resume.

```sh
python3 tools/import_barcelona_grid.py
go run . -recording .recordings/barcelona-2026-grid.json
```

Full-race recordings open in local analysis mode. Jump to a race lap from the playback controls, or pick a pit stop, Virtual Safety Car message, or the chequered flag in Race events. The map shows the three timing sectors, race-control state, and the leader’s lap. Select a car on the map, in the driver picker, or in the standings to see its current position, gap, tyres, and last completed lap. The comparison panel starts with Hamilton and Russell and lets you select any two drivers to review their gaps to the leader and completed lap times. Choose **Join shared playback** to follow the server's clock over WebSocket.

The two-driver importer remains available for the 50-second Virtual Safety Car clip and the full Hamilton–Russell comparison.

```sh
python3 tools/import_barcelona.py
python3 tools/import_barcelona.py --full
```

For a recording already stored in S3, pass its object URI. Configure the bucket's region and AWS credentials with permission to read that object. [AWS SDK configuration](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)

```sh
go run . -recording s3://your-bucket/clips/barcelona-2026.json
```

## Shared playback across servers

Set `REPLAY_REDIS_URL` on each server and use identical recording bytes. A local Redis can be started with `docker run --rm -p 127.0.0.1:6379:6379 redis:7.4.12-alpine`. Run the two Go commands in separate terminals:

```sh
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -addr 127.0.0.1:8080
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -addr 127.0.0.1:8081
```

Those commands use the bundled demo. For Barcelona, add `-recording .recordings/barcelona-2026-grid.json` to both, then choose **Join shared playback** in each viewer. To reset the shared clock, run the command below with the same recording selection. It changes the timeline for every connected shared viewer.

```sh
REPLAY_REDIS_URL=redis://127.0.0.1:6379/0 go run . -reset-shared-clock
```

The default startup uses a process-local clock. The [Redis design notes](docs/REDIS_CLOCK.md) explain clock ownership and how to verify cross-server playback.

## Kubernetes

The local kind deployment runs the Go viewer, Redis, and Prometheus. KEDA scales the Go Deployment on open WebSocket connections. The optional Grafana dashboard shows the total connection count, connections per pod, and how many replay pods are responding to Prometheus. [Deploy to kind](docs/DEPLOYMENT.md#start-the-local-cluster) · [Open Grafana](docs/DEPLOYMENT.md#view-the-grafana-dashboard)

In a recorded local load test, 220 simultaneous WebSockets drove a scale-up from two to four pods. After a serving pod was deleted, all 129 affected viewers received a correctly aligned frame within 1.208 seconds. The test measures the gap between valid frames from the viewer’s perspective. [Test setup and results](docs/DEPLOYMENT.md#verified-local-run) · [Grafana reruns](docs/DEPLOYMENT.md#grafana-verification-runs)

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

New to the stack? The [technology walkthrough](docs/STACK_GUIDE.md) follows one race frame to the browser and one viewer through a pod failure. See the [project milestones](docs/ROADMAP.md) for implementation and verification status.
