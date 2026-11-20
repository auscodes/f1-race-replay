"""Build the Barcelona 2026 replay from OpenF1 position samples."""

import argparse
from bisect import bisect_left
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import json
from math import isfinite
from pathlib import Path
from tempfile import NamedTemporaryFile
from time import sleep
from urllib.parse import quote
from urllib.request import urlopen


SESSION_KEY = 11307
DRIVERS = {"44": "Lewis Hamilton", "63": "George Russell"}
CLIP_START = "2026-06-14T14:01:08.000+00:00"
CLIP_LENGTH_MS = 50_000
# Russell's lap 40 provides a complete circuit outline in the same coordinates.
TRACK_START = "2026-06-14T13:57:57.108+00:00"
TRACK_END = "2026-06-14T13:59:25.324+00:00"
INTERVAL_MS = 200
MAX_GAP_MS = 1_000
MAP_WIDTH, MAP_HEIGHT, MAP_PADDING = 800, 480, 55
EPOCH = datetime(1970, 1, 1, tzinfo=timezone.utc)
MILLISECOND = timedelta(milliseconds=1)


@dataclass(frozen=True)
class Sample:
    time_ms: int
    x: float
    y: float


def timestamp_ms(value):
    timestamp = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if timestamp.tzinfo is None:
        raise ValueError("position timestamps must include a time zone")
    return (timestamp - EPOCH) // MILLISECOND


def parse_samples(rows):
    by_time = {}
    for row in rows:
        x, y = row["x"], row["y"]
        if x is None or y is None or not (isfinite(x) and isfinite(y)):
            continue
        sample = Sample(timestamp_ms(row["date"]), x, y)
        if sample.time_ms in by_time and by_time[sample.time_ms] != sample:
            raise ValueError(f"conflicting positions at {row['date']}")
        by_time[sample.time_ms] = sample
    return sorted(by_time.values(), key=lambda sample: sample.time_ms)


def resample(samples, ticks):
    timestamps = [sample.time_ms for sample in samples]
    for tick in ticks:
        index = bisect_left(timestamps, tick)
        if index < len(samples) and timestamps[index] == tick:
            sample = samples[index]
            yield sample.x, sample.y
        elif 0 < index < len(samples):
            before, after = samples[index - 1], samples[index]
            gap = after.time_ms - before.time_ms
            if gap > MAX_GAP_MS:
                yield None
                continue
            fraction = (tick - before.time_ms) / gap
            yield before.x + fraction * (after.x - before.x), before.y + fraction * (after.y - before.y)
        else:
            yield None


def build_recording(track, observations, start_utc=CLIP_START, length_ms=CLIP_LENGTH_MS):
    if len(track) < 3:
        raise ValueError("the reference lap needs at least three position samples")
    left, right = min(point.x for point in track), max(point.x for point in track)
    bottom, top = min(point.y for point in track), max(point.y for point in track)
    if left == right or bottom == top:
        raise ValueError("the reference lap needs a two-dimensional track outline")
    scale = min((MAP_WIDTH - 2 * MAP_PADDING) / (right - left),
                (MAP_HEIGHT - 2 * MAP_PADDING) / (top - bottom))
    center_x, center_y = (left + right) / 2, (bottom + top) / 2

    def map_point(x, y):
        return {
            "x": round(MAP_WIDTH / 2 + (x - center_x) * scale, 2),
            "y": round(MAP_HEIGHT / 2 - (y - center_y) * scale, 2),
        }

    start = timestamp_ms(start_utc)
    ticks = range(start, start + length_ms, INTERVAL_MS)
    frames = [[] for _ in ticks]
    for driver in DRIVERS:
        positions = list(resample(observations[driver], ticks))
        if all(point is None for point in positions):
            raise ValueError(f"driver {driver} has no usable positions in the clip")
        for frame, point in zip(frames, positions):
            if point is not None:
                frame.append({"driver": driver, **map_point(*point)})

    return {
        "session": {
            "name": "2026 Barcelona-Catalunya Grand Prix",
            "caption": "Hamilton vs Russell · Hamilton's final stop under the Virtual Safety Car",
            "drivers": list(DRIVERS),
            "driverNames": DRIVERS,
            "track": [map_point(sample.x, sample.y) for sample in track],
            "source": {"provider": "OpenF1", "sessionKey": SESSION_KEY, "clipStartUtc": start_utc},
        },
        "intervalMs": INTERVAL_MS,
        "frames": frames,
    }


def fetch_rows(endpoint, filters=""):
    sleep(0.35)  # Respect OpenF1's public request limit.
    with urlopen(f"https://api.openf1.org/v1/{endpoint}?session_key={SESSION_KEY}{filters}", timeout=35) as response:
        return json.load(response)


def fetch_positions(driver, start, end):
    def date_filter(time_ms):
        return quote((EPOCH + time_ms * MILLISECOND).replace(tzinfo=None).isoformat(timespec="milliseconds"))

    # Include samples on either side of the window for interpolation at its edges.
    rows = []
    window = 15 * 60_000
    for low in range(start, end, window):
        high = min(end, low + window)
        rows.extend(fetch_rows("location", f"&driver_number={driver}"
                               f"&date%3E{date_filter(low - MAX_GAP_MS)}&date%3C{date_filter(high + MAX_GAP_MS)}"))
    return parse_samples(rows)


