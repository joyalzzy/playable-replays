#!/usr/bin/env python3
"""Standalone inference runtime for unit-policy-v2-carry-safety.

No third-party packages are required. The runtime accepts either:
1. a Playable Replays schema 2.0 snapshot, or
2. a 72-element feature vector.
"""
from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path
from typing import Any, Iterable

ACTION_TYPES = ("move", "hold", "contest", "retreat")
UNIT_CLASSES = ("tank", "fighter", "marksman", "mage", "support", "assassin")
UNIT_POLICIES = ("controlled", "support", "protector", "aggressive", "skirmisher")
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

MODEL_PATH = Path(__file__).with_name("unit_policy_v2_carry_safety.json")
MAP_DIAGONAL = math.sqrt(2.0)
LOCAL_RADIUS = 25.0
PEEL_RADIUS = 16.0
CARRY_CLASSES = {"marksman", "mage"}
PEEL_CLASSES = {"tank", "fighter", "support"}


def _finite_number(value: Any, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name} must be a number")
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite")
    return value


def load_model(path: Path = MODEL_PATH) -> dict[str, Any]:
    model = json.loads(path.read_text(encoding="utf-8"))
    if model.get("format") != "playable-replays-linear-unit-policy":
        raise ValueError("unsupported model format")
    if model.get("formatVersion") != 1:
        raise ValueError("unsupported model format version")
    if model.get("featureNames") != list(FEATURE_NAMES):
        raise ValueError("feature order does not match the inference runtime")
    if model.get("actionOrder") != list(ACTION_TYPES):
        raise ValueError("action order does not match the inference runtime")
    width = len(FEATURE_NAMES)
    for action in ACTION_TYPES:
        weights = model.get("actionWeights", {}).get(action)
        if not isinstance(weights, list) or len(weights) != width:
            raise ValueError(f"invalid action weights for {action}")
        for i, value in enumerate(weights):
            _finite_number(value, f"actionWeights.{action}[{i}]")
    for axis in ("dx", "dy"):
        weights = model.get("movementWeights", {}).get(axis)
        if not isinstance(weights, list) or len(weights) != width:
            raise ValueError(f"invalid movement weights for {axis}")
        for i, value in enumerate(weights):
            _finite_number(value, f"movementWeights.{axis}[{i}]")
    return model


def _dot(weights: Iterable[float], features: Iterable[float]) -> float:
    return sum(w * x for w, x in zip(weights, features, strict=True))


def _softmax(logits: dict[str, float]) -> dict[str, float]:
    maximum = max(logits.values())
    exps = {key: math.exp(value - maximum) for key, value in logits.items()}
    total = sum(exps.values())
    return {key: value / total for key, value in exps.items()}


def _normalize_features(raw: Any) -> tuple[float, ...]:
    if isinstance(raw, dict) and "features" in raw:
        raw = raw["features"]
    if not isinstance(raw, list) or len(raw) != len(FEATURE_NAMES):
        raise ValueError(f"features must contain exactly {len(FEATURE_NAMES)} numbers")
    return tuple(_finite_number(value, f"features[{i}]") for i, value in enumerate(raw))


def infer_features(
    model: dict[str, Any],
    features: tuple[float, ...],
    legal_actions: Iterable[str] = ACTION_TYPES,
) -> dict[str, Any]:
    legal = tuple(legal_actions)
    if not legal or any(action not in ACTION_TYPES for action in legal):
        raise ValueError("legal actions must be a non-empty subset of the four actions")
    logits = {
        action: _dot(model["actionWeights"][action], features)
        for action in legal
    }
    probabilities = _softmax(logits)
    action = max(legal, key=lambda name: (logits[name], -ACTION_TYPES.index(name)))
    dx = math.tanh(_dot(model["movementWeights"]["dx"], features))
    dy = math.tanh(_dot(model["movementWeights"]["dy"], features))
    magnitude = math.hypot(dx, dy)
    if magnitude > 1.0:
        dx /= magnitude
        dy /= magnitude
    return {
        "action": action,
        "logits": logits,
        "probabilities": probabilities,
        "movementDelta": {"dx": dx, "dy": dy},
    }


def _position(unit: dict[str, Any]) -> tuple[float, float]:
    pos = unit["position"]
    return float(pos["x"]), float(pos["y"])


def _distance(left: dict[str, Any], right: dict[str, Any]) -> float:
    lx, ly = _position(left)
    rx, ry = _position(right)
    return math.hypot(rx - lx, ry - ly)


def _nearest(unit: dict[str, Any], candidates: Iterable[dict[str, Any]]) -> dict[str, Any] | None:
    live = [candidate for candidate in candidates if candidate.get("alive")]
    if not live:
        return None
    return min(live, key=lambda candidate: (_distance(unit, candidate), candidate["id"]))


