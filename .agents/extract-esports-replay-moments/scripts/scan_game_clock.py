#!/usr/bin/env python3
"""OCR a visible game clock over a bounded VOD range and emit candidate anchors."""

from __future__ import annotations

import argparse
import csv
import io
import json
import re
import shutil
import subprocess
from pathlib import Path


CLOCK_RE = re.compile(r"(?<!\d)(\d{1,2})\s*[:;.]\s*(\d{2})(?!\d)")


def run(command: list[str], input_bytes: bytes | None = None) -> bytes:
    completed = subprocess.run(
        command,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if completed.returncode:
        detail = completed.stderr.decode("utf-8", errors="replace").strip()[-3000:]
        raise RuntimeError(f"command failed ({completed.returncode}): {detail}")
    return completed.stdout


def parse_roi(value: str) -> tuple[int, int, int, int]:
    try:
        x, y, width, height = (int(part.strip()) for part in value.split(","))
    except (TypeError, ValueError) as exc:
        raise ValueError("ROI must be X,Y,W,H using integers") from exc
    if min(x, y) < 0 or min(width, height) <= 0:
        raise ValueError("ROI coordinates must be non-negative and dimensions positive")
    return x, y, width, height


def parse_tsv(value: str) -> tuple[str, float]:
    reader = csv.DictReader(io.StringIO(value), delimiter="\t")
    tokens: list[str] = []
    confidences: list[float] = []
    for row in reader:
        text = (row.get("text") or "").strip()
        if not text:
            continue
        tokens.append(text)
        try:
            confidence = float(row.get("conf") or -1)
        except ValueError:
            confidence = -1
        if confidence >= 0:
            confidences.append(confidence)
    average = sum(confidences) / len(confidences) if confidences else -1.0
    return " ".join(tokens), average


def parse_clock(value: str) -> tuple[str, int] | None:
    normalized = value.translate(str.maketrans({"O": "0", "o": "0", "I": "1", "l": "1"}))
    match = CLOCK_RE.search(normalized)
    if not match:
        return None
    minutes = int(match.group(1))
    seconds = int(match.group(2))
    if seconds >= 60:
        return None
    return f"{minutes:02d}:{seconds:02d}", minutes * 60 + seconds


def frame_png(
    source: Path,
    vod_time_s: float,
    roi: tuple[int, int, int, int],
    ffmpeg: str,
) -> bytes:
    x, y, width, height = roi
    video_filter = (
        f"crop={width}:{height}:{x}:{y},"
        "scale=iw*4:ih*4:flags=neighbor,format=gray,eq=contrast=1.8"
    )
    return run([
        ffmpeg,
        "-nostdin",
        "-hide_banner",
        "-loglevel",
        "error",
        "-ss",
        f"{vod_time_s:.3f}",
        "-i",
        str(source),
        "-frames:v",
        "1",
        "-vf",
        video_filter,
        "-f",
        "image2pipe",
        "-vcodec",
        "png",
        "pipe:1",
    ])


def ocr(png: bytes, tesseract: str) -> tuple[str, float]:
    output = run(
        [tesseract, "stdin", "stdout", "--psm", "7", "tsv"],
        input_bytes=png,
    )
    return parse_tsv(output.decode("utf-8", errors="replace"))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--start", required=True, type=float, help="start VOD seconds")
    parser.add_argument("--end", required=True, type=float, help="end VOD seconds")
    parser.add_argument("--step", type=float, default=1.0, help="sampling interval seconds")
    parser.add_argument("--roi", required=True, help="clock crop as X,Y,W,H")
    parser.add_argument("--game-id", required=True)
    parser.add_argument("--segment-id", required=True)
    parser.add_argument("--min-confidence", type=float, default=20.0)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--ffmpeg", default="ffmpeg")
    parser.add_argument("--tesseract", default="tesseract")
    args = parser.parse_args()

    if not args.source.is_file():
        raise SystemExit(f"source not found: {args.source}")
    if not 0 <= args.start < args.end or args.step <= 0:
        raise SystemExit("require 0 <= start < end and a positive step")
    for executable in (args.ffmpeg, args.tesseract):
        if shutil.which(executable) is None:
            raise SystemExit(f"required executable not found: {executable}")
    roi = parse_roi(args.roi)

    rows = []
    sample_index = 0
    vod_time = args.start
    while vod_time <= args.end + 1e-9:
        raw_text, confidence = ocr(
            frame_png(args.source, vod_time, roi, args.ffmpeg),
            args.tesseract,
        )
        parsed = parse_clock(raw_text)
        if parsed and confidence >= args.min_confidence:
            display, game_seconds = parsed
            rows.append({
                "id": f"{args.game_id}-ocr-{sample_index:05d}",
                "game_id": args.game_id,
                "segment_id": args.segment_id,
                "game_time": display,
                "game_time_s": game_seconds,
                "vod_time_s": round(vod_time, 3),
                "method": "scoreboard-ocr",
                "ocr_confidence": round(confidence, 2),
                "raw_text": raw_text,
                "verified": False,
            })
        sample_index += 1
        vod_time = args.start + sample_index * args.step

    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_suffix(args.output.suffix + ".partial")
    with temporary.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=False) + "\n")
    temporary.replace(args.output)
    print(json.dumps({"status": "succeeded", "anchors": len(rows), "output": str(args.output)}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
