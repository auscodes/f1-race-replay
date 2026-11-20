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


def build_recording(track, observations):
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

    start = timestamp_ms(CLIP_START)
    ticks = range(start, start + CLIP_LENGTH_MS, INTERVAL_MS)
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
            "source": {"provider": "OpenF1", "sessionKey": SESSION_KEY, "clipStartUtc": CLIP_START},
        },
        "intervalMs": INTERVAL_MS,
        "frames": frames,
    }


def fetch_positions(driver, start, end):
    def date_filter(time_ms):
        return quote((EPOCH + time_ms * MILLISECOND).replace(tzinfo=None).isoformat(timespec="milliseconds"))

    # Include samples on either side of the window for interpolation at its edges.
    url = (f"https://api.openf1.org/v1/location?session_key={SESSION_KEY}&driver_number={driver}"
           f"&date%3E{date_filter(start - MAX_GAP_MS)}&date%3C{date_filter(end + MAX_GAP_MS)}")
    with urlopen(url, timeout=25) as response:
        return parse_samples(json.load(response))


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
    parser.add_argument("--output", type=Path, default=Path(".recordings/barcelona-2026.json"))
    args = parser.parse_args()

    start = timestamp_ms(CLIP_START)
    try:
        observations = {}
        for driver in DRIVERS:
            observations[driver] = fetch_positions(driver, start, start + CLIP_LENGTH_MS)
            sleep(0.4)  # Keep the archive requests within OpenF1's public rate limit.
        track_start, track_end = timestamp_ms(TRACK_START), timestamp_ms(TRACK_END)
        track = [sample for sample in fetch_positions("63", track_start, track_end)
                 if track_start <= sample.time_ms <= track_end]
        recording = build_recording(track, observations)
        write_recording(args.output, recording)
    except (OSError, ValueError, TypeError, KeyError) as error:
        parser.exit(1, f"Import failed: {error}\n")
    print(f"Wrote {len(recording['frames'])} frames to {args.output}")


if __name__ == "__main__":
    main()
