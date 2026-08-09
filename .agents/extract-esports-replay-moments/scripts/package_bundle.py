#!/usr/bin/env python3
"""Create deterministic complete/data replay archives and a download checksum file."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import zipfile
from pathlib import Path

from validate_bundle import sha256, validate_archive, validate_checksums


FIXED_TIME = (1980, 1, 1, 0, 0, 0)
STORED_SUFFIXES = {".mp4", ".flac", ".jpg", ".jpeg", ".png", ".webp", ".zip"}


def zip_info(name: str, suffix: str) -> zipfile.ZipInfo:
    info = zipfile.ZipInfo(name, FIXED_TIME)
    info.create_system = 3
    info.external_attr = 0o100644 << 16
    info.compress_type = (
        zipfile.ZIP_STORED if suffix.lower() in STORED_SUFFIXES else zipfile.ZIP_DEFLATED
    )
    return info


def write_archive(
    root: Path,
    files: list[Path],
    destination: Path,
    archive_root: str,
    virtual_checksum: str | None = None,
) -> None:
    temporary = destination.with_name(f"{destination.name}.{os.getpid()}.partial")
    try:
        with zipfile.ZipFile(temporary, "w", allowZip64=True, compresslevel=9) as archive:
            for path in sorted(files, key=lambda item: item.relative_to(root).as_posix()):
                relative = path.relative_to(root).as_posix()
                if relative.startswith("/") or ".." in Path(relative).parts:
                    raise ValueError(f"unsafe archive member: {relative}")
                info = zip_info(f"{archive_root}/{relative}", path.suffix)
                with path.open("rb") as source, archive.open(
                    info, "w", force_zip64=True
                ) as target:
                    shutil.copyfileobj(source, target, 4 * 1024 * 1024)
            if virtual_checksum is not None:
                info = zip_info(f"{archive_root}/CHECKSUMS.sha256", ".sha256")
                archive.writestr(info, virtual_checksum.encode("utf-8"))
        validate_archive(temporary)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)


def checksums_for(root: Path, files: list[Path]) -> str:
    rows = []
    for path in sorted(files, key=lambda item: item.relative_to(root).as_posix()):
        relative = path.relative_to(root).as_posix()
        rows.append(f"{sha256(path)}  {relative}\n")
    return "".join(rows)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path, help="validated deliverable root")
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--prefix", help="archive prefix; defaults to the deliverable directory name")
    parser.add_argument("--source", type=Path, help="optional validated full source download")
    parser.add_argument("--overwrite", action="store_true")
    args = parser.parse_args()

    root = args.root.resolve()
    if not root.is_dir():
        raise SystemExit(f"deliverable not found: {root}")
    validate_checksums(root, root / "CHECKSUMS.sha256")
    manifest_path = root / "analysis" / "moments.json"
    data = json.loads(manifest_path.read_text(encoding="utf-8"))
    source_manifest = data.get("source", {})
    if args.source:
        if not args.source.is_file():
            raise SystemExit(f"source not found: {args.source}")
        expected = source_manifest.get("video_sha256")
        if expected and sha256(args.source) != expected:
            raise SystemExit("source SHA-256 does not match analysis/moments.json")

    output_dir = args.output_dir.resolve()
    if output_dir == root or root in output_dir.parents:
        raise SystemExit("--output-dir must be outside the deliverable")
    output_dir.mkdir(parents=True, exist_ok=True)
    prefix = args.prefix or root.name
    if not prefix or any(character not in "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_" for character in prefix):
        raise SystemExit("--prefix must use only letters, digits, hyphens, or underscores")
    complete = output_dir / f"{prefix}-bundle.zip"
    data_only = output_dir / f"{prefix}-data.zip"
    downloads = output_dir / "DOWNLOADS.sha256"
    existing = [path for path in (complete, data_only, downloads) if path.exists()]
    if existing and not args.overwrite:
        raise SystemExit("refusing to overwrite: " + ", ".join(path.name for path in existing))

    all_files = [path for path in root.rglob("*") if path.is_file()]
    data_files = [
        path
        for path in all_files
        if path.name != "CHECKSUMS.sha256"
        and not path.relative_to(root).parts[0] in {"clips", "audio"}
    ]
    write_archive(root, all_files, complete, prefix)
    write_archive(
        root,
        data_files,
        data_only,
        prefix,
        virtual_checksum=checksums_for(root, data_files),
    )

    download_rows = [
        f"{sha256(complete)}  {complete.name}\n",
        f"{sha256(data_only)}  {data_only.name}\n",
    ]
    if args.source:
        download_rows.append(f"{sha256(args.source)}  {args.source.name}\n")
    temporary = downloads.with_suffix(downloads.suffix + ".partial")
    temporary.write_text("".join(download_rows), encoding="utf-8")
    os.replace(temporary, downloads)

    print(json.dumps({
        "status": "succeeded",
        "complete_bundle": str(complete),
        "complete_members": validate_archive(complete),
        "data_bundle": str(data_only),
        "data_members": validate_archive(data_only),
        "checksums": str(downloads),
    }, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
