"""Evaluate the deterministic highlight detector on analyst-labelled telemetry.

The evaluation pack is intentionally small, synthetic, identity-free, and
checked into the repository. It is a regression suite, not evidence of
production accuracy on real matches.
"""

from __future__ import annotations

import argparse
import json
import math
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from ml.highlight import Candidate
from ml.telemetry import (
    category_for_reason_tags,
    load_frames,
    select_pivotal_windows,
)


PACK_VERSION = "1.0"
VALID_CATEGORIES = frozenset(
    {
        "objective-contest",
        "team-fight-engagement",
        "escape",
        "positioning",
        "resource-trade",
        "vision-uncertainty",
    }
)


@dataclass(frozen=True)
class DetectorConfig:
    threshold: float
    window_seconds: int
    stride_seconds: int
    max_overlap_fraction: float
    minimum_match_overlap: float


@dataclass(frozen=True)
class RegressionThresholds:
    minimum_precision: float
    minimum_recall: float
    minimum_category_accuracy: float
    minimum_top_rank_accuracy: float
    maximum_false_positive_match_rate: float


@dataclass(frozen=True)
class ExpectedHighlight:
    start_second: int
    end_second: int
    category: str
    primary: bool


@dataclass(frozen=True)
class EvaluationCase:
    id: str
    description: str
    telemetry_path: Path
    expected_highlights: tuple[ExpectedHighlight, ...]


@dataclass(frozen=True)
class EvaluationPack:
    path: Path
    detector: DetectorConfig
    thresholds: RegressionThresholds
    cases: tuple[EvaluationCase, ...]


@dataclass(frozen=True)
class Prediction:
    candidate: Candidate
    category: str


@dataclass(frozen=True)
class Match:
    expected_index: int
    prediction_index: int
    overlap: float


@dataclass(frozen=True)
class CaseResult:
    case: EvaluationCase
    predictions: tuple[Prediction, ...]
    matches: tuple[Match, ...]
    false_positive_indices: tuple[int, ...]
    missed_indices: tuple[int, ...]
    category_correct: int
    top_rank_correct: bool | None


@dataclass(frozen=True)
class Metrics:
    case_count: int
    positive_case_count: int
    negative_case_count: int
    expected_highlight_count: int
    prediction_count: int
    true_positive_count: int
    false_positive_count: int
    missed_highlight_count: int
    false_positive_match_count: int
    precision: float
    recall: float
    category_accuracy: float
    top_rank_accuracy: float
    false_positive_match_rate: float


@dataclass(frozen=True)
class EvaluationReport:
    pack: EvaluationPack
    cases: tuple[CaseResult, ...]
    metrics: Metrics
    failures: tuple[str, ...]

    @property
    def passed(self) -> bool:
        return not self.failures


def load_pack(path: Path) -> EvaluationPack:
    """Load and strictly validate one versioned evaluation manifest."""
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot load evaluation pack: {error}") from error
    root = _mapping(payload, "root")
    _exact_keys(
        root,
        {"version", "detector", "regressionThresholds", "cases"},
        "root",
    )
    if root["version"] != PACK_VERSION:
        raise ValueError(f"unsupported evaluation pack version {root['version']!r}")
    detector = _parse_detector(root["detector"])
    thresholds = _parse_thresholds(root["regressionThresholds"])
    raw_cases = root["cases"]
    if not isinstance(raw_cases, list) or not raw_cases:
        raise ValueError("root.cases must be a non-empty array")
    base = path.resolve().parent
    cases = tuple(
        _parse_case(raw_case, f"cases[{index}]", base)
        for index, raw_case in enumerate(raw_cases)
    )
    ids = [case.id for case in cases]
    if len(ids) != len(set(ids)):
        raise ValueError("evaluation case IDs must be unique")
    covered = {
        expected.category
        for case in cases
        for expected in case.expected_highlights
    }
    missing_categories = VALID_CATEGORIES - covered
    if missing_categories:
        raise ValueError(
            "evaluation pack is missing categories: "
            + ", ".join(sorted(missing_categories))
        )
    if not any(not case.expected_highlights for case in cases):
        raise ValueError("evaluation pack requires an ordinary no-highlight case")
    return EvaluationPack(path.resolve(), detector, thresholds, cases)


