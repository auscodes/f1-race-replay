function clockLabel(milliseconds) {
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = String(Math.floor(milliseconds % 60_000 / 1000)).padStart(2, "0");
  return `${minutes}:${seconds}`;
}

export function startAnalysis(session, receiveFrame, setStatus) {
  const scrub = document.querySelector("#scrub");
  const play = document.querySelector("#play");
  const speed = document.querySelector("#speed");
  const lapJump = document.querySelector("#lap-jump");
  const eventJump = document.querySelector("#event-jump");
  const inspector = document.querySelector("#inspector");
  const last = session.durationMs - session.intervalMs;
  scrub.max = last;
  scrub.step = session.intervalMs;

  lapJump.add(new Option("Choose lap", ""));
  for (const lap of session.timeline.drivers?.[session.drivers[0]]?.laps ?? []) {
    lapJump.add(new Option(`Lap ${lap.value}`, lap.elapsedMs));
  }
  eventJump.add(new Option("Choose event", ""));
  for (const event of session.timeline.events ?? []) {
    eventJump.add(new Option(`${event.label} · ${clockLabel(event.elapsedMs)}`, event.elapsedMs));
    const mark = document.createElement("span");
    mark.className = event.kind;
    mark.style.left = `${100 * event.elapsedMs / session.durationMs}%`;
    document.querySelector("#event-marks").append(mark);
  }

  const segments = new Map();
  const pending = new Map();

  function loadSegment(index) {
    if (!pending.has(index)) {
      pending.set(index, fetch(`/api/segments/${index}`).then(async (response) => {
        if (!response.ok) throw new Error("Segment unavailable");
        const segment = await response.json();
        segments.set(index, segment);
        if (segments.size > 5) segments.delete(segments.keys().next().value);
        return segment;
      }).finally(() => pending.delete(index)));
    }
    return pending.get(index);
  }

  let cursor = 0;
  let shownIndex = -1;
  let lastUpdate = performance.now();
  let playing = false;
  let buffering = false;
  let generation = 0;

  function setPlaying(value) {
    playing = value;
    play.textContent = value ? "Pause" : "Play";
    lastUpdate = performance.now();
  }

  async function showAt(time) {
    cursor = Math.min(last, Math.max(0, time));
    const index = Math.floor(cursor / session.intervalMs);
    const segmentIndex = Math.floor(index / session.segmentSize);
    const request = ++generation;
    let segment = segments.get(segmentIndex);
    if (!segment) {
      buffering = true;
      setStatus("Loading frames…");
      try {
        segment = await loadSegment(segmentIndex);
      } catch {
        if (request !== generation) return;
        buffering = false;
        setPlaying(false);
        setStatus("Could not load this part of the race. Seek or press Play to retry.", "error");
        return;
      }
      if (request !== generation) return;
    }
    buffering = false;
    setStatus("Local analysis", "connected");
    lastUpdate = performance.now();
    shownIndex = index;
    const elapsedMs = index * session.intervalMs;
    scrub.value = elapsedMs;
    scrub.setAttribute("aria-valuetext", clockLabel(elapsedMs));
    receiveFrame({sequence: index, elapsedMs, positions: segment.frames[index - segment.startFrame]});

    const nextSegment = segmentIndex + 1;
    if (index % session.segmentSize >= session.segmentSize * 0.8 &&
        nextSegment * session.segmentSize * session.intervalMs <= last &&
        !segments.has(nextSegment) && !pending.has(nextSegment)) {
      loadSegment(nextSegment).catch(() => {});
    }
    if (cursor === last) setPlaying(false);
  }

  function seek(time) {
    inspector.open = false;
    setPlaying(false);
    showAt(time);
  }

  function jump(time) {
    const frame = Math.ceil(time / session.intervalMs);
    seek(frame * session.intervalMs);
  }

  scrub.addEventListener("change", () => seek(Number(scrub.value)));
  for (const select of [lapJump, eventJump]) {
    select.addEventListener("change", () => {
      if (select.value !== "") jump(Number(select.value));
      select.value = "";
    });
  }
  play.addEventListener("click", () => {
    if (playing) {
      setPlaying(false);
      return;
    }
    inspector.open = false;
    setPlaying(true);
    if (!buffering) showAt(cursor === last && shownIndex === last / session.intervalMs ? 0 : cursor);
  });
  speed.addEventListener("change", () => { lastUpdate = performance.now(); });
  setInterval(() => {
    if (!playing || buffering) return;
    const next = Math.min(last, cursor + (performance.now() - lastUpdate) * Number(speed.value));
    if (Math.floor(next / session.intervalMs) !== shownIndex) showAt(next);
  }, 40);

  document.querySelector("#analysis-controls").hidden = false;
  showAt(0);
  return jump;
}
