"""Convert packet streams into deterministic next-action examples."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

ACTION_BY_PACKET = {
    "WaypointGroup": "move",
    "WaypointGroupWithSpeed": "move",
    "BasicAttackPos": "attack",
    "CastSpellAns": "cast",
    "BuyItem": "item",
    "RemoveItem": "item",
    "SwapItem": "item",
    "UseItem": "item",
}
FEATURE_SCHEMA_VERSION = "1.0"
START_ACTION = "start"


@dataclass(frozen=True)
class Example:
    context: tuple[str, str, str]
    label: str


def game_phase(seconds: float) -> str:
    if seconds < 600:
        return "early"
    if seconds < 1200:
        return "mid"
    return "late"


def extract_examples(events: list[dict[str, Any]]) -> list[Example]:
    """Create global action-sequence examples from one chronological match."""
    examples: list[Example] = []
    previous = START_ACTION
    for event in events:
        if not isinstance(event, dict) or len(event) != 1:
            continue
        packet_type, payload = next(iter(event.items()))
        label = ACTION_BY_PACKET.get(packet_type)
        if label is None or not isinstance(payload, dict):
            continue
        timestamp = payload.get("time")
        if not isinstance(timestamp, (int, float)) or timestamp < 0:
            continue
        phase = game_phase(float(timestamp))
        context = (phase, previous, packet_type if label == "item" else "action")
        examples.append(Example(context, label))
        previous = label
    return examples