def _relative(
    source: dict[str, Any],
    target: dict[str, Any] | None,
    span_x: float,
    span_y: float,
) -> tuple[float, float, float]:
    if target is None:
        return 0.0, 0.0, 1.0
    sx, sy = _position(source)
    tx, ty = _position(target)
    dx = (tx - sx) / span_x
    dy = (ty - sy) / span_y
    return dx, dy, min(1.0, math.hypot(dx, dy) / MAP_DIAGONAL)


def _clamp(value: float, lower: float = 0.0, upper: float = 1.0) -> float:
    return min(upper, max(lower, value))


def _safe_direction(
    unit: dict[str, Any], enemy: dict[str, Any] | None, peel: dict[str, Any] | None
) -> tuple[float, float]:
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


def _validate_snapshot(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        raise ValueError("snapshot must be a JSON object")
    if raw.get("schemaVersion") != "2.0":
        raise ValueError("schemaVersion must be 2.0")
    if raw.get("stateScope") != "authoritative_server_state":
        raise ValueError("stateScope must be authoritative_server_state")
    bounds = raw.get("mapBounds")
    if not isinstance(bounds, dict):
        raise ValueError("mapBounds must be an object")
    min_x = _finite_number(bounds.get("minX"), "mapBounds.minX")
    max_x = _finite_number(bounds.get("maxX"), "mapBounds.maxX")
    min_y = _finite_number(bounds.get("minY"), "mapBounds.minY")
    max_y = _finite_number(bounds.get("maxY"), "mapBounds.maxY")
    if min_x >= max_x or min_y >= max_y:
        raise ValueError("invalid map bounds")
    legal = raw.get("legalActions")
    if not isinstance(legal, list) or not legal or len(set(legal)) != len(legal):
        raise ValueError("legalActions must be a non-empty unique array")
    if any(action not in ACTION_TYPES for action in legal):
        raise ValueError("legalActions contains an unsupported action")
    units = raw.get("units")
    if not isinstance(units, list) or len(units) < 2:
        raise ValueError("units must contain at least two entries")
    units_by_id: dict[str, dict[str, Any]] = {}
    for unit in units:
        if not isinstance(unit, dict) or not isinstance(unit.get("id"), str):
            raise ValueError("each unit must be an object with a string id")
        uid = unit["id"]
        if uid in units_by_id:
            raise ValueError("unit IDs must be unique")
        if unit.get("team") not in {"blue", "red"}:
            raise ValueError(f"invalid team for {uid}")
        if unit.get("class") not in UNIT_CLASSES:
            raise ValueError(f"invalid class for {uid}")
        if unit.get("fallbackPolicy") not in UNIT_POLICIES:
            raise ValueError(f"invalid fallbackPolicy for {uid}")
        for key in ("hp", "maxHp", "moveRange", "attackRange", "cooldownTurns", "shield"):
            _finite_number(unit.get(key), f"{uid}.{key}")
        for key in ("guarded", "visible", "alive"):
            if not isinstance(unit.get(key), bool):
                raise ValueError(f"{uid}.{key} must be boolean")
        x, y = _position(unit)
        if not min_x <= x <= max_x or not min_y <= y <= max_y:
            raise ValueError(f"{uid}.position is outside map bounds")
        units_by_id[uid] = unit
    controlled = raw.get("controlledUnitId")
    if controlled not in units_by_id or not units_by_id[controlled].get("alive"):
        raise ValueError("controlledUnitId must identify a live unit")
    if not isinstance(raw.get("projectiles"), list):
        raise ValueError("projectiles must be an array")
    raw = dict(raw)
    raw["_bounds"] = (min_x, max_x, min_y, max_y)
    raw["_units_by_id"] = units_by_id
    return raw


def extract_features(snapshot: dict[str, Any], unit_id: str) -> tuple[float, ...]:
    units = snapshot["_units_by_id"]
    unit = units[unit_id]
    min_x, max_x, min_y, max_y = snapshot["_bounds"]
    span_x, span_y = max_x - min_x, max_y - min_y
    x, y = _position(unit)
    x_norm, y_norm = (x - min_x) / span_x, (y - min_y) / span_y
    hp_ratio = float(unit["hp"]) / max(1.0, float(unit["maxHp"]))
    shield_ratio = min(1.0, float(unit["shield"]) / max(1.0, float(unit["maxHp"])))

    enemy = _nearest(unit, (u for u in units.values() if u["team"] != unit["team"]))
    ally = _nearest(unit, (u for u in units.values() if u["team"] == unit["team"] and u["id"] != unit_id))
    peel = _nearest(unit, (u for u in units.values() if u["team"] == unit["team"] and u["id"] != unit_id and u["class"] in PEEL_CLASSES))
    controlled = units[snapshot["controlledUnitId"]]

    enemy_dx, enemy_dy, enemy_distance = _relative(unit, enemy, span_x, span_y)
    ally_dx, ally_dy, ally_distance = _relative(unit, ally, span_x, span_y)
    peel_dx, peel_dy, peel_distance = _relative(unit, peel, span_x, span_y)
    controlled_dx, controlled_dy, controlled_distance = _relative(unit, controlled, span_x, span_y)

    enemy_abs = _distance(unit, enemy) if enemy is not None else math.inf
    enemy_in_attack = float(enemy is not None and enemy_abs <= float(unit["attackRange"]))
    enemy_in_move_attack = float(enemy is not None and enemy_abs <= float(unit["attackRange"]) + float(unit["moveRange"]))
    enemy_hp = float(enemy["hp"]) / max(1.0, float(enemy["maxHp"])) if enemy is not None else 1.0
    enemy_ready = float(enemy is not None and int(enemy["cooldownTurns"]) == 0)
    enemy_engage = float(enemy["attackRange"]) + float(enemy["moveRange"]) if enemy is not None else 0.0
    enemy_threatens = float(enemy is not None and enemy_ready and enemy_abs <= enemy_engage)
    engage_margin = _clamp((enemy_engage - enemy_abs) / 30.0, -1.0, 1.0) if enemy is not None else -1.0
    ally_hp = float(ally["hp"]) / max(1.0, float(ally["maxHp"])) if ally is not None else 1.0
    peel_abs = _distance(unit, peel) if peel is not None else math.inf
    peel_available = float(peel is not None and peel_abs <= PEEL_RADIUS)

    local_allies = local_enemies = 0
    for candidate in units.values():
        if not candidate["alive"] or candidate["id"] == unit_id or _distance(unit, candidate) > LOCAL_RADIUS:
            continue
        if candidate["team"] == unit["team"]:
            local_allies += 1
        else:
            local_enemies += 1
    local_allies_norm = min(1.0, local_allies / 5.0)
    local_enemies_norm = min(1.0, local_enemies / 5.0)
    local_advantage = _clamp((local_allies - local_enemies) / 5.0, -1.0, 1.0)

    objective = snapshot.get("objective")
    if isinstance(objective, dict):
        pseudo = {"position": objective["position"]}
        objective_dx, objective_dy, objective_distance = _relative(unit, pseudo, span_x, span_y)
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

    incoming_projectile = float(any(
        projectile.get("team") != unit["team"] and projectile.get("targetUnitId") == unit_id
        for projectile in snapshot["projectiles"]
    ))
    edge_distance = min(x_norm, 1.0 - x_norm, y_norm, 1.0 - y_norm)
    edge_pressure = 1.0 - _clamp(edge_distance / 0.25)
    is_carry = float(unit["class"] in CARRY_CLASSES)
    peel_closeness = 0.0 if peel is None else 1.0 - _clamp(peel_abs / 30.0)
    assassin_or_fighter = float(enemy is not None and enemy["class"] in {"assassin", "fighter"})
    caught_out_risk = is_carry * _clamp(
        0.42 * enemy_threatens + 0.20 * (1.0 - peel_closeness)
        + 0.18 * max(0.0, -local_advantage) + 0.12 * (1.0 - hp_ratio)
        + 0.08 * assassin_or_fighter
    )
    safe_damage_access = is_carry * enemy_in_attack * _clamp(
        0.65 * (1.0 - enemy_threatens) + 0.25 * peel_available
        + 0.10 * max(0.0, local_advantage)
    )
    safe_dx, safe_dy = _safe_direction(unit, enemy, peel) if unit["class"] in CARRY_CLASSES else (0.0, 0.0)

    values: list[float] = [
        1.0, float(unit["team"] == "red"), x_norm, y_norm, hp_ratio,
        1.0 - hp_ratio, min(1.0, float(unit["moveRange"]) / 20.0),
        min(1.0, float(unit["attackRange"]) / 35.0),
        float(int(unit["cooldownTurns"]) == 0), float(bool(unit["guarded"])),
        shield_ratio,
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
        min(1.0, int(snapshot["turn"]) / 20.0),
    ])
    values.extend(float(enemy is not None and enemy["class"] == name) for name in UNIT_CLASSES)
    values.extend(float(ally is not None and ally["class"] == name) for name in UNIT_CLASSES)
    values.extend([
        min(1.0, enemy_engage / 55.0), engage_margin, peel_dx, peel_dy,
        peel_distance, peel_available, caught_out_risk, safe_damage_access,
        safe_dx, safe_dy,
    ])
    if len(values) != len(FEATURE_NAMES):
        raise AssertionError(f"feature shape mismatch: {len(values)}")
    return tuple(values)


