"""Reviewed demonstration loading and deterministic synthetic bootstrap data.

Synthetic examples are training/test illustrations only. They are not replay
telemetry, professional annotations, calibrated optimal play, or evidence of a
player's intent or style.
"""
from __future__ import annotations

import math
import random
from pathlib import Path
from typing import Any

from .features import distance, extract_features, nearest_ally, nearest_enemy
from .schema import ACTION_TYPES, Snapshot, strict_json_loads, validate_snapshot

CLASS_PROFILES: dict[str, tuple[int, float, float, str]] = {
    "tank": (160, 7.0, 10.0, "protector"),
    "fighter": (125, 10.0, 14.0, "skirmisher"),
    "marksman": (90, 11.0, 28.0, "aggressive"),
    "mage": (95, 9.0, 24.0, "aggressive"),
    "support": (110, 8.0, 20.0, "support"),
    "assassin": (100, 13.0, 12.0, "aggressive"),
}


def _position(unit: dict[str, Any]) -> tuple[float, float]:
    p = unit["position"]
    return float(p["x"]), float(p["y"])


def _toward(source: tuple[float, float], target: tuple[float, float], limit: float) -> tuple[float, float]:
    dx, dy = target[0] - source[0], target[1] - source[1]
    magnitude = math.hypot(dx, dy)
    if magnitude <= 1e-12:
        return source
    scale = min(limit, magnitude) / magnitude
    return source[0] + dx * scale, source[1] + dy * scale


def _away(source: tuple[float, float], threat: tuple[float, float], limit: float) -> tuple[float, float]:
    dx, dy = source[0] - threat[0], source[1] - threat[1]
    magnitude = math.hypot(dx, dy) or 1.0
    return source[0] + dx / magnitude * limit, source[1] + dy / magnitude * limit


def _bounded(point: tuple[float, float]) -> dict[str, float]:
    return {"x": min(100.0, max(0.0, point[0])), "y": min(100.0, max(0.0, point[1]))}


def _local_balance(snapshot: Snapshot, unit: dict[str, Any]) -> int:
    allies = enemies = 0
    for candidate in snapshot.units_by_id.values():
        if not candidate["alive"] or candidate["id"] == unit["id"] or distance(unit, candidate) > 25.0:
            continue
        if candidate["team"] == unit["team"]:
            allies += 1
        else:
            enemies += 1
    return allies - enemies


