import http from "k6/http";
import { Counter, Rate, Trend } from "k6/metrics";
import { WebSocket } from "k6/websockets";

const base = __ENV.REPLAY_URL || "http://127.0.0.1:18089";
const viewers = Number(__ENV.REPLAY_VIEWERS || 220);
const holdSeconds = Number(__ENV.REPLAY_HOLD_SECONDS || 90);
const expectFailure = __ENV.REPLAY_EXPECT_FAILURE !== "0";
const started = new Counter("replay_viewers_started");
const interrupted = new Counter("replay_interruptions");
const recovered = new Rate("replay_recovered_within_10s");
const outage = new Trend("replay_recovery_ms", true);
const valid = new Rate("replay_frames_valid");

export const options = {
  scenarios: {
    replay: {
      executor: "per-vu-iterations",
      vus: viewers,
      iterations: 1,
      maxDuration: `${holdSeconds + 15}s`,
    },
  },
  thresholds: {
    iterations: [`count>=${viewers}`],
    replay_viewers_started: [`count>=${viewers}`],
    replay_frames_valid: ["rate==1"],
    ...(expectFailure ? {
      replay_interruptions: ["count>0"],
      replay_recovered_within_10s: ["rate==1"],
      replay_recovery_ms: ["max<=10000"],
    } : {}),
  },
  summaryTrendStats: ["avg", "p(95)", "max"],
};

export function setup() {
  const response = http.get(`${base}/api/session`, {timeout: "5s"});
  if (response.status !== 200) throw new Error(`session request returned ${response.status}`);
  const {intervalMs, durationMs} = response.json();
  if (!intervalMs || durationMs < intervalMs) throw new Error("invalid recording metadata");
  return {intervalMs, frames: durationMs / intervalMs};
}

export default async function ({intervalMs, frames}) {
  const endpoint = base.replace(/^http/, "ws") + "/ws";
  const finishAt = Date.now() + holdSeconds * 1000;
  let lastFrame, interruptedAt;

  function attempt() {
    return new Promise((resolve) => {
      const socket = new WebSocket(endpoint);
      let completed = false;
      let open = false;
      let watchdog = setTimeout(() => stop(false), 3000);
      const timer = setTimeout(() => stop(true), Math.max(1, finishAt - Date.now()));

      function stop(scheduled) {
        if (completed) return;
        completed = true;
        clearTimeout(timer);
        clearTimeout(watchdog);
        if (!scheduled && lastFrame && interruptedAt === undefined) {
          // Measure the visible gap between valid frames, including close
          // detection and reconnect time.
          interruptedAt = lastFrame.at;
          interrupted.add(1);
        }
        if (open) socket.close();
        resolve();
      }

      // Retry independently of a pending k6 handshake and close that socket
      // if it later opens.
      socket.addEventListener("open", () => {
        open = true;
        if (completed) socket.close();
      });
      socket.addEventListener("message", ({data}) => {
        if (completed) return;
        const now = Date.now();
        let frame;
        try {
          frame = JSON.parse(data);
        } catch {
          valid.add(false);
          return;
        }
        const {sequence, elapsedMs, positions} = frame ?? {};
        const correct = Number.isSafeInteger(sequence) && sequence >= 0 &&
          elapsedMs === (sequence % frames) * intervalMs && Array.isArray(positions);
        valid.add(correct);
        if (!correct) return;
        clearTimeout(watchdog);
        watchdog = setTimeout(() => stop(false), 3000);

        if (interruptedAt !== undefined) {
          const expected = lastFrame.sequence + (now - lastFrame.at) / intervalMs;
          // Allow up to one second of jitter when comparing two server clocks
          // from the load generator's arrival times.
          if (Math.abs(sequence - expected) * intervalMs > 1000) return;
          const gap = now - interruptedAt;
          outage.add(gap);
          recovered.add(gap <= 10_000);
          interruptedAt = undefined;
        } else if (!lastFrame) {
          started.add(1);
        }
        lastFrame = {sequence, at: now};
      });

      socket.addEventListener("close", () => {
        open = false;
        stop(false);
      });
    });
  }

  while (Date.now() < finishAt) {
    await attempt();
    if (Date.now() < finishAt) await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  if (interruptedAt !== undefined) recovered.add(false);
}
