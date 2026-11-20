# The F1 Replay stack from scratch

You can understand both résumé bullets by following **one race frame** and **one interrupted viewer**. Start with [the verified résumé wording](ROADMAP.md#résumé-wording-supported-by-the-current-evidence). The two walkthroughs below explain what each tool did, why the project used it, and what evidence supports the claim.

## Bullet 1 — How a race frame gets to the browser

```text
OpenF1 archive → Python importer → prepared race file → Go server → browser
                                                          ↑
                                          Redis supplies the shared start time
```

### 1. Python prepares the recording

**Telemetry** here means historical race data, including car locations and timing. OpenF1 gives us separate observations with timestamps. They arrive at uneven intervals. The importer uses milliseconds since lights out as a common timeline, then builds a position snapshot every 200 milliseconds.

A **frame** is one of those snapshots. It tells the map where the cars were at a particular instant. Race positions, gaps, lap times, and race-control events stay as separate timing records. A lapped car can be physically ahead of the leader on the straight, so you need the timing data to know its place in the race.

**Engineering choice:** Preparing the frames once makes playback an array lookup. It costs disk space and requires regenerating the recording when the importer changes. The importer interpolates between nearby location samples, with a one-second cap on the gap. Across a larger gap, it leaves the car out of that frame. [Full-grid importer](../tools/import_barcelona_grid.py) · [Location resampling](../tools/import_barcelona.py)

### 2. S3 stores the prepared file and Go loads it

**S3** is object storage. Think of a **bucket** as the named container and a **key** as the name of a stored object inside it. In `s3://my-bucket/barcelona.json`, `my-bucket` is the bucket and `barcelona.json` is the key. The **AWS SDK** is the Go library that makes the S3 request and handles details such as request signing. [Amazon S3 overview](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html)

At startup, Go reads the whole recording from a local file or calls S3’s `GetObject` through the SDK. It decodes the JSON and keeps the frames in memory. The app serves each viewer from that loaded recording. The benefit is inexpensive seeks and frame lookups. The cost is memory and startup work that grow with the recording size. **Moto** is the local emulator we used to verify the S3 request path with the full Barcelona file. [Startup](../main.go) · [S3 request](../s3.go)

### 3. Go chooses frames and sends them over WebSockets

Think of **HTTP** as asking for something and getting a response. The browser uses HTTP to fetch the track, driver information, and timing. In personal analysis it also requests the 20-second block of position frames around whatever moment you choose.

A **WebSocket** starts with an HTTP upgrade and then stays open so the server can send messages as playback advances. The connection handler in `stream.go` sends the current frame immediately, then uses a timer to send newer frames. Every message is a **full snapshot** of the available car positions at that time. A late viewer can draw the first message they receive. Full snapshots repeat coordinates, which trades bandwidth for simpler recovery.

For example, if shared playback has been running for 10 seconds, Go divides 10,000 ms by 200 ms and gets sequence **50**. Before the first loop ends, that means frame **50**. After the end, Go uses the remainder after dividing by the frame count to wrap to the start. If a send is slow, the next iteration computes the current frame from the clock. [Frame calculation](../replay.go) · [WebSocket send loop](../stream.go) · [Browser modes](../web/viewer.js)

### 4. Redis makes independent Go servers agree on time

**Redis** stores data under named keys. The replay uses one key for the start time of each recording. The key includes a hash of the recording bytes, so servers playing different recordings get separate clocks. A Redis script initializes the key atomically, which means two servers starting together agree on one value.

**Pub/Sub** is a way to notify subscribers through a named channel. When the operator resets playback, Redis updates the key and publishes a notification in the same operation. Each server then reads the current value. A server can miss a message while disconnected, so it also reads the key periodically. Redis supplies the start time and a time sample. Go converts that reference to its local monotonic clock, which is meant for measuring elapsed time. Frame selection stays local to each server. [Redis delivery semantics](https://redis.io/docs/latest/develop/pubsub/#delivery-semantics) · [Clock implementation](../replay_clock.go)

**Say this aloud**

> “The race file tells each server which frames exist. Redis tells the servers when playback started. The servers choose frames from memory and send them over WebSockets. We use Pub/Sub to announce a clock reset, and the Redis key lets a new server join the current timeline or recover a missed notification. We verified this with two Go processes reading the full race through the S3 SDK against Moto and sharing a real Redis clock.”

## Bullet 2 — How the app scales and a viewer recovers

```text
Go pods expose open connection counts → Prometheus → KEDA → Kubernetes HPA
                                                            ↓
                                                  changes pod count

k6 opens WebSockets → serving pod is deleted → viewers reconnect → k6 checks frames
```

### 1. Docker packages the app and Kubernetes runs copies

A **container image** packages the executable and files needed to run it. A **container** is a running instance of that image. Kubernetes runs containers inside **pods**. In this project, each replay pod runs a Go server. The Kubernetes **Deployment** describes the desired number of replicas and Kubernetes works to maintain it. A **Service** gives clients one address for reaching available pods.

We ran Kubernetes locally using **kind**. The local test image includes the full Barcelona file, and the cluster also runs Redis and Prometheus. **EKS** is Amazon’s managed Kubernetes service. It is the AWS environment named in the original bullet. The recorded scale-up and recovery results are from kind. [Container build](../Dockerfile) · [Deployments, Services, and scrape config](../deploy/local.yaml)

### 2. Prometheus measures open connections

A **metric** is a numeric measurement. A **gauge** can go up or down. A **counter** accumulates events. The Go server increments the `f1_replay_websocket_connections` gauge after accepting a connection and decrements it when the handler exits. A count of how many clients ever connected would keep rising even after viewers left.

**Prometheus** periodically requests each pod’s `/metrics` endpoint. That is called a **scrape**. It reads the per-pod gauges so the cluster can add them together. A single scrape through the app’s load-balanced Service might reach only one pod. [Prometheus metric types](https://prometheus.io/docs/concepts/metric_types/) · [Gauge setup](../main.go) · [Connection handling](../stream.go)

### 3. KEDA connects the metric to the Kubernetes autoscaler

**KEDA** configures autoscaling based on a measurement from an external system. Here it queries the sum of open WebSockets in Prometheus and exposes that number to Kubernetes. The **Horizontal Pod Autoscaler (HPA)** changes the replica count. “Horizontal” means changing the number of copies of the app.

In our test, the target was **60 active connections per replica**, with a minimum of two and a maximum of four. At 220 total connections, 220 ÷ 60 is about 3.67, so the target rounds up to **four pods**. This is a scaling target for the local test. Existing connections stay with their original pods, and extra pods provide capacity for new connections or reconnects. [KEDA Prometheus scaler](https://keda.sh/docs/2.21/scalers/prometheus/) · [Kubernetes autoscaling](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/) · [Replay scaling policy](../deploy/autoscale.yaml)

### 4. k6 tests the viewer experience when a pod disappears

**k6** is a load-testing tool. Each **virtual user** in this test opens a WebSocket and reads frames, with a new connection if the first one breaks. The test waits for Prometheus to observe at least 200 active connections and Kubernetes to report four available pods. The test driver then force-deletes the pod with the most connections. On reconnect, the new pod uses the shared Redis clock to pick a frame on the ongoing timeline. [k6 WebSocket API](https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/) · [Load scenario](../load/recovery.js) · [Failure injection](../load/verify-kubernetes.sh)

**What recovery means:** Measure the time from the last valid frame before the interruption to the first correctly aligned frame after reconnect. The script estimates the expected sequence from how much time passed and allows up to one second of alignment jitter. The **10-second target** is the maximum interruption a viewer can experience in this test. A surviving pod can restore playback while Kubernetes replaces the deleted one.

**What the experiment showed:** The first run reached 220 viewers and four pods. Only 84 of 114 affected viewers recovered within ten seconds. Some reconnect attempts stalled for about a minute. We added a three-second check for missing frames or a stalled connection and bounded retries. In the verified local run, **all 129 affected viewers recovered**, the longest gap was **1.208 seconds**, and **all 220 k6 sessions completed**. The test covers one app pod failure with Redis and other pods still available. [Full run evidence](DEPLOYMENT.md#verified-local-run)

**Say this aloud**

> “Go counts active WebSockets on each pod. Prometheus scrapes the counts, and KEDA makes their total available to the Kubernetes autoscaler. k6 opened 220 concurrent connections and the deployment grew from two pods to four. Then we deleted the pod serving 129 viewers and measured when each one received a correctly aligned frame again. The longest gap was about 1.21 seconds. That result came from local Kubernetes.”

## What you personally did

> “I started with Hamilton and Russell to make the first version manageable, then expanded the scope to the whole field so someone could follow drivers outside the lead battle. I asked how full-race loading would affect startup and chose a local approach for testing the S3 integration and deployment. Codex assisted with much of the implementation. My role was to shape the product, work through the engineering choices, and understand what the tests demonstrate. I can walk through the Go stream, explain how Redis coordinates replicas, and show the evidence from deleting a pod under load.”

If the interviewer asks about the résumé wording, use this version:

> “I verified the S3 SDK path with Moto and tested autoscaling and recovery on local Kubernetes. A run on AWS would verify the same workflow with Amazon S3 and EKS.”

[Original scope and project stories](../PROJECT_GUIDE.md) · [Milestones and evidence](ROADMAP.md)

## See it for yourself

1. Open the replay in two shared tabs. Refresh the second tab after the first has advanced. Explain why it starts in progress. Switch one tab to personal analysis and seek to see how its clock is independent.
2. With the local server running, request `/api/session` and `/metrics`, then open a shared tab and look at the gauge again. Explain the difference between the recording metadata and the current connection count.
3. Walk through the test result from the original 10-second failure to the passing run. Use the recorded connection count, number of affected viewers, and maximum gap as three distinct measurements. The deployment guide contains the local run commands and explains the pod deletion step. [Local setup](../README.md) · [Load test instructions](DEPLOYMENT.md#run-the-load-and-recovery-test)