def expert_action(snapshot: Snapshot, unit_id: str) -> dict[str, Any]:
    """Deterministic synthetic teacher used for bootstrap examples."""
    unit = snapshot.units_by_id[unit_id]
    hp_ratio = float(unit["hp"]) / float(unit["maxHp"])
    enemy = nearest_enemy(snapshot, unit)
    ally = nearest_ally(snapshot, unit)
    enemy_distance = distance(unit, enemy) if enemy is not None else math.inf
    local_balance = _local_balance(snapshot, unit)
    incoming = any(p["team"] != unit["team"] and p["targetUnitId"] == unit_id for p in snapshot.raw["projectiles"])
    threatened = bool(enemy is not None and int(enemy["cooldownTurns"]) == 0 and enemy_distance <= float(enemy["attackRange"]) + float(enemy["moveRange"]))

    if hp_ratio < 0.28 or (incoming and hp_ratio < 0.65) or (threatened and local_balance <= -2 and hp_ratio < 0.52):
        return {"type": "retreat"}
    if unit["fallbackPolicy"] in {"support", "protector"} and ally is not None and distance(unit, ally) <= 16.0 and float(ally["hp"]) / float(ally["maxHp"]) < 0.45 and threatened:
        return {"type": "hold"}

    ready = int(unit["cooldownTurns"]) == 0
    if enemy is not None and ready and enemy_distance <= float(unit["attackRange"]):
        return {"type": "contest"}
    if enemy is not None and ready and enemy_distance <= float(unit["attackRange"]) + float(unit["moveRange"]) and hp_ratio >= 0.45 and local_balance >= -1 and unit["fallbackPolicy"] in {"aggressive", "skirmisher"}:
        return {"type": "contest"}

    origin = _position(unit)
    move_range = float(unit["moveRange"])
    objective = snapshot.raw.get("objective")
    if isinstance(objective, dict) and hp_ratio >= 0.36:
        objective_position = (float(objective["position"]["x"]), float(objective["position"]["y"]))
        if math.hypot(objective_position[0] - origin[0], objective_position[1] - origin[1]) > float(objective["radius"]) * 0.7:
            return {"type": "move", "target": _bounded(_toward(origin, objective_position, move_range))}

    if enemy is not None and unit["class"] in {"marksman", "mage"}:
        desired = float(unit["attackRange"]) * 0.78
        if enemy_distance < desired * 0.62:
            return {"type": "move", "target": _bounded(_away(origin, _position(enemy), move_range))}
        if enemy_distance > desired * 1.25 and hp_ratio >= 0.5:
            return {"type": "move", "target": _bounded(_toward(origin, _position(enemy), min(move_range, enemy_distance - desired)))}
        return {"type": "hold"}

    if unit["fallbackPolicy"] in {"support", "protector"}:
        anchor = ally if ally is not None and float(ally["hp"]) < float(ally["maxHp"]) else snapshot.units_by_id[snapshot.controlled_unit_id]
        if distance(unit, anchor) > 10.0:
            return {"type": "move", "target": _bounded(_toward(origin, _position(anchor), move_range))}
        return {"type": "hold"}

    if enemy is not None and unit["fallbackPolicy"] in {"aggressive", "skirmisher"} and hp_ratio >= 0.52:
        return {"type": "move", "target": _bounded(_toward(origin, _position(enemy), move_range))}
    return {"type": "hold"}


def make_example(snapshot: Snapshot, unit_id: str, action: dict[str, Any]) -> tuple[tuple[float, ...], str, float | None, float | None]:
    action_type = action.get("type")
    target = action.get("target")
    if action_type not in ACTION_TYPES:
        raise ValueError("demonstration action type is unsupported")
    move_dx = move_dy = None
    if action_type == "move":
        if not isinstance(target, dict):
            raise ValueError("move demonstration requires a target")
        x, y = target.get("x"), target.get("y")
        if isinstance(x, bool) or not isinstance(x, (int, float)) or isinstance(y, bool) or not isinstance(y, (int, float)):
            raise ValueError("move target coordinates must be numeric")
        unit = snapshot.units_by_id[unit_id]
        origin_x, origin_y = _position(unit)
        move_range = max(1e-9, float(unit["moveRange"]))
        move_dx = max(-1.0, min(1.0, (float(x) - origin_x) / move_range))
        move_dy = max(-1.0, min(1.0, (float(y) - origin_y) / move_range))
        magnitude = math.hypot(move_dx, move_dy)
        if magnitude > 1.0:
            move_dx, move_dy = move_dx / magnitude, move_dy / magnitude
    elif target is not None:
        raise ValueError("only move demonstrations may include a target")
    return extract_features(snapshot, unit_id), action_type, move_dx, move_dy


def load_demonstration_records(path: str | Path) -> list[tuple[Any, ...]]:
    examples: list[tuple[Any, ...]] = []
    with Path(path).open("r", encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, 1):
            if not line.strip():
                continue
            try:
                record = strict_json_loads(line)
                if not isinstance(record, dict):
                    raise ValueError("record must be an object")
                snapshot = validate_snapshot(record.get("snapshot"))
                suggestions = record.get("actions")
                if not isinstance(suggestions, list):
                    raise ValueError("actions must be an array")
                by_id: dict[str, dict[str, Any]] = {}
                for suggestion in suggestions:
                    if not isinstance(suggestion, dict):
                        raise ValueError("each suggestion must be an object")
                    unit_id, action = suggestion.get("unitId"), suggestion.get("action")
                    if not isinstance(unit_id, str) or unit_id not in snapshot.eligible_ids or unit_id in by_id or not isinstance(action, dict):
                        raise ValueError("actions must uniquely cover eligible units")
                    by_id[unit_id] = action
                if set(by_id) != set(snapshot.eligible_ids):
                    raise ValueError("actions must cover every eligible unit")
                examples.extend(make_example(snapshot, unit_id, by_id[unit_id]) for unit_id in snapshot.eligible_ids)
            except Exception as error:
                raise ValueError(f"invalid demonstration at line {line_number}: {error}") from error
    if not examples:
        raise ValueError("demonstration file contained no examples")
    return examples


