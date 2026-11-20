const chartColors = ["#ab3d30", "#167469"];

function clockLabel(ms) {
  const seconds = Math.floor(ms / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

export function lapLabel(lap) {
  if (!lap) return "—";
  const fraction = String(lap.durationMs % 1000).padStart(3, "0");
  return `${clockLabel(lap.durationMs)}.${fraction}`;
}

function driverName(session, driver) {
  return session.driverNames?.[driver] || `Car ${driver}`;
}

function gapChart(session) {
  const canvas = document.querySelector("#gap-chart");
  const legend = document.querySelector("#gap-legend");
  const panel = document.querySelector("#gap-panel");
  const ctx = canvas.getContext("2d");
  let series = [], tick = 5, current = 0, lastCursor = -1;

  function setDrivers(drivers) {
    series = drivers.map((driver, index) => {
      const timing = session.timeline.drivers?.[driver];
      return {
        name: driverName(session, driver),
        color: chartColors[index % chartColors.length],
        positions: timing?.positions ?? [],
        points: (timing?.gaps ?? []).map(({elapsedMs, value}) => ({
          elapsedMs, gap: value === "" || Number(value) < 0 ? NaN : Number(value),
        })),
      };
    });
    const hasData = series.some(({points, positions}) =>
      points.some(({gap}) => Number.isFinite(gap)) || positions.some(({value}) => value === "1"));
    panel.hidden = !hasData;
    document.querySelector("#gap-unit").hidden = !hasData;
    document.querySelector("#gap-empty").hidden = hasData || !session.grid;

    legend.replaceChildren();
    let max = 0;
    for (const {name, color, points} of series) {
      const item = document.createElement("li");
      const swatch = document.createElement("span");
      swatch.style.background = color;
      item.append(swatch, name);
      legend.append(item);
      for (const {gap} of points) if (Number.isFinite(gap)) max = Math.max(max, gap);
    }
    tick = Math.max(5, Math.ceil(max / 20) * 5);
    lastCursor = -1;
    return hasData;
  }

  function draw() {
    const width = canvas.clientWidth, height = canvas.clientHeight;
    if (width < 60 || height === 0) return;
    const ratio = window.devicePixelRatio || 1;
    const resized = canvas.width !== Math.round(width * ratio) || canvas.height !== Math.round(height * ratio);
    const left = 32, right = width - 16, top = 10, bottom = height - 26;
    const x = (ms) => left + (right - left) * ms / session.durationMs;
    const y = (gap) => bottom - (bottom - top) * gap / (4 * tick);
    const cursor = Math.round(x(current));
    if (!resized && cursor === lastCursor) return;
    lastCursor = cursor;
    if (resized) {
      canvas.width = Math.round(width * ratio);
      canvas.height = Math.round(height * ratio);
    }
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);
    ctx.font = "11px system-ui, sans-serif";
    ctx.fillStyle = "#655654";
    ctx.strokeStyle = "#e6ddd9";
    ctx.lineWidth = 1;
    ctx.textAlign = "right";
    for (let i = 0; i <= 4; i++) {
      const row = y(i * tick);
      ctx.beginPath();
      ctx.moveTo(left, row);
      ctx.lineTo(right, row);
      ctx.stroke();
      ctx.fillText(String(i * tick), left - 7, row + 4);
    }
    ctx.textAlign = "center";
    for (let i = 0; i <= 4; i++) {
      const time = session.durationMs * i / 4;
      ctx.textAlign = i === 0 ? "left" : i === 4 ? "right" : "center";
      ctx.fillText(clockLabel(time), x(time), height - 7);
    }

    for (const {points, positions, color} of series) {
      ctx.beginPath();
      ctx.strokeStyle = color;
      ctx.lineWidth = 1.75;
      let previous;
      for (const point of points) {
        if (!Number.isFinite(point.gap)) {
          previous = undefined;
          continue;
        }
        if (previous && point.elapsedMs - previous.elapsedMs <= 15_000) ctx.lineTo(x(point.elapsedMs), y(point.gap));
        else ctx.moveTo(x(point.elapsedMs), y(point.gap));
        previous = point;
      }
      positions.forEach((position, index) => {
        if (position.value !== "1") return;
        ctx.moveTo(x(position.elapsedMs), y(0));
        ctx.lineTo(x(positions[index + 1]?.elapsedMs ?? session.durationMs), y(0));
      });
      ctx.stroke();
    }

    ctx.beginPath();
    ctx.strokeStyle = "#241b1d";
    ctx.lineWidth = 1;
    ctx.moveTo(cursor, top);
    ctx.lineTo(cursor, bottom);
    ctx.stroke();
  }

  new ResizeObserver(draw).observe(canvas);
  return {setDrivers, update(elapsed) { current = elapsed; draw(); }};
}

export function setupRaceReview(session) {
  const timeline = session.timeline;
  const chart = gapChart(session);
  const panel = document.querySelector("#comparison");
  const lapPanel = document.querySelector("#lap-panel");
  const lapRows = document.querySelector("#lap-rows");
  const pickers = document.querySelector("#comparison-pickers");
  const selects = [...pickers.querySelectorAll("select")];
  const canPick = Boolean(session.grid && session.drivers.length >= 2);
  let rows = [], current = 0, jump;

  function compare(drivers) {
    const hasGap = chart.setDrivers(drivers);
    lapRows.replaceChildren();
    rows = [];
    for (const driver of drivers) {
      const laps = timeline.drivers?.[driver]?.completedLaps ?? [];
      if (!canPick && laps.length === 0) continue;
      const row = lapRows.insertRow();
      const name = document.createElement("th");
      name.scope = "row";
      name.textContent = driverName(session, driver);
      row.append(name);
      rows.push({laps, latest: row.insertCell(), fastest: row.insertCell()});
    }
    lapPanel.hidden = rows.length === 0;
    panel.hidden = !canPick && !hasGap && rows.length === 0;
    updateComparison(current);
  }

  function updateComparison(elapsed) {
    chart.update(elapsed);
    // The last frame represents the remaining fraction of the recording.
    const completedBy = elapsed >= session.durationMs - session.intervalMs ? session.durationMs : elapsed;
    for (const {laps, latest, fastest} of rows) {
      let last, best;
      for (const lap of laps) {
        if (lap.elapsedMs > completedBy) break;
        last = lap;
        if (!best || lap.durationMs < best.durationMs) best = lap;
      }
      latest.textContent = lapLabel(last);
      fastest.textContent = lapLabel(best);
      if (best) fastest.title = `Lap ${best.lap}`;
      else fastest.removeAttribute("title");
    }
  }

  if (canPick) {
    function selectDrivers() {
      const drivers = selects.map((select) => select.value);
      selects.forEach((select, index) => {
        for (const option of select.options) option.disabled = option.value === drivers[1 - index];
      });
      compare(drivers);
    }

    selects.forEach((select, index) => {
      for (const driver of session.drivers) select.add(new Option(driverName(session, driver), driver));
      select.value = session.drivers[index];
      select.addEventListener("change", selectDrivers);
    });
    pickers.hidden = false;
    selectDrivers();
  } else {
    compare(session.drivers);
  }

  const list = document.querySelector("#race-events");
  const eventRows = (timeline.events ?? []).map((event) => {
    const item = document.createElement("li");
    const button = document.createElement("button");
    const time = document.createElement("span");
    const text = document.createElement("span");
    time.className = "event-time";
    time.textContent = clockLabel(event.elapsedMs);
    text.textContent = event.label;
    if (event.stopMs || event.laneMs) {
      const detail = document.createElement("small");
      const durations = [];
      if (event.stopMs) durations.push(`${(event.stopMs / 1000).toFixed(1)} s stationary`);
      if (event.laneMs) durations.push(`${(event.laneMs / 1000).toFixed(1)} s in pit lane`);
      detail.textContent = durations.join(" · ");
      text.append(detail);
    }
    button.type = "button";
    button.disabled = true;
    button.append(time, text);
    button.addEventListener("click", () => jump?.(event.elapsedMs));
    item.append(button);
    list.append(item);
    return {elapsed: event.elapsedMs, button};
  });
  document.querySelector("#events-panel").hidden = eventRows.length === 0;

  let active;
  return {
    setJump(callback) {
      jump = callback;
      for (const {button} of eventRows) button.disabled = false;
    },
    update(elapsed) {
      current = elapsed;
      updateComparison(elapsed);
      const next = eventRows.findLast((event) => event.elapsed <= elapsed)?.button;
      if (next !== active) {
        active?.removeAttribute("aria-current");
        next?.setAttribute("aria-current", "true");
        active = next;
      }
    },
  };
}
