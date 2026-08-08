"""Interpretable offline highlight scoring for authored scenario signals.

This module intentionally uses no model or network service. It preserves the
small explainable score used by the three reviewed fixture summaries.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


@dataclass(frozen=True)
class Signals:
    win_probability_swing: float
    event_density: float
    entity_proximity: float
    resource_asymmetry: float

    def validate(self) -> None:
        for name, value in self.__dict__.items():
            if not 0 <= value <= 1:
                raise ValueError(f"{name} must be between 0 and 1")


@dataclass(frozen=True)
class Candidate:
    start_second: int
    end_second: int
    score: float
    reason_tags: tuple[str, ...]


def score(signals: Signals) -> float:
    """Return a bounded, explainable highlight score."""
    signals.validate()
    value = (
        signals.win_probability_swing * 0.45
        + signals.event_density * 0.20
        + (1 - signals.entity_proximity) * 0.20
        + signals.resource_asymmetry * 0.15
    )
    return min(1.0, max(0.0, value))


def reason_tags(signals: Signals) -> tuple[str, ...]:
    tags: list[str] = []
    if signals.win_probability_swing >= 0.65:
        tags.append("win-probability-swing")
    if signals.event_density >= 0.75:
        tags.append("high-event-density")
    if signals.entity_proximity <= 0.25:
        tags.append("team-fight")
    if signals.resource_asymmetry >= 0.6:
        tags.append("resource-disadvantage")
    return tuple(tags or ["strategic-decision"])


def select_candidates(
    windows: Iterable[tuple[int, int, Signals]], threshold: float = 0.65
) -> list[Candidate]:
    selected = [
        Candidate(start, end, score(signals), reason_tags(signals))
        for start, end, signals in windows
        if end > start and score(signals) >= threshold
    ]
    return sorted(selected, key=lambda candidate: (-candidate.score, candidate.start_second))


def fixtures_as_candidates(path: Path) -> list[Candidate]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    candidates = []
    for moment in payload["moments"]:
        raw = moment["signals"]
        signals = Signals(
            raw["winProbabilitySwing"],
            raw["eventDensity"],
            raw["entityProximity"],
            raw["resourceAsymmetry"],
        )
        start = moment["startTimeSeconds"]
        candidates.append(
            Candidate(start, start + 12, score(signals), tuple(moment["reasonTags"]))
        )
    return candidates


if __name__ == "__main__":
    root = Path(__file__).resolve().parents[1]
    for candidate in fixtures_as_candidates(root / "fixtures" / "moments.json"):
        print(
            json.dumps(
                {
                    "startSecond": candidate.start_second,
                    "endSecond": candidate.end_second,
                    "score": round(candidate.score, 4),
                    "reasonTags": candidate.reason_tags,
                }
            )
        )
