import { leaderAt } from "./race-context.js";
import { lapLabel } from "./race-review.js";
import { driverSnapshot } from "./timing-tower.js";

const width = 800;
const height = 480;
const sectorColors = ["#75cfc2", "#e4b26c", "#c0aef0"];

function projection(track, rotate) {
  if (!rotate) return (point) => point;
  const angle = Math.PI / 3;
  const cos = Math.cos(angle), sin = Math.sin(angle);
  const transform = ({x, y}) => ({x: cos * x - sin * y, y: sin * x + cos * y});
  const points = track.map(transform);
  const left = Math.min(...points.map(({x}) => x)), right = Math.max(...points.map(({x}) => x));
  const top = Math.min(...points.map(({y}) => y)), bottom = Math.max(...points.map(({y}) => y));
  const scale = Math.min(628 / (right - left), 278 / (bottom - top));
  return (point) => {
    const {x, y} = transform(point);
    return {
      x: width / 2 + (x - (left + right) / 2) * scale,
      y: height / 2 + (y - (top + bottom) / 2) * scale,
    };
  };
}

function trace(points, closed = false) {
  const path = new Path2D();
  path.moveTo(points[0].x, points[0].y);
  for (const {x, y} of points.slice(1)) path.lineTo(x, y);
  if (closed) path.closePath();
  return path;
}

