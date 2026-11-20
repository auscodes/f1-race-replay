import { startAnalysis } from "./analysis.js";
import { setupRaceReview } from "./race-review.js";
import { setupTimingTower, teamColor } from "./timing-tower.js";

const canvas = document.querySelector("#circuit");
const connection = document.querySelector("#connection");
const elapsed = document.querySelector("#elapsed");
const progress = document.querySelector("#progress");
const sequence = document.querySelector("#sequence");
const inspector = document.querySelector("#inspector");
const capture = document.querySelector("#capture");
const payload = document.querySelector("#payload");

// Keep the data and drawing in the same 800 × 480 coordinate space. The
// canvas resolution changes with screen size without moving any positions.
const mapWidth = 800;
const mapHeight = 480;
const palette = ["#ff856b", "#72e4c7"];

function raceTime(milliseconds) {
  const minutes = Math.floor(milliseconds / 60000);
  const seconds = (milliseconds % 60000) / 1000;
  return `${minutes}:${seconds.toFixed(1).padStart(4, "0")}`;
}

function drawMap(track, colors, grid) {
  const ctx = canvas.getContext("2d");
  const outline = new Path2D();
  const [start, next] = track;
  outline.moveTo(start.x, start.y);
  for (const point of track.slice(1)) outline.lineTo(point.x, point.y);
  outline.closePath();

  return function draw(positions = [], focused) {
    const displayWidth = canvas.clientWidth;
    if (displayWidth === 0) return;
    const width = Math.round(displayWidth * window.devicePixelRatio);
    const height = Math.round(canvas.clientHeight * window.devicePixelRatio);
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width;
      canvas.height = height;
    }
    ctx.setTransform(width / mapWidth, 0, 0, height / mapHeight, 0, 0);
    ctx.clearRect(0, 0, mapWidth, mapHeight);

    ctx.lineJoin = "round";
    ctx.strokeStyle = "#b7a9a3";
    ctx.lineWidth = 25;
    ctx.stroke(outline);
    ctx.strokeStyle = "#413737";
    ctx.lineWidth = 22;
    ctx.stroke(outline);

    ctx.save();
    ctx.translate(start.x, start.y);
    ctx.rotate(Math.atan2(next.y - start.y, next.x - start.x));
    for (let i = 0; i < 6; i++) {
      ctx.fillStyle = i % 2 === 0 ? "#f4eee5" : "#211a1b";
      ctx.fillRect(0, i * 4 - 12, 4, 4);
      ctx.fillStyle = i % 2 === 0 ? "#211a1b" : "#f4eee5";
      ctx.fillRect(4, i * 4 - 12, 4, 4);
    }
    ctx.restore();
    const labelScale = Math.max(1, 640 / displayWidth);
    ctx.font = `${14 * labelScale}px ui-monospace, monospace`;
    ctx.fillStyle = "#c3b5b0";
    ctx.textAlign = "center";
    ctx.fillText("START / FINISH", start.x + 4, start.y + 43);

    const tracked = focused && positions.find(({driver}) => driver === focused);
    const cars = tracked ? [...positions.filter(({driver}) => driver !== focused), tracked] : positions;
    for (const { driver, x, y } of cars) {
      const showNumber = !grid || driver === focused;
      ctx.beginPath();
      ctx.arc(x, y, (showNumber ? 13 * labelScale : 6), 0, 2 * Math.PI);
      ctx.fillStyle = colors.get(driver);
      ctx.fill();
      ctx.lineWidth = 3;
      ctx.strokeStyle = driver === focused ? "#fff" : "#211a1b";
      ctx.stroke();
      if (showNumber) {
        ctx.fillStyle = grid ? "#fff" : "#211a1b";
        if (grid) {
          ctx.beginPath();
          ctx.arc(x, y, 10 * labelScale, 0, 2 * Math.PI);
          ctx.fillStyle = "#211a1b";
          ctx.fill();
          ctx.fillStyle = "#fff";
        }
        ctx.font = `bold ${12 * labelScale}px ui-monospace, monospace`;
        ctx.fillText(driver, x, y + 4 * labelScale);
      }
    }
  };
}

function connect(onFrame) {
  const url = new URL("/ws", window.location.href);
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const socket = new WebSocket(url);

  socket.addEventListener("message", (event) => {
    if (connection.dataset.state !== "connected") {
      connection.dataset.state = "connected";
      connection.textContent = "Receiving replay";
      capture.disabled = false;
    }
    onFrame(JSON.parse(event.data));
  });
  socket.addEventListener("close", () => {
    connection.dataset.state = "reconnecting";
    connection.textContent = "Connection lost. Retrying…";
    capture.disabled = true;
    setTimeout(() => connect(onFrame), 1000);
  });
}

