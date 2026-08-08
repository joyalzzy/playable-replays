"""Streaming reader for decoded League of Legends replay packet shards."""

from __future__ import annotations

import gzip
import json
from collections.abc import Iterator
from pathlib import Path
from typing import Any


def read_matches(path: Path, max_matches: int | None = None) -> Iterator[list[dict[str, Any]]]:
    """Yield validated event lists without retaining the full shard in memory."""
    if max_matches is not None and max_matches < 1:
        raise ValueError("max_matches must be positive")
    opener = gzip.open if path.suffix == ".gz" else path.open
    with opener(path, "rt", encoding="utf-8") as source:
        for match_index, line in enumerate(source):
            if max_matches is not None and match_index >= max_matches:
                return
            payload = json.loads(line)
            events = payload.get("events")
            if not isinstance(events, list):
                raise ValueError(f"match {match_index} has no events list")
            yield events
