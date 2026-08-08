"""Ten-player next-frame position prediction data and evaluation helpers."""

from __future__ import annotations

import json
import math
import random
import statistics
from copy import deepcopy
from pathlib import Path
from typing import Any, Iterable


TASK = "ten_player_next_frame_position"
POSITION_SYSTEM_PROMPT = (
    "Given one replay frame containing exactly ten players and their dataset-native x/z "
    "positions, predict the possible positions of the same ten players in the next fixed-time "
    "frame. Return JSON only with task, frameTimeSeconds, and exactly ten players. Preserve "
    "every playerId exactly and return one finite x/z position per player. Do not add actions, "
    "intent, outcomes, normalized coordinates, or players not present in the input."
)


def _is_number(value: Any) -> bool:
    return (
        isinstance(value, (int, float))
        and not isinstance(value, bool)
        and math.isfinite(value)
    )


def _validate_position(value: Any, field: str) -> None:
    if not isinstance(value, dict) or set(value) != {"x", "z"}:
        raise ValueError(f"{field} must contain exactly x and z")
    if not _is_number(value["x"]) or not _is_number(value["z"]):
        raise ValueError(f"{field}.x and {field}.z must be finite numbers")


def _validate_players(
    players: Any, field: str, *, require_champion: bool
) -> list[str]:
    if not isinstance(players, list) or len(players) != 10:
        raise ValueError(f"{field} must contain exactly ten players")
    player_ids: list[str] = []
    for index, player in enumerate(players):
        if not isinstance(player, dict):
            raise ValueError(f"{field}[{index}] must be an object")
        player_id = player.get("playerId")
        if not isinstance(player_id, str) or not player_id.strip():
            raise ValueError(f"{field}[{index}].playerId must be non-empty")
        if require_champion and (
            not isinstance(player.get("champion"), str) or not player["champion"].strip()
        ):
            raise ValueError(f"{field}[{index}].champion must be non-empty")
        _validate_position(player.get("position"), f"{field}[{index}].position")
        player_ids.append(player_id)
    if len(player_ids) != len(set(player_ids)):
        raise ValueError(f"{field} playerId values must be unique")
    return player_ids


def validate_frame_pair(record: Any, index: int) -> dict[str, Any]:
    if not isinstance(record, dict):
        raise ValueError(f"record {index}: expected an object")
    if not isinstance(record.get("matchId"), str) or not record["matchId"].strip():
        raise ValueError(f"record {index}: matchId must be non-empty")
    frame_index = record.get("frameIndex")
    next_frame_index = record.get("nextFrameIndex")
    if not isinstance(frame_index, int) or isinstance(frame_index, bool) or frame_index < 0:
        raise ValueError(f"record {index}: frameIndex must be a non-negative integer")
    if not isinstance(next_frame_index, int) or isinstance(next_frame_index, bool):
        raise ValueError(f"record {index}: nextFrameIndex must be an integer")
    if next_frame_index != frame_index + 1:
        raise ValueError(f"record {index}: nextFrameIndex must equal frameIndex + 1")
    frame_time = record.get("frameTimeSeconds")
    next_frame_time = record.get("nextFrameTimeSeconds")
    if not _is_number(frame_time) or not _is_number(next_frame_time):
        raise ValueError(f"record {index}: frame times must be finite numbers")
    if next_frame_time <= frame_time:
        raise ValueError(f"record {index}: next frame time must be later")
    current_ids = _validate_players(
        record.get("players"), f"record {index}.players", require_champion=True
    )
    next_ids = _validate_players(
        record.get("nextPlayers"), f"record {index}.nextPlayers", require_champion=False
    )
    if set(current_ids) != set(next_ids):
        raise ValueError(f"record {index}: current and next frames must contain identical players")
    metadata = record.get("metadata")
    if not isinstance(metadata, dict):
        raise ValueError(f"record {index}: metadata must be an object")
    for name in (
        "datasetRepo",
        "datasetRevision",
        "datasetFile",
        "datasetFileSha256",
        "coordinateMethod",
    ):
        if not isinstance(metadata.get(name), str) or not metadata[name].strip():
            raise ValueError(f"record {index}: metadata.{name} must be non-empty")
    result = deepcopy(record)
    result["players"].sort(key=lambda player: player["playerId"])
    result["nextPlayers"].sort(key=lambda player: player["playerId"])
    return result


