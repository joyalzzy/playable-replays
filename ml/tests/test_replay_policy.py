import gzip
import json
import tempfile
import unittest
from pathlib import Path

from ml.features.replay_actions import extract_examples, game_phase
from ml.ingest.replays import read_matches
from ml.train.replay_policy import evaluate, probabilities, split_for_match, train


class ReplayPolicyTests(unittest.TestCase):
    def test_extracts_coarse_actions_in_order(self):
        events = [
            {"WaypointGroup": {"time": 5, "waypoints": {}}},
            {"LeaveFog": {"time": 6, "net_id": 1}},
            {"CastSpellAns": {"time": 700, "caster_net_id": 1}},
        ]
        examples = extract_examples(events)
        self.assertEqual([example.label for example in examples], ["move", "cast"])
        self.assertEqual(examples[1].context[:2], ("mid", "move"))

    def test_phases_have_stable_boundaries(self):
        self.assertEqual(game_phase(599.9), "early")
        self.assertEqual(game_phase(600), "mid")
        self.assertEqual(game_phase(1200), "late")

    def test_reader_streams_gzip_and_respects_limit(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "sample.jsonl.gz"
            with gzip.open(path, "wt", encoding="utf-8") as target:
                target.write(json.dumps({"events": []}) + "\n")
                target.write(json.dumps({"events": [{"UseItem": {"time": 1}}]}) + "\n")
            self.assertEqual(len(list(read_matches(path, 1))), 1)

    def test_model_is_smoothed_and_metrics_are_bounded(self):
        examples = extract_examples([
            {"WaypointGroup": {"time": 1}},
            {"BasicAttackPos": {"time": 2}},
            {"WaypointGroup": {"time": 3}},
        ])
        model = train(examples)
        probs = probabilities(model, examples[0].context)
        self.assertAlmostEqual(sum(probs.values()), 1)
        metrics = evaluate(model, examples)
        self.assertGreaterEqual(metrics.top1_accuracy, 0)
        self.assertLessEqual(metrics.brier_score, 2)

    def test_match_split_is_deterministic(self):
        self.assertEqual(split_for_match(7, 742), split_for_match(7, 742))


if __name__ == "__main__":
    unittest.main()
