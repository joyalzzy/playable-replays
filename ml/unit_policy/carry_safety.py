"""Carry-focused curriculum, retraining command, and behavior demo."""
from __future__ import annotations

import argparse
import json
import math
import random
from dataclasses import replace
from pathlib import Path
from typing import Any

from .features import extract_features, nearest_peel
from .policy import UnitPolicy
from .schema import ACTION_TYPES, Snapshot, validate_snapshot
from .training import TrainingExample, generate_synthetic_examples, train_policy

PROFILES = {
    "tank": (160, 7.0, 10.0, "protector"),
    "fighter": (125, 10.0, 14.0, "skirmisher"),
    "marksman": (90, 11.0, 28.0, "aggressive"),
    "mage": (95, 9.0, 24.0, "aggressive"),
    "support": (110, 8.0, 20.0, "support"),
    "assassin": (100, 13.0, 12.0, "aggressive"),
}


def _unit(unit_id: str, team: str, unit_class: str, x: float, y: float, *, hp_ratio: float = 1.0, cooldown: int = 0, controlled: bool = False) -> dict[str, Any]:
    max_hp, move_range, attack_range, fallback = PROFILES[unit_class]
    return {
        "id": unit_id,
        "team": team,
        "role": unit_class,
        "class": unit_class,
        "fallbackPolicy": "controlled" if controlled else fallback,
        "position": {"x": min(96.0, max(4.0, x)), "y": min(96.0, max(4.0, y))},
        "hp": max(1, round(max_hp * hp_ratio)),
        "maxHp": max_hp,
        "moveRange": move_range,
        "attackRange": attack_range,
        "cooldownTurns": cooldown,
        "shield": 0,
        "guarded": False,
        "visible": True,
        "alive": True,
    }


def _snapshot(index: int, units: list[dict[str, Any]], name: str) -> Snapshot:
    return validate_snapshot({
        "schemaVersion": "2.0",
        "stateScope": "authoritative_server_state",
        "sessionId": f"carry-{name}-{index}",
        "momentId": name,
        "turn": 4,
        "mapBounds": {"minX": 0, "maxX": 100, "minY": 0, "maxY": 100},
        "controlledUnitId": "controlled",
        "legalActions": list(ACTION_TYPES),
        "objective": None,
        "projectiles": [],
        "units": units,
    })


def _safe_target(snapshot: Snapshot, carry: dict[str, Any], threat: dict[str, Any]) -> dict[str, float]:
    cx, cy = float(carry["position"]["x"]), float(carry["position"]["y"])
    tx, ty = float(threat["position"]["x"]), float(threat["position"]["y"])
    dx, dy = cx - tx, cy - ty
    magnitude = math.hypot(dx, dy) or 1.0
    dx, dy = dx / magnitude, dy / magnitude
    peel = nearest_peel(snapshot, carry)
    if peel is not None:
        px, py = float(peel["position"]["x"]), float(peel["position"]["y"])
        ax, ay = px - cx, py - cy
        ally_magnitude = math.hypot(ax, ay)
        if ally_magnitude > 1e-9:
            ax, ay = ax / ally_magnitude, ay / ally_magnitude
            dx, dy = 0.72 * dx + 0.28 * ax, 0.72 * dy + 0.28 * ay
            magnitude = math.hypot(dx, dy) or 1.0
            dx, dy = dx / magnitude, dy / magnitude
    step = float(carry["moveRange"])
    return {"x": min(100.0, max(0.0, cx + dx * step)), "y": min(100.0, max(0.0, cy + dy * step))}


def _example(snapshot: Snapshot, unit_id: str, action: dict[str, Any], weight: float, group: str, source: str = "carry-safety-curriculum") -> TrainingExample:
    move_dx = move_dy = None
    if action["type"] == "move":
        unit = snapshot.units_by_id[unit_id]
        origin = unit["position"]
        target = action["target"]
        scale = max(1e-9, float(unit["moveRange"]))
        move_dx = max(-1.0, min(1.0, (float(target["x"]) - float(origin["x"])) / scale))
        move_dy = max(-1.0, min(1.0, (float(target["y"]) - float(origin["y"])) / scale))
        magnitude = math.hypot(move_dx, move_dy)
        if magnitude > 1.0:
            move_dx, move_dy = move_dx / magnitude, move_dy / magnitude
    return TrainingExample(extract_features(snapshot, unit_id), action["type"], move_dx, move_dy, weight, group, source)


