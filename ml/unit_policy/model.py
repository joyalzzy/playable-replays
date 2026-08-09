"""Transparent multi-head linear policy used by the training pipeline."""
from __future__ import annotations

import json
import math
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

from .features import FEATURE_NAMES
from .schema import ACTION_TYPES, strict_json_loads

MODEL_FORMAT = "playable-replays-linear-unit-policy"
MODEL_FORMAT_VERSION = 1


def _dot(weights: Iterable[float], features: Iterable[float]) -> float:
    return sum(weight * feature for weight, feature in zip(weights, features, strict=True))


def _softmax(logits: dict[str, float]) -> dict[str, float]:
    maximum = max(logits.values())
    exponentials = {name: math.exp(value - maximum) for name, value in logits.items()}
    total = sum(exponentials.values())
    return {name: value / total for name, value in exponentials.items()}


def _finite_vector(value: Any, expected: int, name: str) -> tuple[float, ...]:
    if not isinstance(value, list) or len(value) != expected:
        raise ValueError(f"{name} must contain {expected} numbers")
    converted: list[float] = []
    for item in value:
        if not isinstance(item, (int, float)) or isinstance(item, bool) or not math.isfinite(float(item)):
            raise ValueError(f"{name} contains a non-finite number")
        converted.append(float(item))
    return tuple(converted)


@dataclass(frozen=True, slots=True)
class LinearUnitPolicy:
    version: str
    action_weights: dict[str, tuple[float, ...]]
    movement_x_weights: tuple[float, ...]
    movement_y_weights: tuple[float, ...]
    metadata: dict[str, Any]

    @classmethod
    def empty(cls, version: str = "unit-policy-v1") -> "LinearUnitPolicy":
        width = len(FEATURE_NAMES)
        return cls(version, {action: (0.0,) * width for action in ACTION_TYPES}, (0.0,) * width, (0.0,) * width, {})

    def action_logits(self, features: tuple[float, ...], legal_actions: Iterable[str] = ACTION_TYPES) -> dict[str, float]:
        legal = tuple(legal_actions)
        if not legal:
            raise ValueError("at least one legal action is required")
        return {action: _dot(self.action_weights[action], features) for action in legal}

    def action_probabilities(self, features: tuple[float, ...], legal_actions: Iterable[str] = ACTION_TYPES) -> dict[str, float]:
        return _softmax(self.action_logits(features, legal_actions))

    def choose_action(self, features: tuple[float, ...], legal_actions: Iterable[str] = ACTION_TYPES) -> str:
        legal = tuple(legal_actions)
        logits = self.action_logits(features, legal)
        return max(legal, key=lambda action: (logits[action], -ACTION_TYPES.index(action)))

    def movement_delta(self, features: tuple[float, ...]) -> tuple[float, float]:
        dx = math.tanh(_dot(self.movement_x_weights, features))
        dy = math.tanh(_dot(self.movement_y_weights, features))
        magnitude = math.hypot(dx, dy)
        if magnitude > 1.0:
            dx /= magnitude
            dy /= magnitude
        return dx, dy

    def to_dict(self) -> dict[str, Any]:
        return {
            "format": MODEL_FORMAT,
            "formatVersion": MODEL_FORMAT_VERSION,
            "policyVersion": self.version,
            "featureNames": list(FEATURE_NAMES),
            "actionOrder": list(ACTION_TYPES),
            "actionWeights": {action: list(self.action_weights[action]) for action in ACTION_TYPES},
            "movementWeights": {"dx": list(self.movement_x_weights), "dy": list(self.movement_y_weights)},
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, value: Any) -> "LinearUnitPolicy":
        if not isinstance(value, dict):
            raise ValueError("model artifact must be an object")
        if value.get("format") != MODEL_FORMAT or value.get("formatVersion") != MODEL_FORMAT_VERSION:
            raise ValueError("unsupported model artifact format/version")
        version = value.get("policyVersion")
        if not isinstance(version, str) or not version.strip() or len(version) > 128:
            raise ValueError("policyVersion must be a non-empty string")
        if value.get("featureNames") != list(FEATURE_NAMES):
            raise ValueError("model featureNames do not match this runtime")
        if value.get("actionOrder") != list(ACTION_TYPES):
            raise ValueError("model actionOrder does not match this runtime")
        action_value = value.get("actionWeights")
        if not isinstance(action_value, dict) or set(action_value) != set(ACTION_TYPES):
            raise ValueError("actionWeights must contain exactly the four actions")
        width = len(FEATURE_NAMES)
        action_weights = {action: _finite_vector(action_value[action], width, f"actionWeights.{action}") for action in ACTION_TYPES}
        movement = value.get("movementWeights")
        if not isinstance(movement, dict) or set(movement) != {"dx", "dy"}:
            raise ValueError("movementWeights must contain dx and dy")
        metadata = value.get("metadata", {})
        if not isinstance(metadata, dict):
            raise ValueError("metadata must be an object")
        return cls(version, action_weights, _finite_vector(movement["dx"], width, "movementWeights.dx"), _finite_vector(movement["dy"], width, "movementWeights.dy"), metadata)

    @classmethod
    def load(cls, path: str | Path) -> "LinearUnitPolicy":
        return cls.from_dict(strict_json_loads(Path(path).read_text(encoding="utf-8")))

    def save(self, path: str | Path) -> None:
        destination = Path(path)
        destination.parent.mkdir(parents=True, exist_ok=True)
        with destination.open("w", encoding="utf-8", newline="\n") as handle:
            json.dump(self.to_dict(), handle, indent=2, sort_keys=True, ensure_ascii=True)
            handle.write("\n")