def evaluate_pack(pack: EvaluationPack) -> EvaluationReport:
    """Run every case and calculate deterministic match-level metrics."""
    case_results = tuple(_evaluate_case(case, pack.detector) for case in pack.cases)
    expected_count = sum(len(case.case.expected_highlights) for case in case_results)
    prediction_count = sum(len(case.predictions) for case in case_results)
    true_positives = sum(len(case.matches) for case in case_results)
    false_positives = sum(len(case.false_positive_indices) for case in case_results)
    missed = sum(len(case.missed_indices) for case in case_results)
    category_correct = sum(case.category_correct for case in case_results)
    positive_cases = tuple(case for case in case_results if case.case.expected_highlights)
    negative_cases = tuple(case for case in case_results if not case.case.expected_highlights)
    false_positive_matches = sum(bool(case.predictions) for case in negative_cases)
    top_rank_correct = sum(case.top_rank_correct is True for case in positive_cases)
    metrics = Metrics(
        case_count=len(case_results),
        positive_case_count=len(positive_cases),
        negative_case_count=len(negative_cases),
        expected_highlight_count=expected_count,
        prediction_count=prediction_count,
        true_positive_count=true_positives,
        false_positive_count=false_positives,
        missed_highlight_count=missed,
        false_positive_match_count=false_positive_matches,
        precision=_ratio(true_positives, true_positives + false_positives),
        recall=_ratio(true_positives, expected_count),
        category_accuracy=_ratio(category_correct, true_positives),
        top_rank_accuracy=_ratio(top_rank_correct, len(positive_cases)),
        false_positive_match_rate=_ratio(false_positive_matches, len(negative_cases)),
    )
    return EvaluationReport(
        pack,
        case_results,
        metrics,
        _regression_failures(metrics, pack.thresholds),
    )


def render_markdown(report: EvaluationReport) -> str:
    """Render a compact human-readable report suitable for analyst review."""
    metrics = report.metrics
    lines = [
        "# Detector evaluation report",
        "",
        f"Pack: `{report.pack.path.name}` (version {PACK_VERSION})",
        "",
        "| Case | Expected | Detected | TP | FP | Missed | Top-ranked primary |",
        "| --- | ---: | ---: | ---: | ---: | ---: | --- |",
    ]
    for result in report.cases:
        if result.top_rank_correct is None:
            top_rank = "n/a"
        else:
            top_rank = "yes" if result.top_rank_correct else "no"
        lines.append(
            f"| {result.case.id} | {len(result.case.expected_highlights)} | "
            f"{len(result.predictions)} | {len(result.matches)} | "
            f"{len(result.false_positive_indices)} | {len(result.missed_indices)} | "
            f"{top_rank} |"
        )
    lines.extend(
        [
            "",
            "## Metrics",
            "",
            f"- Precision: {_percent(metrics.precision)} "
            f"({metrics.true_positive_count}/{metrics.true_positive_count + metrics.false_positive_count})",
            f"- Recall: {_percent(metrics.recall)} "
            f"({metrics.true_positive_count}/{metrics.expected_highlight_count})",
            f"- Category accuracy: {_percent(metrics.category_accuracy)}",
            f"- Best moment ranked first: {_percent(metrics.top_rank_accuracy)}",
            f"- Ordinary matches with a false positive: "
            f"{_percent(metrics.false_positive_match_rate)} "
            f"({metrics.false_positive_match_count}/{metrics.negative_case_count})",
            f"- Window false positives: {metrics.false_positive_count}",
            f"- Missed highlights: {metrics.missed_highlight_count}",
            "",
            "## Regression gate",
            "",
        ]
    )
    if report.passed:
        lines.append("PASS — all configured thresholds were met.")
    else:
        lines.append("FAIL")
        lines.extend(f"- {failure}" for failure in report.failures)
    lines.extend(
        [
            "",
            "> This synthetic pack is a deterministic regression suite. It does not "
            "estimate production accuracy on real or publisher telemetry.",
        ]
    )
    return "\n".join(lines) + "\n"


