from __future__ import annotations

import unittest

from ml.unit_policy.replay_adapter import ReplayAdapterConfig, replay_row_to_example


class ReplayAdapterTests(unittest.TestCase):
    def test_positive_verified_row_converts(self) -> None:
        row = {
            "schemaVersion": "pro-labeled-movements-v1",
            "verifiedProfessional": True,
            "trainingEligible": True,
            "sceneId": "match-1-scene-1",
            "team": "blue",
            "label": "positive",
            "labelConfidence": 0.9,
            "outcomeAssociationScore": 0.5,
            "from": {"x": 7000, "z": 7000},
            "to": {"x": 6500, "z": 7000},
            "movementDistance": 500,
            "preFightContext": {
                "playerState": {"hpRatio": 0.7},
                "nearestEnemyDistance": 900,
                "nearestAllyDistance": 600,
                "enemyCentroid": {"x": 7800, "z": 7000},
                "allyCentroid": {"x": 6500, "z": 7050},
                "nearbyAllies": 2,
                "nearbyEnemies": 1,
                "localNumberAdvantage": 1,
                "sceneProgress": 0.4
            },
            "rolePositioning": {
                "profile": "marksman",
                "profileConfidence": 0.9,
                "featureCoverage": 0.9,
                "profileSource": "manifest-tacticalClass",
                "pre": {"threatExposure": 0.5, "isolationRisk": 0.2},
                "movementIntent": {"towardEnemyCentroid": -0.8},
                "trainingInput": {"movementIntent": {"towardEnemyCentroid": -0.8}},
                "delta": {"profileFitScore": 0.1}
            }
        }
        example = replay_row_to_example(row, ReplayAdapterConfig())
        self.assertEqual(example.action, "move")
        self.assertEqual(len(example.features), 72)
        self.assertTrue(example.group_id.startswith("replay-match:"))


if __name__ == "__main__":
    unittest.main()