def record_order_key(record: dict[str, Any]) -> tuple[str, int]:
    return record["matchId"], record["frameIndex"]


def grouped_split(
    records: list[dict[str, Any]], *, eval_fraction: float, seed: int
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    if not 0 < eval_fraction < 1:
        raise ValueError("eval_fraction must be in (0, 1)")
    match_ids = sorted({record["matchId"] for record in records})
    if len(match_ids) < 2:
        raise ValueError("At least two matches are required for a held-out split")
    generator = random.Random(seed)
    generator.shuffle(match_ids)
    eval_count = min(len(match_ids) - 1, max(1, round(len(match_ids) * eval_fraction)))
    eval_ids = set(match_ids[:eval_count])
    return (
        [record for record in records if record["matchId"] not in eval_ids],
        [record for record in records if record["matchId"] in eval_ids],
    )


def build_training_example(record: dict[str, Any]) -> dict[str, Any]:
    model_input = {
        "schemaVersion": "1.0",
        "task": TASK,
        "matchId": record["matchId"],
        "frameIndex": record["frameIndex"],
        "frameTimeSeconds": record["frameTimeSeconds"],
        "players": deepcopy(record["players"]),
        "coordinateMethod": record["metadata"]["coordinateMethod"],
    }
    target = {
        "task": TASK,
        "frameTimeSeconds": record["nextFrameTimeSeconds"],
        "players": deepcopy(record["nextPlayers"]),
    }
    return {
        "messages": [
            {"role": "system", "content": POSITION_SYSTEM_PROMPT},
            {
                "role": "user",
                "content": json.dumps(model_input, sort_keys=True, separators=(",", ":")),
            },
            {"role": "assistant", "content": json.dumps(target, sort_keys=True)},
        ],
        "metadata": {
            "matchId": record["matchId"],
            "frameIndex": record["frameIndex"],
            "nextFrameIndex": record["nextFrameIndex"],
            "datasetRepo": record["metadata"]["datasetRepo"],
            "datasetRevision": record["metadata"]["datasetRevision"],
            "datasetFile": record["metadata"]["datasetFile"],
        },
    }


def score_position_prediction(
    prediction: Any, expected: dict[str, Any]
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "validJson": isinstance(prediction, dict),
        "validTenPlayerFrame": False,
    }
    if not isinstance(prediction, dict):
        return result
    try:
        predicted_players = prediction["players"]
        expected_players = expected["players"]
        predicted_ids = _validate_players(
            predicted_players, "prediction.players", require_champion=False
        )
        expected_ids = _validate_players(
            expected_players, "expected.players", require_champion=False
        )
        if set(predicted_ids) != set(expected_ids):
            return result
        predicted_by_id = {player["playerId"]: player for player in predicted_players}
        errors = []
        for expected_player in expected_players:
            player_id = expected_player["playerId"]
            predicted_position = predicted_by_id[player_id]["position"]
            expected_position = expected_player["position"]
            errors.append(
                math.hypot(
                    predicted_position["x"] - expected_position["x"],
                    predicted_position["z"] - expected_position["z"],
                )
            )
        result.update(
            {
                "validTenPlayerFrame": True,
                "meanPlayerPositionError": statistics.fmean(errors),
                "medianPlayerPositionError": statistics.median(errors),
                "maxPlayerPositionError": max(errors),
                "playerErrors": errors,
            }
        )
    except (KeyError, TypeError, ValueError):
        pass
    return result


def summarize_results(results: list[dict[str, Any]]) -> dict[str, Any]:
    valid = [result for result in results if result.get("validTenPlayerFrame")]
    mean_errors = [result["meanPlayerPositionError"] for result in valid]
    return {
        "heldOutOnly": True,
        "examples": len(results),
        "validJsonRate": sum(result.get("validJson", False) for result in results) / len(results) if results else 0,
        "validTenPlayerFrameRate": len(valid) / len(results) if results else 0,
        "meanFramePlayerPositionError": statistics.fmean(mean_errors) if mean_errors else None,
        "medianFramePlayerPositionError": statistics.median(mean_errors) if mean_errors else None,
    }


def write_jsonl(path: Path, records: Iterable[dict[str, Any]]) -> None:
    with Path(path).open("w", encoding="utf-8") as handle:
        for record in records:
            handle.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n")
