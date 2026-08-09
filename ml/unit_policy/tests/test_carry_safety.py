from __future__ import annotations

import unittest

from ml.unit_policy.carry_safety import curriculum_examples, train_carry_model


class CarrySafetyTests(unittest.TestCase):
    def test_curriculum_contains_multiple_actions(self) -> None:
        examples = curriculum_examples(60, 11)
        actions = {example.action for example in examples}
        self.assertIn("move", actions)
        self.assertIn("contest", actions)
        self.assertIn("retreat", actions)
        self.assertIn("hold", actions)

    def test_small_carry_training_run(self) -> None:
        model, metrics = train_carry_model(seed=12, bootstrap_examples=60, curriculum_count=80, epochs=2, policy_version="carry-smoke")
        self.assertEqual(model.version, "carry-smoke")
        self.assertEqual(metrics.group_overlap, 0)


if __name__ == "__main__":
    unittest.main()
