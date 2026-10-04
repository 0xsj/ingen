from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import os
import tarfile
import tempfile
import unittest
import gzip
from unittest import mock
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("ingen_package.py")
SPEC = importlib.util.spec_from_file_location("ingen_package", MODULE_PATH)
assert SPEC and SPEC.loader
package = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(package)


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def add_tar_file(archive: tarfile.TarFile, name: str, data: bytes) -> None:
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.mode = 0o644
    info.mtime = 0
    archive.addfile(info, io.BytesIO(data))


class BundleFixture:
    def __init__(self, root: Path):
        self.root = root
        root.mkdir(parents=True, exist_ok=True)
        self.bundle = root / "bundle"
        self.bundle.mkdir()
        self.entries = []
        for name, module, kind, _ in package.CLI_COMMANDS:
            self.add("bin/" + name, ("#!/bin/sh\n# " + name + "\n").encode(), module, "bridge" if kind == "go-bridge" else "cli", 0o755)
        self.add("sdk/amber-go/go.mod", b"module example\n", "amber", "amber-go-source")
        self.add("sdk/amber-go/go.sum", b"", "amber", "amber-go-source")
        self.add("sdk/amber-go/README.md", b"Amber Go SDK\n", "amber", "amber-go-source")
        self.add("sdk/amber-go/LICENSE", b"MIT\n", "amber", "amber-go-source")
        self.add("sdk/amber-go/amber.go", b"package amber\n", "amber", "amber-go-source")
        package_json = json.dumps({"name": "@0xsj/amber", "version": "1.2.3"}).encode()
        tar_path = self.bundle / "sdk" / "amber-typescript" / "amber-1.2.3.tgz"
        tar_path.parent.mkdir(parents=True)
        with tarfile.open(tar_path, "w:gz") as archive:
            for name, data in (
                ("package/package.json", package_json),
                ("package/README.md", b"Amber TS SDK\n"),
                ("package/LICENSE", b"MIT\n"),
                ("package/dist/index.js", b"export {};\n"),
                ("package/dist/index.d.ts", b"export {};\n"),
            ):
                add_tar_file(archive, name, data)
        self.entries.append({"path": "sdk/amber-typescript/amber-1.2.3.tgz", "module": "amber", "kind": "amber-typescript-package", "size": tar_path.stat().st_size, "sha256": package.sha256_file(tar_path), "mode": "0644"})
        self.add("share/ingen-package.py", MODULE_PATH.read_bytes(), "distribution", "verifier")
        self.add("LICENSE", b"InGen MIT license\n", "ecosystem", "license")
        self.add("share/licenses/README.md", b"Third party notices\n", "ecosystem", "notice-readme")
        for rel, data in (
            ("share/licenses/Go-LICENSE", b"Go BSD license\n"),
            ("share/licenses/yaml.v3-LICENSE", b"YAML MIT license\n"),
            ("share/licenses/yaml.v3-NOTICE", b"YAML Apache notice\n"),
            ("share/licenses/rust/COPYRIGHT-library.html", b"<html>Rust licenses</html>\n"),
            ("share/licenses/rust/licenses/MIT.txt", b"Rust MIT license\n"),
        ):
            self.add(rel, data, "ecosystem", "license")
        self.write_manifest()

    def add(self, relative: str, data: bytes, module: str, kind: str, mode: int = 0o644) -> None:
        path = self.bundle / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        path.chmod(mode)
        self.entries.append({"path": relative, "module": module, "kind": kind, "size": len(data), "sha256": sha(data), "mode": f"{mode:04o}"})

    def manifest(self) -> dict:
        source_rows = [{"path": "core/version.go", "size": 1, "sha256": "b" * 64}]
        source_payload = {"algorithm": package.SOURCE_INPUTS_SCHEMA, "files": source_rows}
        toolchains = {"go": "go version go1.27.1 darwin/arm64", "go_env": {"goos": "darwin", "goarch": "arm64", "version": "go1.27.1"}, "rustc": "rustc 1.90.0", "cargo": "cargo 1.90.0", "node": "v22.0.0", "npm": "10.0.0", "typescript": "5.0.0"}
        release = "0.0.0-test"
        source = {"revision": "a" * 40, "dirty": True, "build_date": "2026-10-04T00:00:00Z"}
        target = {"goos": "darwin", "goarch": "arm64", "rust_target": "aarch64-apple-darwin"}
        version_probes = []
        for name, module, _, _ in package.CLI_COMMANDS:
            version_probes.append({"executable": name, "report": package._version_report(name, version=release, revision=source["revision"], build_date=source["build_date"], source_inputs_sha256=package.sha256_bytes(package.canonical_json(source_payload)), dirty="dirty", goos=target["goos"], goarch=target["goarch"], toolchain=toolchains["rustc"] if module == "malcolm" else toolchains["go_env"]["version"])})
        licenses = []
        for entry in self.entries:
            if entry["kind"] == "license":
                licenses.append({"path": entry["path"], "component": entry["path"], "license": "MIT", "size": entry["size"], "sha256": entry["sha256"]})
        return {
            "schema": package.SCHEMA,
            "release_status": "local-review-only",
            "license_status": "MIT; selected dependency and compiler notices are included; this is not a complete legal review",
            "release": {"version": release},
            "source": source,
            "source_inputs": {**source_payload, "sha256": package.sha256_bytes(package.canonical_json(source_payload))},
            "target": target,
            "toolchains": toolchains,
            "reproducibility": {"go": ["-trimpath"], "rust": ["--locked", "--offline"], "typescript": ["npm pack --ignore-scripts"], "native_target_only": True},
            "modules": package._expected_modules(),
            "files": sorted(self.entries, key=lambda entry: entry["path"]),
            "licenses": licenses,
            "distribution_tool": "share/ingen-package.py",
            "help_probes": [{"name": name, "exit_code": 0} for name, _, _, _ in package.CLI_COMMANDS],
            "version_probes": sorted(version_probes, key=lambda item: item["executable"]),
        }

    def write_manifest(self) -> str:
        data = package.canonical_json(self.manifest())
        (self.bundle / "manifest.json").write_bytes(data)
        digest = sha(data)
        (self.bundle / "manifest.sha256").write_text(digest + "\n", encoding="ascii")
        return digest


class PackageTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve(strict=True)
        self.fixture = BundleFixture(self.root)
        self.digest = self.fixture.write_manifest()

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_valid_bundle_verifies_and_installs_exact_bytes(self) -> None:
        result = package.verify_bundle(str(self.fixture.bundle), self.digest, "darwin/arm64")
        self.assertEqual(len(result["modules"]), 9)
        prefix = self.root / "user-tools"
        package.install_bundle(str(self.fixture.bundle), str(prefix), self.digest, "darwin/arm64")
        installed = package.verify_bundle(str(prefix), self.digest, "darwin/arm64")
        self.assertEqual(installed["files"], result["files"])
        self.assertEqual((prefix / "bin" / "sorna-malcolm").read_bytes(), (self.fixture.bundle / "bin" / "sorna-malcolm").read_bytes())

    def test_artifact_tampering_and_wrong_manifest_digest_fail(self) -> None:
        binary = self.fixture.bundle / "bin" / "sorna"
        binary.write_bytes(binary.read_bytes() + b"tampered")
        with self.assertRaisesRegex(package.PackageError, "artifact bytes"):
            package.verify_bundle(str(self.fixture.bundle), self.digest)
        with self.assertRaisesRegex(package.PackageError, "selected digest"):
            package.verify_bundle(str(self.fixture.bundle), "0" * 64)

    def test_traversal_and_wrong_target_fail_even_with_rehashed_manifest(self) -> None:
        manifest = self.fixture.manifest()
        manifest["files"][0]["path"] = "../outside"
        data = package.canonical_json(manifest)
        digest = sha(data)
        (self.fixture.bundle / "manifest.json").write_bytes(data)
        (self.fixture.bundle / "manifest.sha256").write_text(digest + "\n")
        with self.assertRaisesRegex(package.PackageError, "escaping artifact path"):
            package.verify_bundle(str(self.fixture.bundle), digest)
        self.fixture.write_manifest()
        self.fixture = BundleFixture(self.root / "fresh")
        with self.assertRaisesRegex(package.PackageError, "does not match requested target"):
            package.verify_bundle(str(self.fixture.bundle), self.fixture.write_manifest(), "linux/arm64")

    def test_duplicate_json_keys_and_symlinks_are_rejected(self) -> None:
        manifest_path = self.fixture.bundle / "manifest.json"
        manifest_path.write_bytes(b'{"schema":"x","schema":"y"}\n')
        digest = sha(manifest_path.read_bytes())
        (self.fixture.bundle / "manifest.sha256").write_text(digest + "\n")
        with self.assertRaisesRegex(package.PackageError, "duplicate object keys"):
            package.verify_bundle(str(self.fixture.bundle), digest)
        self.fixture.write_manifest()
        link = self.fixture.bundle / "bin" / "aliased"
        link.symlink_to(self.fixture.bundle / "bin" / "sorna")
        with self.assertRaisesRegex(package.PackageError, "symlink"):
            package.verify_bundle(str(self.fixture.bundle), self.fixture.write_manifest())

    def test_install_refuses_nonempty_prefix_without_replacing_it(self) -> None:
        prefix = self.root / "existing"
        prefix.mkdir()
        sentinel = prefix / "keep.txt"
        sentinel.write_text("keep", encoding="utf-8")
        with self.assertRaisesRegex(package.PackageError, "empty directory"):
            package.install_bundle(str(self.fixture.bundle), str(prefix), self.digest)
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "keep")

    def test_manifest_changed_during_install_fails_and_removes_partial_prefix(self) -> None:
        prefix = self.root / "raced-install"
        original_copy = package._copy_exact

        def copy_then_race(source: Path, destination: Path, entry: dict | None = None) -> None:
            original_copy(source, destination, entry)
            if destination.name == "manifest.json":
                destination.write_bytes(destination.read_bytes() + b" ")

        with mock.patch.object(package, "_copy_exact", side_effect=copy_then_race):
            with self.assertRaisesRegex(package.PackageError, "manifest bytes"):
                package.install_bundle(str(self.fixture.bundle), str(prefix), self.digest)
        self.assertFalse(prefix.exists())

    def test_archive_verifies_and_installs_from_same_snapshot(self) -> None:
        archive_path = self.root / "toolchain.tar.gz"
        archive, archive_digest = package.export_archive(
            str(self.fixture.bundle), str(archive_path), self.digest, "darwin/arm64"
        )
        self.assertEqual(archive, archive_path)
        self.assertRegex(archive_digest, r"^[0-9a-f]{64}$")
        verified = package.verify_archive(str(archive), self.digest, "darwin/arm64")
        self.assertEqual(verified["modules"], package._expected_modules())
        prefix = self.root / "archive-install"
        package.install_archive(str(archive), str(prefix), self.digest, "darwin/arm64")
        installed = package.verify_bundle(str(prefix), self.digest, "darwin/arm64")
        self.assertEqual(installed["files"], verified["files"])

    def test_archive_rejects_traversal_and_tampering(self) -> None:
        traversal = self.root / "traversal.tar.gz"
        with tarfile.open(traversal, "w:gz") as archive:
            info = tarfile.TarInfo("../outside")
            info.size = 1
            info.mode = 0o644
            archive.addfile(info, io.BytesIO(b"x"))
        with self.assertRaisesRegex(package.PackageError, "escaping artifact path"):
            package.verify_archive(str(traversal), self.digest)

        valid = self.root / "valid.tar.gz"
        package.export_archive(str(self.fixture.bundle), str(valid), self.digest)
        changed = bytearray(valid.read_bytes())
        changed[len(changed) // 2] ^= 0x20
        valid.write_bytes(changed)
        with self.assertRaises(package.PackageError):
            package.verify_archive(str(valid), self.digest)

    def test_archive_drains_tar_end_and_checks_gzip_crc(self) -> None:
        clean = self.root / "clean.tar.gz"
        package.export_archive(str(self.fixture.bundle), str(clean), self.digest)
        tar_bytes = gzip.decompress(clean.read_bytes())

        trailing = self.root / "trailing.tar.gz"
        trailing.write_bytes(gzip.compress(tar_bytes + b"unlisted trailing payload", mtime=0))
        with self.assertRaisesRegex(package.PackageError, "after tar end markers"):
            package.verify_archive(str(trailing), self.digest)

        bad_crc = bytearray(clean.read_bytes())
        bad_crc[-8] ^= 1
        crc_path = self.root / "bad-crc.tar.gz"
        crc_path.write_bytes(bad_crc)
        with self.assertRaises(package.PackageError):
            package.verify_archive(str(crc_path), self.digest)

    def test_nested_typescript_archive_drains_tar_end_and_checks_crc(self) -> None:
        nested = self.root / "nested.tgz"
        with tarfile.open(nested, "w:gz") as archive:
            for name, data in (
                ("package/package.json", b'{"name":"@0xsj/amber","version":"1.2.3"}'),
                ("package/README.md", b"readme"),
                ("package/LICENSE", b"license"),
                ("package/dist/index.js", b"export {};"),
            ):
                add_tar_file(archive, name, data)
        tar_bytes = gzip.decompress(nested.read_bytes())
        nested.write_bytes(gzip.compress(tar_bytes + b"unlisted trailing payload", mtime=0))
        with self.assertRaisesRegex(package.PackageError, "after tar end markers"):
            package._verify_typescript_archive(nested)
        valid_bytes = gzip.compress(tar_bytes, mtime=0)
        bad_crc = bytearray(valid_bytes)
        bad_crc[-8] ^= 1
        nested.write_bytes(bad_crc)
        with self.assertRaises(package.PackageError):
            package._verify_typescript_archive(nested)

    def test_archive_bounds_chained_pax_headers(self) -> None:
        archive_path = self.root / "pax-chain.tar.gz"
        with tarfile.open(archive_path, "w:gz", format=tarfile.PAX_FORMAT) as archive:
            for _ in range(4):
                info = tarfile.TarInfo("pax")
                info.type = tarfile.XHDTYPE
                info.size = 0
                archive.addfile(info, io.BytesIO(b""))
            add_tar_file(archive, "manifest.json", b"{}\n")
        with mock.patch.object(package, "MAX_TAR_EXTENSION_HEADERS", 2):
            with self.assertRaisesRegex(package.PackageError, "chained extension headers"):
                package.verify_archive(str(archive_path), self.digest)

    def test_archive_rejects_links_special_files_and_privileged_modes(self) -> None:
        for kind, mode, reason in (
            (tarfile.SYMTYPE, 0o644, "link or special file"),
            (tarfile.LNKTYPE, 0o644, "link or special file"),
            (tarfile.FIFOTYPE, 0o644, "link or special file"),
            (tarfile.GNUTYPE_SPARSE, 0o644, "sparse"),
            (tarfile.REGTYPE, 0o4644, "file metadata"),
        ):
            with self.subTest(kind=kind, mode=mode):
                path = self.root / "unsafe.tar.gz"
                with tarfile.open(path, "w:gz") as archive:
                    info = tarfile.TarInfo("bin/sentinel")
                    info.type, info.mode = kind, mode
                    info.linkname = "../../outside"
                    archive.addfile(info)
                with self.assertRaisesRegex(package.PackageError, reason):
                    package.verify_archive(str(path), self.digest)

    def test_archive_rejects_duplicate_paths(self) -> None:
        path = self.root / "duplicates.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            add_tar_file(archive, "bin/sentinel", b"first")
            add_tar_file(archive, "bin/sentinel", b"second")
        with self.assertRaisesRegex(package.PackageError, "duplicate paths"):
            package.verify_archive(str(path), self.digest)

    def test_archive_rejects_pax_sparse_before_reading_maps(self) -> None:
        for headers in (
            {"GNU.sparse.map": "0,1"},
            {"GNU.sparse.size": "1"},
            {"GNU.sparse.major": "1", "GNU.sparse.minor": "0"},
        ):
            with self.subTest(headers=headers):
                path = self.root / "sparse-pax.tar.gz"
                with tarfile.open(path, "w:gz", format=tarfile.PAX_FORMAT) as archive:
                    info = tarfile.TarInfo("bin/sentinel")
                    info.size, info.mode, info.pax_headers = 1, 0o644, headers
                    archive.addfile(info, io.BytesIO(b"x"))
                with self.assertRaisesRegex(package.PackageError, "sparse"):
                    package.verify_archive(str(path), self.digest)

    def test_archive_bounds_extension_payload_before_parsing(self) -> None:
        path = self.root / "oversized-header.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            info = tarfile.TarInfo("extension")
            info.type, info.size = tarfile.GNUTYPE_LONGNAME, 64
            archive.addfile(info, io.BytesIO(b"x" * 64))
        with mock.patch.object(package, "MAX_TAR_EXTENSION_BYTES", 32):
            with self.assertRaisesRegex(package.PackageError, "extension header exceeds"):
                package.verify_archive(str(path), self.digest)

    def test_nested_typescript_archive_has_expansion_bound(self) -> None:
        path = self.root / "nested-bomb.tgz"
        with tarfile.open(path, "w:gz") as archive:
            add_tar_file(archive, "package/dist/index.js", b"x" * 32768)
        with mock.patch.object(package, "MAX_EXPANDED_BYTES", 16384):
            with self.assertRaisesRegex(package.PackageError, "expanded size limit"):
                package._verify_typescript_archive(path)

    def test_archive_publish_race_preserves_unowned_output(self) -> None:
        output = self.root / "raced.tar.gz"
        original_link = os.link

        def create_competing_output(source: Path, destination: Path, *args: object, **kwargs: object) -> None:
            Path(destination).write_bytes(b"unowned concurrent output")
            raise FileExistsError(str(destination))

        with mock.patch.object(package.os, "link", side_effect=create_competing_output):
            with self.assertRaises(FileExistsError):
                package.export_archive(str(self.fixture.bundle), str(output), self.digest)
        self.assertEqual(output.read_bytes(), b"unowned concurrent output")
        self.assertTrue(callable(original_link))

    def test_source_snapshot_rejects_input_edit(self) -> None:
        checkout = self.root / "source"
        for relative, contents in (
            ("LICENSE", b"root license"),
            ("amber/LICENSE", b"Amber license"),
            ("amber/go/README.md", b"Amber Go"),
            ("go.mod", b"module example\n"),
            ("go.sum", b""),
            ("core/version.go", b"package core\n"),
            ("packaging/ingen_package.py", MODULE_PATH.read_bytes()),
            ("packaging/licenses/README.md", b"notices"),
            ("packaging/licenses/Go-LICENSE", b"Go"),
            ("packaging/licenses/yaml.v3-LICENSE", b"MIT"),
            ("packaging/licenses/yaml.v3-NOTICE", b"Apache"),
        ):
            path = checkout / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(contents)
        rows, _ = package.build_input_inventory(checkout)
        source = checkout / "core/version.go"
        source.write_bytes(b"package core // changed after fingerprint\n")
        with self.assertRaisesRegex(package.PackageError, "changed while copying"):
            package.copy_build_snapshot(checkout, self.root / "snapshot", rows)


if __name__ == "__main__":
    unittest.main()
