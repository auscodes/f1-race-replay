# Performance and reliability improvements

## Position segments transferred more data than necessary

**Why it mattered.** The browser already loaded the race in 20-second segments. On a cold seek, it still had to download an entire segment before it could draw the requested frame. The original segment route encoded and sent plain JSON. Many viewers can request the same segment independently, and the browser’s in-memory cache lasts for one analysis instance.

**Change.** Go now offers gzip for position segments when the client accepts it. It uses a fast compression level and keeps plain JSON available. The browser also includes the recording version in its segment URLs. Go derives that version from a SHA-256 hash of the decoded recording JSON, with an API format prefix. A revised recording gets a different URL. Successful versioned responses carry immutable HTTP cache headers, while an unknown version receives a non-cacheable 404. The older segment route remains available. [HTTP handlers](../../main.go) · [Browser fetching](../../web/analysis.js)

**Evidence.** I requested three segments from the running server, once with plain JSON and once with gzip. Decompressing each gzip response produced the exact plain body.

| Segment | Plain JSON | Gzip |
| --- | ---: | ---: |
| 0 | 82,926 B | 18,663 B |
| 1 | 82,846 B | 19,356 B |
| 140 | 82,667 B | 17,942 B |

In Chromium with the HTTP cache disabled, I ran five trials per encoding against the same local Go server. A local proxy selected the encoding for position segments only. The browser simulated 1 Mbps download throughput and 40 ms of latency. When opening the replay, the median time from navigation to displaying frame zero was **1.52 s with plain JSON and 1.00 s with gzip**. When seeking to an uncached 20-second segment, the median time from triggering the seek to displaying the target frame was **1.40 s with plain JSON and 0.38 s with gzip**. These are controlled local measurements. Viewer devices, network paths, and server load can change the results.

**Trade-off.** Compressing a response takes CPU on an origin request. The HTTP cache can avoid repeated requests for a versioned segment. A deploy that changes the recording can make an old version unavailable on the current server. Keeping older versions addressable would help if we add a catalog or deploy recording updates while people are watching. Measure origin CPU and cache hit rate before considering a precompressed segment store.

## Startup combines file transfer, decoding, and server setup

**Why it mattered.** Every new Go replica loads the recording before it serves traffic. The startup log now reports data size, time to read, time to decompress, and time to parse and hash. The listener startup log also reports total initialization time.

**Change and evidence.** Go accepts gzip recordings from a local file or S3. The hash is calculated after decompression, so two replicas reading plain and compressed copies of the same JSON still share the same playback clock. A local check reduced the stored artifact from about 24 MiB to 4.4 MiB, but one warm-disk run started in roughly 371 ms with plain JSON and 468 ms with gzip. The faster read saved less time than decompression added. Gzip is useful when transfer or storage cost dominates. Startup over real S3 needs its own benchmark, and each replica still holds decoded frames in memory. [Recording loader](../../main.go) · [Configuration](../CONFIGURATION.md#load-a-recording-from-s3)

## Fresh imports could stop after a temporary provider error

**Why it mattered.** A transient timeout or throttled response could interrupt an import. Cached raw responses let the next run resume, but it still needed manual intervention.

**Change and evidence.** The fetch helper now tries at most three times on transient HTTP or connection failures. It backs off and respects a usable `Retry-After` value, returning the error if the requested wait exceeds 30 seconds. The full-grid command reports when it loads sources, prepares frames and timing, and writes the result. Tests cover recovery after a throttled response and immediate failure on a 404. A cached full import produced 28,135 frames for 22 drivers and matched the previous JSON byte for byte. That check used cached sources. It did not measure current OpenF1 request latency. [Fetch helper](../../tools/import_barcelona.py) · [Full-grid importer](../../tools/import_barcelona_grid.py)

## One recovery number hid where the time went

**Why it mattered.** The measured recovery gap tells us how long a viewer waited for a valid aligned frame. A slow result can come from waiting to detect the interruption, failed or delayed reconnect attempts, the successful handshake, or waiting for an aligned frame after the handshake.

**Change.** The k6 test records those four durations and counts abandoned reconnect attempts. Detection samples and abandoned attempts include a tag distinguishing a socket close from a frame watchdog timeout. The total recovery threshold and frame-alignment check still determine whether the run passes. The stage metrics help explain the result. [Load scenario](../../load/recovery.js)

**Evidence.** Four test sockets dropped through a local proxy all recovered within 1.134 seconds, which confirmed the new metrics were recorded. We then reran the local Kubernetes pod-deletion test at 220 concurrent connections. All 118 affected viewers recovered within 1.090 seconds. The p95 for the wait before the successful attempt was 1,001 ms, consistent with the configured one-second retry interval. The new run explains where the time went in this experiment. The cause of the older 47.7-second outlier remains unknown. [Cluster results by stage](scaling.md#pod-deletion-with-recovery-stage-timings)

## How to explain the change in an interview

> “I looked at startup, seeking, and recovery separately because each waits on different work. Gzip cut segment transfer size by about 77%, and the median cold seek dropped from 1.40 seconds to 0.38 seconds in a controlled browser test. I also broke reconnect time into stages. In the local Kubernetes rerun, all 118 affected viewers recovered within 1.090 seconds, and most of the gap came from the configured retry wait. The earlier slow run predates those measurements, so I’d collect the same breakdown if the issue recurs.”