def _direction_toward(source: tuple[float, float], target: tuple[float, float]) -> tuple[float, float]:
    dx, dy = target[0] - source[0], target[1] - source[1]
    magnitude = math.hypot(dx, dy)
    return (0.0, 0.0) if magnitude <= 1e-12 else (dx / magnitude, dy / magnitude)


def _movement_target(snapshot: dict[str, Any], unit_id: str, inference: dict[str, Any]) -> dict[str, float] | None:
    units = snapshot["_units_by_id"]
    unit = units[unit_id]
    origin = _position(unit)
    dx = float(inference["movementDelta"]["dx"])
    dy = float(inference["movementDelta"]["dy"])
    magnitude = math.hypot(dx, dy)
    if magnitude < 0.05:
        objective = snapshot.get("objective")
        if isinstance(objective, dict):
            direction = _direction_toward(origin, (float(objective["position"]["x"]), float(objective["position"]["y"])))
        else:
            enemy = _nearest(unit, (u for u in units.values() if u["team"] != unit["team"]))
            if enemy is not None:
                direction = _direction_toward(origin, _position(enemy))
            else:
                direction = _direction_toward(origin, _position(units[snapshot["controlledUnitId"]]))
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
    min_x, max_x, min_y, max_y = snapshot["_bounds"]
    target_x = _clamp(origin[0] + dx * step, min_x, max_x)
    target_y = _clamp(origin[1] + dy * step, min_y, max_y)
    if math.hypot(target_x - origin[0], target_y - origin[1]) <= 1e-9:
        return None
    return {"x": round(target_x, 6), "y": round(target_y, 6)}


