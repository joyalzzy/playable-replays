"""Strict schema helpers for unit-policy training and evaluation."""
from __future__ import annotations

import json
import math
from dataclasses import dataclass
from typing import Any

SCHEMA_VERSION = "2.0"
STATE_SCOPE = "authoritative_server_state"
ACTION_TYPES = ("move", "hold", "contest", "retreat")
UNIT_CLASSES = ("tank", "fighter", "marksman", "mage", "support", "assassin")
UNIT_POLICIES = ("controlled", "support", "protector", "aggressive", "skirmisher")
MAX_UNITS = 64
MAX_PROJECTILES = 128


class SnapshotError(ValueError):
    """Raised when a schema 2.0 snapshot is invalid."""


@dataclass(frozen=True, slots=True)
class Snapshot:
    raw: dict[str, Any]
    units_by_id: dict[str, dict[str, Any]]
    eligible_ids: tuple[str, ...]
    legal_actions: tuple[str, ...]
    controlled_unit_id: str
    min_x: float
    max_x: float
    min_y: float
    max_y: float


def strict_json_loads(data: bytes | str) -> Any:
    """Decode one JSON value while rejecting duplicate keys and NaN/Infinity."""
    if isinstance(data, bytes):
        data = data.decode("utf-8")

    def reject_constant(value: str) -> None:
        raise ValueError(f"invalid JSON constant {value}")

    def unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key {key}")
            result[key] = value
        return result

    decoder = json.JSONDecoder(parse_constant=reject_constant, object_pairs_hook=unique_object)
    start = len(data) - len(data.lstrip())
    value, end = decoder.raw_decode(data, start)
    if data[end:].strip():
        raise ValueError("input must contain exactly one JSON value")
    return value


