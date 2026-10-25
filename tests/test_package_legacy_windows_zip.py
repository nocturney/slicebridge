"""Offline regression tests for the historical Windows single-executable ZIP."""
from __future__ import annotations

import hashlib
import io
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))
from package_legacy_windows_zip import package, verify_pair


def fake_embedded_windows_exe() -> bytes:
    engine = io.BytesIO()
    with zipfile.ZipFile(engine, "w") as archive:
        archive.writestr("slicebridge-prusa-engine.exe", b"MZoffline-sidecar")
        archive.writestr("panda2prusa-LICENSE.txt",
                         b"MIT License\nPermission is hereby granted")
    payload = engine.getvalue()
    return (b"MZsynthetic-Wails-binary" + payload + b"SLICEBRIDGE_PRUSA_V1"
            + struct.pack("<Q", len(payload)) + hashlib.sha256(payload).digest())


class LegacyWindowsZipTests(unittest.TestCase):
    def setUp(self):
        folder = tempfile.TemporaryDirectory(prefix="slicebridge-windows-zip-")
        self.addCleanup(folder.cleanup)
        self.directory = Path(folder.name)
        self.exe = self.directory / "SliceBridge.exe"
        self.zip = self.directory / "SliceBridge-Windows.zip"
        self.exe.write_bytes(fake_embedded_windows_exe())

    def test_deterministic_byte_identical_legacy_zip(self):
        result = package(self.exe, self.zip)
        self.assertTrue(result["legacyUpdaterCompatible"])
        self.assertEqual(result["exeSha256"], result["legacyInnerExeSha256"])
        with zipfile.ZipFile(self.zip) as archive:
            self.assertEqual(archive.namelist(), ["SliceBridge.exe"])
            self.assertEqual(archive.read("SliceBridge.exe"), self.exe.read_bytes())
        other_folder = self.directory / "repeat"
        other_folder.mkdir()
        other = other_folder / "SliceBridge-Windows.zip"
        self.assertEqual(package(self.exe, other)["zipSha256"], result["zipSha256"])
        self.assertEqual(verify_pair(self.exe, self.zip)["exeSha256"],
                         result["exeSha256"])

    def test_refuses_to_overwrite_existing_candidate(self):
        self.zip.write_bytes(b"keep me")
        with self.assertRaises(FileExistsError):
            package(self.exe, self.zip)
        self.assertEqual(self.zip.read_bytes(), b"keep me")

    def test_rejects_invalid_embedded_engine_before_creating_zip(self):
        self.exe.write_bytes(b"MZinvalid app")
        with self.assertRaises(ValueError):
            package(self.exe, self.zip)
        self.assertFalse(self.zip.exists())

    def test_rejects_legacy_zip_with_different_binary(self):
        with zipfile.ZipFile(self.zip, "w") as archive:
            archive.writestr("SliceBridge.exe", self.exe.read_bytes()[:-1] + b"x")
        with self.assertRaisesRegex(ValueError, "differs"):
            verify_pair(self.exe, self.zip)

    def test_rejects_legacy_zip_with_extra_entries(self):
        with zipfile.ZipFile(self.zip, "w") as archive:
            archive.writestr("SliceBridge.exe", self.exe.read_bytes())
            archive.writestr("other.txt", "not a single executable")
        with self.assertRaisesRegex(ValueError, "exactly one"):
            verify_pair(self.exe, self.zip)

    def test_enforces_embedded_exact_commit_when_supplied(self):
        commit = "a" * 40
        with patch("package_legacy_windows_zip.binary_metadata",
                   return_value={"commit": "b" * 40}):
            with self.assertRaisesRegex(ValueError, "commit does not match"):
                package(self.exe, self.zip, commit)
        self.assertFalse(self.zip.exists())
        with patch("package_legacy_windows_zip.binary_metadata",
                   return_value={"commit": commit}):
            result = package(self.exe, self.zip, commit)
            self.assertEqual(result["sourceCommit"], commit)

    def test_verify_only_cli_never_writes(self):
        package(self.exe, self.zip)
        before = hashlib.sha256(self.zip.read_bytes()).digest()
        process = subprocess.run([
            sys.executable, str(ROOT / "tools" / "package_legacy_windows_zip.py"),
            "--exe", str(self.exe), "--zip", str(self.zip), "--verify-only"
        ], capture_output=True, text=True)
        self.assertEqual(process.returncode, 0, process.stderr)
        self.assertIn('"legacyUpdaterCompatible": true', process.stdout)
        self.assertEqual(before, hashlib.sha256(self.zip.read_bytes()).digest())


if __name__ == "__main__":
    unittest.main()
