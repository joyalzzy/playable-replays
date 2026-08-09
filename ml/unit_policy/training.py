"""Weighted, match-grouped training for the local unit policy."""
from __future__ import annotations

import argparse
import json
import math
import random
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Sequence

from .demonstrations import generate_synthetic_records, load_demonstration_records
from .features import FEATURE_NAMES
from .model import LinearUnitPolicy
from .replay_adapter import ReplayAdapterConfig, load_labeled_replay_examples
from .schema import ACTION_TYPES


@dataclass(frozen=True, slots=True)
class TrainingExample:
    features: tuple[float, ...]
    action: str
    move_dx: float | None = None
    move_dy: float | None = None
    weight: float = 1.0
    group_id: str = ""
    source: str = "unknown"


@dataclass(frozen=True, slots=True)
class TrainingMetrics:
    examples: int
    validation_examples: int
    action_accuracy: float
    movement_mae: float | None
    per_action_recall: dict[str, float]
    weighted_action_accuracy: float = 0.0
    weighted_movement_mae: float | None = None
    training_groups: int = 0
    validation_groups: int = 0
    group_overlap: int = 0
    source_examples: dict[str, int] | None = None


def _examples(records: Iterable[tuple[Any, ...]], *, weight: float, source: str, prefix: str) -> list[TrainingExample]:
    return [TrainingExample(*record, weight=weight, group_id=f"{prefix}:{index}", source=source) for index, record in enumerate(records)]


def generate_synthetic_examples(count: int, seed: int, *, weight: float = 1.0) -> list[TrainingExample]:
    return _examples(generate_synthetic_records(count, seed), weight=weight, source="synthetic-bootstrap", prefix=f"synthetic-{seed}")


def load_jsonl(path: str | Path, *, weight: float = 1.0) -> list[TrainingExample]:
    return _examples(load_demonstration_records(path), weight=weight, source="reviewed-demonstration", prefix=f"reviewed-{Path(path).name}")


def _dot(a: Iterable[float], b: Iterable[float]) -> float:
    return sum(x * y for x, y in zip(a, b, strict=True))


def _softmax(logits: Sequence[float]) -> list[float]:
    maximum = max(logits)
    values = [math.exp(value - maximum) for value in logits]
    total = sum(values)
    return [value / total for value in values]


def _validate_examples(examples: Sequence[TrainingExample]) -> None:
    if len(examples) < 20:
        raise ValueError("at least 20 training examples are required")
    for index, example in enumerate(examples):
        if len(example.features) != len(FEATURE_NAMES) or not all(math.isfinite(value) for value in example.features):
            raise ValueError(f"example {index} has invalid features")
        if example.action not in ACTION_TYPES:
            raise ValueError(f"example {index} has invalid action")
        if not math.isfinite(example.weight) or example.weight <= 0:
            raise ValueError(f"example {index} has invalid weight")
        if (example.move_dx is None) != (example.move_dy is None):
            raise ValueError(f"example {index} has incomplete movement target")
        if example.move_dx is not None:
            if example.action != "move":
                raise ValueError(f"example {index}: only move may have a target")
            if not math.isfinite(example.move_dx) or not math.isfinite(example.move_dy):
                raise ValueError(f"example {index} has invalid movement target")


def grouped_split(examples: Sequence[TrainingExample], validation_fraction: float, seed: int) -> tuple[list[int], list[int], set[str], set[str]]:
    if not 0.01 <= validation_fraction <= 0.5:
        raise ValueError("validation_fraction must be between 0.01 and 0.5")
    groups: dict[str, list[int]] = defaultdict(list)
    for index, example in enumerate(examples):
        groups[example.group_id or f"ungrouped:{index}"].append(index)
    keys = list(groups)
    if len(keys) < 2:
        raise ValueError("at least two groups are required for leakage-safe validation")
    rng = random.Random(seed)
    rng.shuffle(keys)
    target = max(1, round(len(examples) * validation_fraction))
    selected: list[str] = []
    selected_count = 0
    for key in keys:
        if len(selected) >= len(keys) - 1:
            break
        selected.append(key)
        selected_count += len(groups[key])
        if selected_count >= target:
            break
    validation_groups = set(selected) or {keys[0]}
    training_groups = set(keys) - validation_groups
    if not training_groups:
        moved = next(iter(validation_groups))
        validation_groups.remove(moved)
        training_groups.add(moved)
    train = [index for group in sorted(training_groups) for index in groups[group]]
    validation = [index for group in sorted(validation_groups) for index in groups[group]]
    return train, validation, training_groups, validation_groups


def _source_stats(examples: Sequence[TrainingExample], indices: Iterable[int]) -> tuple[dict[str, int], dict[str, float]]:
    indices = list(indices)
    counts = Counter(examples[index].source for index in indices)
    weights: dict[str, float] = defaultdict(float)
    for index in indices:
        weights[examples[index].source] += examples[index].weight
    return dict(sorted(counts.items())), {key: round(value, 6) for key, value in sorted(weights.items())}


