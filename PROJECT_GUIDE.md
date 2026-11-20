# F1 Race Replay project guide

F1 Race Replay lets people study a race moment by replaying car positions on a track map. A shared timeline lets two viewers discuss the same moment, even if one joins midway. The historical clip follows Lewis Hamilton and George Russell for 50 seconds around Hamilton's final pit stop in Barcelona in 2026. [Formula 1 race report](https://www.formula1.com/en/latest/article/hamilton-claims-stellar-maiden-grand-prix-victory-for-ferrari-in-barcelona-as-antonelli-suffers-shock-retirement.4yCXiPLHUdcnl2BwNpqUIa)

## Architecture

```mermaid
flowchart LR
    S["OpenF1 position samples"] --> P["Offline Python importer"]
    P --> A["Prepared JSON<br/>Local file or S3 object"]
    A --> B["Go server<br/>One shared playback clock"]
    C["Browser"] -->|"HTTP GET / and /api/session"| B
    B -->|"HTTP responses<br/>Page, assets, session metadata"| C
    B -->|"WebSocket /ws<br/>Current car positions"| C
    C --> D["Canvas<br/>Draws the track and cars"]
```

### When HTTP and WebSocket come in

1. **Load the page over HTTP.** The browser requests `GET /` for the HTML, then fetches its JavaScript and CSS with separate HTTP requests.
2. **Fetch the map over HTTP.** The JavaScript calls `GET /api/session` once on page load. Go returns a JSON response containing the circuit outline, driver IDs, and clip duration.
3. **Open the position stream.** The browser connects to `/ws`. In this app, that starts as an HTTP/1.1 upgrade request. Go replies with `101 Switching Protocols` and the connection stays open as a WebSocket. The server sends the current frame immediately, then sends a new snapshot every 200 ms during steady playback.

HTTP request and response fit the page and map because the browser needs them once. A WebSocket lets the server push changing positions over one open connection. Polling could do the job with repeated requests. Server-Sent Events (SSE) could also carry this one-way stream. WebSockets support messages in both directions if shared playback controls are added later.

To see the difference, open the browser's Network tab and reload. Inspect the JSON response from `/api/session`, then open the `/ws` connection and watch messages keep arriving.

## Technologies to know

| Technology | What to understand |
| --- | --- |
| **Python and OpenF1** | The importer turns uneven position samples into regular frames once, before anyone opens the viewer. The generated clip can be replayed without calling OpenF1. |
| **Amazon S3** | An object is a stored recording identified by a bucket and key. Go fetches it once at startup. The server then serves frames from memory. |
| **Go and `net/http`** | A route maps to a handler. Each WebSocket connection has its own send loop, timer, and write timeout. Learn how handlers share the recording and start time while each manages its own connection. |
| **HTTP and WebSocket** | Be able to explain the page request, the metadata response, the upgrade, and what happens when a socket disconnects. |
| **JSON, JavaScript, and Canvas** | JSON is the data format both sides agree on. JavaScript parses each frame and draws a dot at each driver's `x` and `y` on the canvas. |

## Data model and decisions

| Model | What it stores and why |
| --- | --- |
| **Session metadata** | Circuit name, outline, driver IDs, and clip duration. Historical clips also carry driver names and source details, including the original UTC start time. The browser fetches metadata once. |
| **Recording** | An interval and an array of position snapshots. The snapshots are 200 ms apart, so Go can look up a row by dividing elapsed time by the interval. The small demo file is loaded into memory at startup. |
| **Stream frame** | `sequence` counts intervals since the process started. `elapsedMs` is the time within the clip and wraps at the end. `positions` contains each driver's ID and map coordinates. Driver IDs are strings, so `"01"` keeps its leading zero. |

One server clock means late joiners receive the current frame as soon as they connect. Sending **full snapshots** means any one message is enough to draw the cars, even after a skipped update. The trade-off is repeating every available car position in each message. Smaller change-only messages would require an initial snapshot and a way to recover missed changes.

