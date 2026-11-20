# F1 Race Replay milestones

The project starts with a single shared replay and builds toward a load-tested Kubernetes deployment. Each milestone adds an observable behavior to the previous one.

| Milestone | Status | What it adds | Evidence |
| --- | --- | --- | --- |
| **1 · Shared replay prototype** | Complete | Go and WebSockets deliver a synthetic race on one process clock. Late viewers receive the current frame. | [Frame lookup](../replay.go) · [WebSocket tests](../stream_test.go) |
| **2 · Historical race and analysis** | Complete | Python imports the Barcelona race. Go serves the prepared recording from disk or the S3 SDK. Local analysis can seek independently. The map and timing board cover all 22 drivers. | [Full-grid replay](https://github.com/auscodes/f1-race-replay/pull/8) · [Circuit map](https://github.com/auscodes/f1-race-replay/pull/10) |
| **3 · Shared clock across server replicas** | Complete locally | Redis stores one start time per recording and publishes clock changes. Independently started Go servers use that clock, while each serves its own viewers. | [Architecture and validation](REDIS_CLOCK.md) · [Redis integration tests](../replay_clock_test.go) |
| **4 · Kubernetes and autoscaling** | Verified on local Kubernetes | Package the app, expose the active WebSocket connection count, scrape it with Prometheus, and have KEDA use it to adjust replica count. | [Deployment and metrics](DEPLOYMENT.md) · 220 connections, two to four available pods. |
| **5 · Load and recovery** | Verified on local Kubernetes | k6 held 220 concurrent WebSockets while a serving pod was deleted. | [Verified load run](DEPLOYMENT.md#verified-local-run) · All 129 affected viewers recovered within 1.208 seconds. |
| **6 · AWS verification** | Pending | Run the same workload with real Amazon S3 storage and an EKS deployment. | Evidence of S3 reads, pod scaling, and recovery measurements from AWS. |

Two separate Go processes streamed the full Barcelona recording from Moto, a local S3 emulator. Redis kept both late joins on frame 18 and delivered a reset that both sockets showed at frame 0 in that run. A local Kubernetes run can establish scaling and recovery on Kubernetes. Cloud verification is a separate milestone. A run on AWS would verify real S3 access, KEDA scaling on EKS, and the same k6 recovery threshold in that environment. [AWS EKS pricing](https://aws.amazon.com/eks/pricing/)

## Résumé wording supported by the current evidence

- Built a Go F1 replay platform that loads historical telemetry through the S3 SDK and streams it over WebSockets. Synchronized playback across replicas using a Redis clock and Pub/Sub, with the S3 flow verified using Moto.
- Autoscaled WebSocket pods on local Kubernetes with Prometheus and KEDA. Tested 220 concurrent k6 connections and verified recovery for all 129 affected viewers within 1.21 seconds after deleting a serving pod.

For the spoken walkthrough, see the [project guide](../PROJECT_GUIDE.md). For running the app, see the [README](../README.md).
