"""Deterministic pivotal-window extraction from normalized match telemetry.

The adapter is intentionally game-agnostic. Authorized telemetry must first be
normalized to the small JSON contract accepted by :func:`load_frames`; this
module then derives the four interpretable signals used by ``ml.highlight``.
"""

from __future__ import annotations

import argparse
import json
import math
from bisect import bisect_left
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from ml.highlight import Candidate, Signals, select_candidates


SCHEMA_VERSION = "1.0"
MAP_MIN = 0.0
MAP_MAX = 100.0
MAP_DIAGONAL = math.hypot(MAP_MAX - MAP_MIN, MAP_MAX - MAP_MIN)
DEFAULT_EVENT_RATE_CAP = 2.0
COUNTED_EVENT_TYPES = frozenset({"damage", "kill", "objective"})


@dataclass(frozen=True)
class TelemetryUnit:
    id: str
    team: str
    x: float
    y: float
    hp: float
    max_hp: float
    gold: float
    alive: bool

    def validate(self) -> None:
        if not self.id or not self.team:
            raise ValueError("unit id and team must be non-empty")
        for name, value in (("x", self.x), ("y", self.y)):
            if not math.isfinite(value) or not MAP_MIN <= value <= MAP_MAX:
                raise ValueError(f"unit {self.id} {name} must be between 0 and 100")
        if not math.isfinite(self.max_hp) or self.max_hp <= 0:
            raise ValueError(f"unit {self.id} max_hp must be positive")
        if not math.isfinite(self.hp) or not 0 <= self.hp <= self.max_hp:
            raise ValueError(f"unit {self.id} hp must be between 0 and max_hp")
        if not math.isfinite(self.gold) or self.gold < 0:
            raise ValueError(f"unit {self.id} gold cannot be negative")


@dataclass(frozen=True)
class TelemetryFrame:
    second: int
    win_probability: float
    units: tuple[TelemetryUnit, ...]
    event_types: tuple[str, ...] = ()

    def validate(self) -> None:
        if self.second < 0:
            raise ValueError("frame second cannot be negative")
        if not math.isfinite(self.win_probability) or not 0 <= self.win_probability <= 1:
            raise ValueError("win_probability must be between 0 and 1")
        if len(self.units) < 2:
            raise ValueError("each frame must contain at least two units")
        ids: set[str] = set()
        for unit in self.units:
            unit.validate()
            if unit.id in ids:
                raise ValueError(f"duplicate unit id {unit.id!r} in frame {self.second}")
            ids.add(unit.id)
        unknown_events = set(self.event_types) - COUNTED_EVENT_TYPES
        if unknown_events:
            formatted = ", ".join(sorted(unknown_events))
            raise ValueError(f"unknown event types: {formatted}")


def validate_frames(frames: Sequence[TelemetryFrame]) -> tuple[TelemetryFrame, ...]:
    """Validate ordering and roster identity, returning an immutable snapshot."""
    immutable = tuple(frames)
    if len(immutable) < 2:
        raise ValueError("telemetry requires at least two frames")

    previous_second = -1
    expected_roster: dict[str, str] | None = None
    for frame in immutable:
        frame.validate()
        if frame.second <= previous_second:
            raise ValueError("frame seconds must be strictly increasing")
        previous_second = frame.second

        roster = {unit.id: unit.team for unit in frame.units}
        if expected_roster is None:
            expected_roster = roster
            teams = set(roster.values())
            if len(teams) != 2:
                raise ValueError("telemetry must contain exactly two teams")
        elif roster != expected_roster:
            raise ValueError("unit ids and teams must remain stable across frames")

    return immutable


