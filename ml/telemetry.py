"""Deterministic pivotal-window extraction from normalized match telemetry.

The adapter is intentionally game-agnostic. Authorized telemetry must first be
normalized to the small JSON contract accepted by :func:`load_frames`; this
module then derives the four interpretable signals used by ``ml.highlight``.
"""

from __future__ import annotations

import argparse
import json
import math
from bisect import bisect_left, bisect_right
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any

from ml.highlight import Candidate, Signals, score as highlight_score, select_candidates


SCHEMA_VERSION = "1.0"
DETECTION_SCHEMA_VERSION = "1.0"
MAP_MIN = 0.0
MAP_MAX = 100.0
MAP_DIAGONAL = math.hypot(MAP_MAX - MAP_MIN, MAP_MAX - MAP_MIN)
DEFAULT_EVENT_RATE_CAP = 2.0
DEFAULT_ENGAGEMENT_RADIUS = 20.0
DEFAULT_MINIMUM_EXPOSURE_SECONDS = 2
DEFAULT_ESCAPE_SAFE_RADIUS = 35.0
DEFAULT_LOW_HEALTH_FRACTION = 0.35
DEFAULT_REVERSAL_SWING = 0.25
DEFAULT_MINIMUM_COMBAT_EVENTS = 2
COUNTED_EVENT_TYPES = frozenset({"damage", "kill", "objective", "vision-loss"})
COMBAT_EVENT_TYPES = frozenset({"damage", "kill"})


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


