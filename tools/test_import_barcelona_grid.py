import unittest

from import_barcelona import EPOCH, MILLISECOND
from import_barcelona_grid import classification


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