def extract_signals(
    frames: Sequence[TelemetryFrame],
    *,
    event_rate_cap: float = DEFAULT_EVENT_RATE_CAP,
) -> Signals:
    """Derive normalized highlight signals for one fully covered time window.

    ``event_rate_cap`` is the events-per-second rate that maps to a density of
    one. Proximity uses the closest live opposing pair anywhere in the window.
    Resource asymmetry is the largest mean of normalized team HP and gold gaps.
    """
    checked = validate_frames(frames)
    if not math.isfinite(event_rate_cap) or event_rate_cap <= 0:
        raise ValueError("event_rate_cap must be positive")
    duration = checked[-1].second - checked[0].second
    if duration <= 0:
        raise ValueError("telemetry window must span at least one second")

    probabilities = [frame.win_probability for frame in checked]
    win_probability_swing = max(probabilities) - min(probabilities)
    event_count = sum(len(frame.event_types) for frame in checked)
    event_density = min(1.0, event_count / duration / event_rate_cap)
    entity_proximity = min(_nearest_opponent_distance(frame) for frame in checked)
    entity_proximity /= MAP_DIAGONAL
    resource_asymmetry = max(_resource_asymmetry(frame) for frame in checked)

    signals = Signals(
        win_probability_swing=win_probability_swing,
        event_density=event_density,
        entity_proximity=entity_proximity,
        resource_asymmetry=resource_asymmetry,
    )
    signals.validate()
    return signals


def telemetry_windows(
    frames: Sequence[TelemetryFrame],
    *,
    window_seconds: int = 12,
    stride_seconds: int = 2,
    event_rate_cap: float = DEFAULT_EVENT_RATE_CAP,
) -> list[tuple[int, int, Signals]]:
    """Build fully covered sliding windows from an irregular frame sequence."""
    checked = validate_frames(frames)
    if window_seconds <= 0:
        raise ValueError("window_seconds must be positive")
    if stride_seconds <= 0:
        raise ValueError("stride_seconds must be positive")

    timestamps = [frame.second for frame in checked]
    windows: list[tuple[int, int, Signals]] = []
    next_start = timestamps[0]
    for start_index, frame in enumerate(checked):
        if frame.second < next_start:
            continue
        target_end = frame.second + window_seconds
        end_index = bisect_left(timestamps, target_end, lo=start_index + 1)
        if end_index == len(checked):
            break
        window = checked[start_index : end_index + 1]
        windows.append(
            (
                frame.second,
                checked[end_index].second,
                extract_signals(window, event_rate_cap=event_rate_cap),
            )
        )
        next_start = frame.second + stride_seconds
    return windows


def select_pivotal_windows(
    frames: Sequence[TelemetryFrame],
    *,
    threshold: float = 0.65,
    window_seconds: int = 12,
    stride_seconds: int = 2,
    max_overlap_fraction: float = 0.5,
    event_rate_cap: float = DEFAULT_EVENT_RATE_CAP,
) -> list[Candidate]:
    """Rank pivotal windows and suppress near-duplicate overlapping moments."""
    if not math.isfinite(threshold) or not 0 <= threshold <= 1:
        raise ValueError("threshold must be between 0 and 1")
    if not math.isfinite(max_overlap_fraction) or not 0 <= max_overlap_fraction <= 1:
        raise ValueError("max_overlap_fraction must be between 0 and 1")

    ranked = select_candidates(
        telemetry_windows(
            frames,
            window_seconds=window_seconds,
            stride_seconds=stride_seconds,
            event_rate_cap=event_rate_cap,
        ),
        threshold=threshold,
    )
    selected: list[Candidate] = []
    for candidate in ranked:
        if all(
            _overlap_fraction(candidate, existing) <= max_overlap_fraction
            for existing in selected
        ):
            selected.append(candidate)
    return selected


def load_frames(path: Path) -> tuple[TelemetryFrame, ...]:
    """Load the strict normalized telemetry JSON contract from ``path``."""
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot load telemetry: {error}") from error

    root = _mapping(payload, "root")
    _exact_keys(root, {"version", "frames"}, set(), "root")
    if root["version"] != SCHEMA_VERSION:
        raise ValueError(f"unsupported telemetry version {root['version']!r}")
    raw_frames = root["frames"]
    if not isinstance(raw_frames, list):
        raise ValueError("root.frames must be an array")
    return validate_frames(
        tuple(_parse_frame(raw, f"frames[{index}]") for index, raw in enumerate(raw_frames))
    )


def _parse_frame(value: Any, path: str) -> TelemetryFrame:
    raw = _mapping(value, path)
    _exact_keys(raw, {"second", "winProbability", "units"}, {"events"}, path)
    units = raw["units"]
    events = raw.get("events", [])
    if not isinstance(units, list):
        raise ValueError(f"{path}.units must be an array")
    if not isinstance(events, list) or not all(isinstance(event, str) for event in events):
        raise ValueError(f"{path}.events must be an array of strings")
    return TelemetryFrame(
        second=_integer(raw["second"], f"{path}.second"),
        win_probability=_number(raw["winProbability"], f"{path}.winProbability"),
        units=tuple(
            _parse_unit(unit, f"{path}.units[{index}]")
            for index, unit in enumerate(units)
        ),
        event_types=tuple(events),
    )


