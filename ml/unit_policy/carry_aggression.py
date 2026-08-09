"""Optional aggression-focused carry curriculum and retraining command."""
from __future__ import annotations

import argparse
import json
import random
from dataclasses import replace
from pathlib import Path

from .carry_safety import _carry_state, curriculum_examples as safety_curriculum_examples
from .training import TrainingExample, generate_synthetic_examples, train_policy


def aggression_curriculum_examples(count: int, seed: int) -> list[TrainingExample]:
    """Create contest-heavy carry examples while retaining safety guardrails."""
    if count < 1:
        raise ValueError("count must be positive")
    rng = random.Random(seed)
    examples: list[TrainingExample] = []
    kinds = ("safe", "screened", "finisher", "safe", "screened", "finisher", "caught", "critical")
    for index in range(count):
        example = _carry_state(rng, 100_000 + index, kinds[index % len(kinds)])
        if kinds[index % len(kinds)] in {"safe", "screened", "finisher"}:
            example = replace(example, weight=example.weight * 2.0, source="carry-aggression-curriculum")
        else:
            example = replace(example, weight=example.weight * 0.35, source="carry-safety-guardrail")
        examples.append(example)
    return examples


def train_aggression_model(*, seed: int = 2026, bootstrap_examples: int = 4000, safety_examples: int = 500, aggression_examples: int = 12000, epochs: int = 40, output: Path | None = None):
    synthetic = [replace(item, weight=0.5, source="synthetic-bootstrap") for item in generate_synthetic_examples(bootstrap_examples, seed, weight=0.5)]
    safety = [replace(item, weight=item.weight * 0.25, source="carry-safety-guardrail") for item in safety_curriculum_examples(safety_examples, seed + 1)]
    aggression = aggression_curriculum_examples(aggression_examples, seed + 2)
    model, metrics = train_policy(
        [*synthetic, *safety, *aggression],
        seed=seed,
        epochs=epochs,
        learning_rate=0.045,
        validation_fraction=0.18,
        policy_version="unit-policy-v4-carry-aggression",
        extra_metadata={
            "carryAggressionExamples": aggression_examples,
            "carrySafetyGuardrailExamples": safety_examples,
            "runtimeHardcodedCarryOverride": False,
            "trainingIntent": "Increase safe damage uptime while retaining learned kiting and retreat guardrails.",
        },
    )
    if output is not None:
        model.save(output)
    return model, metrics


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path(__file__).with_name("models") / "unit_policy_aggression.json")
    parser.add_argument("--seed", type=int, default=2026)
    parser.add_argument("--bootstrap-examples", type=int, default=4000)
    parser.add_argument("--safety-examples", type=int, default=500)
    parser.add_argument("--aggression-examples", type=int, default=12000)
    parser.add_argument("--epochs", type=int, default=40)
    args = parser.parse_args()
    model, metrics = train_aggression_model(seed=args.seed, bootstrap_examples=args.bootstrap_examples, safety_examples=args.safety_examples, aggression_examples=args.aggression_examples, epochs=args.epochs, output=args.output)
    print(json.dumps({"output": str(args.output), "policyVersion": model.version, "actionAccuracy": round(metrics.action_accuracy, 6), "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 6), "groupOverlap": metrics.group_overlap}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