def train_policy(
    examples: Sequence[TrainingExample],
    *,
    seed: int = 2026,
    epochs: int = 55,
    learning_rate: float = 0.055,
    l2: float = 0.0008,
    validation_fraction: float = 0.15,
    policy_version: str = "unit-policy-v1",
    extra_metadata: dict[str, Any] | None = None,
) -> tuple[LinearUnitPolicy, TrainingMetrics]:
    _validate_examples(examples)
    if epochs < 1 or learning_rate <= 0 or l2 < 0:
        raise ValueError("training hyperparameters are invalid")
    train_indices, validation_indices, train_groups, validation_groups = grouped_split(examples, validation_fraction, seed)
    if train_groups & validation_groups:
        raise AssertionError("group leakage detected")

    width = len(FEATURE_NAMES)
    action_weights = [[0.0] * width for _ in ACTION_TYPES]
    movement_x = [0.0] * width
    movement_y = [0.0] * width
    weighted_counts = {action: sum(examples[index].weight for index in train_indices if examples[index].action == action) for action in ACTION_TYPES}
    total_weight = sum(examples[index].weight for index in train_indices)
    class_weights = {action: (total_weight / (len(ACTION_TYPES) * max(1e-9, weighted_counts[action]))) ** 0.65 if weighted_counts[action] > 0 else 1.0 for action in ACTION_TYPES}

    rng = random.Random(seed)
    for epoch in range(epochs):
        rng.shuffle(train_indices)
        rate = learning_rate / math.sqrt(1.0 + epoch * 0.08)
        for index in train_indices:
            example = examples[index]
            probabilities = _softmax([_dot(weights, example.features) for weights in action_weights])
            label = ACTION_TYPES.index(example.action)
            sample_weight = example.weight * class_weights[example.action]
            for action_index, weights in enumerate(action_weights):
                error = sample_weight * (probabilities[action_index] - float(action_index == label))
                for feature_index, feature in enumerate(example.features):
                    weights[feature_index] -= rate * (error * feature + l2 * weights[feature_index])
            if example.move_dx is None:
                continue
            prediction_x = math.tanh(_dot(movement_x, example.features))
            prediction_y = math.tanh(_dot(movement_y, example.features))
            gradient_x = sample_weight * (prediction_x - example.move_dx) * (1.0 - prediction_x**2)
            gradient_y = sample_weight * (prediction_y - example.move_dy) * (1.0 - prediction_y**2)
            for feature_index, feature in enumerate(example.features):
                movement_x[feature_index] -= rate * 0.42 * (gradient_x * feature + l2 * movement_x[feature_index])
                movement_y[feature_index] -= rate * 0.42 * (gradient_y * feature + l2 * movement_y[feature_index])

    bare_model = LinearUnitPolicy(
        policy_version,
        {action: tuple(action_weights[index]) for index, action in enumerate(ACTION_TYPES)},
        tuple(movement_x),
        tuple(movement_y),
        {},
    )
    metrics = evaluate_policy(
        bare_model,
        examples,
        validation_indices,
        training_groups=len(train_groups),
        validation_groups=len(validation_groups),
        group_overlap=len(train_groups & validation_groups),
    )
    train_sources, train_weights = _source_stats(examples, train_indices)
    validation_sources, validation_weights = _source_stats(examples, validation_indices)
    metadata: dict[str, Any] = {
        "trainingSource": "weighted mixed demonstrations",
        "trainingExamples": len(train_indices),
        "validationExamples": len(validation_indices),
        "trainingGroups": len(train_groups),
        "validationGroups": len(validation_groups),
        "groupOverlap": 0,
        "splitStrategy": "deterministic group-isolated split",
        "seed": seed,
        "epochs": epochs,
        "actionAccuracy": round(metrics.action_accuracy, 6),
        "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 6),
        "movementMae": None if metrics.movement_mae is None else round(metrics.movement_mae, 6),
        "weightedMovementMae": None if metrics.weighted_movement_mae is None else round(metrics.weighted_movement_mae, 6),
        "perActionRecall": {key: round(value, 6) for key, value in metrics.per_action_recall.items()},
        "classWeights": {key: round(value, 6) for key, value in class_weights.items()},
        "sourceExamples": {"training": train_sources, "validation": validation_sources},
        "sourceWeights": {"training": train_weights, "validation": validation_weights},
        "leakageAudit": {"groupOverlap": 0, "matchGroupedReplaySplit": True, "futureOutcomeFieldsUsedAsInputs": False},
        "disclosure": "Replay labels are observational outcome associations; the model is not proof of optimal play or player intent.",
    }
    if extra_metadata:
        metadata.update(extra_metadata)
    return LinearUnitPolicy(bare_model.version, bare_model.action_weights, bare_model.movement_x_weights, bare_model.movement_y_weights, metadata), metrics