def _parse_unit(value: Any, path: str) -> TelemetryUnit:
    raw = _mapping(value, path)
    _exact_keys(
        raw,
        {"id", "team", "position", "hp", "maxHp", "gold"},
        {"alive"},
        path,
    )
    position = _mapping(raw["position"], f"{path}.position")
    _exact_keys(position, {"x", "y"}, set(), f"{path}.position")
    if not isinstance(raw["id"], str) or not isinstance(raw["team"], str):
        raise ValueError(f"{path}.id and team must be strings")
    alive = raw.get("alive", True)
    if not isinstance(alive, bool):
        raise ValueError(f"{path}.alive must be a boolean")
    return TelemetryUnit(
        id=raw["id"],
        team=raw["team"],
        x=_number(position["x"], f"{path}.position.x"),
        y=_number(position["y"], f"{path}.position.y"),
        hp=_number(raw["hp"], f"{path}.hp"),
        max_hp=_number(raw["maxHp"], f"{path}.maxHp"),
        gold=_number(raw["gold"], f"{path}.gold"),
        alive=alive,
    )


def _nearest_opponent_distance(frame: TelemetryFrame) -> float:
    live_units = [unit for unit in frame.units if unit.alive]
    nearest = MAP_DIAGONAL
    for index, unit in enumerate(live_units):
        for opponent in live_units[index + 1 :]:
            if unit.team == opponent.team:
                continue
            nearest = min(nearest, math.hypot(unit.x - opponent.x, unit.y - opponent.y))
    return nearest


def _resource_asymmetry(frame: TelemetryFrame) -> float:
    totals: dict[str, list[float]] = {}
    for unit in frame.units:
        current_hp, max_hp, gold = totals.setdefault(unit.team, [0.0, 0.0, 0.0])
        totals[unit.team] = [current_hp + unit.hp, max_hp + unit.max_hp, gold + unit.gold]
    if len(totals) != 2:
        raise ValueError("resource asymmetry requires exactly two teams")
    first, second = totals.values()
    hp_gap = abs(first[0] / first[1] - second[0] / second[1])
    combined_gold = first[2] + second[2]
    gold_gap = abs(first[2] - second[2]) / combined_gold if combined_gold else 0.0
    return (hp_gap + gold_gap) / 2


def _overlap_fraction(first: Candidate, second: Candidate) -> float:
    overlap = max(
        0,
        min(first.end_second, second.end_second)
        - max(first.start_second, second.start_second),
    )
    shorter = min(
        first.end_second - first.start_second,
        second.end_second - second.start_second,
    )
    return overlap / shorter if shorter > 0 else 0.0


def _mapping(value: Any, path: str) -> Mapping[str, Any]:
    if not isinstance(value, dict):
        raise ValueError(f"{path} must be an object")
    return value


def _exact_keys(
    value: Mapping[str, Any], required: set[str], optional: set[str], path: str
) -> None:
    missing = required - value.keys()
    unknown = value.keys() - required - optional
    if missing:
        raise ValueError(f"{path} is missing {', '.join(sorted(missing))}")
    if unknown:
        raise ValueError(f"{path} has unknown fields: {', '.join(sorted(unknown))}")


def _number(value: Any, path: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{path} must be a number")
    number = float(value)
    if not math.isfinite(number):
        raise ValueError(f"{path} must be finite")
    return number


def _integer(value: Any, path: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ValueError(f"{path} must be an integer")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("path", type=Path, help="normalized telemetry JSON")
    parser.add_argument("--threshold", type=float, default=0.65)
    parser.add_argument("--window-seconds", type=int, default=12)
    parser.add_argument("--stride-seconds", type=int, default=2)
    parser.add_argument("--max-overlap", type=float, default=0.5)
    args = parser.parse_args()

    candidates = select_pivotal_windows(
        load_frames(args.path),
        threshold=args.threshold,
        window_seconds=args.window_seconds,
        stride_seconds=args.stride_seconds,
        max_overlap_fraction=args.max_overlap,
    )
    for candidate in candidates:
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


if __name__ == "__main__":
    main()
