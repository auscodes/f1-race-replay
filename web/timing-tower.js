import { latestAt } from "./race-context.js";
import { lapLabel } from "./race-review.js";

const unknown = "—";
const statusLabels = {
  dnf: ["DNF", "Did not finish"],
  dns: ["DNS", "Did not start"],
  dsq: ["DSQ", "Disqualified"],
  nc: ["NC", "Not classified"],
};
const compoundCodes = { SOFT: "S", MEDIUM: "M", HARD: "H", INTERMEDIATE: "I", WET: "W" };

export function teamColor(gridDriver) {
  return /^[\da-f]{6}$/i.test(gridDriver?.color ?? "") ? `#${gridDriver.color}` : "#c8bcb9";
}

function gapLabel(value) {
  if (value == null || value === "" || value === "Unavailable") return unknown;
  const seconds = Number(value);
  if (Number.isFinite(seconds)) return seconds >= 0 ? `+${seconds.toFixed(3)}` : unknown;
  return /^\+\d+ laps?$/i.test(value) ? value.toLowerCase() : unknown;
}

export function driverSnapshot(session, data, elapsed, useInterval = false) {
  const classified = elapsed >= session.durationMs - session.intervalMs;
  const outcome = data?.outcome;
  const status = outcome && outcome.elapsedMs <= elapsed ? outcome.status : undefined;
  const at = status === "finished" ? outcome.elapsedMs : elapsed;
  const position = classified ? outcome?.position : Number(latestAt(data?.positions, at)?.value);
  const sample = latestAt(useInterval ? data?.intervals : data?.gaps, at);
  const value = classified ? outcome?.gap : sample && at - sample.elapsedMs <= 15_000 ? sample.value : undefined;
  const retired = status && status !== "finished";
  const gap = retired ? unknown : position === 1 ? "Leader" : gapLabel(value);
  const lastLap = latestAt(data?.completedLaps, at);
  const lap = lastLap ? lastLap.lap + (status ? 0 : 1) : Number(latestAt(data?.laps, at)?.value);
  const stint = retired ? undefined : data?.tyres?.find(({lapStart, lapEnd}) => lapStart <= lap && lap <= lapEnd);
  return {position, gap, lastLap, lap, stint, status};
}

function setText(node, value) {
  if (node.textContent !== value) node.textContent = value;
}

export function setupTimingTower(session, focusDriver) {
  const tower = document.querySelector("#timing-tower");
  const body = document.querySelector("#tower-rows");
  const mode = document.querySelector("#gap-mode");
  const gapHeading = document.querySelector("#tower-gap");
  const message = document.querySelector("#tower-message");
  const rows = session.drivers.map((driver) => {
    const grid = session.grid[driver];
    const name = session.driverNames?.[driver] ?? `Car ${driver}`;
    const row = body.insertRow();
    row.dataset.driver = driver;
    const position = row.insertCell();
    const driverCell = document.createElement("th");
    driverCell.scope = "row";
    const button = document.createElement("button");
    button.type = "button";
    button.className = "track-driver";
    button.setAttribute("aria-pressed", "false");
    button.setAttribute("aria-label", `Highlight ${name} on the map`);
    button.title = grid?.team ? `${name} · ${grid.team}` : name;
    button.style.setProperty("--team", teamColor(grid));
    button.textContent = grid?.code || driver;
    driverCell.append(button);
    row.append(driverCell);
    const gap = row.insertCell(), tyre = row.insertCell(), lap = row.insertCell();
    const status = row.insertCell();
    const flag = document.createElement("span");
    flag.className = "finish-flag";
    flag.setAttribute("role", "img");
    flag.setAttribute("aria-label", "Finished");
    flag.title = "Finished";
    flag.hidden = true;
    const result = document.createElement("abbr");
    status.append(flag, result);
    for (const cell of [position, gap, tyre, lap]) cell.textContent = unknown;
    return {driver, row, button, position, gap, tyre, lap, flag, result};
  });

  let selected, elapsed = 0, timing;
  let preference = mode.value;
  let order = session.drivers.join(",");
  tower.hidden = false;

  for (const {driver, button} of rows) {
    button.addEventListener("click", () => focusDriver(selected === driver ? undefined : driver));
  }

  function setFocus(driver) {
    selected = driver;
    for (const entry of rows) {
      const active = selected === entry.driver;
      entry.button.setAttribute("aria-pressed", String(active));
      entry.row.classList.toggle("is-tracked", active);
    }
  }

  function update(time) {
    elapsed = time;
    if (!timing) return;
    const classified = elapsed >= session.durationMs - session.intervalMs;
    const useInterval = preference === "interval" && !classified;
    mode.value = useInterval ? "interval" : "leader";
    mode.disabled = classified;
    setText(gapHeading, useInterval ? "Interval" : "Gap");
    const ranked = rows.map((entry) => {
      const {position, gap, lastLap, lap, stint, status} =
        driverSnapshot(session, timing.drivers[entry.driver], elapsed, useInterval);
      setText(entry.position, position > 0 ? String(position) : unknown);
      setText(entry.gap, gap);
      setText(entry.lap, lapLabel(lastLap));
      const code = compoundCodes[stint?.compound];
      if (code) {
        const age = stint.startingAge == null ? undefined : stint.startingAge + lap - stint.lapStart;
        setText(entry.tyre, age === undefined ? code : `${code} ${age}`);
        entry.tyre.dataset.compound = code;
        entry.tyre.title = age === undefined ? stint.compound : `${stint.compound} · ${age} laps of use at start of lap`;
      } else {
        setText(entry.tyre, unknown);
        delete entry.tyre.dataset.compound;
        entry.tyre.removeAttribute("title");
      }
      entry.flag.hidden = status !== "finished";
      const [label, title] = statusLabels[status] ?? ["", ""];
      setText(entry.result, label);
      entry.result.title = title;
      return { rank: position > 0 ? position : Infinity, ...entry };
    }).sort((a, b) => a.rank - b.rank);
    const next = ranked.map(({driver}) => driver).join(",");
    if (next !== order) {
      const active = document.activeElement;
      const focused = body.contains(active);
      for (const {row} of ranked) body.append(row);
      if (focused) active.focus({preventScroll: true});
      order = next;
    }
  }

  mode.addEventListener("change", () => {
    preference = mode.value;
    update(elapsed);
  });
  return {
    update,
    setFocus,
    setTiming(timeline) {
      timing = timeline;
      message.hidden = true;
      update(elapsed);
    },
    showError() { message.textContent = "Race timing could not load. Refresh to retry."; },
  };
}