def report_as_dict(report: EvaluationReport) -> dict[str, Any]:
    """Return a stable machine-readable form for CI and later dashboards."""
    metrics = report.metrics
    return {
        "schemaVersion": PACK_VERSION,
        "pack": report.pack.path.name,
        "passed": report.passed,
        "metrics": {
            "caseCount": metrics.case_count,
            "positiveCaseCount": metrics.positive_case_count,
            "negativeCaseCount": metrics.negative_case_count,
            "expectedHighlightCount": metrics.expected_highlight_count,
            "predictionCount": metrics.prediction_count,
            "truePositiveCount": metrics.true_positive_count,
            "falsePositiveCount": metrics.false_positive_count,
            "missedHighlightCount": metrics.missed_highlight_count,
            "falsePositiveMatchCount": metrics.false_positive_match_count,
            "precision": metrics.precision,
            "recall": metrics.recall,
            "categoryAccuracy": metrics.category_accuracy,
            "topRankAccuracy": metrics.top_rank_accuracy,
            "falsePositiveMatchRate": metrics.false_positive_match_rate,
        },
        "failures": list(report.failures),
        "cases": [
            {
                "id": result.case.id,
                "expected": len(result.case.expected_highlights),
                "detected": len(result.predictions),
                "truePositives": len(result.matches),
                "falsePositives": len(result.false_positive_indices),
                "missed": len(result.missed_indices),
                "topRankCorrect": result.top_rank_correct,
                "predictions": [
                    {
                        "startSecond": prediction.candidate.start_second,
                        "endSecond": prediction.candidate.end_second,
                        "score": round(prediction.candidate.score, 4),
                        "category": prediction.category,
                        "reasonTags": list(prediction.candidate.reason_tags),
                    }
                    for prediction in result.predictions
                ],
            }
            for result in report.cases
        ],
    }


def _evaluate_case(case: EvaluationCase, config: DetectorConfig) -> CaseResult:
    candidates = select_pivotal_windows(
        load_frames(case.telemetry_path),
        threshold=config.threshold,
        window_seconds=config.window_seconds,
        stride_seconds=config.stride_seconds,
        max_overlap_fraction=config.max_overlap_fraction,
    )
    predictions = tuple(
        Prediction(candidate, category_for_reason_tags(candidate.reason_tags))
        for candidate in candidates
    )
    unused_predictions = set(range(len(predictions)))
    matches: list[Match] = []
    for expected_index, expected in enumerate(case.expected_highlights):
        choices = [
            (
                _interval_overlap(
                    expected.start_second,
                    expected.end_second,
                    prediction.candidate.start_second,
                    prediction.candidate.end_second,
                ),
                -prediction_index,
                prediction_index,
            )
            for prediction_index, prediction in enumerate(predictions)
            if prediction_index in unused_predictions
        ]
        overlap, _, prediction_index = max(choices, default=(0.0, 0, -1))
        if overlap >= config.minimum_match_overlap:
            matches.append(Match(expected_index, prediction_index, overlap))
            unused_predictions.remove(prediction_index)
    matched_expected = {match.expected_index for match in matches}
    category_correct = sum(
        predictions[match.prediction_index].category
        == case.expected_highlights[match.expected_index].category
        for match in matches
    )
    primary_index = next(
        (
            index
            for index, expected in enumerate(case.expected_highlights)
            if expected.primary
        ),
        None,
    )
    top_rank_correct: bool | None = None
    if primary_index is not None:
        top_rank_correct = any(
            match.expected_index == primary_index and match.prediction_index == 0
            for match in matches
        )
    return CaseResult(
        case=case,
        predictions=predictions,
        matches=tuple(matches),
        false_positive_indices=tuple(sorted(unused_predictions)),
        missed_indices=tuple(
            index
            for index in range(len(case.expected_highlights))
            if index not in matched_expected
        ),
        category_correct=category_correct,
        top_rank_correct=top_rank_correct,
    )


