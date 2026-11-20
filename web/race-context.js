export function latestAt(records, elapsed) {
  let low = 0, high = records?.length ?? 0;
  while (low < high) {
    const mid = Math.floor((low + high) / 2);
    if (records[mid].elapsedMs <= elapsed) low = mid + 1;
    else high = mid;
  }
  return records?.[low - 1];
}

export function leaderAt(drivers, elapsed) {
  let leader, lastChange = -1;
  for (const [id, driver] of Object.entries(drivers ?? {})) {
    const position = latestAt(driver.positions, elapsed);
    if (position?.value === "1" && position.elapsedMs > lastChange) {
      leader = id;
      lastChange = position.elapsedMs;
    }
  }
  return leader;
}

const stateLabels = {
  green: "Green flag",
  vsc: "Virtual Safety Car",
  "vsc-ending": "Virtual Safety Car ending",
  chequered: "Chequered flag",
};

export function setupRaceContext(session) {
  const strip = document.querySelector("#race-state");
  const lapLabel = document.querySelector("#race-lap");
  const control = document.querySelector("#race-control");
  return (elapsed) => {
    const {raceStates, totalLaps, drivers} = session.timeline;
    const state = latestAt(raceStates, elapsed)?.value;
    const label = stateLabels[state];
    if (label && control.dataset.state !== state) {
      control.dataset.state = state;
      control.textContent = label;
    }
    strip.hidden = !label;
    if (!label || !totalLaps) return;

    const raceLap = latestAt(session.timeline.raceLaps, elapsed);
    const leader = raceLap ? undefined : drivers?.[leaderAt(drivers, elapsed)];
    const current = Number(raceLap?.value ?? latestAt(leader?.laps, elapsed)?.value);
    const lap = state === "chequered" ? totalLaps : Math.min(current, totalLaps);
    if (Number.isFinite(lap) && lap > 0) {
      const text = `Lap ${lap} / ${totalLaps}`;
      if (lapLabel.textContent !== text) lapLabel.textContent = text;
      lapLabel.hidden = false;
    } else {
      lapLabel.hidden = true;
    }
  };
}
