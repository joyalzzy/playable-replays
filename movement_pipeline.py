"""Validated data preparation for the separate critical-objective movement model."""

from __future__ import annotations

import json
import math
import random
import statistics
from copy import deepcopy
from pathlib import Path
from typing import Any, Iterable


DRAGON_OBJECTIVE_TYPES = frozenset(
    {"dragon", "elemental_dragon", "elder_dragon", "dragon_soul"}
)
CRITICAL_OBJECTIVE_TYPES = DRAGON_OBJECTIVE_TYPES | {"baron_nashor"}
TRAINABLE_SOURCE_TYPES = frozenset(
    {"licensed_replay_export", "manually_reviewed_replay"}
)
ALLOWED_SOURCE_TYPES = TRAINABLE_SOURCE_TYPES | {"schema_example"}
MOVEMENT_SYSTEM_PROMPT = (
    "Predict only the controlled player's movement during a labeled critical-objective "
    "fight window. Use the controlled player, four teammates, and any visible enemies, "
    "including their positions and alive/dead states. "
    "Return JSON with task='movement_prediction' and movement.waypoints in the supplied "
    "coordinate system. Do not predict combat actions, objective outcome, match outcome, "
    "player intent, or unseen enemy state."
)


def _is_number(value: Any) -> bool:
    return (
        isinstance(value, (int, float))
        and not isinstance(value, bool)
        and math.isfinite(value)
    )


def _validate_point(value: Any, field: str, *, nullable: bool = False) -> None:
    if value is None and nullable:
        return
    if not isinstance(value, dict) or set(value) != {"x", "z"}:
        raise ValueError(f"{field} must contain exactly numeric x and z coordinates")
    if not _is_number(value["x"]) or not _is_number(value["z"]):
        raise ValueError(f"{field}.x and {field}.z must be finite numbers")


def _validate_player(value: Any, field: str, *, controlled: bool = False) -> None:
    if not isinstance(value, dict):
        raise ValueError(f"{field} must be an object")
    player_id = value.get("id")
    alive = value.get("alive")
    if not isinstance(player_id, str) or not player_id.strip():
        raise ValueError(f"{field}.id must be a non-empty string")
    if not isinstance(alive, bool):
        raise ValueError(f"{field}.alive must be a boolean")
    if "position" not in value:
        raise ValueError(f"{field}.position is required")
    _validate_point(value["position"], f"{field}.position", nullable=not alive)
    if controlled and not alive:
        raise ValueError("controlledPlayer must be alive for a movement target")


