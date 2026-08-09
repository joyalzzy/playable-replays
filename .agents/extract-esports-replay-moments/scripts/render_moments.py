#!/usr/bin/env python3
"""Atomically render replay clips, audio, thumbnails, and QA frames."""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import re
import shutil
import subprocess
import tempfile
from dataclasses import dataclass, asdict
from pathlib import Path


ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")


@dataclass(frozen=True)
class Moment:
    id: str
    start: float
    decision: float
    end: float


def run(command: list[str]) -> None:
    completed = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if completed.returncode:
        detail = completed.stderr.strip()[-4000:]
        raise RuntimeError(f"command failed ({completed.returncode}): {' '.join(command[:5])}\n{detail}")


def probe(path: Path, ffprobe: str) -> dict:
    completed = subprocess.run(
        [ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", str(path)],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if completed.returncode:
        raise RuntimeError(f"ffprobe failed for {path}: {completed.stderr.strip()}")
    return json.loads(completed.stdout)


def duration(path: Path, ffprobe: str) -> float:
    data = probe(path, ffprobe)
    try:
        return float(data["format"]["duration"])
    except (KeyError, TypeError, ValueError) as exc:
        raise RuntimeError(f"missing duration for {path}") from exc


def decode(path: Path, ffmpeg: str) -> None:
    run([ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-i", str(path), "-f", "null", "-"])


def parse_moments(path: Path, source_duration: float) -> list[Moment]:
    data = json.loads(path.read_text(encoding="utf-8"))
    rows = data.get("moments") if isinstance(data, dict) else data
    if not isinstance(rows, list) or not rows:
        raise ValueError("moments JSON must contain a non-empty 'moments' array")

    moments: list[Moment] = []
    ids: set[str] = set()
    for row in rows:
        moment = Moment(
            id=str(row["id"]),
            start=float(row["vod_start_s"]),
            decision=float(row["vod_decision_s"]),
            end=float(row["vod_end_s"]),
        )
        if not ID_RE.fullmatch(moment.id):
            raise ValueError(f"unsafe moment id: {moment.id}")
        if moment.id in ids:
            raise ValueError(f"duplicate moment id: {moment.id}")
        if not (0 <= moment.start < moment.decision < moment.end <= source_duration + 0.25):
            raise ValueError(f"invalid timestamps for {moment.id}: {asdict(moment)}")
        ids.add(moment.id)
        moments.append(moment)
    return moments


def output_paths(root: Path, moment_id: str) -> dict[str, Path]:
    return {
        "clip": root / "clips" / f"{moment_id}.mp4",
        "audio": root / "audio" / f"{moment_id}.flac",
        "thumbnail": root / "thumbnails" / f"{moment_id}.jpg",
        "start": root / "qa" / f"{moment_id}_start.jpg",
        "decision": root / "qa" / f"{moment_id}_decision.jpg",
        "aftermath": root / "qa" / f"{moment_id}_aftermath.jpg",
    }


def render_one(
    source: Path,
    root: Path,
    moment: Moment,
    ffmpeg: str,
    ffprobe: str,
    overwrite: bool,
) -> dict:
    finals = output_paths(root, moment.id)
    existing = [str(path) for path in finals.values() if path.exists()]
    if existing and not overwrite:
        raise FileExistsError(f"outputs already exist for {moment.id}: {existing}")

    temp_dir = Path(tempfile.mkdtemp(prefix=f"._render-{moment.id}-", dir=root))
    try:
        temps = {name: temp_dir / path.name for name, path in finals.items()}
        clip_duration = moment.end - moment.start
        common = [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-y"]

        run(common + [
            "-ss", f"{moment.start:.3f}", "-i", str(source), "-t", f"{clip_duration:.3f}",
            "-map", "0:v:0", "-map", "0:a:0", "-c:v", "libx264", "-preset", "medium",
            "-crf", "18", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", str(temps["clip"]),
        ])
        run(common + [
            "-ss", f"{moment.start:.3f}", "-i", str(source), "-t", f"{clip_duration:.3f}",
            "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "flac", str(temps["audio"]),
        ])

        frame_times = {
            "thumbnail": moment.decision,
            "start": min(moment.end - 0.05, moment.start + min(0.5, clip_duration * 0.1)),
            "decision": moment.decision,
            "aftermath": max(moment.start, moment.end - min(0.5, clip_duration * 0.1)),
        }
        for name, timestamp in frame_times.items():
            run(common + [
                "-ss", f"{timestamp:.3f}", "-i", str(source),
                "-frames:v", "1", "-q:v", "2", str(temps[name]),
            ])

        decode(temps["clip"], ffmpeg)
        decode(temps["audio"], ffmpeg)
        actual_duration = duration(temps["clip"], ffprobe)
        tolerance = max(1.0, clip_duration * 0.01)
        if abs(actual_duration - clip_duration) > tolerance:
            raise RuntimeError(
                f"duration mismatch for {moment.id}: expected {clip_duration:.3f}, got {actual_duration:.3f}"
            )
        for name in ("thumbnail", "start", "decision", "aftermath"):
            probe(temps[name], ffprobe)

        for name, final in finals.items():
            final.parent.mkdir(parents=True, exist_ok=True)
            os.replace(temps[name], final)
        return {
            "id": moment.id,
            "status": "succeeded",
            "expected_duration_s": round(clip_duration, 3),
            "actual_duration_s": round(actual_duration, 3),
            "assets": {name: str(path.relative_to(root)) for name, path in finals.items()},
        }
    finally:
        shutil.rmtree(temp_dir, ignore_errors=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--moments", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--jobs", type=int, default=1)
    parser.add_argument("--overwrite", action="store_true")
    parser.add_argument("--ffmpeg", default="ffmpeg")
    parser.add_argument("--ffprobe", default="ffprobe")
    args = parser.parse_args()

    if args.jobs < 1 or args.jobs > 8:
        raise SystemExit("--jobs must be between 1 and 8")
    if not args.source.is_file():
        raise SystemExit(f"source not found: {args.source}")
    for executable in (args.ffmpeg, args.ffprobe):
        if shutil.which(executable) is None:
            raise SystemExit(f"required executable not found: {executable}")

    source_info = probe(args.source, args.ffprobe)
    streams = source_info.get("streams", [])
    if not any(stream.get("codec_type") == "video" for stream in streams):
        raise SystemExit("source has no video stream")
    if not any(stream.get("codec_type") == "audio" for stream in streams):
        raise SystemExit("source has no audio stream")
    source_duration = duration(args.source, args.ffprobe)
    moments = parse_moments(args.moments, source_duration)
    args.output.mkdir(parents=True, exist_ok=True)

    worker = lambda item: render_one(
        args.source, args.output, item, args.ffmpeg, args.ffprobe, args.overwrite
    )
    if args.jobs == 1:
        results = [worker(moment) for moment in moments]
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as executor:
            futures = {executor.submit(worker, moment): moment.id for moment in moments}
            completed = [future.result() for future in concurrent.futures.as_completed(futures)]
            by_id = {result["id"]: result for result in completed}
        results = [by_id[moment.id] for moment in moments]

    report = {
        "source": str(args.source.resolve()),
        "source_duration_s": source_duration,
        "moment_count": len(results),
        "results": results,
    }
    report_path = args.output / "qa" / "render-report.json"
    report_path.parent.mkdir(parents=True, exist_ok=True)
    temp_report = report_path.with_suffix(".json.partial")
    temp_report.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    os.replace(temp_report, report_path)
    print(json.dumps({"status": "succeeded", "report": str(report_path), "moments": len(results)}, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
