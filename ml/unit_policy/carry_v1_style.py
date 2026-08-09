"""Optional contest-heavy curriculum resembling the original tester behavior."""
from __future__ import annotations

import argparse
import json
from dataclasses import replace
from pathlib import Path

from .carry_aggression import aggression_curriculum_examples
from .carry_safety import curriculum_examples as safety_curriculum_examples
from .training import generate_synthetic_examples, train_policy


def train_v1_style_model(*, seed: int = 2026, bootstrap_examples: int = 4000, safety_examples: int = 250, aggression_examples: int = 32000, epochs: int = 42, output: Path | None = None):
    synthetic = [replace(item, weight=0.65, source="v1-synthetic-bootstrap") for item in generate_synthetic_examples(bootstrap_examples, seed, weight=0.65)]
    safety = [replace(item, weight=max(0.10, item.weight * 0.08), source="carry-safety-guardrail") for item in safety_curriculum_examples(safety_examples, seed + 1)]
    aggression = [replace(item, weight=item.weight * 2.5, source="v1-style-carry-aggression") for item in aggression_curriculum_examples(aggression_examples, seed + 2)]
    model, metrics = train_policy(
        [*synthetic, *safety, *aggression],
        seed=seed,
        epochs=epochs,
        learning_rate=0.043,
        validation_fraction=0.18,
        policy_version="unit-policy-v5-v1-style-aggression",
        extra_metadata={
            "runtimeHardcodedCarryOverride": False,
            "v1StyleAggressionExamples": aggression_examples,
            "carrySafetyGuardrailExamples": safety_examples,
            "trainingIntent": "Move carry behavior toward contest-heavy v1 behavior without an inference-time override.",
        },
    )
    if output is not None:
        model.save(output)
    return model, metrics


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path(__file__).with_name("models") / "unit_policy_v1_style.json")
    parser.add_argument("--seed", type=int, default=2026)
    parser.add_argument("--bootstrap-examples", type=int, default=4000)
    parser.add_argument("--safety-examples", type=int, default=250)
    parser.add_argument("--aggression-examples", type=int, default=32000)
    parser.add_argument("--epochs", type=int, default=42)
    args = parser.parse_args()
    model, metrics = train_v1_style_model(seed=args.seed, bootstrap_examples=args.bootstrap_examples, safety_examples=args.safety_examples, aggression_examples=args.aggression_examples, epochs=args.epochs, output=args.output)
    print(json.dumps({"output": str(args.output), "policyVersion": model.version, "actionAccuracy": round(metrics.action_accuracy, 6), "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 6), "groupOverlap": metrics.group_overlap}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
