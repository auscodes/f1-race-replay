import unittest

from import_barcelona import (
    CLIP_START,
    Sample,
    build_recording,
    completed_laps,
    parse_samples,
    race_window,
    resample,
    timestamp_ms,
    timing_series,
)


class ImportTests(unittest.TestCase):
    def test_lap_result_is_available_at_finish(self):
        start = timestamp_ms("2026-06-14T13:00:00Z")
        rows = [{"date_start": "2026-06-14T13:00:20Z", "lap_number": 1, "lap_duration": 81.123},
                {"date_start": "2026-06-14T13:01:41Z", "lap_number": 2, "lap_duration": None}]
        self.assertEqual(completed_laps(rows, start, 102_000),
                         [{"elapsedMs": 101123, "lap": 1, "durationMs": 81123}])

    def test_race_window_extends_until_both_drivers_finish(self):
        start = "2026-06-14T13:00:00+00:00"
        control = [{"date": start, "message": "SESSION STARTED"}]
        laps = {"44": [{"date_start": start, "lap_duration": 81.1}],
                "63": [{"date_start": start, "lap_duration": 83.2}]}
        self.assertEqual(race_window(control, laps), (start, 83_200))

    def test_timing_series_uses_race_clock_and_optional_seed(self):
        start = timestamp_ms("2026-06-14T13:00:00Z")
        rows = [{"date": "2026-06-14T12:59:59Z", "value": 2},
                {"date": "2026-06-14T13:00:01Z", "value": 1},
                {"date": "2026-06-14T13:00:03Z", "value": 3}]
        sample = {"elapsedMs": 1000, "value": "1"}
        self.assertEqual(timing_series(rows, "date", "value", start, 3000), [sample])
        self.assertEqual(timing_series(rows, "date", "value", start, 3000, seed=True),
                         [{"elapsedMs": 0, "value": "2"}, sample])

    def test_timestamp_uses_one_utc_clock(self):
        self.assertEqual(timestamp_ms("2026-06-14T13:00:00.123Z"),
                         timestamp_ms("2026-06-14T15:00:00.123+02:00"))
        with self.assertRaisesRegex(ValueError, "time zone"):
            timestamp_ms("2026-06-14T13:00:00")

    def test_sort_samples_deduplicate_and_skip_missing_coordinates(self):
        earlier = {"date": "2026-06-14T13:00:00.100Z", "x": 1, "y": 2}
        later = {"date": "2026-06-14T13:00:00.420Z", "x": 2, "y": 3}
        missing = {"date": "2026-06-14T13:00:00.200Z", "x": None, "y": 3}
        samples = parse_samples([later, earlier, missing, earlier])
        self.assertEqual([(point.x, point.y) for point in samples], [(1, 2), (2, 3)])
        self.assertEqual(samples[1].time_ms - samples[0].time_ms, 320)

    def test_conflicting_duplicates_are_rejected(self):
        first = {"date": "2026-06-14T13:00:00.100Z", "x": 1, "y": 2}
        with self.assertRaisesRegex(ValueError, "conflicting positions"):
            parse_samples([first, {**first, "x": 3}])

    def test_interpolate_only_between_nearby_samples(self):
        samples = [Sample(100, 0, 0), Sample(400, 30, 60), Sample(1400, 40, 80), Sample(2401, 50, 100)]
        ticks = [99, 100, 200, 900, 1400, 2000, 2401, 2500]
        self.assertEqual(list(resample(samples, ticks)),
                         [None, (0, 0), (10, 20), (35, 70), (40, 80), None, (50, 100), None])

    def test_both_drivers_use_the_same_tick_grid_and_map(self):
        track = [Sample(0, 0, 0), Sample(1, 10, 0), Sample(2, 10, 10)]
        start = timestamp_ms(CLIP_START)
        observations = {
            "44": [Sample(start, 0, 0), Sample(start + 400, 10, 0)],
            "63": [Sample(start + 200, 10, 10)],
        }
        recording = build_recording(track, observations)
        self.assertEqual(recording["intervalMs"], 200)
        self.assertEqual(len(recording["frames"]), 250)
        self.assertEqual(recording["session"]["track"],
                         [{"x": 215, "y": 425}, {"x": 585, "y": 425}, {"x": 585, "y": 55}])
        self.assertEqual(recording["frames"][0], [{"driver": "44", "x": 215, "y": 425}])
        self.assertEqual(recording["frames"][1], [
            {"driver": "44", "x": 400, "y": 425}, {"driver": "63", "x": 585, "y": 55},
        ])
        self.assertEqual(recording["frames"][3], [])

    def test_clip_rejects_a_driver_without_usable_positions(self):
        track = [Sample(0, 0, 0), Sample(1, 10, 0), Sample(2, 10, 10)]
        with self.assertRaisesRegex(ValueError, "driver 44 has no usable positions"):
            build_recording(track, {"44": [], "63": []})


if __name__ == "__main__":
    unittest.main()