def _parse_detector(value: Any) -> DetectorConfig:
    raw = _mapping(value, "detector")
    _exact_keys(
        raw,
        {
            "threshold",
            "windowSeconds",
            "strideSeconds",
            "maxOverlapFraction",
            "minimumMatchOverlap",
        },
        "detector",
    )
    return DetectorConfig(
        threshold=_fraction(raw["threshold"], "detector.threshold"),
        window_seconds=_positive_integer(
            raw["windowSeconds"], "detector.windowSeconds"
        ),
        stride_seconds=_positive_integer(
            raw["strideSeconds"], "detector.strideSeconds"
        ),
        max_overlap_fraction=_fraction(
            raw["maxOverlapFraction"], "detector.maxOverlapFraction"
        ),
        minimum_match_overlap=_positive_fraction(
            raw["minimumMatchOverlap"], "detector.minimumMatchOverlap"
        ),
    )


def _parse_thresholds(value: Any) -> RegressionThresholds:
    raw = _mapping(value, "regressionThresholds")
    required = {
        "minimumPrecision",
        "minimumRecall",
        "minimumCategoryAccuracy",
        "minimumTopRankAccuracy",
        "maximumFalsePositiveMatchRate",
    }
    _exact_keys(raw, required, "regressionThresholds")
    return RegressionThresholds(
        minimum_precision=_fraction(
            raw["minimumPrecision"], "regressionThresholds.minimumPrecision"
        ),
        minimum_recall=_fraction(
            raw["minimumRecall"], "regressionThresholds.minimumRecall"
        ),
        minimum_category_accuracy=_fraction(
            raw["minimumCategoryAccuracy"],
            "regressionThresholds.minimumCategoryAccuracy",
        ),
        minimum_top_rank_accuracy=_fraction(
            raw["minimumTopRankAccuracy"],
            "regressionThresholds.minimumTopRankAccuracy",
        ),
        maximum_false_positive_match_rate=_fraction(
            raw["maximumFalsePositiveMatchRate"],
            "regressionThresholds.maximumFalsePositiveMatchRate",
        ),
    )


def _parse_case(value: Any, path: str, base: Path) -> EvaluationCase:
    raw = _mapping(value, path)
    _exact_keys(raw, {"id", "description", "telemetry", "expectedHighlights"}, path)
    case_id = _non_empty_string(raw["id"], f"{path}.id")
    description = _non_empty_string(raw["description"], f"{path}.description")
    telemetry_name = _non_empty_string(raw["telemetry"], f"{path}.telemetry")
    telemetry_path = (base / telemetry_name).resolve()
    if not telemetry_path.is_relative_to(base):
        raise ValueError(f"{path}.telemetry must stay inside the pack directory")
    if not telemetry_path.is_file():
        raise ValueError(f"{path}.telemetry does not exist: {telemetry_name}")
    raw_expected = raw["expectedHighlights"]
    if not isinstance(raw_expected, list):
        raise ValueError(f"{path}.expectedHighlights must be an array")
    expected = tuple(
        _parse_expected(item, f"{path}.expectedHighlights[{index}]")
        for index, item in enumerate(raw_expected)
    )
    primary_count = sum(item.primary for item in expected)
    if expected and primary_count != 1:
        raise ValueError(f"{path} must mark exactly one expected highlight primary")
    if not expected and primary_count:
        raise ValueError(f"{path} ordinary case cannot have a primary highlight")
    return EvaluationCase(case_id, description, telemetry_path, expected)


