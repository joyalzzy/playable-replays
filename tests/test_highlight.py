import json
import tempfile
import unittest
from pathlib import Path

from ml.highlight import Signals, fixtures_as_candidates, reason_tags, score, select_candidates


class HighlightTests(unittest.TestCase):
    def test_score_is_interpretable_and_bounded(self):
        value = score(Signals(1, 1, 0, 1))
        self.assertEqual(value, 1)

    def test_invalid_signal_rejected(self):
        with self.assertRaises(ValueError):
            score(Signals(1.1, 0.5, 0.5, 0.5))

    def test_threshold_and_ordering(self):
        windows = [
            (30, 40, Signals(0.8, 0.9, 0.1, 0.7)),
            (10, 20, Signals(0.1, 0.1, 0.9, 0.1)),
            (50, 60, Signals(0.7, 0.7, 0.2, 0.6)),
        ]
        candidates = select_candidates(windows, threshold=0.6)
        self.assertEqual(len(candidates), 2)
        self.assertGreaterEqual(candidates[0].score, candidates[1].score)

    def test_invalid_window_ignored(self):
        candidates = select_candidates([(10, 9, Signals(1, 1, 0, 1))])
        self.assertEqual(candidates, [])

    def test_reason_tags(self):
        tags = reason_tags(Signals(0.9, 0.9, 0.1, 0.9))
        self.assertIn("team-fight", tags)
        self.assertIn("resource-disadvantage", tags)

    def test_fixture_conversion(self):
        payload = {
            "moments": [{
                "startTimeSeconds": 5,
                "reasonTags": ["clutch"],
                "signals": {
                    "winProbabilitySwing": 0.8,
                    "eventDensity": 0.8,
                    "entityProximity": 0.2,
                    "resourceAsymmetry": 0.7,
                },
            }]
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "moments.json"
            path.write_text(json.dumps(payload), encoding="utf-8")
            candidates = fixtures_as_candidates(path)
        self.assertEqual(candidates[0].start_second, 5)
        self.assertEqual(candidates[0].end_second, 17)


if __name__ == "__main__":
    unittest.main()