@dataclass(frozen=True)
class SemanticEvidence:
    """Auditable detector evidence for one selected telemetry window."""

    one_versus_many_unit_ids: tuple[str, ...] = ()
    successful_escape_unit_ids: tuple[str, ...] = ()
    team_fight_reversal_second: int | None = None


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

    checked = validate_frames(frames)
    ranked = select_candidates(
        telemetry_windows(
            checked,
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
            selected.append(_add_semantic_tags(candidate, checked))
    return selected


def one_versus_many_unit_ids(
    frames: Sequence[TelemetryFrame],
    *,
    engagement_radius: float = DEFAULT_ENGAGEMENT_RADIUS,
    minimum_exposure_seconds: int = DEFAULT_MINIMUM_EXPOSURE_SECONDS,
) -> tuple[str, ...]:
    """Return isolated live units continuously exposed to two or more opponents.

    A unit qualifies while at least two live opponents, but no other live ally,
    are within ``engagement_radius``. The same unit must remain qualified across
    sampled frames spanning ``minimum_exposure_seconds``; a single-frame
    proximity spike is therefore not enough for the default configuration.
    """
    checked = validate_frames(frames)
    _validate_radius(engagement_radius, "engagement_radius")
    if (
        isinstance(minimum_exposure_seconds, bool)
        or not isinstance(minimum_exposure_seconds, int)
        or minimum_exposure_seconds < 0
    ):
        raise ValueError("minimum_exposure_seconds must be a non-negative integer")

    streak_starts: dict[str, int] = {}
    previous_qualifiers: set[str] = set()
    detected: set[str] = set()
    for frame in checked:
        live_units = tuple(unit for unit in frame.units if unit.alive)
        qualifiers = {
            unit.id
            for unit in live_units
            if _is_one_versus_many(unit, live_units, engagement_radius)
        }
        for unit_id in qualifiers:
            if unit_id not in previous_qualifiers:
                streak_starts[unit_id] = frame.second
            if frame.second - streak_starts[unit_id] >= minimum_exposure_seconds:
                detected.add(unit_id)
        for unit_id in previous_qualifiers - qualifiers:
            streak_starts.pop(unit_id, None)
        previous_qualifiers = qualifiers
    return tuple(sorted(detected))


def successful_escape_unit_ids(
    frames: Sequence[TelemetryFrame],
    *,
    danger_radius: float = DEFAULT_ENGAGEMENT_RADIUS,
    safe_radius: float = DEFAULT_ESCAPE_SAFE_RADIUS,
    low_health_fraction: float = DEFAULT_LOW_HEALTH_FRACTION,
    minimum_safe_seconds: int = DEFAULT_MINIMUM_EXPOSURE_SECONDS,
) -> tuple[str, ...]:
    """Return low-health units transitioning from danger to safe separation.

    A qualifying unit must first be alive, at or below ``low_health_fraction``,
    and within ``danger_radius`` of a live opponent. It must finish the sampled
    window alive and at least ``safe_radius`` from every live opponent for a
    continuous span of ``minimum_safe_seconds``. At least one opponent must
    remain alive, preventing an elimination from being classified as an escape.
    """
    checked = validate_frames(frames)
    _validate_radius(danger_radius, "danger_radius")
    _validate_radius(safe_radius, "safe_radius")
    if safe_radius <= danger_radius:
        raise ValueError("safe_radius must be greater than danger_radius")
    if (
        isinstance(low_health_fraction, bool)
        or not math.isfinite(low_health_fraction)
        or low_health_fraction <= 0
        or low_health_fraction > 1
    ):
        raise ValueError("low_health_fraction must be between 0 and 1")
    if (
        isinstance(minimum_safe_seconds, bool)
        or not isinstance(minimum_safe_seconds, int)
        or minimum_safe_seconds < 0
    ):
        raise ValueError("minimum_safe_seconds must be a non-negative integer")

    exposed: set[str] = set()
    safe_starts: dict[str, int] = {}
    detected: set[str] = set()
    for frame in checked:
        live_units = tuple(unit for unit in frame.units if unit.alive)
        for unit in frame.units:
            if not unit.alive:
                exposed.discard(unit.id)
                safe_starts.pop(unit.id, None)
                detected.discard(unit.id)
                continue
            nearest = _nearest_distance_to_opponent(unit, live_units)
            if (
                nearest is not None
                and unit.hp / unit.max_hp <= low_health_fraction
                and nearest <= danger_radius
            ):
                exposed.add(unit.id)
            safe = (
                unit.id in exposed
                and nearest is not None
                and nearest >= safe_radius
            )
            if safe:
                safe_starts.setdefault(unit.id, frame.second)
                if frame.second - safe_starts[unit.id] >= minimum_safe_seconds:
                    detected.add(unit.id)
            else:
                safe_starts.pop(unit.id, None)
                detected.discard(unit.id)
    return tuple(sorted(detected))


def team_fight_reversal_second(
    frames: Sequence[TelemetryFrame],
    *,
    minimum_reversal_swing: float = DEFAULT_REVERSAL_SWING,
    engagement_radius: float = DEFAULT_ENGAGEMENT_RADIUS,
    minimum_combat_events: int = DEFAULT_MINIMUM_COMBAT_EVENTS,
) -> int | None:
    """Return the strongest combat-backed probability turning point, if any.

    The reference team's probability must move at least
    ``minimum_reversal_swing`` into an interior trough or peak and then move at
    least that far in the opposite direction by the final frame. The turning
    frame must contain nearby live opponents, and the window must contain enough
    damage or kill events to distinguish a team fight from a quiet oscillation.
    Ties resolve to the earliest turning point for deterministic fixture IDs.
    """
    checked = validate_frames(frames)
    if (
        isinstance(minimum_reversal_swing, bool)
        or not math.isfinite(minimum_reversal_swing)
        or minimum_reversal_swing <= 0
        or minimum_reversal_swing > 1
    ):
        raise ValueError("minimum_reversal_swing must be between 0 and 1")
    _validate_radius(engagement_radius, "engagement_radius")
    if (
        isinstance(minimum_combat_events, bool)
        or not isinstance(minimum_combat_events, int)
        or minimum_combat_events <= 0
    ):
        raise ValueError("minimum_combat_events must be a positive integer")
    combat_events = sum(
        event_type in COMBAT_EVENT_TYPES
        for frame in checked
        for event_type in frame.event_types
    )
    if combat_events < minimum_combat_events or len(checked) < 3:
        return None

    probabilities = [frame.win_probability for frame in checked]
    final_probability = probabilities[-1]
    reversals: list[tuple[float, int]] = []
    for index in range(1, len(checked) - 1):
        turning_probability = probabilities[index]
        prior = probabilities[:index]
        recovery = min(
            max(prior) - turning_probability,
            final_probability - turning_probability,
        )
        collapse = min(
            turning_probability - min(prior),
            turning_probability - final_probability,
        )
        magnitude = max(recovery, collapse)
        if (
            magnitude >= minimum_reversal_swing
            and _nearest_opponent_distance(checked[index]) <= engagement_radius
        ):
            reversals.append((magnitude, checked[index].second))
    if not reversals:
        return None
    return max(reversals, key=lambda reversal: (reversal[0], -reversal[1]))[1]


def semantic_evidence(
    frames: Sequence[TelemetryFrame], candidate: Candidate
) -> SemanticEvidence:
    """Return deterministic evidence from the exact selected candidate window."""
    window = _candidate_window(frames, candidate)
    return SemanticEvidence(
        one_versus_many_unit_ids=one_versus_many_unit_ids(window),
        successful_escape_unit_ids=successful_escape_unit_ids(window),
        team_fight_reversal_second=team_fight_reversal_second(window),
    )


def signals_for_candidate(
    frames: Sequence[TelemetryFrame],
    candidate: Candidate,
    *,
    event_rate_cap: float = DEFAULT_EVENT_RATE_CAP,
) -> Signals:
    """Recompute canonical signals from the exact selected candidate window."""
    return extract_signals(
        _candidate_window(frames, candidate), event_rate_cap=event_rate_cap
    )


def category_for_reason_tags(reason_tags: Sequence[str]) -> str:
    """Map detector evidence to the analyst draft category.

    Specific semantic evidence takes priority over the generic ``team-fight``
    proximity tag. Keep this ordering aligned with Go's ``drafts.CategoryFor``.
    """
    tags = tuple(tag.strip().lower() for tag in reason_tags)
    if "team-fight-reversal" in tags:
        return "team-fight-engagement"
    if "successful-escape" in tags:
        return "escape"
    if any("objective" in tag for tag in tags):
        return "objective-contest"
    if any(
        fragment in tag
        for tag in tags
        for fragment in ("vision", "fog", "uncertainty")
    ):
        return "vision-uncertainty"
    if any(
        fragment in tag
        for tag in tags
        for fragment in ("resource", "gold", "trade")
    ):
        return "resource-trade"
    if "one-versus-many" in tags:
        return "positioning"
    if "team-fight" in tags:
        return "team-fight-engagement"
    return "positioning"


def detection_record(
    candidate: Candidate, evidence: SemanticEvidence, signals: Signals
) -> dict[str, Any]:
    """Build one versioned, JSON-serializable detector output record."""
    if not math.isclose(
        candidate.score,
        highlight_score(signals),
        rel_tol=0,
        abs_tol=1e-12,
    ):
        raise ValueError("candidate score does not match signals")
    return {
        "schemaVersion": DETECTION_SCHEMA_VERSION,
        "startSecond": candidate.start_second,
        "endSecond": candidate.end_second,
        "score": round(candidate.score, 4),
        "reasonTags": list(candidate.reason_tags),
        "signals": {
            "winProbabilitySwing": signals.win_probability_swing,
            "eventDensity": signals.event_density,
            "entityProximity": signals.entity_proximity,
            "resourceAsymmetry": signals.resource_asymmetry,
        },
        "semanticEvidence": {
            "oneVersusManyUnitIds": list(evidence.one_versus_many_unit_ids),
            "successfulEscapeUnitIds": list(evidence.successful_escape_unit_ids),
            "teamFightReversalSecond": evidence.team_fight_reversal_second,
        },
    }


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


def _nearest_distance_to_opponent(
    focal: TelemetryUnit, live_units: Sequence[TelemetryUnit]
) -> float | None:
    distances = (
        math.hypot(focal.x - other.x, focal.y - other.y)
        for other in live_units
        if other.team != focal.team
    )
    return min(distances, default=None)


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


def _is_one_versus_many(
    focal: TelemetryUnit,
    live_units: Sequence[TelemetryUnit],
    engagement_radius: float,
) -> bool:
    nearby_allies = 0
    nearby_opponents = 0
    for other in live_units:
        if other.id == focal.id:
            continue
        if math.hypot(focal.x - other.x, focal.y - other.y) > engagement_radius:
            continue
        if other.team == focal.team:
            nearby_allies += 1
        else:
            nearby_opponents += 1
    return nearby_allies == 0 and nearby_opponents >= 2


def _add_semantic_tags(
    candidate: Candidate, frames: Sequence[TelemetryFrame]
) -> Candidate:
    evidence = semantic_evidence(frames, candidate)
    window = _candidate_window(frames, candidate)
    event_types = {
        event_type for frame in window for event_type in frame.event_types
    }
    tags = list(candidate.reason_tags)
    if evidence.one_versus_many_unit_ids:
        tags.append("one-versus-many")
    if evidence.successful_escape_unit_ids:
        tags.append("successful-escape")
    if evidence.team_fight_reversal_second is not None:
        tags.append("team-fight-reversal")
    if "objective" in event_types:
        tags.append("objective-contest")
    if "vision-loss" in event_types:
        tags.append("vision-uncertainty")
    return replace(candidate, reason_tags=tuple(tags))


def _candidate_window(
    frames: Sequence[TelemetryFrame], candidate: Candidate
) -> tuple[TelemetryFrame, ...]:
    checked = validate_frames(frames)
    timestamps = [frame.second for frame in checked]
    start_index = bisect_left(timestamps, candidate.start_second)
    end_index = bisect_right(timestamps, candidate.end_second)
    window = checked[start_index:end_index]
    if (
        len(window) < 2
        or window[0].second != candidate.start_second
        or window[-1].second != candidate.end_second
    ):
        raise ValueError("candidate window must be fully covered by telemetry")
    return window


def _validate_radius(value: float, name: str) -> None:
    if (
        isinstance(value, bool)
        or not math.isfinite(value)
        or value <= 0
        or value > MAP_DIAGONAL
    ):
        raise ValueError(f"{name} must be between 0 and the map diagonal")


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

    frames = load_frames(args.path)
    candidates = select_pivotal_windows(
        frames,
        threshold=args.threshold,
        window_seconds=args.window_seconds,
        stride_seconds=args.stride_seconds,
        max_overlap_fraction=args.max_overlap,
    )
    for candidate in candidates:
        print(
            json.dumps(
                detection_record(
                    candidate,
                    semantic_evidence(frames, candidate),
                    signals_for_candidate(frames, candidate),
                )
            )
        )


if __name__ == "__main__":
    main()
