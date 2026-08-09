"""Deterministic feature extraction for unit-level tactical decisions."""
from __future__ import annotations

import math
from typing import Any, Iterable

from .schema import Snapshot, UNIT_CLASSES, UNIT_POLICIES

FEATURE_NAMES = (
    "bias", "team_red", "position_x", "position_y", "hp_ratio",
    "missing_hp_ratio", "move_range", "attack_range", "cooldown_ready",
    "guarded", "shield_ratio",
    *(f"class_{name}" for name in UNIT_CLASSES),
    *(f"policy_{name}" for name in UNIT_POLICIES),
    "nearest_enemy_dx", "nearest_enemy_dy", "nearest_enemy_distance",
    "enemy_in_attack_range", "enemy_in_move_attack_range",
    "nearest_enemy_hp_ratio", "nearest_enemy_attack_ready",
    "nearest_enemy_threatens_unit", "nearest_ally_dx", "nearest_ally_dy",
    "nearest_ally_distance", "nearest_ally_hp_ratio", "controlled_dx",
    "controlled_dy", "controlled_distance", "local_allies", "local_enemies",
    "local_number_advantage", "objective_present", "objective_dx",
    "objective_dy", "objective_distance", "objective_friendly_progress",
    "objective_enemy_progress", "objective_contested", "incoming_projectile",
    "edge_pressure", "turn_fraction",
    *(f"nearest_enemy_class_{name}" for name in UNIT_CLASSES),
    *(f"nearest_ally_class_{name}" for name in UNIT_CLASSES),
    "nearest_enemy_engage_range", "enemy_engage_margin", "nearest_peel_dx",
    "nearest_peel_dy", "nearest_peel_distance", "peel_available",
    "carry_caught_out_risk", "carry_safe_damage_access",
    "safe_reposition_dx", "safe_reposition_dy",
)

MAP_DIAGONAL = math.sqrt(2.0)
LOCAL_RADIUS = 25.0
PEEL_RADIUS = 16.0
CARRY_CLASSES = {"marksman", "mage"}
PEEL_CLASSES = {"tank", "fighter", "support"}


def _position(unit: dict[str, Any]) -> tuple[float, float]:
    p = unit["position"]
    return float(p["x"]), float(p["y"])


def distance(left: dict[str, Any], right: dict[str, Any]) -> float:
    lx, ly = _position(left)
    rx, ry = _position(right)
    return math.hypot(rx - lx, ry - ly)


def nearest_unit(unit: dict[str, Any], candidates: Iterable[dict[str, Any]]) -> dict[str, Any] | None:
    live = [candidate for candidate in candidates if candidate.get("alive")]
    if not live:
        return None
    return min(live, key=lambda candidate: (distance(unit, candidate), candidate["id"]))


def nearest_enemy(snapshot: Snapshot, unit: dict[str, Any]) -> dict[str, Any] | None:
    return nearest_unit(unit, (candidate for candidate in snapshot.units_by_id.values() if candidate["team"] != unit["team"]))


def nearest_ally(snapshot: Snapshot, unit: dict[str, Any]) -> dict[str, Any] | None:
    return nearest_unit(unit, (candidate for candidate in snapshot.units_by_id.values() if candidate["team"] == unit["team"] and candidate["id"] != unit["id"]))


def nearest_peel(snapshot: Snapshot, unit: dict[str, Any]) -> dict[str, Any] | None:
    return nearest_unit(unit, (candidate for candidate in snapshot.units_by_id.values() if candidate["team"] == unit["team"] and candidate["id"] != unit["id"] and candidate["class"] in PEEL_CLASSES))


def _relative(source: dict[str, Any], target: dict[str, Any] | None, *, span_x: float, span_y: float) -> tuple[float, float, float]:
    if target is None:
        return 0.0, 0.0, 1.0
    sx, sy = _position(source)
    tx, ty = _position(target)
    dx = (tx - sx) / span_x
    dy = (ty - sy) / span_y
    return dx, dy, min(1.0, math.hypot(dx, dy) / MAP_DIAGONAL)