def _parse_expected(value: Any, path: str) -> ExpectedHighlight:
    raw = _mapping(value, path)
    _exact_keys(raw, {"startSecond", "endSecond", "category", "primary"}, path)
    start = _non_negative_integer(raw["startSecond"], f"{path}.startSecond")
    end = _non_negative_integer(raw["endSecond"], f"{path}.endSecond")
    if end <= start:
        raise ValueError(f"{path}.endSecond must be after startSecond")
    category = _non_empty_string(raw["category"], f"{path}.category")
    if category not in VALID_CATEGORIES:
        raise ValueError(f"{path}.category is unsupported: {category!r}")
    primary = raw["primary"]
    if not isinstance(primary, bool):
        raise ValueError(f"{path}.primary must be a boolean")
    return ExpectedHighlight(start, end, category, primary)


def _regression_failures(
    metrics: Metrics, thresholds: RegressionThresholds
) -> tuple[str, ...]:
    checks = (
        ("precision", metrics.precision, thresholds.minimum_precision, "minimum"),
        ("recall", metrics.recall, thresholds.minimum_recall, "minimum"),
        (
            "category accuracy",
            metrics.category_accuracy,
            thresholds.minimum_category_accuracy,
            "minimum",
        ),
        (
            "top-rank accuracy",
            metrics.top_rank_accuracy,
            thresholds.minimum_top_rank_accuracy,
            "minimum",
        ),
        (
            "false-positive match rate",
            metrics.false_positive_match_rate,
            thresholds.maximum_false_positive_match_rate,
            "maximum",
        ),
    )
    failures: list[str] = []
    for name, actual, threshold, direction in checks:
        failed = actual < threshold if direction == "minimum" else actual > threshold
        if failed:
            failures.append(
                f"{name} {_percent(actual)} did not meet {direction} {_percent(threshold)}"
            )
    return tuple(failures)


def _interval_overlap(
    first_start: int, first_end: int, second_start: int, second_end: int
) -> float:
    intersection = max(0, min(first_end, second_end) - max(first_start, second_start))
    union = max(first_end, second_end) - min(first_start, second_start)
    return intersection / union if union else 0.0


def _ratio(numerator: int, denominator: int) -> float:
    return numerator / denominator if denominator else 1.0


def _percent(value: float) -> str:
    return f"{value * 100:.1f}%"


def _mapping(value: Any, path: str) -> Mapping[str, Any]:
    if not isinstance(value, dict):
        raise ValueError(f"{path} must be an object")
    return value


def _exact_keys(value: Mapping[str, Any], required: set[str], path: str) -> None:
    missing = required - value.keys()
    unknown = value.keys() - required
    if missing:
        raise ValueError(f"{path} is missing {', '.join(sorted(missing))}")
    if unknown:
        raise ValueError(f"{path} has unknown fields: {', '.join(sorted(unknown))}")


def _non_empty_string(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{path} must be a non-empty string")
    return value.strip()


def _fraction(value: Any, path: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{path} must be a number")
    number = float(value)
    if not math.isfinite(number) or not 0 <= number <= 1:
        raise ValueError(f"{path} must be between 0 and 1")
    return number


def _positive_integer(value: Any, path: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise ValueError(f"{path} must be a positive integer")
    return value


def _positive_fraction(value: Any, path: str) -> float:
    number = _fraction(value, path)
    if number == 0:
        raise ValueError(f"{path} must be greater than 0")
    return number


def _non_negative_integer(value: Any, path: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValueError(f"{path} must be a non-negative integer")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--pack",
        type=Path,
        default=Path("fixtures/telemetry-evaluation/manifest.json"),
        help="versioned evaluation manifest",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="emit the stable machine-readable report instead of Markdown",
    )
    args = parser.parse_args()
    try:
        report = evaluate_pack(load_pack(args.pack))
    except ValueError as error:
        parser.error(str(error))
    if args.json:
        print(json.dumps(report_as_dict(report), indent=2, sort_keys=True))
    else:
        print(render_markdown(report), end="")
    if not report.passed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