async function start() {
  const response = await fetch("/api/session");
  if (!response.ok) throw new Error(`Session request failed (${response.status})`);
  const session = await response.json();
  if (session.timeline) document.body.classList.add("race-review");
  document.querySelector("#circuit-name").textContent = session.name;
  document.querySelector("#recording-caption").textContent = session.caption || "Synthetic recording";
  document.querySelector("#duration").textContent = `/ ${raceTime(session.durationMs)}`;
  progress.max = session.durationMs;

  const drivers = document.querySelector("#drivers");
  const colors = new Map();
  const timing = new Map();
  const grid = session.grid;
  if (grid) {
    document.body.classList.add("full-grid");
    document.querySelector("#compact-drivers").hidden = true;
  }
  session.drivers.forEach((driver, i) => {
    const color = grid ? teamColor(grid[driver]) : palette[i % palette.length];
    colors.set(driver, color);
    if (grid) return;
    const item = document.createElement("li");
    const marker = document.createElement("span");
    marker.className = "driver-marker";
    marker.style.backgroundColor = color;
    marker.setAttribute("aria-hidden", "true");
    item.append(marker, session.driverNames?.[driver] || `Car ${driver}`);
    if (session.timeline?.drivers?.[driver]) {
      item.className = "driver-data";
      const details = document.createElement("small");
      details.textContent = "Timing unavailable";
      timing.set(driver, details);
      item.append(details);
    }
    drivers.append(item);
  });

  const draw = drawMap(session.track, colors, grid);
  let review = session.timeline && !grid ? setupRaceReview(session) : undefined;
  let latestFrame, capturedFrame, focused, jump;
  let requestedTiming = false;
  const tower = grid ? setupTimingTower(session, (driver) => {
    focused = driver;
    draw((capturedFrame ?? latestFrame)?.positions, focused);
  }) : undefined;

  async function loadTiming() {
    try {
      const response = await fetch("/api/timing");
      if (!response.ok) throw new Error(`Timing request failed (${response.status})`);
      session.timeline = await response.json();
      tower.setTiming(session.timeline);
      review = setupRaceReview(session, session.drivers.slice(0, 2));
      if (jump) review.setJump(jump);
      showFrame(capturedFrame ?? latestFrame);
    } catch {
      tower.showError();
    }
  }

  function showFrame(frame) {
    if (!frame) return;
    elapsed.textContent = raceTime(frame.elapsedMs);
    sequence.textContent = frame.sequence;
    progress.value = frame.elapsedMs;
    draw(frame.positions, focused);
    tower?.update(frame.elapsedMs);
    review?.update(frame.elapsedMs);
    for (const [driver, details] of timing) {
      const data = session.timeline.drivers[driver];
      const at = (records) => records?.findLast((record) => record.elapsedMs <= frame.elapsedMs);
      const lap = at(data.laps)?.value;
      const position = at(data.positions)?.value;
      const gap = at(data.gaps);
      const hasGap = gap && frame.elapsedMs - gap.elapsedMs <= 15_000 && gap.value !== "Unavailable";
      const gapLabel = hasGap ? (Number.isFinite(Number(gap.value)) ? `${gap.value}s` : gap.value) : "—";
      const gapText = position === "1" ? "Leader" : `Gap to leader ${gapLabel}`;
      const missing = frame.positions.some((car) => car.driver === driver) ? "" : " · Map position unavailable";
      details.textContent = `P${position ?? "—"} · Lap ${lap ?? "—"} · ${gapText}${missing}`;
    }
  }

  function captureFrame() {
    if (!latestFrame) return;
    capturedFrame = latestFrame;
    payload.textContent = JSON.stringify(capturedFrame, null, 2);
    showFrame(capturedFrame);
  }

  new ResizeObserver(() => draw((capturedFrame ?? latestFrame)?.positions, focused)).observe(canvas);
  capture.addEventListener("click", captureFrame);
  inspector.addEventListener("toggle", () => {
    if (inspector.open) {
      captureFrame();
    } else {
      capturedFrame = undefined;
      showFrame(latestFrame);
    }
  });

  function receiveFrame(frame) {
    capture.disabled = false;
    latestFrame = frame;
    if (!inspector.open) showFrame(frame);
    else if (!capturedFrame) captureFrame();
    if (tower && !requestedTiming) {
      requestedTiming = true;
      void loadTiming();
    }
  }

  if (session.timeline) {
    const shared = new URLSearchParams(location.search).get("mode") === "shared";
    const link = document.querySelector("#mode-link");
    link.href = shared ? "?mode=analysis" : "?mode=shared";
    link.textContent = shared ? "Analyze on your own" : "Join shared playback";
    link.hidden = false;
    if (!shared) {
      document.querySelector("#playback-mode").textContent = "Local analysis";
      progress.hidden = true;
      jump = startAnalysis(session, receiveFrame, (message, state = "loading") => {
        if (connection.dataset.state === state && connection.textContent === message) return;
        connection.dataset.state = state;
        connection.textContent = message;
      });
      review?.setJump(jump);
      return;
    }
  }
  connection.textContent = "Connecting to replay";
  connect(receiveFrame);
}

start().catch(() => {
  connection.dataset.state = "error";
  connection.textContent = "Could not load the recording. Refresh to retry.";
});