def read_records(path: Path) -> list[dict[str, Any]]:
    path = Path(path)
    if path.suffix.lower() == ".jsonl":
        return [
            json.loads(line)
            for line in path.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
    payload = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(payload, list):
        return payload
    if isinstance(payload, dict) and isinstance(payload.get("records"), list):
        return payload["records"]
    raise ValueError("Movement input must be JSONL, a JSON array, or {records: [...]}")


def validate_record(record: Any, index: int) -> dict[str, Any]:
    if not isinstance(record, dict):
        raise ValueError(f"record {index}: expected an object")
    match_id = record.get("matchId")
    tick = record.get("tick")
    movement_index = record.get("movementIndex")
    state = record.get("state")
    label = record.get("label")
    metadata = record.get("metadata")
    if not isinstance(match_id, str) or not match_id.strip():
        raise ValueError(f"record {index}: matchId must be a non-empty string")
    for name, value in (("tick", tick), ("movementIndex", movement_index)):
        if not isinstance(value, int) or isinstance(value, bool) or value < 0:
            raise ValueError(f"record {index}: {name} must be a non-negative integer")
    if not isinstance(state, dict):
        raise ValueError(f"record {index}: state must be an object")
    controlled = state.get("controlledPlayer")
    _validate_player(controlled, f"record {index}.state.controlledPlayer", controlled=True)
    teammates = state.get("teammates")
    if not isinstance(teammates, list) or len(teammates) != 4:
        raise ValueError(f"record {index}: state.teammates must contain exactly four players")
    for teammate_index, teammate in enumerate(teammates):
        _validate_player(teammate, f"record {index}.state.teammates[{teammate_index}]")
    player_ids = [controlled["id"], *(teammate["id"] for teammate in teammates)]
    if len(player_ids) != len(set(player_ids)):
        raise ValueError(f"record {index}: controlled player and teammate IDs must be unique")

    objective = state.get("objective")
    if not isinstance(objective, dict):
        raise ValueError(f"record {index}: state.objective must be an object")
    if not isinstance(objective.get("type"), str) or not objective["type"].strip():
        raise ValueError(f"record {index}: state.objective.type must be non-empty")
    _validate_point(objective.get("position"), f"record {index}.state.objective.position")
    for name in ("isCritical", "teamCommitted", "fightActive"):
        if not isinstance(objective.get(name), bool):
            raise ValueError(f"record {index}: state.objective.{name} must be a boolean")
    if not _is_number(objective.get("secondsToResolution")):
        raise ValueError(f"record {index}: objective.secondsToResolution must be finite")
    visible_enemies = state.get("visibleEnemies", [])
    if not isinstance(visible_enemies, list) or len(visible_enemies) > 5:
        raise ValueError(f"record {index}: state.visibleEnemies must contain at most five players")
    for enemy_index, enemy in enumerate(visible_enemies):
        _validate_player(enemy, f"record {index}.state.visibleEnemies[{enemy_index}]")
    enemy_ids = [enemy["id"] for enemy in visible_enemies]
    if len(enemy_ids) != len(set(enemy_ids)) or set(enemy_ids) & set(player_ids):
        raise ValueError(f"record {index}: visible enemy IDs must be unique and distinct from allied IDs")

    if not isinstance(label, dict):
        raise ValueError(f"record {index}: label must be an object")
    movement = label.get("movement")
    waypoints = movement.get("waypoints") if isinstance(movement, dict) else None
    if not isinstance(waypoints, list) or not 1 <= len(waypoints) <= 16:
        raise ValueError(f"record {index}: label.movement.waypoints must contain 1..16 points")
    for waypoint_index, waypoint in enumerate(waypoints):
        _validate_point(waypoint, f"record {index}.label.movement.waypoints[{waypoint_index}]")
    for name in ("matchWonByTeam", "objectiveSecuredByTeam"):
        if not isinstance(label.get(name), bool):
            raise ValueError(f"record {index}: label.{name} must be a boolean")
    source_type = label.get("sourceType")
    if source_type not in ALLOWED_SOURCE_TYPES:
        raise ValueError(f"record {index}: unsupported label.sourceType {source_type!r}")
    if not isinstance(label.get("evidenceId"), str) or not label["evidenceId"].strip():
        raise ValueError(f"record {index}: label.evidenceId must be non-empty")
    if not isinstance(metadata, dict):
        raise ValueError(f"record {index}: metadata must be an object")
    for name in ("coordinateMethod", "labelProvenance"):
        if not isinstance(metadata.get(name), str) or not metadata[name].strip():
            raise ValueError(f"record {index}: metadata.{name} must be non-empty")
    return deepcopy(record)


def record_order_key(record: dict[str, Any]) -> tuple[str, int, int]:
    return record["matchId"], record["tick"], record["movementIndex"]


def exclusion_reason(
    record: dict[str, Any], *, objective_window_seconds: float = 90
) -> str | None:
    objective = record["state"]["objective"]
    if objective["type"] not in CRITICAL_OBJECTIVE_TYPES:
        return "not_allowlisted_critical_objective"
    if not objective["isCritical"]:
        return "not_labeled_critical"
    if not objective["teamCommitted"]:
        return "team_not_committed"
    if not objective["fightActive"]:
        return "not_active_objective_fight"
    if abs(objective["secondsToResolution"]) > objective_window_seconds:
        return "outside_objective_window"
    return None


def select_eligible_records(
    records: Iterable[dict[str, Any]], *, objective_window_seconds: float = 90
) -> tuple[list[dict[str, Any]], dict[str, int]]:
    selected: list[dict[str, Any]] = []
    exclusions: dict[str, int] = {}
    for record in records:
        reason = exclusion_reason(
            record, objective_window_seconds=objective_window_seconds
        )
        if reason is None:
            selected.append(record)
        else:
            exclusions[reason] = exclusions.get(reason, 0) + 1
    return sorted(selected, key=record_order_key), exclusions


def grouped_split(
    records: list[dict[str, Any]],
    *,
    eval_fraction: float,
    seed: int,
    required_eval_objective_types: frozenset[str] | set[str] | None = None,
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    if not 0 <= eval_fraction < 1:
        raise ValueError("eval_fraction must be in [0, 1)")
    match_ids = sorted({record["matchId"] for record in records})
    if len(match_ids) < 2 or eval_fraction == 0:
        return list(records), []
    generator = random.Random(seed)
    generator.shuffle(match_ids)
    eval_count = max(1, round(len(match_ids) * eval_fraction))
    eval_ids = set(match_ids[:eval_count])
    if required_eval_objective_types:
        required_match_ids = {
            record["matchId"]
            for record in records
            if record["state"]["objective"]["type"] in required_eval_objective_types
        }
        if required_match_ids and not eval_ids & required_match_ids:
            replacement = sorted(required_match_ids - eval_ids)[0]
            displaced = sorted(eval_ids)[0]
            eval_ids.remove(displaced)
            eval_ids.add(replacement)
    return (
        [record for record in records if record["matchId"] not in eval_ids],
        [record for record in records if record["matchId"] in eval_ids],
    )


def is_positive(record: dict[str, Any]) -> bool:
    label = record["label"]
    return label["matchWonByTeam"] and label["objectiveSecuredByTeam"]


def positive_fraction(records: Iterable[dict[str, Any]]) -> float:
    records = list(records)
    return sum(is_positive(record) for record in records) / len(records) if records else 0


def balance_training_records(
    records: list[dict[str, Any]], *, target_positive_fraction: float, seed: int
) -> list[dict[str, Any]]:
    if not 0.5 < target_positive_fraction <= 1:
        raise ValueError("target_positive_fraction must be in (0.5, 1]")
    positives = [record for record in records if is_positive(record)]
    contrasts = [record for record in records if not is_positive(record)]
    if not positives:
        raise ValueError("Training requires labeled examples where the team won and secured the objective")
    generator = random.Random(seed)
    generator.shuffle(contrasts)
    if target_positive_fraction == 1:
        selected_contrasts: list[dict[str, Any]] = []
    else:
        contrast_cap = math.floor(
            len(positives) * (1 - target_positive_fraction) / target_positive_fraction
        )
        if contrasts:
            contrast_cap = max(1, contrast_cap)
        selected_contrasts = contrasts[:contrast_cap]
    balanced = [deepcopy(record) for record in positives + selected_contrasts]
    repeat_index = 0
    while positive_fraction(balanced) + 1e-12 < target_positive_fraction:
        repeated = deepcopy(positives[repeat_index % len(positives)])
        repeated["metadata"]["samplingRepeat"] = repeat_index + 1
        balanced.append(repeated)
        repeat_index += 1
    generator.shuffle(balanced)
    return balanced


def build_training_example(record: dict[str, Any]) -> dict[str, Any]:
    state = record["state"]
    model_input = {
        "schemaVersion": "1.0",
        "task": "movement_prediction",
        "matchId": record["matchId"],
        "tick": record["tick"],
        "movementIndex": record["movementIndex"],
        "controlledPlayer": deepcopy(state["controlledPlayer"]),
        "teammates": deepcopy(state["teammates"]),
        "visibleEnemies": deepcopy(state.get("visibleEnemies", [])),
        "objective": deepcopy(state["objective"]),
        "coordinateMethod": record["metadata"]["coordinateMethod"],
    }
    target = {
        "task": "movement_prediction",
        "movement": deepcopy(record["label"]["movement"]),
    }
    user_content = json.dumps(model_input, sort_keys=True, separators=(",", ":"))
    if "matchWonByTeam" in user_content or "objectiveSecuredByTeam" in user_content:
        raise AssertionError("Outcome labels must never enter movement-model input")
    return {
        "messages": [
            {"role": "system", "content": MOVEMENT_SYSTEM_PROMPT},
            {"role": "user", "content": user_content},
            {"role": "assistant", "content": json.dumps(target, sort_keys=True)},
        ],
        "metadata": {
            "matchId": record["matchId"],
            "tick": record["tick"],
            "movementIndex": record["movementIndex"],
            "evidenceId": record["label"]["evidenceId"],
            "sourceType": record["label"]["sourceType"],
            "matchWonByTeam": record["label"]["matchWonByTeam"],
            "objectiveSecuredByTeam": record["label"]["objectiveSecuredByTeam"],
            "positiveSelectionClass": is_positive(record),
            "objectiveType": state["objective"]["type"],
            "fightActive": state["objective"]["fightActive"],
            "livingTeammates": sum(teammate["alive"] for teammate in state["teammates"]),
            "samplingRepeat": record["metadata"].get("samplingRepeat", 0),
            "labelProvenance": record["metadata"]["labelProvenance"],
        },
    }


def write_jsonl(path: Path, records: Iterable[dict[str, Any]]) -> None:
    with Path(path).open("w", encoding="utf-8") as handle:
        for record in records:
            handle.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n")


def score_movement_prediction(
    prediction: Any, expected: dict[str, Any]
) -> dict[str, Any]:
    result: dict[str, Any] = {"validJson": isinstance(prediction, dict), "validMovement": False}
    try:
        predicted_waypoints = prediction["movement"]["waypoints"]
        expected_waypoints = expected["movement"]["waypoints"]
        if not isinstance(predicted_waypoints, list) or not predicted_waypoints:
            return result
        for index, waypoint in enumerate(predicted_waypoints):
            _validate_point(waypoint, f"prediction.movement.waypoints[{index}]")
        result["validMovement"] = True
        predicted_endpoint = predicted_waypoints[-1]
        expected_endpoint = expected_waypoints[-1]
        result["endpointError"] = math.hypot(
            predicted_endpoint["x"] - expected_endpoint["x"],
            predicted_endpoint["z"] - expected_endpoint["z"],
        )
        result["predictedWaypointCount"] = len(predicted_waypoints)
        result["expectedWaypointCount"] = len(expected_waypoints)
    except (KeyError, TypeError, ValueError):
        pass
    return result


def summarize_heldout_results(results: list[dict[str, Any]]) -> dict[str, Any]:
    def summarize(items: list[dict[str, Any]]) -> dict[str, Any]:
        errors = [item["endpointError"] for item in items if "endpointError" in item]
        return {
            "examples": len(items),
            "validJsonRate": sum(item.get("validJson", False) for item in items) / len(items) if items else 0,
            "validMovementRate": sum(item.get("validMovement", False) for item in items) / len(items) if items else 0,
            "meanEndpointError": statistics.fmean(errors) if errors else None,
            "medianEndpointError": statistics.median(errors) if errors else None,
        }

    dragon_results = [
        result for result in results if result.get("objectiveType") in DRAGON_OBJECTIVE_TYPES
    ]
    objective_types = sorted({result.get("objectiveType") for result in results if result.get("objectiveType")})
    return {
        "heldOutOnly": True,
        "overall": summarize(results),
        "dragonObjectiveFights": summarize(dragon_results),
        "byObjectiveType": {
            objective_type: summarize(
                [result for result in results if result.get("objectiveType") == objective_type]
            )
            for objective_type in objective_types
        },
    }
