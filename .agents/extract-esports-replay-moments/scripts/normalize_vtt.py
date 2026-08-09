#!/usr/bin/env python3
"""Convert WebVTT or YouTube JSON3 captions into stable replay-dataset formats."""

from __future__ import annotations

import argparse
import csv
import html
import json
import re
import unicodedata
from dataclasses import dataclass, asdict
from pathlib import Path


TIMING_RE = re.compile(
    r"^(?P<start>(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{3})\s+-->\s+"
    r"(?P<end>(?:\d{1,2}:)?\d{2}:\d{2}[.,]\d{3})(?:\s+.*)?$"
)
TAG_RE = re.compile(r"<[^>]+>")
SPACE_RE = re.compile(r"\s+")


@dataclass(frozen=True)
class Cue:
    caption_id: str
    start_s: float
    end_s: float
    start_timestamp: str
    end_timestamp: str
    text: str
    source: str


def parse_timestamp(value: str) -> float:
    parts = value.replace(",", ".").split(":")
    if len(parts) == 2:
        hours = 0
        minutes, seconds = parts
    elif len(parts) == 3:
        hours, minutes, seconds = parts
    else:
        raise ValueError(f"invalid WebVTT timestamp: {value}")
    return int(hours) * 3600 + int(minutes) * 60 + float(seconds)


def format_timestamp(seconds: float, comma: bool = False) -> str:
    millis = max(0, round(seconds * 1000))
    hours, millis = divmod(millis, 3_600_000)
    minutes, millis = divmod(millis, 60_000)
    secs, millis = divmod(millis, 1000)
    sep = "," if comma else "."
    return f"{hours:02d}:{minutes:02d}:{secs:02d}{sep}{millis:03d}"


def clean_text(lines: list[str]) -> str:
    value = " ".join(lines)
    value = TAG_RE.sub(" ", value)
    value = unicodedata.normalize("NFC", html.unescape(value))
    return SPACE_RE.sub(" ", value).strip()


def parse_vtt(path: Path, source: str, dedupe_exact: bool) -> list[Cue]:
    text = path.read_text(encoding="utf-8-sig").replace("\r\n", "\n")
    blocks = re.split(r"\n{2,}", text)
    parsed: list[tuple[float, float, str]] = []

    for block in blocks:
        lines = [line.strip() for line in block.splitlines() if line.strip()]
        if not lines or lines[0].startswith(("WEBVTT", "NOTE", "STYLE", "REGION")):
            continue
        timing_index = next((i for i, line in enumerate(lines) if "-->" in line), None)
        if timing_index is None:
            continue
        match = TIMING_RE.match(lines[timing_index])
        if not match:
            raise ValueError(f"unrecognized timing line: {lines[timing_index]}")
        start_s = parse_timestamp(match.group("start"))
        end_s = parse_timestamp(match.group("end"))
        cue_text = clean_text(lines[timing_index + 1 :])
        if not cue_text or end_s <= start_s:
            continue
        record = (start_s, end_s, cue_text)
        if dedupe_exact and parsed and record == parsed[-1]:
            continue
        parsed.append(record)

    return [
        Cue(
            caption_id=f"cap-{index:06d}",
            start_s=start_s,
            end_s=end_s,
            start_timestamp=format_timestamp(start_s),
            end_timestamp=format_timestamp(end_s),
            text=cue_text,
            source=source,
        )
        for index, (start_s, end_s, cue_text) in enumerate(parsed, start=1)
    ]


def parse_json3(path: Path, source: str, dedupe_exact: bool) -> list[Cue]:
    data = json.loads(path.read_text(encoding="utf-8"))
    events = data.get("events") if isinstance(data, dict) else None
    if not isinstance(events, list):
        raise ValueError("JSON3 captions must contain an events array")

    parsed: list[tuple[float, float, str]] = []
    for event in events:
        if not isinstance(event, dict) or "tStartMs" not in event:
            continue
        segments = event.get("segs")
        if not isinstance(segments, list):
            continue
        cue_text = clean_text([
            str(segment.get("utf8", ""))
            for segment in segments
            if isinstance(segment, dict)
        ])
        if not cue_text:
            continue
        try:
            start_s = int(event["tStartMs"]) / 1000
            end_s = start_s + int(event.get("dDurationMs") or 0) / 1000
        except (TypeError, ValueError):
            continue
        if end_s <= start_s:
            continue
        record = (start_s, end_s, cue_text)
        if dedupe_exact and parsed and record == parsed[-1]:
            continue
        parsed.append(record)

    return [
        Cue(
            caption_id=f"cap-{index:06d}",
            start_s=start_s,
            end_s=end_s,
            start_timestamp=format_timestamp(start_s),
            end_timestamp=format_timestamp(end_s),
            text=cue_text,
            source=source,
        )
        for index, (start_s, end_s, cue_text) in enumerate(parsed, start=1)
    ]


def write_outputs(cues: list[Cue], out_dir: Path, stem: str) -> dict[str, str]:
    out_dir.mkdir(parents=True, exist_ok=True)
    paths = {
        "jsonl": out_dir / f"{stem}.jsonl",
        "csv": out_dir / f"{stem}.csv",
        "srt": out_dir / f"{stem}.srt",
        "text": out_dir / f"{stem}.txt",
    }

    with paths["jsonl"].open("w", encoding="utf-8") as handle:
        for cue in cues:
            handle.write(json.dumps(asdict(cue), ensure_ascii=False) + "\n")

    with paths["csv"].open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(asdict(cues[0]).keys()) if cues else [
            "caption_id", "start_s", "end_s", "start_timestamp", "end_timestamp", "text", "source"
        ])
        writer.writeheader()
        writer.writerows(asdict(cue) for cue in cues)

    with paths["srt"].open("w", encoding="utf-8") as handle:
        for index, cue in enumerate(cues, start=1):
            handle.write(
                f"{index}\n{format_timestamp(cue.start_s, comma=True)} --> "
                f"{format_timestamp(cue.end_s, comma=True)}\n{cue.text}\n\n"
            )

    with paths["text"].open("w", encoding="utf-8") as handle:
        for cue in cues:
            handle.write(f"[{cue.start_timestamp} --> {cue.end_timestamp}] {cue.text}\n")

    return {name: str(path) for name, path in paths.items()}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="input WebVTT or YouTube JSON3 file")
    parser.add_argument("--out-dir", required=True, type=Path)
    parser.add_argument("--stem", help="output basename; defaults to the input stem")
    parser.add_argument("--source", default="automatic-captions", help="caption source label")
    parser.add_argument("--dedupe-exact", action="store_true", help="drop only identical adjacent cues")
    args = parser.parse_args()

    if args.input.suffix.lower() in {".json", ".json3"}:
        cues = parse_json3(args.input, args.source, args.dedupe_exact)
    else:
        cues = parse_vtt(args.input, args.source, args.dedupe_exact)
    if not cues:
        raise SystemExit("no caption cues found")
    outputs = write_outputs(cues, args.out_dir, args.stem or args.input.stem)
    print(json.dumps({"cue_count": len(cues), "outputs": outputs}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