### How historical samples become frames

- **Align on source timestamps.** Both drivers use the same UTC ticks, 200 ms apart. We sort their samples, collapse identical duplicates, and reject conflicting positions at the same timestamp.
- **Use nearby observations.** If a frame falls between two samples at most one second apart, estimate the position between them. Across a longer gap or beyond the available data, leave that driver's position out of the frame. The Barcelona source gave both drivers positions in all 250 frames.
- **Use one map transform.** Russell's lap 40 gives the main circuit outline. The track and both cars get the same scale, offset, and vertical-axis flip. The outline approximates a driven lap. Pit-lane positions can appear beside it, and the source coordinates only support an approximate view of each car's progress. [OpenF1 location data](https://openf1.org/docs/#location)

## Where to look in the code

| Start here | What it does |
| --- | --- |
| [tools/import_barcelona.py](tools/import_barcelona.py) | Fetches OpenF1 data, aligns the two cars, and writes the prepared JSON. |
| [testdata/demo.json](testdata/demo.json) | Stores the synthetic track and 60 sets of car positions. |
| [main.go](main.go) | Chooses the bundled demo, a local file, or S3. Starts the clock after reading the recording. |
| [s3.go](s3.go) | Uses the AWS SDK to read the prepared JSON from S3. |
| [replay.go](replay.go) | Defines the data types and uses `frameAt` to select a row in the clip. |
| [stream.go](stream.go) | Upgrades the connection, sends frames, and skips ahead if a write falls behind. |
| [web/viewer.js](web/viewer.js) | Fetches the session, opens the socket, and calls `ctx.arc` to place each car. |

## Trace one frame

At **0:28.2** in the Barcelona clip, the importer uses source time **14:01:36.200 UTC**. Hamilton's surrounding observations are at **14:01:36.171** and **14:01:36.411**. The importer estimates his position between them, transforms it onto the map, and writes it to frame index **141**. Go later sends that row to the browser. For any snapshot, divide the displayed elapsed milliseconds by 200 to find its row in the prepared recording.

## How we verify changes

| Test layer | What it checks |
| --- | --- |
| **Unit** | Replay timing at startup and loop boundaries, a one-frame recording, and rejection of malformed or incomplete recordings. [Replay tests](replay_test.go) |
| **Importer** | UTC alignment, duplicate samples, exact matches, gaps, and a shared coordinate transform. CI uses synthetic inputs. [Importer tests](tools/test_import_barcelona.py) |
| **Integration** | File loading, a signed S3 request against a local endpoint, HTTP session metadata, and advancing frames on a real WebSocket. [File tests](main_test.go) · [S3 tests](s3_test.go) · [Route tests](routes_test.go) · [WebSocket tests](stream_test.go) |
| **Browser** | Playwright starts the Go app and checks that the inspector can freeze and resume, playback continues after reconnect, and a failed metadata request shows an error. [Browser tests](tests/browser/replay.spec.mjs) |

GitHub Actions runs these checks on pull requests. The real Barcelona clip is generated locally. The S3 test uses a local endpoint with test credentials.

## Think like a system designer

- **Where does the authoritative state live?** The prepared recording contains historical positions. The running Go process owns the playback clock. Two servers reading the same S3 object can still disagree about the current frame if they started at different times.
- **How does the client recover?** After a disconnect, the browser retries and can draw the first full frame it gets. If the process itself restarts, it starts a new timeline because the old start time lived in memory.
- **What grows with more viewers?** At five frames per second, 200 connected viewers would mean about 1,000 outbound messages per second at full cadence. That's a rough estimate for a future load test. Measure message size and watch CPU and network use to see what actually limits the app.

**Next checkpoint.** Verify the recording against a live S3 bucket. Sharing one playback timeline across servers is the next architecture milestone.

Update this guide when a milestone ships. Revise the flow and file list in place, and keep the decisions and limits that still apply.
