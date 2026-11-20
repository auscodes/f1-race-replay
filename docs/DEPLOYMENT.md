# Local Kubernetes deployment

## Start the local cluster

Run these commands from the repository root with Docker running, kind 0.33, and kubectl. [Import the Barcelona recording](../README.md#historical-replay) before building the image. The kind config pins Kubernetes 1.36.4 for KEDA 2.21.0. [KEDA compatibility](https://keda.sh/docs/2.21/operate/cluster/#kubernetes-compatibility)

```sh
export KUBECONFIG="$PWD/.recordings/kind-kubeconfig"
kind create cluster --name f1-replay --config deploy/kind.yaml

docker build --target barcelona --build-context recording=.recordings -t f1-race-replay:local .
kind load docker-image f1-race-replay:local --name f1-replay

kubectl --context kind-f1-replay apply --server-side -f https://github.com/kedacore/keda/releases/download/v2.21.0/keda-2.21.0.yaml
kubectl --context kind-f1-replay apply -f deploy/local.yaml
kubectl --context kind-f1-replay -n keda wait --for=condition=available deployment --all --timeout=120s
kubectl --context kind-f1-replay -n f1-replay rollout status deployment/replay --timeout=120s
kubectl --context kind-f1-replay apply -f deploy/autoscale.yaml
```

The [local cluster viewer](http://127.0.0.1:18089/) is bound to loopback. This build packages the generated recording into a local test image so the cluster can run the full race. See [server configuration](CONFIGURATION.md#load-a-recording-from-s3) for S3.

## View the Grafana dashboard

Once the local stack is running, install the optional dashboard. The manifests configure the Prometheus data source and dashboard.

```sh
kubectl --context kind-f1-replay apply -k deploy/grafana
kubectl --context kind-f1-replay -n f1-replay rollout status deployment/grafana --timeout=180s
kubectl --context kind-f1-replay -n f1-replay port-forward --address 127.0.0.1 service/grafana 13000:3000
```

Open [F1 Race Replay in Grafana](http://127.0.0.1:13000/d/f1-replay-overview/f1-race-replay) and keep the port-forward running while you run the [load test](#run-the-load-and-recovery-test) in another terminal. The local dashboard uses anonymous viewer access over the loopback port-forward.

| Panel | What to look for |
| --- | --- |
| **Total open WebSockets** | The same Prometheus sum KEDA uses. Watch the load ramp up as k6 viewers connect. |
| **Replay pods responding to Prometheus** | Counts pods with a successful scrape. The test driver separately checks the Deployment’s available replicas through Kubernetes. |
| **Open WebSockets by replay pod** | Watch new series appear after scale-up, the deleted pod’s series end, and connections appear on surviving pods. Use the UTC timestamp printed by the driver to locate the deletion. |

Grafana shows sampled server metrics. It can miss short disconnects between scrapes. Use k6’s recovery and frame checks to determine what viewers experienced. To remove Grafana, stop the port-forward and run `kubectl --context kind-f1-replay delete -k deploy/grafana`.

[Screenshot from the 220-viewer rerun](images/grafana.png) · [Dashboard definition](../deploy/grafana/replay.json) · [Grafana provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)

## Run the load and recovery test

Install k6 2.1. Run the driver from the repository root when the Deployment has two available replay pods. The driver deletes one serving pod on the local `kind-f1-replay` cluster.

```sh
export KUBECONFIG="$PWD/.recordings/kind-kubeconfig"
load/verify-kubernetes.sh
```

The driver prints the results directory. `summary.json` contains the recovery trend and thresholds, `k6.log` has the test output, `fault.tsv` records the observed active connections, available replicas, disrupted pod, and that pod’s connection count, and `fault-time.txt` records when the driver requested the deletion in UTC. For another run, wait for the autoscaler to return to two available pods.

[Local Kubernetes manifests](../deploy/local.yaml) · [Autoscaling config](../deploy/autoscale.yaml) · [k6 scenario](../load/recovery.js) · [Run driver](../load/verify-kubernetes.sh)

[Scaling design and load-test results](learning/scaling.md)
