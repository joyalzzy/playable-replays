#!/usr/bin/env python3
"""Slice normalized VOD captions into clip-local JSON, SRT, and text transcripts."""

from __future__ import annotations

import argparse
import json
import os
import re
from pathlib import Path


ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")


def timestamp(seconds: float, comma: bool = False) -> str:
    millis = max(0, round(seconds * 1000))
    hours, millis = divmod(millis, 3_600_000)
    minutes, millis = divmod(millis, 60_000)
    secs, millis = divmod(millis, 1000)
    separator = "," if comma else "."
    return f"{hours:02d}:{minutes:02d}:{secs:02d}{separator}{millis:03d}"


def atomic_write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f"{path.name}.{os.getpid()}.partial")
    temporary.write_text(text, encoding="utf-8")
    os.replace(temporary, path)


def load_captions(path: Path) -> list[dict]:
    rows: list[dict] = []
    seen: set[str] = set()
    previous_start = -1.0
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        row = json.loads(line)
        caption_id = row.get("caption_id")
        if not isinstance(caption_id, str) or not caption_id:
            raise ValueError(f"caption line {line_number} has no caption_id")
        if caption_id in seen:
            raise ValueError(f"duplicate caption_id: {caption_id}")
        start_s = float(row["start_s"])
        end_s = float(row["end_s"])
        if start_s < previous_start or start_s < 0 or end_s <= start_s:
            raise ValueError(f"invalid or unordered timing for {caption_id}")
        if not isinstance(row.get("text"), str) or not row["text"]:
            raise ValueError(f"caption {caption_id} has no text")
        seen.add(caption_id)
        previous_start = start_s
        rows.append(row)
    if not rows:
        raise ValueError("normalized caption file is empty")
    return rows


def load_moments(path: Path) -> list[dict]:
    data = json.loads(path.read_text(encoding="utf-8"))
    rows = data.get("moments") if isinstance(data, dict) else data
    if not isinstance(rows, list) or not rows:
        raise ValueError("moments file must contain a non-empty moments array")
    ids: set[str] = set()
    for row in rows:
        moment_id = str(row.get("id", ""))
        if not ID_RE.fullmatch(moment_id) or moment_id in ids:
            raise ValueError(f"invalid or duplicate moment id: {moment_id}")
        start_s = float(row["vod_start_s"])
        end_s = float(row["vod_end_s"])
        if not 0 <= start_s < end_s:
            raise ValueError(f"invalid clip window for {moment_id}")
        ids.add(moment_id)
    return rows


def cited_ids_by_moment(path: Path | None) -> dict[str, set[str]]:
    if path is None:
        return {}
    data = json.loads(path.read_text(encoding="utf-8"))
    moments = data.get("moments") if isinstance(data, dict) else None
    if not isinstance(moments, list):
        raise ValueError("analysis manifest must contain a moments array")
    result: dict[str, set[str]] = {}
    for moment in moments:
        cited: set[str] = set()
        transcript = moment.get("transcript")
        if isinstance(transcript, dict):
            cited.update(str(value) for value in transcript.get("caption_ids", []))
        caster = moment.get("caster_context")
        if isinstance(caster, dict):
            cited.update(str(value) for value in caster.get("caption_ids", []))
        result[str(moment.get("id"))] = cited
    return result


def slice_rows(captions: list[dict], start_s: float, end_s: float) -> list[dict]:
    selected: list[dict] = []
    for row in captions:
        source_start = float(row["start_s"])
        source_end = float(row["end_s"])
        if source_end <= start_s or source_start >= end_s:
            continue
        item = dict(row)
        item["source_start_s"] = source_start
        item["source_end_s"] = source_end
        item["clip_start_s"] = round(max(0.0, source_start - start_s), 3)
        item["clip_end_s"] = round(min(end_s, source_end) - start_s, 3)
        selected.append(item)
    return selected


def write_transcript(root: Path, moment_id: str, rows: list[dict]) -> dict[str, str]:
    json_path = root / f"{moment_id}.json"
    srt_path = root / f"{moment_id}.srt"
    text_path = root / f"{moment_id}.txt"
    atomic_write(json_path, json.dumps(rows, ensure_ascii=False, indent=2) + "\n")

    srt_blocks = []
    text_lines = []
    for index, row in enumerate(rows, start=1):
        start = float(row["clip_start_s"])
        end = float(row["clip_end_s"])
        srt_blocks.append(
            f"{index}\n{timestamp(start, comma=True)} --> {timestamp(end, comma=True)}\n"
            f"{row['text']}"
        )
        text_lines.append(
            f"[clip {timestamp(start)} --> {timestamp(end)}; "
            f"VOD {timestamp(float(row['source_start_s']))}] {row['text']}"
        )
    atomic_write(srt_path, "\n\n".join(srt_blocks) + ("\n" if srt_blocks else ""))
    atomic_write(text_path, "\n".join(text_lines) + ("\n" if text_lines else ""))
    return {
        "json": str(json_path),
        "srt": str(srt_path),
        "text": str(text_path),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--captions", required=True, type=Path, help="normalized JSONL captions")
    parser.add_argument("--moments", required=True, type=Path, help="render-moments JSON")
    parser.add_argument("--output", required=True, type=Path, help="transcripts directory")
    parser.add_argument(
        "--analysis",
        type=Path,
        help="optional canonical moments.json whose cited caption IDs must be present",
    )
    args = parser.parse_args()

    captions = load_captions(args.captions)
    moments = load_moments(args.moments)
    citations = cited_ids_by_moment(args.analysis)
    results = []
    for moment in moments:
        moment_id = str(moment["id"])
        rows = slice_rows(
            captions,
            float(moment["vod_start_s"]),
            float(moment["vod_end_s"]),
        )
        present = {str(row["caption_id"]) for row in rows}
        missing = sorted(citations.get(moment_id, set()) - present)
        if missing:
            raise ValueError(f"{moment_id} cites captions outside its clip: {missing}")
        paths = write_transcript(args.output, moment_id, rows)
        results.append({"id": moment_id, "cue_count": len(rows), "paths": paths})

    print(json.dumps({"status": "succeeded", "moments": results}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