def evaluate_policy(
    model: LinearUnitPolicy,
    examples: Sequence[TrainingExample],
    indices: Iterable[int] | None = None,
    *,
    training_groups: int = 0,
    validation_groups: int = 0,
    group_overlap: int = 0,
) -> TrainingMetrics:
    selected = list(indices if indices is not None else range(len(examples)))
    if not selected:
        raise ValueError("evaluation set must not be empty")
    correct = 0
    weighted_correct = 0.0
    total_weight = 0.0
    movement_error = 0.0
    weighted_movement_error = 0.0
    movement_values = 0
    weighted_movement_values = 0.0
    totals = {action: 0 for action in ACTION_TYPES}
    correct_by_action = {action: 0 for action in ACTION_TYPES}
    for index in selected:
        example = examples[index]
        prediction = model.choose_action(example.features)
        totals[example.action] += 1
        total_weight += example.weight
        if prediction == example.action:
            correct += 1
            correct_by_action[example.action] += 1
            weighted_correct += example.weight
        if example.move_dx is not None:
            prediction_x, prediction_y = model.movement_delta(example.features)
            error = abs(prediction_x - example.move_dx) + abs(prediction_y - example.move_dy)
            movement_error += error
            movement_values += 2
            weighted_movement_error += example.weight * error
            weighted_movement_values += 2 * example.weight
    source_counts = dict(sorted(Counter(examples[index].source for index in selected).items()))
    return TrainingMetrics(
        len(examples), len(selected), correct / len(selected),
        movement_error / movement_values if movement_values else None,
        {action: correct_by_action[action] / totals[action] if totals[action] else 0.0 for action in ACTION_TYPES},
        weighted_correct / max(total_weight, 1e-9),
        weighted_movement_error / weighted_movement_values if weighted_movement_values else None,
        training_groups, validation_groups, group_overlap, source_counts,
    )


def _arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input-jsonl", action="append", default=[])
    parser.add_argument("--replay-jsonl", action="append", default=[])
    parser.add_argument("--bootstrap-examples", type=int, default=5000)
    parser.add_argument("--synthetic-weight", type=float, default=0.25)
    parser.add_argument("--reviewed-weight", type=float, default=0.75)
    parser.add_argument("--seed", type=int, default=2026)
    parser.add_argument("--epochs", type=int, default=55)
    parser.add_argument("--learning-rate", type=float, default=0.055)
    parser.add_argument("--validation-fraction", type=float, default=0.15)
    parser.add_argument("--replay-min-confidence", type=float, default=0.60)
    parser.add_argument("--replay-min-feature-coverage", type=float, default=0.70)
    parser.add_argument("--replay-min-profile-confidence", type=float, default=0.60)
    parser.add_argument("--replay-positive-weight", type=float, default=1.0)
    parser.add_argument("--replay-neutral-weight", type=float, default=0.45)
    parser.add_argument("--replay-negative-weight", type=float, default=0.90)
    parser.add_argument("--exclude-neutral-replay", action="store_true")
    parser.add_argument("--output", type=Path, default=Path(__file__).with_name("models") / "unit_policy_v1.json")
    parser.add_argument("--policy-version", default="unit-policy-v1")
    return parser.parse_args()


def main() -> int:
    arguments = _arguments()
    examples = generate_synthetic_examples(arguments.bootstrap_examples, arguments.seed, weight=arguments.synthetic_weight) if arguments.bootstrap_examples else []
    replay_summaries: list[dict[str, Any]] = []
    for path in arguments.input_jsonl:
        examples.extend(load_jsonl(path, weight=arguments.reviewed_weight))
    config = ReplayAdapterConfig(
        arguments.replay_min_confidence,
        arguments.replay_min_feature_coverage,
        arguments.replay_min_profile_confidence,
        arguments.replay_positive_weight,
        arguments.replay_neutral_weight,
        arguments.replay_negative_weight,
        include_neutral=not arguments.exclude_neutral_replay,
    )
    for path in arguments.replay_jsonl:
        converted, stats = load_labeled_replay_examples(path, config)
        examples.extend(TrainingExample(item.features, item.action, item.move_dx, item.move_dy, item.weight, item.group_id, item.source) for item in converted)
        replay_summaries.append({"path": str(path), **stats.to_dict()})
    model, metrics = train_policy(
        examples,
        seed=arguments.seed,
        epochs=arguments.epochs,
        learning_rate=arguments.learning_rate,
        validation_fraction=arguments.validation_fraction,
        policy_version=arguments.policy_version,
        extra_metadata={"replayConversion": replay_summaries},
    )
    model.save(arguments.output)
    print(json.dumps({
        "output": str(arguments.output),
        "policyVersion": model.version,
        "examples": metrics.examples,
        "validationExamples": metrics.validation_examples,
        "actionAccuracy": round(metrics.action_accuracy, 6),
        "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 6),
        "movementMae": None if metrics.movement_mae is None else round(metrics.movement_mae, 6),
        "weightedMovementMae": None if metrics.weighted_movement_mae is None else round(metrics.weighted_movement_mae, 6),
        "groupOverlap": metrics.group_overlap,
        "sources": metrics.source_examples,
        "replay": replay_summaries,
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
