"""Train and evaluate a deterministic replay action-class baseline."""

from __future__ import annotations

import argparse
import hashlib
import json
from collections import Counter, defaultdict
from dataclasses import dataclass
from pathlib import Path

from ml.features.replay_actions import FEATURE_SCHEMA_VERSION, Example, extract_examples
from ml.ingest.replays import read_matches

LABELS = ("attack", "cast", "item", "move")
MODEL_TYPE = "smoothed-context-frequency"


@dataclass(frozen=True)
class Metrics:
    examples: int
    top1_accuracy: float
    top3_accuracy: float
    brier_score: float


def split_for_match(index: int, seed: int) -> str:
    digest = hashlib.sha256(f"{seed}:{index}".encode()).digest()
    bucket = int.from_bytes(digest[:8], "big") % 10
    return "test" if bucket < 2 else "train"


def train(examples: list[Example]) -> dict[str, object]:
    global_counts = Counter(example.label for example in examples)
    context_counts: dict[str, Counter[str]] = defaultdict(Counter)
    for example in examples:
        context_counts["|".join(example.context)][example.label] += 1
    return {
        "labels": list(LABELS),
        "globalCounts": dict(sorted(global_counts.items())),
        "contextCounts": {
            context: dict(sorted(counts.items()))
            for context, counts in sorted(context_counts.items())
        },
    }


def probabilities(model: dict[str, object], context: tuple[str, str, str]) -> dict[str, float]:
    all_contexts = model["contextCounts"]
    assert isinstance(all_contexts, dict)
    counts = all_contexts.get("|".join(context), model["globalCounts"])
    assert isinstance(counts, dict)
    total = sum(int(counts.get(label, 0)) + 1 for label in LABELS)
    return {label: (int(counts.get(label, 0)) + 1) / total for label in LABELS}


def evaluate(model: dict[str, object], examples: list[Example]) -> Metrics:
    if not examples:
        raise ValueError("test split has no examples")
    top1 = top3 = 0
    brier = 0.0
    for example in examples:
        probs = probabilities(model, example.context)
        ranked = sorted(LABELS, key=lambda label: (-probs[label], label))
        top1 += ranked[0] == example.label
        top3 += example.label in ranked[:3]
        brier += sum((probs[label] - (label == example.label)) ** 2 for label in LABELS)
    count = len(examples)
    return Metrics(count, top1 / count, top3 / count, brier / count)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(source: Path, output: Path, max_matches: int, seed: int) -> dict[str, object]:
    train_examples: list[Example] = []
    test_examples: list[Example] = []
    split_counts = Counter()
    for index, events in enumerate(read_matches(source, max_matches)):
        split = split_for_match(index, seed)
        split_counts[split] += 1
        target = test_examples if split == "test" else train_examples
        target.extend(extract_examples(events))
    if not train_examples or not test_examples:
        raise ValueError("sample must produce non-empty train and test match splits")
    model = train(train_examples)
    metrics = evaluate(model, test_examples)
    artifact = {
        "schemaVersion": "1.0",
        "modelType": MODEL_TYPE,
        "featureSchemaVersion": FEATURE_SCHEMA_VERSION,
        "dataset": {
            "repoId": "maknee/league-of-legends-decoded-replay-packets",
            "patch": "12_22",
            "sourceFile": source.name,
            "sourceSha256": sha256(source),
            "maxMatches": max_matches,
            "splitSeed": seed,
            "trainMatches": split_counts["train"],
            "testMatches": split_counts["test"],
        },
        "trainingExamples": len(train_examples),
        "metrics": {
            "examples": metrics.examples,
            "top1ActionAgreement": metrics.top1_accuracy,
            "top3ActionAgreement": metrics.top3_accuracy,
            "multiclassBrierScore": metrics.brier_score,
        },
        "model": model,
        "limitations": [
            "Predicts coarse observed packet action classes, not optimal simulator actions.",
            "The sample has no stable player identity or role labels for player-specific claims.",
            "Not wired into the authoritative Go simulator or online model daemon.",
        ],
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(artifact, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return artifact


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("--output", type=Path, default=Path(".local-data/models/replay-policy.json"))
    parser.add_argument("--max-matches", type=int, default=12)
    parser.add_argument("--seed", type=int, default=742)
    args = parser.parse_args()
    artifact = run(args.source, args.output, args.max_matches, args.seed)
    print(json.dumps({"dataset": artifact["dataset"], "trainingExamples": artifact["trainingExamples"], "metrics": artifact["metrics"]}, indent=2))


if __name__ == "__main__":
    main()