def _carry_state(rng: random.Random, index: int, kind: str) -> TrainingExample:
    cx, cy = rng.uniform(30.0, 70.0), rng.uniform(30.0, 70.0)
    angle = rng.uniform(0.0, 2.0 * math.pi)
    vx, vy = math.cos(angle), math.sin(angle)
    enemy_class = rng.choice(["assassin", "fighter", "tank"])
    _, enemy_move, enemy_attack, _ = PROFILES[enemy_class]
    reach = enemy_move + enemy_attack
    carry_hp = rng.uniform(0.22, 0.44) if kind == "critical" else rng.uniform(0.68, 0.98)
    enemy_hp = rng.uniform(0.08, 0.34) if kind == "finisher" else rng.uniform(0.55, 0.98)
    enemy_cooldown = 0

    if kind == "critical":
        radius = rng.uniform(7.0, max(8.0, reach - 3.0))
    elif kind == "caught":
        radius = rng.uniform(7.0, max(9.0, reach - 1.0))
    elif kind == "preemptive":
        radius = rng.uniform(reach + 1.0, reach + 6.0)
    elif kind == "finisher":
        radius = rng.uniform(10.0, min(25.0, max(11.0, enemy_attack)))
        enemy_cooldown = rng.choice([0, 1, 1, 2])
    elif kind == "screened":
        radius = rng.uniform(max(18.0, reach - 2.0), min(27.0, reach + 4.0))
        enemy_cooldown = rng.choice([0, 1])
    else:
        radius = rng.uniform(max(20.0, reach + 2.0), 27.0)
        enemy_cooldown = rng.choice([1, 1, 2])

    threat_x, threat_y = cx + vx * radius, cy + vy * radius
    allies: list[dict[str, Any]] = []
    if kind in {"preemptive", "safe", "screened", "finisher"} or (kind == "caught" and rng.random() < 0.75):
        allies.append(_unit("support", "blue", "support", cx - vx * rng.uniform(6.0, 12.0), cy - vy * rng.uniform(6.0, 12.0), hp_ratio=0.95))
    if kind == "screened":
        allies.append(_unit("tank", "blue", "tank", cx + vx * radius * 0.38, cy + vy * radius * 0.38, hp_ratio=0.96))

    snapshot = _snapshot(index, [
        _unit("controlled", "blue", "mage", cx - 22.0, cy + 20.0, controlled=True),
        _unit("carry", "blue", "marksman", cx, cy, hp_ratio=carry_hp),
        *allies,
        _unit("threat", "red", enemy_class, threat_x, threat_y, hp_ratio=enemy_hp, cooldown=enemy_cooldown),
        _unit("red-backline", "red", "mage", threat_x + 9.0, threat_y + 9.0, hp_ratio=rng.uniform(0.55, 0.95), cooldown=1),
    ], kind)
    carry, threat = snapshot.units_by_id["carry"], snapshot.units_by_id["threat"]
    if kind == "critical":
        action, weight = {"type": "retreat"}, 2.5
    elif kind in {"caught", "preemptive"}:
        action, weight = {"type": "move", "target": _safe_target(snapshot, carry, threat)}, 3.3 if kind == "caught" else 2.6
    else:
        action, weight = {"type": "contest"}, {"safe": 2.5, "screened": 2.9, "finisher": 3.2}[kind]
    return _example(snapshot, "carry", action, weight, f"carry:{index}")


def _role_state(rng: random.Random, index: int, kind: str) -> TrainingExample:
    cx, cy = rng.uniform(35.0, 65.0), rng.uniform(35.0, 65.0)
    if kind == "tank":
        units = [_unit("controlled", "blue", "mage", cx - 20, cy + 18, controlled=True), _unit("focus", "blue", "tank", cx, cy, hp_ratio=0.92), _unit("support", "blue", "support", cx - 7, cy), _unit("carry", "blue", "marksman", cx - 13, cy), _unit("enemy", "red", "marksman", cx + 9, cy), _unit("enemy-support", "red", "support", cx + 17, cy)]
        action = {"type": "contest"}
    elif kind == "support":
        units = [_unit("controlled", "blue", "mage", cx - 20, cy + 18, controlled=True), _unit("focus", "blue", "support", cx, cy), _unit("carry", "blue", "marksman", cx - 4, cy, hp_ratio=0.42), _unit("tank", "blue", "tank", cx + 3, cy + 3), _unit("enemy", "red", "assassin", cx + 9, cy), _unit("enemy-mage", "red", "mage", cx + 18, cy, cooldown=1)]
        action = {"type": "hold"}
    else:
        units = [_unit("controlled", "blue", "mage", cx - 20, cy + 18, controlled=True), _unit("focus", "blue", "assassin", cx, cy, hp_ratio=0.36), _unit("enemy-one", "red", "tank", cx + 7, cy), _unit("enemy-two", "red", "support", cx, cy + 8), _unit("enemy-three", "red", "marksman", cx + 12, cy + 12)]
        action = {"type": "retreat"}
    return _example(_snapshot(index, units, f"role-{kind}"), "focus", action, 1.5, f"role:{kind}:{index}")