def infer_snapshot(model: dict[str, Any], raw: Any, only_unit: str | None = None) -> dict[str, Any]:
    snapshot = _validate_snapshot(raw)
    eligible = [u["id"] for u in snapshot["units"] if u["alive"] and u["id"] != snapshot["controlledUnitId"]]
    if only_unit is not None:
        if only_unit not in eligible:
            raise ValueError("--unit must identify a live non-controlled unit")
        eligible = [only_unit]
    decisions: list[dict[str, Any]] = []
    wire_actions: list[dict[str, Any]] = []
    for unit_id in eligible:
        features = extract_features(snapshot, unit_id)
        result = infer_features(model, features, snapshot["legalActions"])
        action = result["action"]
        target = _movement_target(snapshot, unit_id, result) if action == "move" else None
        if action == "move" and target is None and "hold" in snapshot["legalActions"]:
            action = "hold"
        wire = {"unitId": unit_id, "action": {"type": action}}
        if action == "move" and target is not None:
            wire["action"]["target"] = target
        wire_actions.append(wire)
        decisions.append({
            "unitId": unit_id,
            "action": action,
            "target": target if action == "move" else None,
            "probabilities": result["probabilities"],
            "logits": result["logits"],
            "movementDelta": result["movementDelta"],
            "features": dict(zip(FEATURE_NAMES, features, strict=True)),
        })
    return {
        "modelVersion": model["policyVersion"],
        "actions": wire_actions,
        "decisions": decisions,
    }


def _read_json(path: str) -> Any:
    if path == "-":
        return json.load(sys.stdin)
    return json.loads(Path(path).read_text(encoding="utf-8"))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", help="snapshot/feature JSON path, or - for stdin")
    parser.add_argument("--model", type=Path, default=MODEL_PATH)
    parser.add_argument("--mode", choices=("auto", "snapshot", "features"), default="auto")
    parser.add_argument("--unit", help="infer one live non-controlled unit from a snapshot")
    parser.add_argument("--wire-only", action="store_true", help="print only the {actions:[...]} wire response")
    parser.add_argument("--compact", action="store_true", help="emit compact JSON")
    args = parser.parse_args()

    try:
        model = load_model(args.model)
        value = _read_json(args.input)
        mode = args.mode
        if mode == "auto":
            mode = "snapshot" if isinstance(value, dict) and "schemaVersion" in value else "features"
        if mode == "features":
            features = _normalize_features(value)
            output = {"modelVersion": model["policyVersion"], **infer_features(model, features)}
        else:
            output = infer_snapshot(model, value, args.unit)
            if args.wire_only:
                output = {"actions": output["actions"]}
        json.dump(output, sys.stdout, indent=None if args.compact else 2, sort_keys=True)
        sys.stdout.write("\n")
        return 0
    except (OSError, ValueError, KeyError, TypeError, json.JSONDecodeError) as exc:
        print(f"inference error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
