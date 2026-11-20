#!/usr/bin/env bash
set -euo pipefail

# This run disrupts one serving pod in the disposable local kind cluster.
root=$(cd "$(dirname "$0")/.." && pwd)
output=${REPLAY_RESULTS_DIR:-$(mktemp -d)}
mkdir -p "$output"
kube=(kubectl --context kind-f1-replay -n f1-replay)
prometheus=http://127.0.0.1:19090
load_pid=
prom_pid=

cleanup() {
  if [[ -n "$load_pid" ]]; then kill "$load_pid" 2>/dev/null || true; fi
  if [[ -n "$prom_pid" ]]; then kill "$prom_pid" 2>/dev/null || true; fi
}
trap cleanup EXIT

query() {
  curl --fail --silent --show-error --max-time 3 --get "$prometheus/api/v1/query" \
    --data-urlencode "query=$1"
}

replicas() {
  "${kube[@]}" get deployment replay -o 'jsonpath={.status.availableReplicas}'
}

"${kube[@]}" rollout status deployment/replay --timeout=90s
if [[ $(replicas) != 2 ]]; then
  echo "Wait for KEDA to return the replay deployment to two available pods." >&2
  exit 1
fi

"${kube[@]}" port-forward service/prometheus 19090:9090 >"$output/prometheus.log" 2>&1 &
prom_pid=$!
for ((attempt=0; attempt<30; attempt++)); do
  if curl --fail --silent --max-time 2 "$prometheus/-/ready" >/dev/null; then break; fi
  sleep 1
done
curl --fail --silent --show-error --max-time 2 "$prometheus/-/ready" >/dev/null
echo "Starting 220 concurrent viewers with two available replicas."
K6_NO_USAGE_REPORT=true REPLAY_VIEWERS=220 REPLAY_HOLD_SECONDS=90 REPLAY_EXPECT_FAILURE=1 \
  REPLAY_URL=http://127.0.0.1:18089 "${K6_BIN:-k6}" run --no-color \
  --summary-export "$output/summary.json" "$root/load/recovery.js" >"$output/k6.log" 2>&1 &
load_pid=$!

victim=
for ((attempt=0; attempt<55; attempt++)); do
  if ! kill -0 "$load_pid" 2>/dev/null; then
    cat "$output/k6.log"
    exit 1
  fi
  if [[ $(replicas) == 4 ]]; then
    if metrics=$(query 'f1_replay_websocket_connections{job="replay"}'); then
      read -r connections busiest < <(printf '%s' "$metrics" | python3 -c '
import json, sys
pods = json.load(sys.stdin)["data"]["result"]
counts = {pod["metric"]["pod"]: int(float(pod["value"][1])) for pod in pods}
print(sum(counts.values()), max(counts, key=counts.get, default="none"))
')
      if (( connections >= 200 )) && [[ "$busiest" != "none" ]]; then
        victim=$busiest
        printf '%s\t%s\t%s\n' "$connections" 4 "$victim" >"$output/fault.tsv"
        echo "Prometheus observed $connections viewers after scale-up to four pods."
        break
      fi
    fi
  fi
  sleep 1
done

if [[ -z "$victim" ]]; then
  echo "The deployment did not reach four available pods with at least 200 connected viewers." >&2
  echo "Partial results: $output" >&2
  exit 1
fi

echo "Deleting serving pod $victim to trigger reconnects."
"${kube[@]}" delete pod "$victim" --grace-period=0 --force --wait=false

result=0
wait "$load_pid" || result=$?
load_pid=
cat "$output/k6.log"
echo "Results: $output"
exit "$result"