def timing_series(rows, date_key, field, start, length, seed=False):
    samples = [(timestamp_ms(row[date_key]) - start,
                "Unavailable" if row[field] is None else str(row[field]))
               for row in rows if row.get(date_key)]
    samples.sort(key=lambda sample: sample[0])
    # Seed race position with the last sample before the start, such as grid order.
    series = {max(0, elapsed): value for elapsed, value in samples
              if elapsed < length and (seed or elapsed >= 0)}
    return [{"elapsedMs": elapsed, "value": value} for elapsed, value in series.items()]


def completed_laps(laps, start, length):
    results = []
    for lap in laps:
        if not lap.get("date_start") or not lap.get("lap_duration"):
            continue
        duration = round(lap["lap_duration"] * 1000)
        finish = timestamp_ms(lap["date_start"]) - start + duration
        if duration > 0 and 0 <= finish <= length:
            results.append({"elapsedMs": finish, "lap": lap["lap_number"], "durationMs": duration})
    return sorted(results, key=lambda lap: lap["elapsedMs"])


def race_timeline(control, laps, start, length):
    events = []
    drivers = {}
    for driver, name in DRIVERS.items():
        filters = f"&driver_number={driver}"
        drivers[driver] = {
            "laps": timing_series(laps[driver], "date_start", "lap_number", start, length),
            "completedLaps": completed_laps(laps[driver], start, length),
            "positions": timing_series(fetch_rows("position", filters), "date", "position", start, length, seed=True),
            "gaps": timing_series(fetch_rows("intervals", filters), "date", "gap_to_leader", start, length),
        }
        stops = sorted(fetch_rows("pit", filters), key=lambda stop: timestamp_ms(stop["date"]))
        for index, stop in enumerate(stops, 1):
            elapsed = timestamp_ms(stop["date"]) - start
            if 0 <= elapsed < length:
                event = {"elapsedMs": elapsed, "kind": "pit", "label": f"{name.split()[-1]} pit stop {index}"}
                for source, target in (("stop_duration", "stopMs"), ("lane_duration", "laneMs")):
                    if stop.get(source) and stop[source] > 0:
                        event[target] = round(stop[source] * 1000)
                events.append(event)

    labels = {"VSC DEPLOYED": "Virtual Safety Car deployed", "VSC ENDING": "Virtual Safety Car ending"}
    for row in control:
        elapsed = timestamp_ms(row["date"]) - start
        if row["message"] in labels and 0 <= elapsed < length:
            events.append({"elapsedMs": elapsed, "kind": "vsc", "label": labels[row["message"]]})
    return {"drivers": drivers, "events": sorted(events, key=lambda event: event["elapsedMs"])}


def race_window(control, laps):
    start_utc = next(row["date"] for row in control if row["message"] == "SESSION STARTED")
    end = max(timestamp_ms(lap["date_start"]) + round(lap["lap_duration"] * 1000)
              for driver_laps in laps.values() for lap in driver_laps
              if lap.get("date_start") and lap.get("lap_duration"))
    return start_utc, end - timestamp_ms(start_utc)


def write_recording(output, recording):
    output.parent.mkdir(parents=True, exist_ok=True)
    file = NamedTemporaryFile(mode="w", encoding="utf-8", dir=output.parent, delete=False)
    temporary = Path(file.name)
    try:
        with file:
            json.dump(recording, file, ensure_ascii=False, allow_nan=False, separators=(",", ":"))
            file.write("\n")
        temporary.replace(output)
    finally:
        temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--full", action="store_true", help="import the full two-driver race")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    output = args.output or Path(".recordings/barcelona-2026-full.json" if args.full else
                                ".recordings/barcelona-2026.json")

    try:
        start_utc, length = CLIP_START, CLIP_LENGTH_MS
        if args.full:
            control = fetch_rows("race_control")
            laps = {driver: fetch_rows("laps", f"&driver_number={driver}") for driver in DRIVERS}
            start_utc, length = race_window(control, laps)
        start = timestamp_ms(start_utc)
        observations = {driver: fetch_positions(driver, start, start + length) for driver in DRIVERS}
        track_start, track_end = timestamp_ms(TRACK_START), timestamp_ms(TRACK_END)
        track = [sample for sample in fetch_positions("63", track_start, track_end)
                 if track_start <= sample.time_ms <= track_end]
        recording = build_recording(track, observations, start_utc, length)
        if args.full:
            recording["session"]["caption"] = "Hamilton vs Russell · Full race"
            recording["session"]["timeline"] = race_timeline(control, laps, start, length)
        write_recording(output, recording)
    except (OSError, ValueError, TypeError, KeyError, StopIteration) as error:
        parser.exit(1, f"Import failed: {error}\n")
    print(f"Wrote {len(recording['frames'])} frames to {output}")
    if args.full:
        for driver, name in DRIVERS.items():
            missing = sum(all(car["driver"] != driver for car in frame) for frame in recording["frames"])
            print(f"{name}: position unavailable in {missing} frames")


if __name__ == "__main__":
    main()
