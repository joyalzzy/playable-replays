from __future__ import annotations

import unittest

from ml.unit_policy.carry_safety import _snapshot, _unit
from ml.unit_policy.features import FEATURE_NAMES, extract_features


class SchemaFeatureTests(unittest.TestCase):
    def test_feature_width_is_72(self) -> None:
        snapshot = _snapshot(1, [
            _unit("controlled", "blue", "mage", 30, 50, controlled=True),
            _unit("carry", "blue", "marksman", 45, 50),
            _unit("enemy", "red", "assassin", 58, 50),
        ], "feature-test")
        features = extract_features(snapshot, "carry")
        self.assertEqual(len(FEATURE_NAMES), 72)
        self.assertEqual(len(features), 72)


if __name__ == "__main__":
    unittest.main()
