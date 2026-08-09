"""Convert learned scores into legal tactical intent for evaluation demos."""
from __future__ import annotations

import math
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .features import extract_features, nearest_enemy
from .model import LinearUnitPolicy
from .schema import Snapshot, validate_snapshot

DEFAULT_MODEL_PATH = Path(__file__).with_name("models") / "unit_policy_v1.json"


def _clamp(value: float, lower: float, upper: float) -> float:
    return min(upper, max(lower, value))


def _unit_position(unit: dict[str, Any]) -> tuple[float, float]:
    position = unit["position"]
    return float(position["x"]), float(position["y"])


def _direction_toward(source: tuple[float, float], target: tuple[float, float]) -> tuple[float, float]:
    dx, dy = target[0] - source[0], target[1] - source[1]
    magnitude = math.hypot(dx, dy)
    if magnitude <= 1e-12:
        return 0.0, 0.0
    return dx / magnitude, dy / magnitude


@dataclass(frozen=True, slots=True)
class UnitDecision:
    unit_id: str
    action_type: str
    probabilities: dict[str, float]
    target: dict[str, float] | None

    def wire_action(self) -> dict[str, Any]:
        action: dict[str, Any] = {"type": self.action_type}
        if self.target is not None:
            action["target"] = self.target
        return {"unitId": self.unit_id, "action": action}


class UnitPolicy:
    def __init__(self, model: LinearUnitPolicy) -> None:
        self.model = model

    @classmethod
    def load(cls, path: str | Path = DEFAULT_MODEL_PATH) -> "UnitPolicy":
        return cls(LinearUnitPolicy.load(path))

    def decide(self, snapshot: Snapshot, unit_id: str) -> UnitDecision:
        features = extract_features(snapshot, unit_id)
        probabilities = self.model.action_probabilities(features, snapshot.legal_actions)
        action_type = self.model.choose_action(features, snapshot.legal_actions)
        target: dict[str, float] | None = None
        if action_type == "move":
            target = self._movement_target(snapshot, unit_id, features)
            if target is None and "hold" in snapshot.legal_actions:
                action_type = "hold"
        return UnitDecision(unit_id, action_type, probabilities, target if action_type == "move" else None)

    def next_actions(self, raw_snapshot: Any) -> dict[str, Any]:
        snapshot = validate_snapshot(raw_snapshot)
        return {"actions": [self.decide(snapshot, unit_id).wire_action() for unit_id in snapshot.eligible_ids]}

    def _movement_target(self, snapshot: Snapshot, unit_id: str, features: tuple[float, ...]) -> dict[str, float] | None:
        unit = snapshot.units_by_id[unit_id]
        origin = _unit_position(unit)
        dx, dy = self.model.movement_delta(features)
        magnitude = math.hypot(dx, dy)
        if magnitude < 0.05:
            objective = snapshot.raw.get("objective")
            if isinstance(objective, dict):
                position = objective["position"]
                direction = _direction_toward(origin, (float(position["x"]), float(position["y"])))
            else:
                enemy = nearest_enemy(snapshot, unit)
                direction = _direction_toward(origin, _unit_position(enemy)) if enemy is not None else _direction_toward(origin, _unit_position(snapshot.units_by_id[snapshot.controlled_unit_id]))
            dx, dy = direction
            magnitude = math.hypot(dx, dy)
        if magnitude <= 1e-12:
            return None
        if magnitude > 1.0:
            dx, dy = dx / magnitude, dy / magnitude
            magnitude = 1.0
        move_range = max(0.0, float(unit["moveRange"]))
        if move_range <= 0:
            return None
        step = move_range * max(0.25, min(1.0, magnitude))
        direction_length = math.hypot(dx, dy)
        dx, dy = dx / direction_length, dy / direction_length
        target_x = _clamp(origin[0] + dx * step, snapshot.min_x, snapshot.max_x)
        target_y = _clamp(origin[1] + dy * step, snapshot.min_y, snapshot.max_y)
        if math.hypot(target_x - origin[0], target_y - origin[1]) <= 1e-9:
            return None
        return {"x": round(target_x, 6), "y": round(target_y, 6)}