def curriculum_examples(count: int, seed: int) -> list[TrainingExample]:
    if count < 1:
        raise ValueError("count must be positive")
    rng = random.Random(seed)
    kinds = (*("caught",) * 4, *("preemptive",) * 3, *("critical",) * 2, *("safe",) * 4, *("screened",) * 3, *("finisher",) * 4)
    examples = [_carry_state(rng, index, kinds[index % len(kinds)]) for index in range(count)]
    role_count = max(300, count // 3)
    role_kinds = ("tank", "support", "assassin")
    examples.extend(_role_state(rng, index, role_kinds[index % 3]) for index in range(role_count))
    return examples


def train_carry_model(*, seed: int = 2026, bootstrap_examples: int = 5000, curriculum_count: int = 6500, epochs: int = 36, output: Path | None = None, policy_version: str = "unit-policy-v2-carry-safety"):
    synthetic = [replace(example, weight=0.35, source="synthetic-bootstrap") for example in generate_synthetic_examples(bootstrap_examples, seed, weight=0.35)]
    examples = [*synthetic, *curriculum_examples(curriculum_count, seed + 1)]
    model, metrics = train_policy(
        examples,
        seed=seed,
        epochs=epochs,
        learning_rate=0.05,
        validation_fraction=0.18,
        policy_version=policy_version,
        extra_metadata={
            "carrySafetyCurriculumExamples": curriculum_count,
            "runtimeHardcodedCarryOverride": False,
            "carryBehavior": ["retreat when caught", "kite before engage", "contest behind peel"],
        },
    )
    if output is not None:
        model.save(output)
    return model, metrics


def demo(model=None) -> dict[str, Any]:
    if model is None:
        model, metrics = train_carry_model(seed=91, bootstrap_examples=600, curriculum_count=800, epochs=8)
    else:
        metrics = None
    policy = UnitPolicy(model)
    scenarios = [
        ("carry-caught-out", [_unit("controlled", "blue", "mage", 35, 55, controlled=True), _unit("carry", "blue", "marksman", 50, 50, hp_ratio=0.40), _unit("threat", "red", "assassin", 57, 50), _unit("red-tank", "red", "tank", 64, 52)]),
        ("carry-kite-with-peel", [_unit("controlled", "blue", "mage", 35, 55, controlled=True), _unit("support", "blue", "support", 41, 50), _unit("carry", "blue", "marksman", 50, 50, hp_ratio=0.80), _unit("threat", "red", "assassin", 59, 50), _unit("red-mage", "red", "mage", 68, 55, cooldown=1)]),
        ("carry-safe-damage", [_unit("controlled", "blue", "mage", 35, 55, controlled=True), _unit("support", "blue", "support", 43, 52), _unit("tank", "blue", "tank", 53, 50), _unit("carry", "blue", "marksman", 45, 50), _unit("threat", "red", "fighter", 69, 50, cooldown=1)]),
    ]
    decisions = []
    for index, (name, units) in enumerate(scenarios):
        decision = policy.decide(_snapshot(index, units, name), "carry")
        decisions.append({"scenario": name, "action": decision.action_type, "target": decision.target, "probabilities": {key: round(value, 4) for key, value in sorted(decision.probabilities.items())}})
    return {"modelVersion": model.version, "metrics": None if metrics is None else {"actionAccuracy": round(metrics.action_accuracy, 4), "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 4), "movementMae": None if metrics.movement_mae is None else round(metrics.movement_mae, 4), "groupOverlap": metrics.group_overlap}, "decisions": decisions}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subcommands = parser.add_subparsers(dest="command", required=True)
    train = subcommands.add_parser("train")
    train.add_argument("--output", type=Path, default=Path(__file__).with_name("models") / "unit_policy_v1.json")
    train.add_argument("--seed", type=int, default=2026)
    train.add_argument("--bootstrap-examples", type=int, default=5000)
    train.add_argument("--curriculum-examples", type=int, default=6500)
    train.add_argument("--epochs", type=int, default=36)
    train.add_argument("--policy-version", default="unit-policy-v2-carry-safety")
    subcommands.add_parser("demo")
    arguments = parser.parse_args()
    if arguments.command == "train":
        model, metrics = train_carry_model(seed=arguments.seed, bootstrap_examples=arguments.bootstrap_examples, curriculum_count=arguments.curriculum_examples, epochs=arguments.epochs, output=arguments.output, policy_version=arguments.policy_version)
        print(json.dumps({"output": str(arguments.output), "policyVersion": model.version, "actionAccuracy": round(metrics.action_accuracy, 6), "weightedActionAccuracy": round(metrics.weighted_action_accuracy, 6), "movementMae": None if metrics.movement_mae is None else round(metrics.movement_mae, 6), "groupOverlap": metrics.group_overlap}, sort_keys=True))
    else:
        print(json.dumps(demo(), indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