export function setupCircuitMap(session, colors, onFocus) {
  const canvas = document.querySelector("#circuit");
  const ctx = canvas.getContext("2d");
  const picker = document.querySelector("#map-driver");
  const driverName = document.querySelector("#map-driver-name");
  const stats = document.querySelector("#map-driver-stats");
  const grid = session.grid;
  const project = projection(session.track, Boolean(grid));
  const track = session.track.map(project);
  const outline = trace(track, true);
  const breaks = session.sectorBreaks;
  const hasSectors = breaks?.length === 2 && Number.isInteger(breaks[0]) && Number.isInteger(breaks[1]) &&
    breaks[0] > 0 && breaks[0] < breaks[1] && breaks[1] < track.length - 1;
  const sectors = hasSectors ? [
    trace(track.slice(0, breaks[0] + 1)),
    trace(track.slice(breaks[0], breaks[1] + 1)),
    trace([...track.slice(breaks[1]), track[0]]),
  ] : [];
  let markers = [], frame, focused;

  if (grid) {
    document.querySelector("#map-toolbar").hidden = false;
    document.querySelector("#map-detail").hidden = false;
    document.querySelector("#map-sectors").hidden = !hasSectors;
    for (const driver of session.drivers) {
      picker.add(new Option(session.driverNames?.[driver] ?? `Car ${driver}`, driver));
    }
    picker.addEventListener("change", () => onFocus(picker.value || undefined));
    canvas.addEventListener("click", (event) => {
      const rect = canvas.getBoundingClientRect();
      let nearest, best = 17 ** 2;
      for (const marker of markers) {
        const dx = rect.left + marker.x * rect.width / width - event.clientX;
        const dy = rect.top + marker.y * rect.height / height - event.clientY;
        const distance = dx ** 2 + dy ** 2;
        if (distance < best || (distance === best && marker.driver === focused)) {
          nearest = marker.driver;
          best = distance;
        }
      }
      if (nearest) onFocus(nearest);
    });
  }

  function startFinish(labelScale) {
    const start = track[0], ahead = track[Math.min(5, track.length - 1)];
    ctx.save();
    ctx.translate(start.x, start.y);
    ctx.rotate(Math.atan2(ahead.y - start.y, ahead.x - start.x));
    for (let row = 0; row < 6; row++) {
      for (let column = 0; column < 2; column++) {
        ctx.fillStyle = (row + column) % 2 === 0 ? "#fcf4ee" : "#211a1b";
        ctx.fillRect(column * 4 - 4, row * 4 - 12, 4, 4);
      }
    }
    if (grid) {
      ctx.strokeStyle = "#faf2eb";
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(16, -4);
      ctx.lineTo(20, 0);
      ctx.lineTo(16, 4);
      ctx.stroke();
    }
    ctx.restore();
    ctx.fillStyle = "#d4c8c4";
    ctx.textAlign = "center";
    ctx.font = `600 ${12 * labelScale}px ui-monospace, monospace`;
    ctx.fillText("START / FINISH", start.x, start.y + 24 + 9 * labelScale);
  }

  function markerLabel(marker, scale, color) {
    const text = grid[marker.driver]?.code ?? marker.driver;
    ctx.font = `700 ${13 * scale}px ui-monospace, monospace`;
    const labelWidth = ctx.measureText(text).width + 15 * scale;
    const boxX = Math.min(width - labelWidth - 6, marker.x + 10 * scale);
    const boxY = Math.max(4, marker.y - 29 * scale);
    ctx.beginPath();
    ctx.roundRect(boxX, boxY, labelWidth, 22 * scale, 4 * scale);
    ctx.fillStyle = "#171215";
    ctx.fill();
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.2 * scale;
    ctx.stroke();
    ctx.fillStyle = "#fff6f0";
    ctx.textAlign = "left";
    ctx.fillText(text, boxX + 7 * scale, boxY + 15 * scale);
  }

  function draw() {
    const displayWidth = canvas.clientWidth;
    if (!displayWidth) return;
    const pixelWidth = Math.round(displayWidth * window.devicePixelRatio);
    const pixelHeight = Math.round(canvas.clientHeight * window.devicePixelRatio);
    if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
      canvas.width = pixelWidth;
      canvas.height = pixelHeight;
    }
    ctx.setTransform(pixelWidth / width, 0, 0, pixelHeight / height, 0, 0);
    ctx.clearRect(0, 0, width, height);
    ctx.lineCap = ctx.lineJoin = "round";
    ctx.strokeStyle = "#a09290";
    ctx.lineWidth = 25;
    ctx.stroke(outline);
    ctx.strokeStyle = "#443a3c";
    ctx.lineWidth = 22;
    ctx.stroke(outline);
    sectors.forEach((route, index) => {
      ctx.strokeStyle = sectorColors[index];
      ctx.lineWidth = 3.2;
      ctx.stroke(route);
    });

    const labelScale = Math.max(1, 690 / displayWidth);
    const lead = grid && frame ? leaderAt(session.timeline?.drivers, frame.elapsedMs) : undefined;
    const highlighted = focused ?? lead;
    markers = (frame?.positions ?? []).map(({driver, ...point}) => ({driver, ...project(point)}));
    startFinish(labelScale);
    if (grid) describe(highlighted);

    // Draw the highlighted car last so it stays visible when the field bunches up.
    const selected = markers.find(({driver}) => driver === highlighted);
    const ordered = selected ? [...markers.filter((marker) => marker !== selected), selected] : markers;
    for (const marker of ordered) {
      const {driver, x, y} = marker;
      const active = marker === selected;
      const color = colors.get(driver) ?? "#ede4dd";
      ctx.beginPath();
      ctx.arc(x, y, grid ? (active ? 8 : 5) * Math.sqrt(labelScale) : 13 * labelScale, 0, 2 * Math.PI);
      ctx.fillStyle = color;
      ctx.fill();
      ctx.lineWidth = active ? 3 : 2;
      ctx.strokeStyle = active ? "#fff6f0" : "#211a1b";
      ctx.stroke();
      if (active && grid) markerLabel(marker, labelScale, color);
      if (!grid) {
        ctx.font = `bold ${12 * labelScale}px ui-monospace, monospace`;
        ctx.textAlign = "center";
        ctx.fillStyle = "#211a1b";
        ctx.fillText(driver, x, y + 4 * labelScale);
      }
    }
  }

  function describe(driver) {
    const name = driver ? session.driverNames?.[driver] ?? `Car ${driver}` : "Race leader";
    const timing = session.timeline?.drivers?.[driver];
    let details = "";
    if (frame && driver) {
      const {position, lap, gap, stint, lastLap} = driverSnapshot(session, timing, frame.elapsedMs);
      details = [
        position > 0 ? `P${position}` : "",
        gap !== "—" && gap !== "Leader" ? (gap.includes("lap") ? gap : `${gap} s`) : "",
        lap > 0 ? `Lap ${lap}` : "",
        stint?.compound ? `${stint.compound[0]}${stint.compound.slice(1).toLowerCase()} tyres` : "",
        lastLap ? `Last lap ${lapLabel(lastLap)}` : "",
        markers.some((marker) => marker.driver === driver) ? "" : "Map position unavailable",
      ].filter(Boolean).join(" · ");
    }
    if (driverName.textContent !== name) driverName.textContent = name;
    if (stats.textContent !== details) stats.textContent = details;
  }

  new ResizeObserver(draw).observe(canvas);
  return {
    update(nextFrame, selection) {
      frame = nextFrame;
      if (focused !== selection) picker.value = selection ?? "";
      focused = selection;
      draw();
    },
  };
}