def _clamp(value: float, lower: float = 0.0, upper: float = 1.0) -> float:
    return min(upper, max(lower, value))


def _safe_direction(unit: dict[str, Any], enemy: dict[str, Any] | None, peel: dict[str, Any] | None) -> tuple[float, float]:
    if enemy is None:
        return 0.0, 0.0
    ox, oy = _position(unit)
    ex, ey = _position(enemy)
    dx, dy = ox - ex, oy - ey
    magnitude = math.hypot(dx, dy)
    if magnitude <= 1e-12:
        dx, dy, magnitude = 1.0, 0.0, 1.0
    dx, dy = dx / magnitude, dy / magnitude
    if peel is not None:
        px, py = _position(peel)
        tx, ty = px - ox, py - oy
        tm = math.hypot(tx, ty)
        if tm > 1e-12:
            tx, ty = tx / tm, ty / tm
            dx = 0.72 * dx + 0.28 * tx
            dy = 0.72 * dy + 0.28 * ty
    magnitude = math.hypot(dx, dy)
    if magnitude <= 1e-12:
        return 0.0, 0.0
    return dx / magnitude, dy / magnitude


def extract_features(snapshot: Snapshot, unit_id: str) -> tuple[float, ...]:
    unit = snapshot.units_by_id[unit_id]
    span_x = snapshot.max_x - snapshot.min_x
    span_y = snapshot.max_y - snapshot.min_y
    x, y = _position(unit)
    x_norm = (x - snapshot.min_x) / span_x
    y_norm = (y - snapshot.min_y) / span_y
    hp_ratio = float(unit["hp"]) / max(1.0, float(unit["maxHp"]))
    shield_ratio = min(1.0, float(unit["shield"]) / max(1.0, float(unit["maxHp"])))

    enemy = nearest_enemy(snapshot, unit)
    ally = nearest_ally(snapshot, unit)
    peel = nearest_peel(snapshot, unit)
    controlled = snapshot.units_by_id[snapshot.controlled_unit_id]
    enemy_dx, enemy_dy, enemy_distance = _relative(unit, enemy, span_x=span_x, span_y=span_y)
    ally_dx, ally_dy, ally_distance = _relative(unit, ally, span_x=span_x, span_y=span_y)
    peel_dx, peel_dy, peel_distance = _relative(unit, peel, span_x=span_x, span_y=span_y)
    controlled_dx, controlled_dy, controlled_distance = _relative(unit, controlled, span_x=span_x, span_y=span_y)

    enemy_abs = distance(unit, enemy) if enemy is not None else math.inf
    enemy_in_attack = float(enemy is not None and enemy_abs <= float(unit["attackRange"]))
    enemy_in_move_attack = float(enemy is not None and enemy_abs <= float(unit["attackRange"]) + float(unit["moveRange"]))
    enemy_hp = float(enemy["hp"]) / max(1.0, float(enemy["maxHp"])) if enemy is not None else 1.0
    enemy_ready = float(enemy is not None and int(enemy["cooldownTurns"]) == 0)
    enemy_engage = float(enemy["attackRange"]) + float(enemy["moveRange"]) if enemy is not None else 0.0
    enemy_threatens = float(enemy is not None and enemy_ready and enemy_abs <= enemy_engage)
    engage_margin = _clamp((enemy_engage - enemy_abs) / 30.0, -1.0, 1.0) if enemy is not None else -1.0
    ally_hp = float(ally["hp"]) / max(1.0, float(ally["maxHp"])) if ally is not None else 1.0
    peel_abs = distance(unit, peel) if peel is not None else math.inf
    peel_available = float(peel is not None and peel_abs <= PEEL_RADIUS)

    local_allies = local_enemies = 0
    for candidate in snapshot.units_by_id.values():
        if not candidate["alive"] or candidate["id"] == unit_id or distance(unit, candidate) > LOCAL_RADIUS:
            continue
        if candidate["team"] == unit["team"]:
            local_allies += 1
        else:
            local_enemies += 1
    local_allies_norm = min(1.0, local_allies / 5.0)
    local_enemies_norm = min(1.0, local_enemies / 5.0)
    local_advantage = _clamp((local_allies - local_enemies) / 5.0, -1.0, 1.0)

    objective = snapshot.raw.get("objective")
    if isinstance(objective, dict):
        pseudo = {"position": objective["position"]}
        objective_dx, objective_dy, objective_distance = _relative(unit, pseudo, span_x=span_x, span_y=span_y)
        required = max(1.0, float(objective["requiredProgress"]))
        if unit["team"] == "blue":
            friendly_progress = float(objective["blueProgress"]) / required
            enemy_progress = float(objective["redProgress"]) / required
        else:
            friendly_progress = float(objective["redProgress"]) / required
            enemy_progress = float(objective["blueProgress"]) / required
        objective_contested = float(objective.get("status") == "contested")
    else:
        objective_dx, objective_dy, objective_distance = 0.0, 0.0, 1.0
        friendly_progress = enemy_progress = objective_contested = 0.0

    incoming_projectile = float(any(p.get("team") != unit["team"] and p.get("targetUnitId") == unit_id for p in snapshot.raw["projectiles"]))
    edge_distance = min(x_norm, 1.0 - x_norm, y_norm, 1.0 - y_norm)
    edge_pressure = 1.0 - _clamp(edge_distance / 0.25)
    is_carry = float(unit["class"] in CARRY_CLASSES)
    peel_closeness = 0.0 if peel is None else 1.0 - _clamp(peel_abs / 30.0)
    assassin_or_fighter = float(enemy is not None and enemy["class"] in {"assassin", "fighter"})
    caught_out_risk = is_carry * _clamp(0.42 * enemy_threatens + 0.20 * (1.0 - peel_closeness) + 0.18 * max(0.0, -local_advantage) + 0.12 * (1.0 - hp_ratio) + 0.08 * assassin_or_fighter)
    safe_damage_access = is_carry * enemy_in_attack * _clamp(0.65 * (1.0 - enemy_threatens) + 0.25 * peel_available + 0.10 * max(0.0, local_advantage))
    safe_dx, safe_dy = _safe_direction(unit, enemy, peel) if unit["class"] in CARRY_CLASSES else (0.0, 0.0)

    values: list[float] = [
        1.0, float(unit["team"] == "red"), x_norm, y_norm, hp_ratio,
        1.0 - hp_ratio, min(1.0, float(unit["moveRange"]) / 20.0),
        min(1.0, float(unit["attackRange"]) / 35.0),
        float(int(unit["cooldownTurns"]) == 0), float(bool(unit["guarded"])), shield_ratio,
    ]
    values.extend(float(unit["class"] == name) for name in UNIT_CLASSES)
    values.extend(float(unit["fallbackPolicy"] == name) for name in UNIT_POLICIES)
    values.extend([
        enemy_dx, enemy_dy, enemy_distance, enemy_in_attack, enemy_in_move_attack,
        enemy_hp, enemy_ready, enemy_threatens, ally_dx, ally_dy, ally_distance,
        ally_hp, controlled_dx, controlled_dy, controlled_distance,
        local_allies_norm, local_enemies_norm, local_advantage,
        float(isinstance(objective, dict)), objective_dx, objective_dy,
        objective_distance, _clamp(friendly_progress), _clamp(enemy_progress),
        objective_contested, incoming_projectile, edge_pressure,
        min(1.0, int(snapshot.raw["turn"]) / 20.0),
    ])
    values.extend(float(enemy is not None and enemy["class"] == name) for name in UNIT_CLASSES)
    values.extend(float(ally is not None and ally["class"] == name) for name in UNIT_CLASSES)
    values.extend([
        min(1.0, enemy_engage / 55.0), engage_margin, peel_dx, peel_dy,
        peel_distance, peel_available, caught_out_risk, safe_damage_access,
        safe_dx, safe_dy,
    ])
    if len(values) != len(FEATURE_NAMES):
        raise AssertionError(f"feature shape mismatch: {len(values)} != {len(FEATURE_NAMES)}")
    return tuple(values)