def _random_unit(rng: random.Random, team: str, index: int, unit_class: str, controlled: bool) -> dict[str, Any]:
    max_hp, move_range, attack_range, policy = CLASS_PROFILES[unit_class]
    return {
        "id": f"{team}-{index}-{unit_class}", "team": team, "role": unit_class,
        "class": unit_class, "fallbackPolicy": "controlled" if controlled else policy,
        "position": {"x": rng.uniform(4.0, 96.0), "y": rng.uniform(4.0, 96.0)},
        "hp": max(1, min(max_hp, round(max_hp * rng.uniform(0.14, 1.0)))),
        "maxHp": max_hp, "moveRange": move_range, "attackRange": attack_range,
        "cooldownTurns": rng.choice([0, 0, 0, 1, 2]), "shield": rng.choice([0, 0, 4, 8]),
        "guarded": rng.random() < 0.15, "visible": True, "alive": True,
    }


def synthetic_snapshot(rng: random.Random, index: int) -> dict[str, Any]:
    classes = ["tank", "fighter", "marksman", "mage", "support"]
    if rng.random() < 0.45:
        classes[rng.choice([0, 1, 3])] = "assassin"
    units = [_random_unit(rng, team, i, cls, team == "blue" and i == 3) for team in ("blue", "red") for i, cls in enumerate(classes)]
    objective = None
    if rng.random() < 0.72:
        required = rng.randint(2, 5)
        objective = {"id": f"objective-{index}", "label": "Synthetic objective", "position": {"x": rng.uniform(20.0, 80.0), "y": rng.uniform(20.0, 80.0)}, "radius": rng.uniform(6.0, 12.0), "blueProgress": rng.randint(0, required), "redProgress": rng.randint(0, required), "requiredProgress": required, "status": "contested" if rng.random() < 0.25 else "neutral"}
    projectiles: list[dict[str, Any]] = []
    if rng.random() < 0.22:
        target = rng.choice(units)
        source = rng.choice([u for u in units if u["team"] != target["team"]])
        projectiles.append({"id": f"projectile-{index}", "team": source["team"], "sourceUnitId": source["id"], "targetUnitId": target["id"], "position": dict(source["position"]), "target": dict(target["position"]), "damage": max(1, (target["maxHp"] + 1) // 2)})
    return {"schemaVersion": "2.0", "stateScope": "authoritative_server_state", "sessionId": f"synthetic-{index}", "momentId": "synthetic-training-only", "turn": rng.randint(1, 12), "mapBounds": {"minX": 0, "maxX": 100, "minY": 0, "maxY": 100}, "controlledUnitId": units[3]["id"], "legalActions": list(ACTION_TYPES), "objective": objective, "projectiles": projectiles, "units": units}


def generate_synthetic_records(count: int, seed: int) -> list[tuple[Any, ...]]:
    if count < 1:
        raise ValueError("count must be positive")
    rng = random.Random(seed)
    examples: list[tuple[Any, ...]] = []
    index = 0
    while len(examples) < count:
        snapshot = validate_snapshot(synthetic_snapshot(rng, index))
        for unit_id in snapshot.eligible_ids:
            examples.append(make_example(snapshot, unit_id, expert_action(snapshot, unit_id)))
            if len(examples) >= count:
                break
        index += 1
    return examples
