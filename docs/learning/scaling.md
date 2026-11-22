# Scaling and recovery

## How scaling and recovery work

The Go WebSocket handler counts open connections. Prometheus scrapes each pod, KEDA exposes the total as an external metric, and Kubernetes’ Horizontal Pod Autoscaler (HPA) adjusts the Deployment. The local policy keeps two pods available and targets an average of 60 open sockets per pod, up to four pods. A new pod can accept new or reconnecting viewers. Existing sockets stay on the pods that accepted them.

When a serving pod goes away, clients reconnect through the Service. The new server computes the current frame from the same Redis clock. The browser retries after a close or after three seconds without a frame. The load test measures from the last valid frame to the first correctly aligned frame after reconnect. It includes time spent detecting the disconnect. A surviving pod can restore playback while Kubernetes is still replacing the failed pod.

## Delivery and control flow

```mermaid
flowchart LR
    K["k6 viewers or browser"] <-->|"WebSocket"| S["Kubernetes Service"]
    S --> P["Go replay pods"]
    P <-->|"Shared clock"| R["Redis"]
    P -->|"/metrics per pod"| M["Prometheus"]
    M -->|"Time series"| G["Grafana dashboard"]
    M -->|"Total open replay sockets"| A["KEDA external metric"]
    A --> H["Kubernetes HPA"]
    H -->|"Adjust replica count"| P
```

| Choice | Why |
| --- | --- |
| **Gauge per pod** | Counts concurrent work and falls on disconnect. Prometheus discovers each pod and includes its name in the target label. Scraping one load-balanced Service address could miss pods and miscount the total. |
| **Total connections with an average target** | Prometheus returns a single value. KEDA and the HPA turn the total into a target number of pods. The local target is a scale-test setting. A production threshold would come from frame sizes, CPU, latency, and headroom under load. |
| **Two-pod floor and delayed scale-down** | The minimum replica count gives viewers another pod to reconnect to after one pod fails. Scale-down stabilization reduces churn during short reconnect waves. Scale-up increases capacity for new connections. |
| **One Redis clock and recording** | A replacement pod can load the same recording and resolve the playhead at startup. The readiness endpoint is reachable after recording and Redis clock initialization. |
| **k6 run paired with pod deletion** | The driver waits for Prometheus to observe at least 200 concurrent connections and Kubernetes to report four available pods. It then deletes a pod with live viewers. The run must observe at least as many successful recoveries as the deleted pod’s connection count at the pre-deletion scrape. k6 requires each interruption to recover within 10 seconds and checks that the first recovered frame matches the expected sequence within one second of timing jitter. |

