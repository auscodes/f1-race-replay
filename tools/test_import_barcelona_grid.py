import unittest

from import_barcelona import EPOCH, MILLISECOND, Sample
from import_barcelona_grid import classification, race_laps, race_states, sector_breaks


class ClassificationTest(unittest.TestCase):
    def test_lapped_driver_finishes_at_own_crossing_and_retirement_has_no_finish(self):
        results = [
            dict(driver_number=1, position=1, gap_to_leader=0, number_of_laps=3),
            dict(driver_number=2, position=2, gap_to_leader="+1 LAP", number_of_laps=2),
            dict(driver_number=3, position=None, gap_to_leader=None, number_of_laps=1),
        ]
        for result in results:
            result.update(dnf=result["driver_number"] == 3, dns=False, dsq=False)

        def lap(number, start):
            return dict(lap_number=number, date_start=(EPOCH + start * MILLISECOND).isoformat(),
                        lap_duration=2)

        outcomes, length = classification(results, {
            "1": [lap(3, 8100)], "2": [lap(2, 9100)], "3": [lap(1, 5000)],
        }, start=0, flag=10_000)
        self.assertEqual(outcomes["1"]["elapsedMs"], 10_100)
        self.assertEqual(outcomes["2"]["elapsedMs"], 11_100)
        self.assertEqual(outcomes["2"]["gap"], "+1 LAP")
        self.assertEqual(outcomes["3"], {"status": "dnf", "elapsedMs": 11_200})
        self.assertEqual(length, 11_400)


class RaceStatesTest(unittest.TestCase):
    def test_vsc_ending_stays_active_until_the_track_is_clear(self):
        observations = [
            (0, "SESSION STARTED", None, None),
            (500, "VSC DEPLOYED", None, None),
            (600, "CLEAR IN TRACK SECTOR 10", "CLEAR", "Sector"),
            (700, "VSC ENDING", None, None),
            (800, "TRACK CLEAR", "CLEAR", "Track"),
            (1000, "CHEQUERED FLAG", "CHEQUERED", "Track"),
        ]
        rows = [dict(date=(EPOCH + ms * MILLISECOND).isoformat(), message=message,
                     flag=flag, scope=scope) for ms, message, flag, scope in observations]
        self.assertEqual(race_states(rows, start=0, length=1200), [
            {"elapsedMs": 0, "value": "green"},
            {"elapsedMs": 500, "value": "vsc"},
            {"elapsedMs": 700, "value": "vsc-ending"},
            {"elapsedMs": 800, "value": "green"},
            {"elapsedMs": 1000, "value": "chequered"},
        ])


class MapMetadataTest(unittest.TestCase):
    def test_sector_splits_snap_to_reference_lap_samples(self):
        track = [Sample(i * 1000, i, i) for i in range(8)]
        reference_lap = {"date_start": EPOCH.isoformat(),
                         "duration_sector_1": 2.2, "duration_sector_2": 3.1}
        self.assertEqual(sector_breaks(track, reference_lap), [2, 5])
        self.assertEqual(sector_breaks(track[:5] + track[7:], reference_lap), [])

    def test_race_lap_starts_use_the_first_crossing_across_drivers(self):
        def lap(number, time):
            return {"lap_number": number, "date_start": (EPOCH + time * MILLISECOND).isoformat()}

        drivers = {"44": [lap(1, 0), lap(2, 1300), lap(3, 2500)],
                   "63": [lap(1, 0), lap(2, 1100)]}
        self.assertEqual(race_laps(drivers, 0, 3000, 2), [
            {"elapsedMs": 0, "value": "1"}, {"elapsedMs": 1100, "value": "2"},
        ])