def _finite_number(value: Any, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise SnapshotError(f"{name} must be a finite number")
    converted = float(value)
    if not math.isfinite(converted):
        raise SnapshotError(f"{name} must be a finite number")
    return converted


def _integer(value: Any, name: str, minimum: int = 0) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum:
        raise SnapshotError(f"{name} must be an integer of at least {minimum}")
    return value


def _string(value: Any, name: str, maximum: int = 256) -> str:
    if not isinstance(value, str) or not value.strip() or len(value) > maximum:
        raise SnapshotError(f"{name} must be a non-empty string")
    return value


def _point(value: Any, name: str, *, min_x: float, max_x: float, min_y: float, max_y: float) -> tuple[float, float]:
    if not isinstance(value, dict):
        raise SnapshotError(f"{name} must be an object")
    x = _finite_number(value.get("x"), f"{name}.x")
    y = _finite_number(value.get("y"), f"{name}.y")
    if not min_x <= x <= max_x or not min_y <= y <= max_y:
        raise SnapshotError(f"{name} is outside map bounds")
    return x, y


def validate_snapshot(value: Any) -> Snapshot:
    if not isinstance(value, dict):
        raise SnapshotError("snapshot must be an object")
    if value.get("schemaVersion") != SCHEMA_VERSION:
        raise SnapshotError(f"schemaVersion must be {SCHEMA_VERSION}")
    if value.get("stateScope") != STATE_SCOPE:
        raise SnapshotError(f"stateScope must be {STATE_SCOPE}")
    _string(value.get("sessionId"), "sessionId")
    _string(value.get("momentId"), "momentId")
    _integer(value.get("turn"), "turn", 1)
    controlled_id = _string(value.get("controlledUnitId"), "controlledUnitId")

    bounds = value.get("mapBounds")
    if not isinstance(bounds, dict):
        raise SnapshotError("mapBounds must be an object")
    min_x = _finite_number(bounds.get("minX"), "mapBounds.minX")
    max_x = _finite_number(bounds.get("maxX"), "mapBounds.maxX")
    min_y = _finite_number(bounds.get("minY"), "mapBounds.minY")
    max_y = _finite_number(bounds.get("maxY"), "mapBounds.maxY")
    if min_x >= max_x or min_y >= max_y:
        raise SnapshotError("mapBounds minimums must be below maximums")

    legal = value.get("legalActions")
    if not isinstance(legal, list) or not legal:
        raise SnapshotError("legalActions must be a non-empty array")
    legal_actions: list[str] = []
    for action in legal:
        if not isinstance(action, str) or action not in ACTION_TYPES or action in legal_actions:
            raise SnapshotError("legalActions must contain unique supported actions")
        legal_actions.append(action)

    units = value.get("units")
    if not isinstance(units, list) or not 2 <= len(units) <= MAX_UNITS:
        raise SnapshotError(f"units must contain between 2 and {MAX_UNITS} entries")
    units_by_id: dict[str, dict[str, Any]] = {}
    for unit in units:
        if not isinstance(unit, dict):
            raise SnapshotError("each unit must be an object")
        unit_id = _string(unit.get("id"), "unit.id")
        if unit_id in units_by_id:
            raise SnapshotError("unit IDs must be unique")
        if unit.get("team") not in {"blue", "red"}:
            raise SnapshotError("unit team must be blue or red")
        _string(unit.get("role"), "unit.role")
        if unit.get("class") not in UNIT_CLASSES:
            raise SnapshotError("unit class is unsupported")
        if unit.get("fallbackPolicy") not in UNIT_POLICIES:
            raise SnapshotError("unit fallbackPolicy is unsupported")
        _point(unit.get("position"), "unit.position", min_x=min_x, max_x=max_x, min_y=min_y, max_y=max_y)
        hp = _integer(unit.get("hp"), "unit.hp")
        max_hp = _integer(unit.get("maxHp"), "unit.maxHp", 1)
        if hp > max_hp:
            raise SnapshotError("unit hp must not exceed maxHp")
        if _finite_number(unit.get("moveRange"), "unit.moveRange") < 0:
            raise SnapshotError("unit moveRange must not be negative")
        if _finite_number(unit.get("attackRange"), "unit.attackRange") < 0:
            raise SnapshotError("unit attackRange must not be negative")
        _integer(unit.get("cooldownTurns"), "unit.cooldownTurns")
        _integer(unit.get("shield"), "unit.shield")
        for key in ("guarded", "visible", "alive"):
            if not isinstance(unit.get(key), bool):
                raise SnapshotError(f"unit {key} must be a boolean")
        units_by_id[unit_id] = unit

    if controlled_id not in units_by_id or not units_by_id[controlled_id]["alive"]:
        raise SnapshotError("controlledUnitId must identify a live unit")

    objective = value.get("objective")
    if objective is not None:
        if not isinstance(objective, dict):
            raise SnapshotError("objective must be an object or null")
        _string(objective.get("id"), "objective.id")
        _string(objective.get("label"), "objective.label")
        _point(objective.get("position"), "objective.position", min_x=min_x, max_x=max_x, min_y=min_y, max_y=max_y)
        if _finite_number(objective.get("radius"), "objective.radius") < 0:
            raise SnapshotError("objective radius must not be negative")
        required = _integer(objective.get("requiredProgress"), "objective.requiredProgress", 1)
        blue = _integer(objective.get("blueProgress"), "objective.blueProgress")
        red = _integer(objective.get("redProgress"), "objective.redProgress")
        if blue > required or red > required:
            raise SnapshotError("objective progress must not exceed requiredProgress")
        _string(objective.get("status"), "objective.status")

    projectiles = value.get("projectiles")
    if not isinstance(projectiles, list) or len(projectiles) > MAX_PROJECTILES:
        raise SnapshotError(f"projectiles must be an array of at most {MAX_PROJECTILES} entries")
    projectile_ids: set[str] = set()
    for projectile in projectiles:
        if not isinstance(projectile, dict):
            raise SnapshotError("each projectile must be an object")
        projectile_id = _string(projectile.get("id"), "projectile.id")
        if projectile_id in projectile_ids:
            raise SnapshotError("projectile IDs must be unique")
        projectile_ids.add(projectile_id)
        if projectile.get("team") not in {"blue", "red"}:
            raise SnapshotError("projectile team must be blue or red")
        source = projectile.get("sourceUnitId")
        if not isinstance(source, str) or (source and source not in units_by_id):
            raise SnapshotError("projectile sourceUnitId is invalid")
        target = _string(projectile.get("targetUnitId"), "projectile.targetUnitId")
        if target not in units_by_id:
            raise SnapshotError("projectile targetUnitId is unknown")
        _point(projectile.get("position"), "projectile.position", min_x=min_x, max_x=max_x, min_y=min_y, max_y=max_y)
        _point(projectile.get("target"), "projectile.target", min_x=min_x, max_x=max_x, min_y=min_y, max_y=max_y)
        _integer(projectile.get("damage"), "projectile.damage", 1)

    eligible_ids = tuple(unit["id"] for unit in units if unit["alive"] and unit["id"] != controlled_id)
    if not eligible_ids:
        raise SnapshotError("snapshot needs at least one live non-controlled unit")
    return Snapshot(value, units_by_id, eligible_ids, tuple(legal_actions), controlled_id, min_x, max_x, min_y, max_y)
