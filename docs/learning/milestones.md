# F1 Race Replay milestones

The project starts with a single shared replay and builds toward a load-tested Kubernetes deployment. Each milestone adds an observable behavior to the previous one.

| Milestone | Status | What it adds | Evidence |
| --- | --- | --- | --- |
| **1 · Shared replay prototype** | Complete | Go and WebSockets deliver a synthetic race on one process clock. Late viewers receive the current frame. | [Frame lookup](../../replay.go) · [WebSocket tests](../../stream_test.go) |
| **2 · Historical race and analysis** | Complete | Python imports the Barcelona race. Go serves the prepared recording from disk or the S3 SDK. Local analysis can seek independently. The map and timing board cover all 22 drivers. | [Full-grid replay](https://github.com/auscodes/f1-race-replay/pull/8) · [Circuit map](https://github.com/auscodes/f1-race-replay/pull/10) |
| **3 · Shared clock across server replicas** | Complete locally | Redis stores one start time per recording and publishes clock changes. Independently started Go servers use that clock, while each serves its own viewers. | [Architecture and validation](redis-clock.md) · [Redis integration tests](../../replay_clock_test.go) |
| **4 · Kubernetes and autoscaling** | Verified on local Kubernetes | Package the app, expose the active WebSocket connection count, scrape it with Prometheus, and have KEDA use it to adjust replica count. | [Scaling and metrics](scaling.md#how-scaling-and-recovery-work) · 220 connections, two to four available pods. |
| **5 · Load and recovery** | Verified on local Kubernetes | k6 held 220 concurrent WebSockets while a serving pod was deleted. | [Verified load run](scaling.md#verified-local-run) · All 129 affected viewers recovered within 1.208 seconds. |
| **6 · Grafana observability** | Verified on local Kubernetes | Provision the dashboard and Prometheus data source together. Inspect active connections and scrapeable pods during the failure test. | [Dashboard and reruns](scaling.md#grafana-verification-runs) |
| **7 · Delivery and recovery diagnostics** | Verified locally | Compress and version HTTP segments, support gzip recordings, retry transient imports, and break reconnect timing into stages. | [Browser and importer checks](improvements.md) · [220-viewer run with stage timings](scaling.md#pod-deletion-with-recovery-stage-timings) |

## Verification scope

Two separate Go processes streamed the full Barcelona recording from Moto, a local S3 emulator, while sharing a real Redis clock. Late viewers joined both servers on frame 18. An operator reset moved both connections to frame 0.

The scaling and recovery experiments used local Kubernetes with the recording packaged into the app image. The performance follow-up measured segment delivery in Chromium against a local Go server. The new k6 timing metrics were exercised first with a four-socket proxy test and then under pod deletion with 220 concurrent connections on kind. End-to-end Amazon S3 and EKS verification would be needed to support an AWS deployment claim. [Follow-up evidence](improvements.md) · [Kubernetes stage timings](scaling.md#pod-deletion-with-recovery-stage-timings)

[Technology walkthrough](stack.md) · [Architecture and code map](architecture.md) · [Run the app](../../README.md)
