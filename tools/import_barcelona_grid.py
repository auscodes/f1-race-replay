"""Build a full-grid Barcelona replay from the OpenF1 archive."""

import argparse
from collections import defaultdict
import json
from pathlib import Path
from urllib.parse import quote

from import_barcelona import (
    DRIVERS, EPOCH, INTERVAL_MS, MAX_GAP_MS, MILLISECOND, TRACK_END, TRACK_START,
    build_recording, completed_laps, fetch_rows, parse_samples, race_timeline,
    timestamp_ms, timing_series, write_recording,
)

LOCATION_WINDOW_MS = 5 * 60_000


def archive_rows(cache, endpoint, name=None, filters=""):
    path = cache / f"{name or endpoint}.json"
    if path.is_file():
        with path.open(encoding="utf-8") as file:
            return json.load(file)
    rows = fetch_rows(endpoint, filters)
    write_recording(path, rows)
    return rows


def by_driver(rows):
    grouped = defaultdict(list)
    for row in rows:
        grouped[str(row["driver_number"])].append(row)
    return grouped


def date_filter(time_ms):
    return quote((EPOCH + time_ms * MILLISECOND).replace(tzinfo=None).isoformat(timespec="milliseconds"))


def classification(results, laps, start, flag):
    outcomes = {}
    for result in results:
        driver = str(result["driver_number"])
        place = result["position"]
        gap = result["gap_to_leader"]
        status = next((key for key in ("dns", "dsq", "dnf") if result[key]),
                      "finished" if place is not None else "nc")
        outcome = {"status": status}
        if place is not None:
            outcome["position"] = place
        if gap is not None:
            outcome["gap"] = str(gap)
        if status == "finished":
            final_laps = [lap for lap in completed_laps(laps[driver], start, float("inf"))
                          if lap["lap"] == result["number_of_laps"] and lap["elapsedMs"] >= flag]
            if final_laps:
                outcome["elapsedMs"] = final_laps[0]["elapsedMs"]
        elif status == "dns":
            outcome["elapsedMs"] = 0
        outcomes[driver] = outcome

    # Leave one fully visible frame after the last recorded finish.
    latest = max([flag] + [outcome.get("elapsedMs", 0) for outcome in outcomes.values()])
    final_frame = ((latest + INTERVAL_MS - 1) // INTERVAL_MS) * INTERVAL_MS
    for outcome in outcomes.values():
        outcome.setdefault("elapsedMs", final_frame)
    return outcomes, final_frame + INTERVAL_MS


def race_states(control, start, length):
    transitions = []
    for row in control:
        elapsed = timestamp_ms(row["date"]) - start
        if not 0 <= elapsed < length:
            continue
        message = row["message"]
        state = None
        if message == "SESSION STARTED" or (message == "TRACK CLEAR" and row["scope"] == "Track"):
            state = "green"
        elif message == "VSC DEPLOYED":
            state = "vsc"
        elif message == "VSC ENDING":
            state = "vsc-ending"
        elif row["flag"] == "CHEQUERED" and row["scope"] == "Track":
            state = "chequered"
        if state:
            transitions.append({"elapsedMs": elapsed, "value": state})
    return sorted(transitions, key=lambda transition: transition["elapsedMs"])


def race_laps(laps, start, length, total):
    first_crossing = {}
    for driver_laps in laps.values():
        for lap in driver_laps:
            if not lap.get("date_start") or not 1 <= lap["lap_number"] <= total:
                continue
            elapsed = timestamp_ms(lap["date_start"]) - start
            if 0 <= elapsed < length:
                number = lap["lap_number"]
                first_crossing[number] = min(elapsed, first_crossing.get(number, elapsed))
    return [{"elapsedMs": elapsed, "value": str(number)}
            for number, elapsed in sorted(first_crossing.items())]


def sector_breaks(track, reference_lap):
    # OpenF1's lap start and locations are approximate. Snap each timing split
    # to the nearest available sample on the same reference lap.
    elapsed = 0
    splits = []
    for field in ("duration_sector_1", "duration_sector_2"):
        seconds = reference_lap.get(field)
        if not seconds or seconds < 0:
            return []
        elapsed += seconds
        timestamp = timestamp_ms(reference_lap["date_start"]) + round(elapsed * 1000)
        index = min(range(len(track)), key=lambda i: abs(track[i].time_ms - timestamp))
        if index <= (splits[-1] if splits else 0) or index >= len(track) - 1:
            return []
        if abs(track[index].time_ms - timestamp) > MAX_GAP_MS:
            return []
        splits.append(index)
    return splits


def import_grid(cache):
    control = archive_rows(cache, "race_control")
    start_utc = next(row["date"] for row in control if row["message"] == "SESSION STARTED")
    start = timestamp_ms(start_utc)
    flag = next(timestamp_ms(row["date"]) - start for row in control if row["flag"] == "CHEQUERED")
    driver_rows = archive_rows(cache, "drivers")
    driver_names = DRIVERS | {str(row["driver_number"]): f"{row['first_name']} {row['last_name']}"
                              for row in driver_rows}
    results = archive_rows(cache, "session_result")
    datasets = {endpoint: by_driver(archive_rows(cache, endpoint))
                for endpoint in ("laps", "position", "intervals", "pit", "stints")}
    laps = datasets["laps"]
    outcomes, length = classification(results, laps, start, flag)

    # Keep the last window bounds fixed so interrupted imports can reuse cached files.
    locations = defaultdict(list)
    for index, low in enumerate(range(start, start + length, LOCATION_WINDOW_MS)):
        high = low + LOCATION_WINDOW_MS
        filters = f"&date%3E{date_filter(low - MAX_GAP_MS)}&date%3C{date_filter(high + MAX_GAP_MS)}"
        for driver, rows in by_driver(archive_rows(cache, "location", f"location-{index}", filters)).items():
            locations[driver].extend(rows)
    samples = {driver: parse_samples(locations[driver]) for driver in driver_names}
    track_start, track_end = timestamp_ms(TRACK_START), timestamp_ms(TRACK_END)
    track = [sample for sample in samples["63"] if track_start <= sample.time_ms <= track_end]
    recording = build_recording(track, samples, start_utc, length, driver_names, require_positions=False)

    def cached_driver_rows(endpoint, filters):
        return datasets[endpoint][filters.removeprefix("&driver_number=")]

    timeline = race_timeline(control, laps, start, length, driver_names, cached_driver_rows)
    timeline["raceStates"] = race_states(control, start, length)
    timeline["totalLaps"] = max(result["number_of_laps"] for result in results)
    timeline["raceLaps"] = race_laps(laps, start, length, timeline["totalLaps"])
    timeline["events"].append({"elapsedMs": flag, "kind": "flag", "label": "Chequered flag"})
    timeline["events"].sort(key=lambda event: event["elapsedMs"])
    for driver, timing in timeline["drivers"].items():
        timing["intervals"] = timing_series(datasets["intervals"][driver], "date", "interval", start, length)
        timing["tyres"] = [{"lapStart": row["lap_start"], "lapEnd": row["lap_end"],
                            "compound": row["compound"], "startingAge": row["tyre_age_at_start"]}
                           for row in datasets["stints"][driver]]
        timing["outcome"] = outcomes.get(driver)

    reference = next((lap for lap in laps["63"]
                      if lap.get("date_start") and timestamp_ms(lap["date_start"]) == track_start), {})
    recording["session"].update({
        "sectorBreaks": sector_breaks(track, reference),
        "caption": f"Full race · {len(driver_names)} drivers",
        "grid": {str(row["driver_number"]): {"code": row["name_acronym"], "team": row["team_name"],
                                             "color": row["team_colour"]} for row in driver_rows},
        "timeline": timeline,
    })
    return recording


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache-dir", type=Path, default=Path(".recordings/openf1-grid"))
    parser.add_argument("--output", type=Path, default=Path(".recordings/barcelona-2026-grid.json"))
    args = parser.parse_args()
    recording = import_grid(args.cache_dir)
    write_recording(args.output, recording)
    print(f"Wrote {len(recording['frames'])} frames for {len(recording['session']['drivers'])} drivers to {args.output}")


if __name__ == "__main__":
    main()
