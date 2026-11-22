# F1 Race Replay

Replay the 2026 Barcelona-Catalunya Grand Prix on an interactive circuit map. Follow any of the 22 drivers, jump to race events, and compare lap times or gaps. Shared playback lets viewers join the same race in progress.

## Quick start

With Go 1.26 or newer, run this from the repository root.

```sh
go run .
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). The bundled demo loops a 12-second fictional race. Select **Inspect a frame** to pause the display. Pass `-addr 127.0.0.1:8090` to use a different local port.

## Historical replay

Import the Barcelona recording with Python 3.9 or newer and access to the [OpenF1 archive](https://openf1.org/docs/). The importer caches downloads so a retry can pick up where it left off.

```sh
python3 tools/import_barcelona_grid.py
go run . -recording .recordings/barcelona-2026-grid.json
```

- **Follow a driver.** Select a car on the map or a driver in the standings to see their gap, tyres, and last completed lap. A checkered flag marks each classified finisher’s crossing.
- **Jump to a moment.** Use the lap picker or select a pit stop or race-control message in **Race events**. Compare any two drivers at the current playhead.
- **Watch together.** Select **Join shared playback** to follow the server’s clock. Use local analysis to pause or seek independently.

## Documentation

- [Configure S3 or shared playback across servers](docs/CONFIGURATION.md)
- [Run on local Kubernetes and open Grafana](docs/DEPLOYMENT.md)
- [Read the technical guide](docs/learning/README.md)

## Development

```sh
go vet ./...
go test -race ./...
python3 -m unittest discover -s tools
```

Use Node.js 24 for the Chromium tests.

```sh
npm ci
npx playwright install chromium
npm run test:browser
```

The same checks run in [GitHub Actions](.github/workflows/ci.yml).
