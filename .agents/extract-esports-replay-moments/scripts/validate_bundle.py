#!/usr/bin/env python3
"""Validate a pivotal-moment deliverable, its media, checksums, and ZIPs."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import zipfile
from pathlib import Path, PurePosixPath
from urllib.parse import urlparse


HASH_RE = re.compile(r"^([0-9a-f]{64})  (.+)$")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def safe_relative(root: Path, value: str) -> Path:
    pure = PurePosixPath(value)
    if pure.is_absolute() or ".." in pure.parts:
        raise ValueError(f"unsafe asset path: {value}")
    path = root.joinpath(*pure.parts)
    resolved_root = root.resolve()
    resolved = path.resolve()
    if resolved != resolved_root and resolved_root not in resolved.parents:
        raise ValueError(f"asset escapes deliverable: {value}")
    return path


def probe_media(path: Path, ffprobe: str) -> dict:
    completed = subprocess.run(
        [ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", str(path)],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if completed.returncode:
        raise RuntimeError(f"ffprobe failed for {path}: {completed.stderr.strip()}")
    return json.loads(completed.stdout)


def probe_duration(path: Path, ffprobe: str) -> float:
    return float(probe_media(path, ffprobe)["format"]["duration"])


def decode(path: Path, ffmpeg: str) -> None:
    completed = subprocess.run(
        [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-i", str(path), "-f", "null", "-"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if completed.returncode:
        raise RuntimeError(f"full decode failed for {path}: {completed.stderr.strip()[-3000:]}")


def load_manifest(root: Path) -> tuple[Path, dict]:
    path = root / "analysis" / "moments.json"
    if not path.is_file():
        raise FileNotFoundError(f"manifest missing: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    return path, data


def load_reference_ids(path: Path, collection_key: str, label: str) -> set[str]:
    if not path.is_file():
        raise FileNotFoundError(f"{label} missing: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    records = data.get(collection_key) if isinstance(data, dict) else data
    if not isinstance(records, list):
        raise ValueError(f"{label} must contain a {collection_key} array")
    ids: list[str] = []
    for index, record in enumerate(records):
        if not isinstance(record, dict) or not isinstance(record.get("id"), str) or not record["id"]:
            raise ValueError(f"{label} record {index} has no id")
        ids.append(record["id"])
    if len(ids) != len(set(ids)):
        raise ValueError(f"{label} ids are not unique")
    return set(ids)


def validate_core_contract(data: dict) -> None:
    if not isinstance(data, dict) or set(data) != {"schema_version", "source", "moments"}:
        raise ValueError("manifest must contain exactly schema_version, source, and moments")
    if data.get("schema_version") != "1.0.0":
        raise ValueError("manifest must be an object with schema_version 1.0.0")
    source = data.get("source")
    if not isinstance(source, dict) or not isinstance(source.get("title"), str) or not source["title"]:
        raise ValueError("source.title is required")
    if (
        isinstance(source.get("duration_s"), bool)
        or not isinstance(source.get("duration_s"), (int, float))
        or source["duration_s"] <= 0
    ):
        raise ValueError("source.duration_s must be positive")
    if "url" in source:
        url = source["url"]
        if not isinstance(url, str) or not urlparse(url).scheme:
            raise ValueError("source.url must be a URI")
    if "platform_id" in source and (not isinstance(source["platform_id"], str) or not source["platform_id"]):
        raise ValueError("source.platform_id must be a non-empty string")
    if "video_sha256" in source and not re.fullmatch(r"[0-9a-f]{64}", str(source["video_sha256"])):
        raise ValueError("source.video_sha256 must be lowercase SHA-256")
    if "caption_sources" in source:
        caption_sources = source["caption_sources"]
        if (
            not isinstance(caption_sources, list)
            or any(not isinstance(item, str) for item in caption_sources)
            or len(caption_sources) != len(set(caption_sources))
        ):
            raise ValueError("source.caption_sources must contain unique strings")
    moments = data.get("moments")
    if not isinstance(moments, list) or not moments:
        raise ValueError("moments must be a non-empty array")
    project_ready_count = 0

    required_assets = {
        "clip", "audio", "thumbnail", "transcript_json", "transcript_srt", "transcript_text"
    }
    for index, moment in enumerate(moments):
        if not isinstance(moment, dict):
            raise ValueError(f"moment {index} must be an object")
        for key in ("id", "game_id", "title", "event_type", "project_ready", "vod", "game_time", "alignment", "transcript", "caster_context", "assessment", "evidence", "assets"):
            if key not in moment:
                raise ValueError(f"moment {index} is missing {key}")
        if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", moment["id"]):
            raise ValueError(f"invalid moment id: {moment['id']}")
        for key in ("game_id", "title", "event_type"):
            if not isinstance(moment[key], str) or not moment[key]:
                raise ValueError(f"{key} must be a non-empty string for {moment['id']}")
        if not isinstance(moment["project_ready"], bool):
            raise ValueError(f"project_ready must be a boolean for {moment['id']}")
        if moment["project_ready"]:
            project_ready_count += 1
        vod = moment["vod"]
        if not isinstance(vod, dict) or set(vod) != {"start_s", "decision_s", "end_s"}:
            raise ValueError(f"vod must contain exactly start_s, decision_s, and end_s for {moment['id']}")
        try:
            if any(isinstance(vod[key], bool) or not isinstance(vod[key], (int, float)) for key in vod):
                raise TypeError
            ordered = 0 <= vod["start_s"] < vod["decision_s"] < vod["end_s"]
        except (KeyError, TypeError, ValueError) as exc:
            raise ValueError(f"invalid VOD times for {moment['id']}") from exc
        if not ordered:
            raise ValueError(f"unordered VOD times for {moment['id']}")
        if vod["end_s"] > source["duration_s"] + 0.25:
            raise ValueError(f"VOD end exceeds source duration for {moment['id']}")
        game_time = moment["game_time"]
        if not isinstance(game_time, dict):
            raise ValueError(f"game_time must be an object for {moment['id']}")
        for key in ("start", "end"):
            if not re.fullmatch(r"[0-9]{1,2}:[0-5][0-9]", str(game_time.get(key, ""))):
                raise ValueError(f"invalid game_time.{key} for {moment['id']}")
        if "decision" in game_time and not re.fullmatch(r"[0-9]{1,2}:[0-5][0-9]", str(game_time["decision"])):
            raise ValueError(f"invalid game_time.decision for {moment['id']}")
        alignment = moment["alignment"]
        if not isinstance(alignment, dict) or not isinstance(alignment.get("method"), str) or not alignment["method"]:
            raise ValueError(f"alignment.method is required for {moment['id']}")
        if alignment.get("confidence") not in {"high", "medium", "low"}:
            raise ValueError(f"invalid alignment confidence for {moment['id']}")
        anchor_ids = alignment.get("anchor_ids")
        if (
            not isinstance(anchor_ids, list)
            or not anchor_ids
            or any(not isinstance(item, str) for item in anchor_ids)
            or len(anchor_ids) != len(set(anchor_ids))
        ):
            raise ValueError(f"alignment.anchor_ids must contain unique strings for {moment['id']}")
        if moment["project_ready"] and alignment.get("confidence") == "low":
            raise ValueError(f"project-ready moment has low alignment confidence: {moment['id']}")
        transcript = moment["transcript"]
        if not isinstance(transcript, dict):
            raise ValueError(f"transcript must be an object for {moment['id']}")
        for key in ("source", "speaker_label"):
            if not isinstance(transcript.get(key), str) or not transcript[key]:
                raise ValueError(f"transcript.{key} is required for {moment['id']}")
        for key in ("caption_ids", "limitations"):
            values = transcript.get(key)
            if (
                not isinstance(values, list)
                or any(not isinstance(item, str) for item in values)
                or len(values) != len(set(values))
            ):
                raise ValueError(f"transcript.{key} must contain unique strings for {moment['id']}")
        caster_context = moment["caster_context"]
        if not isinstance(caster_context, dict) or not isinstance(caster_context.get("summary"), str):
            raise ValueError(f"caster_context.summary must be a string for {moment['id']}")
        caster_ids = caster_context.get("caption_ids")
        if (
            not isinstance(caster_ids, list)
            or any(not isinstance(item, str) for item in caster_ids)
            or len(caster_ids) != len(set(caster_ids))
        ):
            raise ValueError(f"caster_context.caption_ids must contain unique strings for {moment['id']}")
        assessment = moment["assessment"]
        if not isinstance(assessment, dict) or assessment.get("label") not in {
            "correct", "incorrect", "conditional", "uncertain"
        }:
            raise ValueError(f"invalid assessment label for {moment['id']}")
        for key in ("summary", "reasoning"):
            if not isinstance(assessment.get(key), str) or not assessment[key]:
                raise ValueError(f"assessment.{key} is required for {moment['id']}")
        for key in ("better_alternative", "uncertainty"):
            if key in assessment and assessment[key] is not None and not isinstance(assessment[key], str):
                raise ValueError(f"assessment.{key} must be a string or null for {moment['id']}")
        if "recommended_action" in assessment and assessment["recommended_action"] not in {
            "move", "hold", "contest", "retreat"
        }:
            raise ValueError(f"invalid assessment.recommended_action for {moment['id']}")
        if moment["project_ready"] and assessment.get("recommended_action") not in {
            "move", "hold", "contest", "retreat"
        }:
            raise ValueError(f"project-ready moment needs a recommended action: {moment['id']}")
        evidence = moment["evidence"]
        if not isinstance(evidence, dict):
            raise ValueError(f"evidence must be an object for {moment['id']}")
        event_ids = evidence.get("external_event_ids")
        source_urls = evidence.get("source_urls")
        if (
            not isinstance(event_ids, list)
            or not event_ids
            or any(not isinstance(item, str) for item in event_ids)
        ):
            raise ValueError(f"evidence.external_event_ids must be a non-empty string array for {moment['id']}")
        if (
            not isinstance(source_urls, list)
            or not source_urls
            or any(not isinstance(item, str) or not urlparse(item).scheme for item in source_urls)
        ):
            raise ValueError(f"evidence.source_urls must contain URIs for {moment['id']}")
        assets = moment["assets"]
        if not isinstance(assets, dict) or set(assets) != required_assets:
            raise ValueError(f"assets for {moment['id']} must contain exactly {sorted(required_assets)}")
        for rel in assets.values():
            if not isinstance(rel, str) or not rel or PurePosixPath(rel).is_absolute() or ".." in PurePosixPath(rel).parts:
                raise ValueError(f"unsafe asset path for {moment['id']}: {rel}")

    if not 1 <= project_ready_count <= 3:
        raise ValueError("manifest must mark between one and three moments project_ready")


def validate_schema(data: dict, schema_path: Path) -> str:
    schema = json.loads(schema_path.read_text(encoding="utf-8"))
    try:
        import jsonschema
    except ImportError:
        validate_core_contract(data)
        return "bundled-contract-1.0.0"
    jsonschema.Draft202012Validator(schema, format_checker=jsonschema.FormatChecker()).validate(data)
    return "jsonschema-draft-2020-12"


def write_checksums(root: Path, checksum_path: Path) -> None:
    files = sorted(
        path for path in root.rglob("*")
        if path.is_file() and path != checksum_path and ".partial" not in path.name
    )
    temporary = checksum_path.with_suffix(checksum_path.suffix + ".partial")
    with temporary.open("w", encoding="utf-8") as handle:
        for path in files:
            rel = path.relative_to(root).as_posix()
            handle.write(f"{sha256(path)}  {rel}\n")
    os.replace(temporary, checksum_path)


def validate_checksums(root: Path, checksum_path: Path) -> int:
    if not checksum_path.is_file():
        raise FileNotFoundError(f"checksum manifest missing: {checksum_path}")
    count = 0
    listed: set[str] = set()
    for line_number, line in enumerate(checksum_path.read_text(encoding="utf-8").splitlines(), start=1):
        match = HASH_RE.fullmatch(line)
        if not match:
            raise ValueError(f"invalid checksum line {line_number}")
        expected, rel = match.groups()
        if rel in listed:
            raise ValueError(f"duplicate checksum path: {rel}")
        listed.add(rel)
        path = safe_relative(root, rel)
        if not path.is_file():
            raise FileNotFoundError(f"checksummed file missing: {rel}")
        actual = sha256(path)
        if actual != expected:
            raise ValueError(f"checksum mismatch: {rel}")
        count += 1
    expected_paths = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file() and path != checksum_path and ".partial" not in path.name
    }
    if listed != expected_paths:
        missing = sorted(expected_paths - listed)
        unexpected = sorted(listed - expected_paths)
        raise ValueError(f"checksum coverage mismatch; missing={missing}, unexpected={unexpected}")
    return count


def validate_archive(path: Path) -> int:
    with zipfile.ZipFile(path) as archive:
        names: set[str] = set()
        for member in archive.infolist():
            pure = PurePosixPath(member.filename)
            if pure.is_absolute() or ".." in pure.parts:
                raise ValueError(f"unsafe ZIP member in {path}: {member.filename}")
            if member.filename in names:
                raise ValueError(f"duplicate ZIP member in {path}: {member.filename}")
            names.add(member.filename)
            mode = member.external_attr >> 16
            if mode & 0o170000 == 0o120000:
                raise ValueError(f"ZIP symlink is not allowed in {path}: {member.filename}")
        bad = archive.testzip()
        if bad:
            raise ValueError(f"corrupt ZIP member in {path}: {bad}")
        return len(archive.infolist())


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path, help="deliverable root")
    parser.add_argument("--schema", type=Path, help="schema path; defaults to root/schema/pivotal-moments.schema.json")
    parser.add_argument("--archive", action="append", default=[], type=Path, help="ZIP to test; repeat as needed")
    parser.add_argument("--write-checksums", action="store_true")
    parser.add_argument("--skip-decode", action="store_true")
    parser.add_argument("--ffmpeg", default="ffmpeg")
    parser.add_argument("--ffprobe", default="ffprobe")
    args = parser.parse_args()

    root = args.root.resolve()
    schema_path = (args.schema or root / "schema" / "pivotal-moments.schema.json").resolve()
    if not root.is_dir():
        raise SystemExit(f"deliverable not found: {root}")
    if not schema_path.is_file():
        raise SystemExit(f"schema not found: {schema_path}")
    partials = [
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_file() and ".partial" in path.name
    ]
    if partials:
        raise ValueError(f"partial files remain in deliverable: {partials}")
    symlinks = [
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if path.is_symlink()
    ]
    if symlinks:
        raise ValueError(f"symlinks are not allowed in deliverable: {symlinks}")
    for executable in (args.ffmpeg, args.ffprobe):
        if shutil.which(executable) is None:
            raise SystemExit(f"required executable not found: {executable}")

    _, data = load_manifest(root)
    schema_engine = validate_schema(data, schema_path)
    moments = data["moments"]
    ids = [moment["id"] for moment in moments]
    if len(ids) != len(set(ids)):
        raise ValueError("moment ids are not unique")
    alignment_ids = load_reference_ids(
        root / "analysis" / "alignment.json", "anchors", "alignment file"
    )
    timeline_event_ids = load_reference_ids(
        root / "research" / "external_timeline.json", "events", "external timeline"
    )

    referenced: set[Path] = set()
    decoded = 0
    for moment in moments:
        vod = moment["vod"]
        missing_anchors = sorted(set(moment["alignment"]["anchor_ids"]) - alignment_ids)
        if missing_anchors:
            raise ValueError(f"unknown alignment anchors for {moment['id']}: {missing_anchors}")
        missing_events = sorted(
            set(moment["evidence"]["external_event_ids"]) - timeline_event_ids
        )
        if missing_events:
            raise ValueError(f"unknown timeline events for {moment['id']}: {missing_events}")
        if not (vod["start_s"] < vod["decision_s"] < vod["end_s"]):
            raise ValueError(f"unordered VOD times for {moment['id']}")
        expected_duration = vod["end_s"] - vod["start_s"]
        transcript_records = None
        for kind, rel in moment["assets"].items():
            path = safe_relative(root, rel)
            if not path.is_file() or path.stat().st_size == 0:
                raise FileNotFoundError(f"missing or empty {kind} for {moment['id']}: {rel}")
            referenced.add(path)
            if kind == "transcript_json":
                transcript_records = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(transcript_records, list):
            raise ValueError(f"transcript JSON must be an array for {moment['id']}")
        transcript_ids = {
            str(record.get("caption_id") or record.get("id"))
            for record in transcript_records
            if isinstance(record, dict) and (record.get("caption_id") or record.get("id"))
        }
        cited_ids = set(moment["transcript"]["caption_ids"]) | set(
            moment["caster_context"]["caption_ids"]
        )
        missing_ids = sorted(cited_ids - transcript_ids)
        if missing_ids:
            raise ValueError(f"cited captions absent from {moment['id']} transcript: {missing_ids}")
        clip = safe_relative(root, moment["assets"]["clip"])
        audio = safe_relative(root, moment["assets"]["audio"])
        thumbnail = safe_relative(root, moment["assets"]["thumbnail"])
        clip_probe = probe_media(clip, args.ffprobe)
        clip_types = {stream.get("codec_type") for stream in clip_probe.get("streams", [])}
        if not {"video", "audio"}.issubset(clip_types):
            raise ValueError(f"clip must contain video and audio for {moment['id']}")
        actual_duration = float(clip_probe["format"]["duration"])
        tolerance = max(1.0, expected_duration * 0.01)
        if abs(actual_duration - expected_duration) > tolerance:
            raise ValueError(
                f"duration mismatch for {moment['id']}: expected {expected_duration:.3f}, got {actual_duration:.3f}"
            )
        audio_probe = probe_media(audio, args.ffprobe)
        if "audio" not in {stream.get("codec_type") for stream in audio_probe.get("streams", [])}:
            raise ValueError(f"audio asset has no audio stream for {moment['id']}")
        audio_duration = float(audio_probe["format"]["duration"])
        if abs(audio_duration - expected_duration) > tolerance:
            raise ValueError(
                f"audio duration mismatch for {moment['id']}: expected {expected_duration:.3f}, got {audio_duration:.3f}"
            )
        image_paths = [thumbnail]
        for phase in ("start", "decision", "aftermath"):
            qa_path = root / "qa" / f"{moment['id']}_{phase}.jpg"
            if not qa_path.is_file() or qa_path.stat().st_size == 0:
                raise FileNotFoundError(f"missing QA {phase} frame for {moment['id']}")
            image_paths.append(qa_path)
            referenced.add(qa_path)
        for image_path in image_paths:
            image_probe = probe_media(image_path, args.ffprobe)
            if "video" not in {stream.get("codec_type") for stream in image_probe.get("streams", [])}:
                raise ValueError(f"image is not decodable: {image_path}")
        if not args.skip_decode:
            decode(clip, args.ffmpeg)
            decode(audio, args.ffmpeg)
            for image_path in image_paths:
                decode(image_path, args.ffmpeg)
            decoded += 2 + len(image_paths)

    checksum_path = root / "CHECKSUMS.sha256"
    if args.write_checksums:
        write_checksums(root, checksum_path)
    checksum_count = validate_checksums(root, checksum_path)
    archive_results = {str(path): validate_archive(path) for path in args.archive}
    result = {
        "status": "succeeded",
        "schema_engine": schema_engine,
        "moments": len(moments),
        "referenced_assets": len(referenced),
        "decoded_media": decoded,
        "checksums": checksum_count,
        "archives": archive_results,
    }
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