[KEDA Prometheus scaler](https://keda.sh/docs/2.21/scalers/prometheus/) · [k6 WebSockets](https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/) · [Redis clock](redis-clock.md)

With 220 open sockets, the target of 60 sockets per replica implies roughly four replicas (`ceil(220 / 60)`), within the configured range of two to four. Prometheus samples every two seconds in the local cluster. The HPA also has a 120-second scale-down stabilization window to reduce churn. These settings let the test drive scale-up with a small local workload. They do not establish the maximum safe number of viewers per pod. For example, five frames per second and 200 viewers imply about 1,000 frame messages per second at full cadence. CPU and bandwidth also depend on the payload size, and sockets can be unevenly distributed because existing WebSockets stay attached to their original pods. [Scrape configuration](../../deploy/local.yaml) · [Autoscaling policy](../../deploy/autoscale.yaml)

## How Grafana is provisioned

Grafana reads metrics from Prometheus. KEDA queries Prometheus separately to scale the Deployment. Kustomize turns the dashboard and data source definitions into ConfigMaps. When a file changes, Kustomize generates a new ConfigMap name, so applying the manifests updates the pod template and restarts Grafana with the new configuration. [Grafana configuration](../../deploy/grafana)

## What the first load test changed

```mermaid
flowchart TD
    before["Viewer receives frames from pod A"] --> lost["Pod A is deleted"]
    lost --> signal["Socket closes or 3 seconds pass without a frame"]
    signal --> retry["Client attempts a new WebSocket connection"]
    retry --> ready["Kubernetes Service routes to an available pod"]
    ready --> frame["New pod sends its current frame using the shared clock"]
    frame --> resume["Viewer resumes near the shared playhead"]
    frame --> verify["k6 checks frame alignment and measures from the last valid frame"]
```

The initial run reached 220 connected viewers and four available pods. After the pod deletion, 84 of 114 interrupted viewers recovered within 10 seconds. The longest observed gap was 60.6 seconds. The distribution of handshake times pointed to stalled reconnect attempts. We added a three-second frame watchdog to the viewer and a bounded retry cadence to the k6 client. A Playwright test covers an open socket that stops delivering frames. Three seconds allows roughly 15 expected frames to go missing before reconnecting. A shorter timeout would react faster at the cost of extra reconnects during temporary network stalls.

## Verified local run

This recorded passing run used the full Barcelona recording on a local kind cluster with Kubernetes 1.36.4, KEDA 2.21.0, Prometheus 3.16.0, Redis 7.4.12, and k6 2.1.0.

| Measurement | Result |
| --- | --- |
| Concurrent WebSockets observed by Prometheus | 220 |
| Available replay pods before disruption | Scaled from 2 to 4 |
| Connections on the deleted serving pod | 129 |
| Disrupted viewers receiving an aligned frame within 10 seconds | 129 of 129 |
| Recovery gap, measured from last good frame to first aligned frame | 1.207 seconds at p95, 1.208 seconds maximum |
| Completed k6 iterations | 220 of 220 |
| Valid frame messages scored by k6 | 98,442 of 98,442 |

The load driver completed successfully. The result applies to one deleted app pod on local Kubernetes, with Redis and surviving app pods still available.

## Grafana verification runs

Both reruns with Grafana installed reached 220 concurrent WebSockets, grew from two to four available replicas, and finished all 220 k6 iterations. The driver deleted a pod with 118 connections each time. k6 counts interruptions across every viewer, so that total can exceed the pre-deletion connection count.

| Run | Interruptions recovered within 10 seconds | Longest gap | Result |
| --- | --- | --- | --- |
| First rerun | 124 of 134 | 47.682 s | Recovery threshold failed |
| Repeat | 118 of 118 | 1.102 s | All thresholds passed |

The dashboard [screenshot](../images/grafana.png) is from the passing repeat. The driver requested pod deletion at 16:49:30 UTC. The per-pod chart shows the deleted pod’s series ending and surviving pods accepting its viewers. The cause of the slower first rerun remains unconfirmed. The results show that recovery can vary even when the deployment reaches the same connection and replica counts.

## Pod deletion with recovery stage timings

On 22 November 2026, the current build replayed the full Barcelona recording on a fresh local kind cluster with the checked-in KEDA and Prometheus configuration. Prometheus observed 220 concurrent connections after the Deployment scaled from two to four available pods. The driver deleted a pod serving 118 connections. All 118 interrupted viewers received a correctly aligned frame within 10 seconds, all 220 sessions completed, and k6 validated all 98,433 frame messages.

| Measurement | p95 | Maximum |
| --- | ---: | ---: |
| Last valid frame to interruption detection | 69 ms | 72 ms |
| Detection to start of the successful reconnect attempt | 1,001 ms | 1,004 ms |
| Successful WebSocket handshake | 19 ms | 19 ms |
| Open socket to first correctly aligned frame | 1 ms | 1 ms |
| **Last valid frame to first correctly aligned frame** | **1,087.15 ms** | **1,090 ms** |

The one-second client retry wait accounted for most of the gap in this run. Each percentile describes its own set of measurements. Use the reported end-to-end percentile for the viewer experience. The older 47.682-second outlier predates this instrumentation, so its cause remains unknown. [Recorded improvement](improvements.md#one-recovery-number-hid-where-the-time-went)

For a capacity decision, repeat at different loads and record CPU, memory, payload bytes, reconnect attempts, and the recovery time distribution. A rolling update and a Redis interruption would cover failure paths beyond the single replay pod deletion tested here. [Load scenario](../../load/recovery.js) · [Test driver](../../load/verify-kubernetes.sh)

[Run the experiment](../DEPLOYMENT.md#run-the-load-and-recovery-test) · [Open Grafana](../DEPLOYMENT.md#view-the-grafana-dashboard)
