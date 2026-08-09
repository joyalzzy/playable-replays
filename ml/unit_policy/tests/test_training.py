from __future__ import annotations

import unittest

from ml.unit_policy.training import generate_synthetic_examples, grouped_split, train_policy


class TrainingTests(unittest.TestCase):
    def test_grouped_split_has_no_overlap(self) -> None:
        examples = generate_synthetic_examples(80, 7, weight=0.4)
        _, _, training_groups, validation_groups = grouped_split(examples, 0.2, 7)
        self.assertFalse(training_groups & validation_groups)

    def test_small_training_run(self) -> None:
        examples = generate_synthetic_examples(120, 9, weight=0.4)
        model, metrics = train_policy(examples, seed=9, epochs=2, validation_fraction=0.2, policy_version="smoke")
        self.assertEqual(model.version, "smoke")
        self.assertEqual(metrics.group_overlap, 0)
        self.assertGreaterEqual(metrics.action_accuracy, 0.0)
        self.assertLessEqual(metrics.action_accuracy, 1.0)


if __name__ == "__main__":
    unittest.main()
