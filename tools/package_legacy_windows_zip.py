#!/usr/bin/env python3
"""Build and audit the historical Windows updater ZIP without publishing a release.

Older SliceBridge clients need SliceBridge-Windows.zip, while newer clients
download SliceBridge.exe. Both must contain exactly the same approved executable.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
import zipfile

from release_preflight import binary_metadata, inspect_one_file_windows_exe

EXE_NAME = "SliceBridge.exe"
ZIP_NAME = "SliceBridge-Windows.zip"
MAX_EXE_BYTES = 128 * 1024 * 1024


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def read_exe(path: Path, source_sha: str | None) -> bytes:
    if path.name != EXE_NAME or not path.is_file():
        raise ValueError("Expected an existing file named SliceBridge.exe")
    size = path.stat().st_size
    if not 0 < size <= MAX_EXE_BYTES:
        raise ValueError("Windows executable has an unexpected size")
    raw = path.read_bytes()
    inspect_one_file_windows_exe(raw)
    if source_sha is not None:
        if not re.fullmatch(r"[0-9a-fA-F]{40}", source_sha):
            raise ValueError("Source commit must be a full 40-character SHA")
        meta = binary_metadata(raw)
        if not meta or meta["commit"] != source_sha.lower():
            raise ValueError("Executable build commit does not match the approved source commit")
    return raw


def verify_pair(exe_path: Path, zip_path: Path, source_sha: str | None = None) -> dict:
    raw = read_exe(exe_path, source_sha)
    if zip_path.name != ZIP_NAME or not zip_path.is_file():
        raise ValueError("Expected an existing SliceBridge-Windows.zip")
    with zipfile.ZipFile(zip_path) as archive:
        members = archive.infolist()
        if len(members) != 1 or members[0].filename != EXE_NAME or members[0].is_dir():
            raise ValueError("Legacy ZIP must contain exactly one root-level SliceBridge.exe")
        if members[0].file_size != len(raw) or archive.testzip() is not None:
            raise ValueError("Legacy ZIP size or CRC verification failed")
        inside = archive.read(EXE_NAME)
    if inside != raw:
        raise ValueError("Legacy ZIP executable differs from standalone SliceBridge.exe")
    return {
        "status": "verified locally; NOT published",
        "exeSha256": sha256(raw),
        "zipSha256": sha256(zip_path.read_bytes()),
        "legacyInnerExeSha256": sha256(inside),
        "exeBytes": len(raw),
        "sourceCommit": source_sha.lower() if source_sha else None,
        "legacyUpdaterCompatible": True,
    }


def package(exe_path: Path, zip_path: Path, source_sha: str | None = None) -> dict:
    raw = read_exe(exe_path, source_sha)
    if zip_path.name != ZIP_NAME:
        raise ValueError("Output must be named SliceBridge-Windows.zip")
    if not zip_path.parent.is_dir():
        raise ValueError("Output directory must already exist")
    created = False
    try:
        # Exclusive creation; never overwrite an existing tested candidate.
        with zip_path.open("xb") as handle:
            created = True
            with zipfile.ZipFile(handle, mode="w", compression=zipfile.ZIP_DEFLATED,
                                 compresslevel=9) as archive:
                info = zipfile.ZipInfo(EXE_NAME, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = 0o100644 << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, raw, compress_type=zipfile.ZIP_DEFLATED,
                                 compresslevel=9)
        return verify_pair(exe_path, zip_path, source_sha)
    except Exception:
        if created:
            zip_path.unlink(missing_ok=True)
        raise


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", type=Path, required=True)
    parser.add_argument("--zip", type=Path, required=True)
    parser.add_argument("--source-sha", help="Require exact embedded Git commit")
    parser.add_argument("--verify-only", action="store_true")
    args = parser.parse_args()
    try:
        result = (verify_pair if args.verify_only else package)(
            args.exe, args.zip, args.source_sha
        )
    except (ValueError, OSError, zipfile.BadZipFile) as exc:
        print("LEGACY WINDOWS PACKAGE FAILED: " + str(exc), file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
