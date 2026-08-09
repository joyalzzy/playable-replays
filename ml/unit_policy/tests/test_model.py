from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from ml.unit_policy.features import FEATURE_NAMES
from ml.unit_policy.model import LinearUnitPolicy


class ModelTests(unittest.TestCase):
    def test_round_trip_without_bundled_artifact(self) -> None:
        model = LinearUnitPolicy.empty("test-policy")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "model.json"
            model.save(path)
            loaded = LinearUnitPolicy.load(path)
        self.assertEqual(loaded.version, "test-policy")
        self.assertEqual(len(loaded.movement_x_weights), len(FEATURE_NAMES))

    def test_probabilities_sum_to_one(self) -> None:
        model = LinearUnitPolicy.empty()
        probabilities = model.action_probabilities((0.0,) * len(FEATURE_NAMES))
        self.assertAlmostEqual(sum(probabilities.values()), 1.0)


if __name__ == "__main__":
    unittest.main()
