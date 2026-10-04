#!/usr/bin/env python3
"""Build, verify, and install a local InGen toolchain bundle.

This command performs local builds only. It does not publish artifacts, read
credentials, install dependencies, or modify system paths.
"""

from __future__ import annotations

import argparse
import datetime as dt
import fnmatch
import gzip
import hashlib
import io
import json
import os
import platform
import re
import shutil
import shlex
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any


SCHEMA = "ingen.local-toolchain-bundle/v2"
SOURCE_INPUTS_SCHEMA = "ingen-build-inputs-sha256/v1"
TOOL_VERSION_SCHEMA = "ingen.tool-version/v1"
DEFAULT_RELEASE_VERSION = "dev"
CLI_COMMANDS = (
    ("malcolm", "malcolm", "rust-cli", None),
    ("sorna", "sorna", "go-cli", "./sorna/cmd/sorna"),
    ("sentinel", "sentinel", "go-cli", "./herdr-sentinel/cmd/sentinel"),
    ("hammond", "hammond", "go-cli", "./hammond/cmd/hammond"),
    ("paddock", "paddock", "go-cli", "./paddock/cmd/paddock"),
    ("lockwood", "lockwood", "go-cli", "./lockwood/cmd/lockwood"),
    ("nublar", "nublar", "go-cli", "./nublar/cmd/nublar"),
    ("sattler", "sattler", "go-cli", "./sattler/cmd/sattler"),
    ("sorna-malcolm", "sorna", "go-bridge", "./sorna/cmd/sorna-malcolm"),
)
MODULE_ARTIFACTS = {
    "malcolm": ["bin/malcolm"],
    "sorna": ["bin/sorna", "bin/sorna-malcolm"],
    "sentinel": ["bin/sentinel"],
    "hammond": ["bin/hammond"],
    "paddock": ["bin/paddock"],
    "lockwood": ["bin/lockwood"],
    "nublar": ["bin/nublar"],
    "sattler": ["bin/sattler"],
    "amber": ["sdk/amber-go", "sdk/amber-typescript"],
}
MODULE_ORDER = ("malcolm", "sorna", "sentinel", "hammond", "paddock", "lockwood", "nublar", "sattler", "amber")
HEX_SHA256 = re.compile(r"^[0-9a-f]{64}$")
SAFE_RELEASE_VERSION = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$")
ALLOWED_MODES = {"0644", "0755"}
SYSTEM_PREFIXES = tuple(Path(path) for path in ("/bin", "/sbin", "/usr", "/opt", "/System", "/Library", "/Applications"))
GO_SOURCE_ROOTS = ("core", "sorna", "herdr-sentinel", "hammond", "paddock", "lockwood", "nublar", "sattler", "amber/go")
SOURCE_EXCLUDED_DIRS = {".git", ".cache", "node_modules", "target", "dist", ".artifacts", ".ingen", ".venv", "__pycache__"}
PRIVATE_BASENAMES = {".env", ".npmrc", ".pypirc", "id_rsa", "id_ed25519", "credentials", "secrets.json"}
PRIVATE_SUFFIXES = {".pem", ".key", ".p12", ".pfx", ".kubeconfig"}
MAX_ARCHIVE_BYTES = 256 * 1024 * 1024
MAX_EXPANDED_BYTES = 2 * 1024 * 1024 * 1024
MAX_ARCHIVE_MEMBERS = 100_000
MAX_TAR_EXTENSION_HEADERS = 32
MAX_TAR_EXTENSION_BYTES = 1024 * 1024


class PackageError(Exception):
    """An invalid bundle, path, or build operation."""


def _bounded_tarinfo_type() -> type[tarfile.TarInfo]:
    """Create a parser class with bounded extension-header recursion."""
    extension_types = {
        tarfile.GNUTYPE_LONGNAME,
        tarfile.GNUTYPE_LONGLINK,
        tarfile.XHDTYPE,
        tarfile.XGLTYPE,
    }
    if hasattr(tarfile, "SOLARIS_XHDTYPE"):
        extension_types.add(tarfile.SOLARIS_XHDTYPE)

    class BoundedTarInfo(tarfile.TarInfo):
        extension_count = 0

        def _proc_member(self, archive: tarfile.TarFile) -> tarfile.TarInfo:
            if self.type == tarfile.GNUTYPE_SPARSE:
                raise PackageError("sparse archive entries are unsupported")
            if self.type in extension_types:
                type(self).extension_count += 1
                if type(self).extension_count > MAX_TAR_EXTENSION_HEADERS:
                    raise PackageError("archive has too many chained extension headers")
                if self.size < 0 or self.size > MAX_TAR_EXTENSION_BYTES:
                    raise PackageError("archive extension header exceeds its size limit")
            member = super()._proc_member(archive)
            if member is not None and member.sparse is not None:
                raise PackageError("sparse archive entries are unsupported")
            return member

        # PAX sparse processing runs before a member is returned. Refuse every
        # GNU sparse variant before the standard library reads sparse maps.
        def _proc_gnusparse_00(self, *args: Any) -> None:
            raise PackageError("sparse archive entries are unsupported")

        def _proc_gnusparse_01(self, *args: Any) -> None:
            raise PackageError("sparse archive entries are unsupported")

        def _proc_gnusparse_10(self, *args: Any) -> None:
            raise PackageError("sparse archive entries are unsupported")

    return BoundedTarInfo


def canonical_json(value: Any) -> bytes:
    return (json.dumps(value, sort_keys=True, indent=2, ensure_ascii=False) + "\n").encode("utf-8")


def _unique_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise PackageError("JSON contains duplicate object keys")
        result[key] = value
    return result


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def run(argv: list[str], *, cwd: Path, env: dict[str, str] | None = None, capture: bool = True, timeout: int = 600) -> subprocess.CompletedProcess[str]:
    try:
        result = subprocess.run(
            argv,
            cwd=cwd,
            env=env,
            check=False,
            text=True,
            stdout=subprocess.PIPE if capture else None,
            stderr=subprocess.PIPE if capture else None,
            timeout=timeout,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise PackageError(f"command failed to start or timed out: {argv[0]}") from error
    if result.returncode != 0:
        detail = (result.stderr or "").strip().splitlines()
        suffix = f": {detail[-1][:300]}" if detail else ""
        raise PackageError(f"command exited {result.returncode}: {argv[0]}{suffix}")
    return result


def _safe_absolute(path_text: str, *, label: str) -> Path:
    path = Path(path_text)
    if not path.is_absolute() or str(path) != os.path.normpath(path_text):
        raise PackageError(f"{label} must be an absolute normalized path")
    if any(path == prefix or prefix in path.parents for prefix in SYSTEM_PREFIXES):
        raise PackageError(f"{label} cannot be inside a system path")
    parent = path.parent.resolve(strict=True)
    normalized = parent / path.name
    if normalized != path:
        raise PackageError(f"{label} parent must not contain symlink aliases")
    return path


def _reject_overlap(path: Path, root: Path, *, label: str) -> None:
    resolved_root = root.resolve(strict=True)
    try:
        path.relative_to(resolved_root)
        inside = True
    except ValueError:
        inside = False
    try:
        resolved_root.relative_to(path)
        contains = True
    except ValueError:
        contains = False
    if inside or contains:
        raise PackageError(f"{label} must not overlap the source checkout")


def _target_parts(target: str) -> tuple[str, str]:
    pieces = target.split("/")
    if len(pieces) != 2 or not all(pieces) or any(not re.fullmatch(r"[A-Za-z0-9_.-]+", item) for item in pieces):
        raise PackageError("target must use the form GOOS/GOARCH")
    return pieces[0], pieces[1]


def _safe_relative(relative: str) -> PurePosixPath:
    if not isinstance(relative, str) or not relative or "\\" in relative or "\x00" in relative:
        raise PackageError("manifest contains an invalid artifact path")
    path = PurePosixPath(relative)
    if path.is_absolute() or str(path) != relative or any(part in ("", ".", "..") for part in path.parts):
        raise PackageError("manifest contains a non-normalized or escaping artifact path")
    return path


def _relative_file(bundle: Path, relative: str) -> Path:
    rel = _safe_relative(relative)
    path = bundle.joinpath(*rel.parts)
    cursor = bundle
    for part in rel.parts:
        cursor = cursor / part
        try:
            info = cursor.lstat()
        except FileNotFoundError as error:
            raise PackageError(f"bundle file is missing: {relative}") from error
        if stat.S_ISLNK(info.st_mode):
            raise PackageError(f"bundle path contains a symlink: {relative}")
    if not path.is_file():
        raise PackageError(f"bundle artifact is not a regular file: {relative}")
    return path


def _expected_modules() -> list[dict[str, Any]]:
    return [{"name": module, "artifacts": MODULE_ARTIFACTS[module]} for module in MODULE_ORDER]


def _is_private_build_path(path: Path) -> bool:
    name = path.name.lower()
    return name in PRIVATE_BASENAMES or name.startswith(".env.") or path.suffix.lower() in PRIVATE_SUFFIXES


def _source_file_allowed(path: Path, *, go_tree: bool = False, rust_tree: bool = False, ts_tree: bool = False) -> bool:
    name = path.name
    suffix = path.suffix.lower()
    if go_tree:
        return suffix in {".go", ".s", ".c", ".h", ".syso", ".sql"} or name in {"go.mod", "go.sum"}
    if rust_tree:
        return suffix in {".rs", ".toml", ".lock"}
    if ts_tree:
        return suffix in {".ts", ".json"} or name in {"README.md", "LICENSE"}
    return False


def _walk_source(root: Path, *, go_tree: bool = False, rust_tree: bool = False, ts_tree: bool = False) -> list[Path]:
    found: list[Path] = []
    if not root.exists():
        return found
    if root.is_symlink():
        raise PackageError(f"build input root is a symlink: {root.name}")
    for path in root.rglob("*"):
        relative = path.relative_to(root)
        if any(part in SOURCE_EXCLUDED_DIRS for part in relative.parts):
            continue
        if path.is_symlink():
            raise PackageError(f"build input contains a symlink: {path}")
        if not path.is_file() or _is_private_build_path(path):
            continue
        if _source_file_allowed(path, go_tree=go_tree, rust_tree=rust_tree, ts_tree=ts_tree):
            found.append(path)
    return found


def _embedded_paths(checkout: Path, source_files: list[Path], roots: list[Path], language: str) -> list[Path]:
    directives = re.compile(r"^\s*//go:embed\s+(.+?)\s*$") if language == "go" else re.compile(r"\binclude(?:_str|_bytes)?!\s*\(\s*([\"'])(.*?)\1\s*\)")
    found: set[Path] = set()
    root_resolved = [root.resolve(strict=True) for root in roots if root.exists()]
    for source in source_files:
        try:
            text = source.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        for line in text.splitlines():
            if language == "go":
                match = directives.match(line)
                if not match:
                    continue
                try:
                    patterns = shlex.split(match.group(1), posix=True)
                except ValueError as error:
                    raise PackageError(f"unsupported go:embed directive in {source}") from error
                if not patterns:
                    raise PackageError(f"empty go:embed directive in {source}")
            else:
                patterns = [match.group(2) for match in directives.finditer(line)]
            for pattern in patterns:
                if not pattern or "\\" in pattern or "\x00" in pattern:
                    raise PackageError(f"unsupported embedded input pattern in {source}")
                allow_hidden = pattern.startswith("all:")
                raw = pattern[4:] if allow_hidden else pattern
                if not raw or Path(raw).is_absolute() or ".." in PurePosixPath(raw).parts:
                    raise PackageError(f"embedded input escapes its module in {source}")
                matches = sorted(source.parent.glob(raw))
                if not matches:
                    raise PackageError(f"embedded input pattern has no matches in {source}: {raw}")
                for match in matches:
                    if match.is_symlink():
                        raise PackageError(f"embedded input is a symlink: {match}")
                    candidates = [match] if match.is_file() else sorted(match.rglob("*"))
                    for candidate in candidates:
                        if not candidate.is_file() or candidate.is_symlink():
                            continue
                        if any(part in SOURCE_EXCLUDED_DIRS or part == ".git" for part in candidate.relative_to(source.parent).parts):
                            continue
                        if not allow_hidden and any(part.startswith((".", "_")) for part in candidate.relative_to(source.parent).parts):
                            continue
                        if _is_private_build_path(candidate):
                            raise PackageError(f"embedded input matches private file: {candidate}")
                        resolved = candidate.resolve(strict=True)
                        if not any(resolved == root or root in resolved.parents for root in root_resolved):
                            raise PackageError(f"embedded input escapes selected build roots: {candidate}")
                        found.add(resolved)
    return sorted(found)


def build_input_files(checkout: Path) -> list[Path]:
    checkout = checkout.resolve(strict=True)
    files: set[Path] = set()
    go_sources: list[Path] = []
    selected_roots: list[Path] = []
    for item in GO_SOURCE_ROOTS:
        root = checkout / item
        selected_roots.append(root)
        current = _walk_source(root, go_tree=True)
        files.update(current)
        go_sources.extend(path for path in current if path.suffix == ".go")
    for item in ("malcolm",):
        root = checkout / item
        selected_roots.append(root)
        current = _walk_source(root, rust_tree=True)
        files.update(current)
    tsroot = checkout / "amber" / "typescript"
    selected_roots.append(tsroot)
    for rel in ("package.json", "package-lock.json", "tsconfig.json", "README.md", "LICENSE"):
        path = tsroot / rel
        if path.is_file() and not _is_private_build_path(path):
            files.add(path.resolve(strict=True))
    files.update(_walk_source(tsroot / "src", ts_tree=True))
    for rel in ("go.mod", "go.sum", "LICENSE", "amber/LICENSE", "amber/go/README.md"):
        path = checkout / rel
        if path.is_file():
            files.add(path.resolve(strict=True))
    license_root = checkout / "packaging" / "licenses"
    if license_root.is_dir():
        for path in sorted(license_root.iterdir()):
            if path.is_symlink():
                raise PackageError(f"packaging license input is a symlink: {path.name}")
            if path.is_file() and (path.name == "README.md" or path.name.endswith(("LICENSE", "NOTICE")) or path.suffix == ".txt"):
                files.add(path.resolve(strict=True))
    else:
        raise PackageError("packaging license notice inputs are missing")
    packaging_script = checkout / "packaging" / "ingen_package.py"
    if not packaging_script.is_file():
        raise PackageError("packaging build script is missing from selected source inputs")
    packaging_script = packaging_script.resolve(strict=True)
    files.add(packaging_script)
    files.update(_embedded_paths(checkout, go_sources, selected_roots, "go"))
    rust_sources = [path for path in files if path.suffix == ".rs"]
    files.update(_embedded_paths(checkout, rust_sources, selected_roots, "rust"))
    return sorted(files, key=lambda path: path.relative_to(checkout).as_posix())


def build_input_inventory(checkout: Path) -> tuple[list[dict[str, Any]], str]:
    rows = []
    for path in build_input_files(checkout):
        relative = path.relative_to(checkout).as_posix()
        if _is_private_build_path(path):
            raise PackageError(f"private file entered declared build-input inventory: {relative}")
        rows.append({"path": relative, "size": path.stat().st_size, "sha256": sha256_file(path)})
    payload = {"algorithm": SOURCE_INPUTS_SCHEMA, "files": rows}
    return rows, sha256_bytes(canonical_json(payload))


def copy_build_snapshot(checkout: Path, snapshot: Path, rows: list[dict[str, Any]]) -> None:
    checkout = checkout.resolve(strict=True)
    snapshot.mkdir(mode=0o755, parents=True, exist_ok=False)
    for row in rows:
        relative = _safe_relative(row["path"])
        source = checkout.joinpath(*relative.parts)
        cursor = checkout
        for part in relative.parts:
            cursor = cursor / part
            try:
                info = cursor.lstat()
            except FileNotFoundError as error:
                raise PackageError(f"build input disappeared while snapshotting: {row['path']}") from error
            if stat.S_ISLNK(info.st_mode):
                raise PackageError(f"build input became a symlink while snapshotting: {row['path']}")
        if not stat.S_ISREG(source.stat().st_mode):
            raise PackageError(f"build input is not a regular file: {row['path']}")
        destination = snapshot.joinpath(*relative.parts)
        destination.parent.mkdir(parents=True, exist_ok=True)
        entry = {"path": row["path"], "sha256": row["sha256"], "mode": f"{stat.S_IMODE(source.stat().st_mode):04o}"}
        _copy_exact(source, destination, entry)
        if destination.stat().st_size != row["size"]:
            raise PackageError(f"build input changed size while snapshotting: {row['path']}")
    copied_rows, copied_sha = build_input_inventory(snapshot)
    if copied_rows != rows or copied_sha != sha256_bytes(canonical_json({"algorithm": SOURCE_INPUTS_SCHEMA, "files": rows})):
        raise PackageError("private build snapshot does not match the selected source-input inventory")


def _version_report(name: str, *, version: str, revision: str, build_date: str, source_inputs_sha256: str, dirty: str, goos: str, goarch: str, toolchain: str) -> dict[str, str]:
    return {
        "schema": TOOL_VERSION_SCHEMA,
        "name": name,
        "version": version,
        "revision": revision,
        "commit": revision,
        "build_date": build_date,
        "source_inputs_sha256": source_inputs_sha256,
        "dirty": dirty,
        "goos": goos,
        "goarch": goarch,
        "toolchain": toolchain,
    }


def _validate_manifest(manifest: Any) -> list[dict[str, Any]]:
    expected_top = {"schema", "release_status", "license_status", "release", "source", "source_inputs", "target", "toolchains", "reproducibility", "modules", "files", "licenses", "distribution_tool", "help_probes", "version_probes"}
    if not isinstance(manifest, dict) or set(manifest) != expected_top or manifest.get("schema") != SCHEMA:
        raise PackageError("manifest schema or top-level fields are invalid")
    if manifest.get("release_status") != "local-review-only" or manifest.get("license_status") != "MIT; selected dependency and compiler notices are included; this is not a complete legal review":
        raise PackageError("manifest release and licensing limitations are missing")
    release = manifest.get("release")
    if not isinstance(release, dict) or set(release) != {"version"} or not isinstance(release.get("version"), str) or not SAFE_RELEASE_VERSION.fullmatch(release["version"]):
        raise PackageError("manifest release version is invalid")
    if manifest.get("modules") != _expected_modules():
        raise PackageError("manifest module inventory is incomplete or unexpected")
    target = manifest.get("target")
    if not isinstance(target, dict) or set(target) != {"goos", "goarch", "rust_target"}:
        raise PackageError("manifest target metadata is invalid")
    if not isinstance(target.get("goos"), str) or not isinstance(target.get("goarch"), str) or not isinstance(target.get("rust_target"), str):
        raise PackageError("manifest target metadata is incomplete")
    if (target["goos"], target["goarch"]) not in {("darwin", "arm64"), ("darwin", "amd64"), ("linux", "amd64"), ("linux", "arm64")}:
        raise PackageError("manifest target is not a supported native release target")
    if _rust_target(target["rust_target"]) != (target["goos"], target["goarch"]):
        raise PackageError("manifest Rust and Go targets do not match")
    source = manifest.get("source")
    if not isinstance(source, dict) or set(source) != {"revision", "dirty", "build_date"} or not isinstance(source.get("revision"), str) or not isinstance(source.get("dirty"), bool) or not isinstance(source.get("build_date"), str):
        raise PackageError("manifest source metadata is invalid")
    if source["revision"] != "unknown" and not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", source["revision"]):
        raise PackageError("manifest source revision is invalid")
    if source["build_date"] != "unknown" and not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", source["build_date"]):
        raise PackageError("manifest build date is invalid")
    source_inputs = manifest.get("source_inputs")
    if not isinstance(source_inputs, dict) or set(source_inputs) != {"algorithm", "files", "sha256"} or source_inputs.get("algorithm") != SOURCE_INPUTS_SCHEMA or not isinstance(source_inputs.get("files"), list) or not isinstance(source_inputs.get("sha256"), str) or not HEX_SHA256.fullmatch(source_inputs["sha256"]):
        raise PackageError("manifest source-input fingerprint is invalid")
    input_rows = source_inputs["files"]
    input_paths: list[str] = []
    for row in input_rows:
        if not isinstance(row, dict) or set(row) != {"path", "size", "sha256"}:
            raise PackageError("manifest source-input entry is invalid")
        relative = row.get("path")
        _safe_relative(relative)
        if any(part in SOURCE_EXCLUDED_DIRS for part in PurePosixPath(relative).parts) or _is_private_build_path(Path(relative)):
            raise PackageError("manifest source-input inventory includes excluded or private paths")
        if not isinstance(row.get("size"), int) or isinstance(row["size"], bool) or row["size"] < 0 or not isinstance(row.get("sha256"), str) or not HEX_SHA256.fullmatch(row["sha256"]):
            raise PackageError("manifest source-input entry metadata is invalid")
        input_paths.append(relative)
    if not input_rows or input_paths != sorted(set(input_paths)):
        raise PackageError("manifest source-input paths must be unique and sorted")
    expected_inputs_sha = sha256_bytes(canonical_json({"algorithm": SOURCE_INPUTS_SCHEMA, "files": input_rows}))
    if expected_inputs_sha != source_inputs["sha256"]:
        raise PackageError("manifest source-input fingerprint does not match its inventory")
    toolchains = manifest.get("toolchains")
    if not isinstance(toolchains, dict) or set(toolchains) != {"go", "go_env", "rustc", "cargo", "node", "npm", "typescript"}:
        raise PackageError("manifest toolchain metadata is invalid")
    if any(not isinstance(toolchains.get(key), str) or not toolchains[key] for key in ("go", "rustc", "cargo", "node", "npm", "typescript")):
        raise PackageError("manifest toolchain versions are incomplete")
    if not isinstance(toolchains["go_env"], dict) or set(toolchains["go_env"]) != {"goos", "goarch", "version"} or any(not isinstance(value, str) or not value for value in toolchains["go_env"].values()):
        raise PackageError("manifest Go target metadata is invalid")
    reproducibility = manifest.get("reproducibility")
    if not isinstance(reproducibility, dict) or set(reproducibility) != {"go", "rust", "typescript", "native_target_only"} or reproducibility["native_target_only"] is not True:
        raise PackageError("manifest reproducibility settings are invalid")
    if any(not isinstance(reproducibility[key], list) or any(not isinstance(value, str) for value in reproducibility[key]) for key in ("go", "rust", "typescript")):
        raise PackageError("manifest reproducibility settings are invalid")
    distribution_tool = manifest.get("distribution_tool")
    if distribution_tool != "share/ingen-package.py":
        raise PackageError("manifest standalone distribution tool path is invalid")
    files = manifest.get("files")
    if not isinstance(files, list) or not files:
        raise PackageError("manifest file inventory is empty")
    seen: set[str] = set()
    binary_modules = {"bin/" + name: (module, "bridge" if kind == "go-bridge" else "cli") for name, module, kind, _ in CLI_COMMANDS}
    type_script_archives: list[str] = []
    for entry in files:
        if not isinstance(entry, dict) or set(entry) != {"path", "module", "kind", "size", "sha256", "mode"}:
            raise PackageError("manifest contains an invalid file entry")
        relative = entry.get("path")
        _safe_relative(relative)
        if relative in seen:
            raise PackageError("manifest contains duplicate artifact paths")
        seen.add(relative)
        if not isinstance(entry.get("module"), str) or entry["module"] not in set(MODULE_ARTIFACTS) | {"distribution", "ecosystem"}:
            raise PackageError("manifest artifact has an unknown module")
        if entry.get("kind") not in {"cli", "bridge", "amber-go-source", "amber-typescript-package", "verifier", "license", "notice-readme"}:
            raise PackageError("manifest artifact has an unknown kind")
        if not isinstance(entry.get("size"), int) or isinstance(entry["size"], bool) or entry["size"] < 0:
            raise PackageError("manifest artifact size is invalid")
        if not isinstance(entry.get("sha256"), str) or not HEX_SHA256.fullmatch(entry["sha256"]):
            raise PackageError("manifest artifact digest is invalid")
        if entry.get("mode") not in ALLOWED_MODES:
            raise PackageError("manifest artifact mode is invalid")
        if relative in binary_modules:
            expected_module, expected_kind = binary_modules[relative]
            if entry["module"] != expected_module or entry["kind"] != expected_kind or entry["mode"] != "0755":
                raise PackageError("manifest command attribution or mode is invalid")
        elif relative.startswith("sdk/amber-go/"):
            suffix = Path(relative).suffix
            basename = Path(relative).name
            if entry["module"] != "amber" or entry["kind"] != "amber-go-source" or entry["mode"] != "0644" or (suffix not in {".go", ".sql"} and basename not in {"go.mod", "go.sum", "README.md", "LICENSE"}) or basename.endswith("_test.go"):
                raise PackageError("manifest contains a non-distributable Amber Go SDK file")
        elif relative.startswith("sdk/amber-typescript/") and relative.endswith(".tgz") and len(PurePosixPath(relative).parts) == 3:
            if entry["module"] != "amber" or entry["kind"] != "amber-typescript-package" or entry["mode"] != "0644":
                raise PackageError("manifest Amber TypeScript package attribution is invalid")
            type_script_archives.append(relative)
        elif relative == "share/ingen-package.py":
            if entry["module"] != "distribution" or entry["kind"] != "verifier" or entry["mode"] != "0644":
                raise PackageError("manifest standalone verifier attribution is invalid")
        elif relative == "share/licenses/README.md":
            if entry["module"] != "ecosystem" or entry["kind"] != "notice-readme" or entry["mode"] != "0644":
                raise PackageError("manifest notice README attribution is invalid")
        elif relative == "LICENSE" or relative.startswith("share/licenses/"):
            if entry["module"] != "ecosystem" or entry["kind"] != "license" or entry["mode"] != "0644":
                raise PackageError("manifest license file attribution is invalid")
        else:
            raise PackageError("manifest contains an unexpected artifact path")
    required = {"bin/" + name for name, _, _, _ in CLI_COMMANDS}
    if not required.issubset(seen):
        raise PackageError("manifest is missing one or more packaged commands")
    if files != sorted(files, key=lambda item: item["path"]):
        raise PackageError("manifest file inventory must be sorted by path")
    if len(type_script_archives) != 1:
        raise PackageError("manifest must contain exactly one Amber TypeScript package")
    go_sdk_required = {"sdk/amber-go/go.mod", "sdk/amber-go/go.sum", "sdk/amber-go/README.md", "sdk/amber-go/LICENSE"}
    if not go_sdk_required.issubset(seen) or not any(path.startswith("sdk/amber-go/") and path.endswith(".go") for path in seen):
        raise PackageError("manifest Amber Go SDK source package is incomplete")
    if "share/ingen-package.py" not in seen or "LICENSE" not in seen:
        raise PackageError("manifest is missing its standalone verifier or root license")
    license_entries = manifest.get("licenses")
    if not isinstance(license_entries, list) or not license_entries:
        raise PackageError("manifest license inventory is missing")
    file_by_path = {entry["path"]: entry for entry in files}
    license_paths: list[str] = []
    for item in license_entries:
        if not isinstance(item, dict) or set(item) != {"path", "component", "license", "size", "sha256"}:
            raise PackageError("manifest license inventory entry is invalid")
        rel = item.get("path")
        _safe_relative(rel)
        artifact = file_by_path.get(rel)
        if artifact is None or artifact["kind"] != "license" or item["size"] != artifact["size"] or item["sha256"] != artifact["sha256"]:
            raise PackageError("manifest license inventory does not bind its bundled bytes")
        if any(not isinstance(item.get(key), str) or not item[key] for key in ("component", "license")):
            raise PackageError("manifest license component metadata is invalid")
        license_paths.append(rel)
    required_licenses = {"LICENSE", "share/licenses/Go-LICENSE", "share/licenses/yaml.v3-LICENSE", "share/licenses/yaml.v3-NOTICE", "share/licenses/rust/COPYRIGHT-library.html"}
    if not required_licenses.issubset(set(license_paths)) or not any(path.startswith("share/licenses/rust/licenses/") and path.endswith(".txt") for path in license_paths):
        raise PackageError("manifest is missing selected runtime/compiler license notices")
    if set(license_paths) != {path for path, artifact in file_by_path.items() if artifact["kind"] == "license"}:
        raise PackageError("manifest license inventory and bundled license files differ")
    help_probes = manifest.get("help_probes")
    if not isinstance(help_probes, list) or len(help_probes) != len(CLI_COMMANDS):
        raise PackageError("manifest help probe inventory is incomplete")
    probe_names: list[str] = []
    for probe in help_probes:
        if not isinstance(probe, dict) or set(probe) != {"name", "exit_code"} or not isinstance(probe.get("name"), str) or isinstance(probe.get("exit_code"), bool) or probe["exit_code"] not in (0, 2):
            raise PackageError("manifest contains an invalid CLI help probe")
        probe_names.append(probe["name"])
    if len(set(probe_names)) != len(probe_names) or set(probe_names) != {name for name, _, _, _ in CLI_COMMANDS}:
        raise PackageError("manifest help probe inventory is incomplete")
    version_probes = manifest.get("version_probes")
    if not isinstance(version_probes, list) or len(version_probes) != len(CLI_COMMANDS):
        raise PackageError("manifest version probe inventory is incomplete")
    expected_reports: dict[str, dict[str, str]] = {}
    revision = source["revision"]
    dirty = "dirty" if source["dirty"] else "clean"
    for name, module, _, _ in CLI_COMMANDS:
        expected_reports[name] = _version_report(
            name,
            version=release["version"],
            revision=revision,
            build_date=source["build_date"],
            source_inputs_sha256=source_inputs["sha256"],
            dirty=dirty,
            goos=target["goos"],
            goarch=target["goarch"],
            toolchain=toolchains["rustc"] if module == "malcolm" else toolchains["go_env"]["version"],
        )
    probed_names: list[str] = []
    for probe in version_probes:
        if not isinstance(probe, dict) or set(probe) != {"executable", "report"} or not isinstance(probe.get("executable"), str):
            raise PackageError("manifest contains an invalid version probe")
        executable = probe["executable"]
        if executable not in expected_reports or probe.get("report") != expected_reports[executable]:
            raise PackageError("manifest version report does not match selected build metadata")
        probed_names.append(executable)
    if len(set(probed_names)) != len(probed_names) or set(probed_names) != set(expected_reports):
        raise PackageError("manifest version probe inventory is incomplete")
    return files


def verify_bundle(bundle_text: str, expected_manifest_sha256: str, target_text: str | None = None) -> dict[str, Any]:
    bundle = Path(bundle_text)
    if not bundle.is_absolute() or bundle.is_symlink() or not bundle.is_dir():
        raise PackageError("bundle must be an existing absolute directory without symlink aliases")
    bundle = bundle.resolve(strict=True)
    if not HEX_SHA256.fullmatch(expected_manifest_sha256):
        raise PackageError("expected manifest digest must be 64 lowercase SHA-256 hex characters")
    manifest_path = _relative_file(bundle, "manifest.json")
    manifest_bytes = manifest_path.read_bytes()
    actual_manifest_sha = sha256_bytes(manifest_bytes)
    if actual_manifest_sha != expected_manifest_sha256:
        raise PackageError("manifest bytes do not match the selected digest")
    sidecar = _relative_file(bundle, "manifest.sha256").read_bytes()
    if sidecar != (actual_manifest_sha + "\n").encode("ascii"):
        raise PackageError("manifest SHA-256 sidecar does not match exact manifest bytes")
    try:
        manifest = json.loads(manifest_bytes, object_pairs_hook=_unique_object)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise PackageError("manifest is not valid UTF-8 JSON") from error
    if canonical_json(manifest) != manifest_bytes:
        raise PackageError("manifest JSON is not in the required canonical encoding")
    entries = _validate_manifest(manifest)
    target = manifest["target"]
    if target_text is not None and _target_parts(target_text) != (target["goos"], target["goarch"]):
        raise PackageError("bundle target does not match requested target")
    verified_paths = {"manifest.json", "manifest.sha256"}
    for entry in entries:
        path = _relative_file(bundle, entry["path"])
        info = path.stat()
        if info.st_size != entry["size"] or sha256_file(path) != entry["sha256"]:
            raise PackageError(f"artifact bytes do not match manifest: {entry['path']}")
        if stat.S_IMODE(info.st_mode) != int(entry["mode"], 8):
            raise PackageError(f"artifact mode does not match manifest: {entry['path']}")
        verified_paths.add(entry["path"])
        if entry["kind"] == "amber-typescript-package":
            _verify_typescript_archive(path)
    actual_paths: set[str] = set()
    for path in bundle.rglob("*"):
        if path.is_symlink():
            raise PackageError("bundle contains a symlink")
        if path.is_file():
            actual_paths.add(path.relative_to(bundle).as_posix())
        elif not path.is_dir():
            raise PackageError("bundle contains a non-regular filesystem entry")
    if actual_paths != verified_paths:
        raise PackageError("bundle contains unlisted files or omits listed files")
    for path in bundle.rglob("*"):
        if path.is_dir() and not any((bundle / relative).is_relative_to(path) for relative in verified_paths):
            raise PackageError("bundle contains an unlisted directory")
    return manifest


def _tar_member_bytes(archive: tarfile.TarFile, member: tarfile.TarInfo, limit: int = 16 * 1024 * 1024) -> bytes:
    if not member.isfile() or member.size < 0 or member.size > limit:
        raise PackageError("distribution archive contains an invalid metadata file")
    stream = archive.extractfile(member)
    if stream is None:
        raise PackageError("distribution archive metadata is unreadable")
    data = stream.read(limit + 1)
    if len(data) != member.size or len(data) > limit:
        raise PackageError("distribution archive metadata size is invalid")
    return data


def _drain_tar_padding(archive: tarfile.TarFile) -> None:
    """Consume the compressed stream after tar's end markers and allow only zero padding."""
    while True:
        block = archive.fileobj.read(1024 * 1024)
        if not block:
            return
        if any(block):
            raise PackageError("archive contains nonzero data after tar end markers")


class _BoundedReader:
    def __init__(self, source: Any, limit: int):
        self.source = source
        self.limit = limit
        self.total = 0

    def read(self, size: int = -1) -> bytes:
        remaining = self.limit - self.total
        if remaining < 0:
            raise PackageError("distribution archive exceeds the expanded size limit")
        requested = remaining + 1 if size < 0 else min(size, remaining + 1)
        data = self.source.read(requested)
        self.total += len(data)
        if self.total > self.limit:
            raise PackageError("distribution archive exceeds the expanded size limit")
        return data

    def readable(self) -> bool:
        return True


class _CompressedReader:
    def __init__(self, source: Any, limit: int):
        self.source = source
        self.limit = limit
        self.total = 0
        self.single_byte_run = 0

    def read(self, size: int = -1) -> bytes:
        if size < 0:
            size = self.limit - self.total + 1
        data = self.source.read(min(size, self.limit - self.total + 1))
        self.total += len(data)
        if self.total > self.limit:
            raise PackageError("distribution archive exceeds the compressed size limit")
        if size == 1:
            self.single_byte_run += 1
            if self.single_byte_run > 8192:
                raise PackageError("gzip filename or comment exceeds its header limit")
        else:
            self.single_byte_run = 0
        return data

    def readable(self) -> bool:
        return True


def verify_archive(archive_text: str, expected_manifest_sha256: str, target_text: str | None = None) -> dict[str, Any]:
    archive_path = Path(archive_text)
    if not archive_path.is_absolute() or archive_path.is_symlink() or not archive_path.is_file():
        raise PackageError("archive must be an existing absolute regular file without symlink aliases")
    if archive_path.parent.resolve(strict=True) / archive_path.name != archive_path:
        raise PackageError("archive path parent must not contain symlink aliases")
    archive_path = archive_path.resolve(strict=True)
    if archive_path.stat().st_size > MAX_ARCHIVE_BYTES:
        raise PackageError("distribution archive exceeds the compressed size limit")
    if not HEX_SHA256.fullmatch(expected_manifest_sha256):
        raise PackageError("expected manifest digest must be 64 lowercase SHA-256 hex characters")
    manifest_bytes: bytes | None = None
    sidecar_bytes: bytes | None = None
    seen_files: dict[str, dict[str, Any]] = {}
    directories: set[str] = set()
    total_size = 0
    ts_temp: Path | None = None
    ts_stream: Any = None
    try:
        with archive_path.open("rb") as compressed:
            with gzip.GzipFile(fileobj=_CompressedReader(compressed, MAX_ARCHIVE_BYTES), mode="rb") as decompressed:
                bounded = _BoundedReader(decompressed, MAX_EXPANDED_BYTES)
                with tarfile.open(fileobj=bounded, mode="r|", tarinfo=_bounded_tarinfo_type()) as archive:
                    count = 0
                    for member in archive:
                        count += 1
                        if count > MAX_ARCHIVE_MEMBERS:
                            raise PackageError("distribution archive has too many members")
                        if member.name.endswith("/"):
                            raise PackageError("distribution archive member path is not canonical")
                        name = _safe_relative(member.name).as_posix()
                        if name in directories or name in seen_files:
                            raise PackageError("distribution archive contains duplicate paths")
                        if member.isdir():
                            if member.size != 0 or stat.S_IMODE(member.mode) != 0o755:
                                raise PackageError("distribution archive directory metadata is invalid")
                            directories.add(name)
                            continue
                        if not member.isfile():
                            raise PackageError("distribution archive contains a link or special file")
                        if member.mode not in (0o644, 0o755) or member.size < 0 or member.size > MAX_EXPANDED_BYTES:
                            raise PackageError("distribution archive file metadata is invalid")
                        total_size += member.size
                        if total_size > MAX_EXPANDED_BYTES:
                            raise PackageError("distribution archive exceeds the expanded size limit")
                        stream = archive.extractfile(member)
                        if stream is None:
                            raise PackageError(f"archive artifact is unreadable: {name}")
                        digest = hashlib.sha256()
                        captured = bytearray() if name in {"manifest.json", "manifest.sha256"} else None
                        if name.startswith("sdk/amber-typescript/") and name.endswith(".tgz"):
                            if ts_temp is not None:
                                raise PackageError("distribution archive contains multiple Amber TypeScript packages")
                            fd, tmp_name = tempfile.mkstemp(prefix="ingen-verify-ts-")
                            ts_temp = Path(tmp_name)
                            ts_stream = os.fdopen(fd, "wb")
                        remaining = member.size
                        while remaining:
                            block = stream.read(min(1024 * 1024, remaining))
                            if not block:
                                raise PackageError(f"archive artifact ended early: {name}")
                            digest.update(block)
                            if captured is not None:
                                if len(captured) + len(block) > 16 * 1024 * 1024:
                                    raise PackageError("archive metadata exceeds its size limit")
                                captured.extend(block)
                            if ts_stream is not None and name.startswith("sdk/amber-typescript/") and name.endswith(".tgz"):
                                ts_stream.write(block)
                            remaining -= len(block)
                        if ts_stream is not None and name.startswith("sdk/amber-typescript/") and name.endswith(".tgz"):
                            ts_stream.flush()
                            os.fsync(ts_stream.fileno())
                            ts_stream.close()
                            ts_stream = None
                        seen_files[name] = {"size": member.size, "mode": member.mode, "sha256": digest.hexdigest()}
                        if name == "manifest.json":
                            manifest_bytes = bytes(captured or b"")
                        elif name == "manifest.sha256":
                            sidecar_bytes = bytes(captured or b"")
                    if count == 0:
                        raise PackageError("distribution archive is empty")
                    _drain_tar_padding(archive)
        if manifest_bytes is None or sidecar_bytes is None:
            raise PackageError("distribution archive is missing its manifest")
        if sha256_bytes(manifest_bytes) != expected_manifest_sha256:
            raise PackageError("archive manifest bytes do not match the selected digest")
        if sidecar_bytes != (expected_manifest_sha256 + "\n").encode("ascii"):
            raise PackageError("archive manifest sidecar does not match exact manifest bytes")
        try:
            manifest = json.loads(manifest_bytes, object_pairs_hook=_unique_object)
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise PackageError("archive manifest is not valid UTF-8 JSON") from error
        if canonical_json(manifest) != manifest_bytes:
            raise PackageError("archive manifest JSON is not in the required canonical encoding")
        entries = _validate_manifest(manifest)
        target = manifest["target"]
        if target_text is not None and _target_parts(target_text) != (target["goos"], target["goarch"]):
            raise PackageError("archive target does not match requested target")
        expected_files = {"manifest.json", "manifest.sha256"}
        expected_files.update(entry["path"] for entry in entries)
        expected_directories: set[str] = set()
        for path in expected_files:
            parent = PurePosixPath(path).parent
            while str(parent) != ".":
                expected_directories.add(parent.as_posix())
                parent = parent.parent
        if set(seen_files) != expected_files or directories != expected_directories:
            raise PackageError("distribution archive inventory differs from its manifest")
        entry_by_path = {entry["path"]: entry for entry in entries}
        for name, observed in seen_files.items():
            expected = entry_by_path.get(name)
            expected_mode = int(expected["mode"], 8) if expected else 0o644
            expected_size = expected["size"] if expected else (len(manifest_bytes) if name == "manifest.json" else len(expected_manifest_sha256) + 1)
            expected_sha = expected["sha256"] if expected else (expected_manifest_sha256 if name == "manifest.json" else sha256_bytes((expected_manifest_sha256 + "\n").encode("ascii")))
            if observed != {"size": expected_size, "mode": expected_mode, "sha256": expected_sha}:
                raise PackageError(f"archive artifact bytes or metadata do not match manifest: {name}")
        ts_path = next((entry["path"] for entry in entries if entry["kind"] == "amber-typescript-package"), None)
        if ts_path is None or ts_temp is None:
            raise PackageError("distribution archive is missing the TypeScript package")
        _verify_typescript_archive(ts_temp)
        return manifest
    except (tarfile.TarError, gzip.BadGzipFile, OSError, EOFError) as error:
        if isinstance(error, PackageError):
            raise
        raise PackageError("distribution archive is invalid") from error
    finally:
        if ts_stream is not None:
            ts_stream.close()
        if ts_temp is not None:
            ts_temp.unlink(missing_ok=True)


def export_archive(bundle_text: str, output_text: str, expected_manifest_sha256: str, target_text: str | None = None) -> tuple[Path, str]:
    manifest = verify_bundle(bundle_text, expected_manifest_sha256, target_text)
    bundle = Path(bundle_text).resolve(strict=True)
    output = _safe_absolute(output_text, label="archive output")
    _reject_overlap(output, bundle, label="archive output")
    if output.exists() or output.is_symlink():
        raise PackageError("archive output already exists; refusing to replace it")
    fd, temporary_name = tempfile.mkstemp(prefix=".ingen-archive-", dir=output.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(fd, "wb") as raw:
            with gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0, compresslevel=9) as gz:
                with tarfile.open(fileobj=gz, mode="w|") as tar:
                    for path in sorted(bundle.rglob("*"), key=lambda item: item.relative_to(bundle).as_posix()):
                        if path.is_symlink():
                            raise PackageError("bundle changed to contain a symlink during archive export")
                        relative = path.relative_to(bundle).as_posix()
                        info = tarfile.TarInfo(relative)
                        info.uid = 0
                        info.gid = 0
                        info.uname = ""
                        info.gname = ""
                        info.mtime = 0
                        info.pax_headers = {}
                        if path.is_dir():
                            info.type = tarfile.DIRTYPE
                            info.mode = 0o755
                            info.size = 0
                            tar.addfile(info)
                        elif path.is_file():
                            info.type = tarfile.REGTYPE
                            info.mode = stat.S_IMODE(path.stat().st_mode)
                            info.size = path.stat().st_size
                            with path.open("rb") as stream:
                                tar.addfile(info, stream)
                        else:
                            raise PackageError("bundle contains a special file during archive export")
            raw.flush()
            os.fsync(raw.fileno())
        if temporary.stat().st_size > MAX_ARCHIVE_BYTES:
            raise PackageError("distribution archive exceeds the compressed size limit")
        verify_archive(str(temporary), expected_manifest_sha256, target_text)
        published_stat: os.stat_result | None = None
        temporary_stat = temporary.stat()
        os.link(temporary, output)
        published_stat = temporary_stat
        temporary.unlink()
        dir_fd = os.open(output.parent, os.O_RDONLY)
        try:
            os.fsync(dir_fd)
        finally:
            os.close(dir_fd)
        return output, sha256_file(output)
    except Exception:
        temporary.unlink(missing_ok=True)
        if "published_stat" in locals() and published_stat is not None:
            try:
                current = output.stat(follow_symlinks=False)
                if (current.st_dev, current.st_ino) == (published_stat.st_dev, published_stat.st_ino):
                    output.unlink()
            except FileNotFoundError:
                pass
        raise


def install_archive(archive_text: str, prefix_text: str, expected_manifest_sha256: str, target_text: str | None = None) -> Path:
    prefix, _ = _prefix_path(prefix_text)
    with tempfile.TemporaryDirectory(prefix=".ingen-archive-install-", dir=prefix.parent) as work_text:
        work = Path(work_text)
        snapshot = work / "archive.tar.gz"
        source = Path(archive_text)
        if not source.is_absolute() or source.is_symlink() or not source.is_file() or source.parent.resolve(strict=True) / source.name != source:
            raise PackageError("archive must be an absolute regular file without symlink aliases")
        if source.stat().st_size > MAX_ARCHIVE_BYTES:
            raise PackageError("distribution archive exceeds the compressed size limit")
        source_fd = os.open(source, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
        dest_fd = os.open(snapshot, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600)
        try:
            if not stat.S_ISREG(os.fstat(source_fd).st_mode):
                raise PackageError("archive source is not a regular file")
            with os.fdopen(source_fd, "rb", closefd=False) as src, os.fdopen(dest_fd, "wb", closefd=False) as dst:
                total = 0
                while True:
                    block = src.read(1024 * 1024)
                    if not block:
                        break
                    total += len(block)
                    if total > MAX_ARCHIVE_BYTES:
                        raise PackageError("distribution archive exceeds the compressed size limit")
                    dst.write(block)
                dst.flush()
                os.fsync(dst.fileno())
        finally:
            os.close(source_fd)
            os.close(dest_fd)
        snapshot.chmod(0o400)
        manifest = verify_archive(str(snapshot), expected_manifest_sha256, target_text)
        stage = Path(work_text) / "bundle"
        stage.mkdir(mode=0o755)
        with gzip.open(snapshot, "rb") as decompressed:
            bounded = _BoundedReader(decompressed, MAX_EXPANDED_BYTES)
            archive = tarfile.open(fileobj=bounded, mode="r|", tarinfo=_bounded_tarinfo_type())
            entries = {entry["path"]: entry for entry in manifest["files"]}
            expected = {path: (entry["size"], int(entry["mode"], 8), entry["sha256"]) for path, entry in entries.items()}
            manifest_bytes = canonical_json(manifest)
            metadata = {"manifest.json": manifest_bytes, "manifest.sha256": (expected_manifest_sha256 + "\n").encode("ascii")}
            metadata_hashes = {"manifest.json": (len(metadata["manifest.json"]), 0o644, sha256_bytes(metadata["manifest.json"])), "manifest.sha256": (len(metadata["manifest.sha256"]), 0o644, sha256_bytes(metadata["manifest.sha256"]))}
            expected.update(metadata_hashes)
            expected_directories = {str(parent) for path in expected for parent in PurePosixPath(path).parents if str(parent) != "."}
            seen: set[str] = set()
            total = 0
            count = 0
            for member in archive:
                count += 1
                if count > MAX_ARCHIVE_MEMBERS:
                    raise PackageError("distribution archive has too many members")
                if member.name.endswith("/"):
                    raise PackageError("distribution archive member path is not canonical")
                relative = _safe_relative(member.name).as_posix()
                if relative in seen:
                    raise PackageError("distribution archive contains duplicate paths")
                seen.add(relative)
                if member.isdir():
                    if relative not in expected_directories or member.mode != 0o755 or member.size != 0:
                        raise PackageError("distribution archive directory differs from manifest")
                    stage.joinpath(*PurePosixPath(relative).parts).mkdir(mode=0o755, parents=True, exist_ok=True)
                    continue
                if not member.isfile() or relative not in expected:
                    raise PackageError("distribution archive contains an unexpected file type or path")
                expected_size, expected_mode, expected_sha = expected[relative]
                if member.size != expected_size or member.mode != expected_mode:
                    raise PackageError("distribution archive file metadata changed after verification")
                total += member.size
                if total > MAX_EXPANDED_BYTES:
                    raise PackageError("distribution archive exceeds the expanded size limit")
                target = stage.joinpath(*PurePosixPath(relative).parts)
                target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                stream = archive.extractfile(member)
                if stream is None:
                    raise PackageError("archive changed after verification")
                fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600)
                digest = hashlib.sha256()
                remaining = member.size
                try:
                    with os.fdopen(fd, "wb", closefd=False) as output:
                        while remaining:
                            block = stream.read(min(1024 * 1024, remaining))
                            if not block:
                                raise PackageError("archive artifact ended early during install")
                            digest.update(block)
                            output.write(block)
                            remaining -= len(block)
                        output.flush()
                        os.fsync(output.fileno())
                    os.fchmod(fd, member.mode)
                finally:
                    os.close(fd)
                if digest.hexdigest() != expected_sha:
                    raise PackageError("archive artifact digest changed during install")
            _drain_tar_padding(archive)
            if seen != set(expected) | expected_directories:
                raise PackageError("distribution archive inventory changed after verification")
        verify_bundle(str(stage), expected_manifest_sha256, target_text)
        return install_bundle(str(stage), prefix_text, expected_manifest_sha256, target_text)


def _verify_typescript_archive(path: Path) -> None:
    if path.stat().st_size > MAX_ARCHIVE_BYTES:
        raise PackageError("Amber TypeScript archive exceeds its compressed size limit")
    with path.open("rb") as stream:
        _verify_typescript_archive_stream(stream)


def _verify_typescript_archive_stream(stream: Any) -> None:
    try:
        with gzip.GzipFile(fileobj=_CompressedReader(stream, MAX_ARCHIVE_BYTES), mode="rb") as decompressed:
            bounded = _BoundedReader(decompressed, MAX_EXPANDED_BYTES)
            archive = tarfile.open(fileobj=bounded, mode="r|", tarinfo=_bounded_tarinfo_type())
            files: set[str] = set()
            directories: set[str] = set()
            package_json: dict[str, Any] | None = None
            count = 0
            total_size = 0
            for member in archive:
                count += 1
                if count > MAX_ARCHIVE_MEMBERS:
                    raise PackageError("Amber TypeScript archive has too many members")
                name = member.name[:-1] if member.name.endswith("/") else member.name
                _safe_relative(name)
                if not (name == "package" or name == "package/dist" or name.startswith("package/dist/") or name in {"package/package.json", "package/README.md", "package/LICENSE"}):
                    raise PackageError("Amber TypeScript tarball contains an unexpected path")
                if not member.isdir() and not member.isfile():
                    raise PackageError("Amber TypeScript tarball contains a non-regular entry")
                if name in files or name in directories:
                    raise PackageError("Amber TypeScript tarball contains duplicate members")
                if member.isdir():
                    if member.size != 0 or member.mode != 0o755:
                        raise PackageError("Amber TypeScript tarball directory metadata is invalid")
                    directories.add(name)
                    continue
                if member.mode != 0o644 or member.size < 0:
                    raise PackageError("Amber TypeScript tarball file metadata is invalid")
                total_size += member.size
                if total_size > MAX_EXPANDED_BYTES:
                    raise PackageError("Amber TypeScript archive exceeds its expanded size limit")
                member_stream = archive.extractfile(member)
                if member_stream is None:
                    raise PackageError("Amber TypeScript archive member is unreadable")
                digest = hashlib.sha256()
                captured = bytearray() if name == "package/package.json" else None
                remaining = member.size
                while remaining:
                    block = member_stream.read(min(1024 * 1024, remaining))
                    if not block:
                        raise PackageError("Amber TypeScript archive member ended early")
                    digest.update(block)
                    if captured is not None:
                        if len(captured) + len(block) > 1024 * 1024:
                            raise PackageError("Amber TypeScript package metadata is oversized")
                        captured.extend(block)
                    remaining -= len(block)
                if member.isfile():
                    files.add(name)
                    if name == "package/package.json":
                        try:
                            package_json = json.loads(bytes(captured or b""), object_pairs_hook=_unique_object)
                        except json.JSONDecodeError as error:
                            raise PackageError("Amber TypeScript package metadata is invalid") from error
            _drain_tar_padding(archive)
            if package_json is None or package_json.get("name") != "@0xsj/amber" or not isinstance(package_json.get("version"), str):
                raise PackageError("Amber TypeScript tarball has invalid package identity")
            if not {"package/package.json", "package/README.md", "package/LICENSE"}.issubset(files):
                raise PackageError("Amber TypeScript tarball is missing distributable metadata")
            if not any(name.startswith("package/dist/") and name.endswith((".js", ".d.ts")) for name in files):
                raise PackageError("Amber TypeScript tarball contains no compiled SDK entry points")
    except (tarfile.TarError, gzip.BadGzipFile, json.JSONDecodeError, UnicodeDecodeError, EOFError, OSError) as error:
        raise PackageError("Amber TypeScript archive is invalid") from error


def _copy_exact(source: Path, destination: Path, entry: dict[str, Any] | None = None) -> None:
    data_hash = hashlib.sha256()
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    fd = os.open(destination, flags, 0o600)
    try:
        with source.open("rb") as src, os.fdopen(fd, "wb", closefd=False) as dst:
            while True:
                block = src.read(1024 * 1024)
                if not block:
                    break
                data_hash.update(block)
                dst.write(block)
            dst.flush()
            os.fsync(dst.fileno())
        os.fchmod(fd, int(entry["mode"], 8) if entry is not None else stat.S_IMODE(source.stat().st_mode))
    except Exception:
        try:
            destination.unlink(missing_ok=True)
        except OSError:
            pass
        raise
    finally:
        os.close(fd)
    if entry is not None and data_hash.hexdigest() != entry["sha256"]:
        destination.unlink(missing_ok=True)
        raise PackageError(f"artifact changed while copying: {entry['path']}")


def _prefix_path(prefix_text: str) -> tuple[Path, bool]:
    prefix = Path(prefix_text)
    if not prefix.is_absolute() or str(prefix) != os.path.normpath(prefix_text):
        raise PackageError("install prefix must be an absolute normalized path")
    if any(prefix == system or system in prefix.parents for system in SYSTEM_PREFIXES):
        raise PackageError("install prefix cannot be inside a system path")
    parent = prefix.parent.resolve(strict=True)
    normalized = parent / prefix.name
    if normalized != prefix:
        raise PackageError("install prefix parent must not contain symlink aliases")
    if prefix.is_symlink():
        raise PackageError("install prefix cannot be a symlink")
    if prefix.exists():
        if not prefix.is_dir() or any(prefix.iterdir()):
            raise PackageError("install prefix must be nonexistent or an empty directory")
        return prefix, False
    return prefix, True


def install_bundle(bundle_text: str, prefix_text: str, expected_manifest_sha256: str, target_text: str | None = None) -> Path:
    manifest = verify_bundle(bundle_text, expected_manifest_sha256, target_text)
    bundle = Path(bundle_text).resolve(strict=True)
    prefix, create_root = _prefix_path(prefix_text)
    _reject_overlap(prefix, bundle, label="install prefix")
    if create_root:
        prefix.mkdir(mode=0o755)
    created: list[Path] = []
    try:
        for entry in manifest["files"]:
            relative = _safe_relative(entry["path"])
            destination = prefix.joinpath(*relative.parts)
            parent = destination.parent
            missing: list[Path] = []
            cursor = parent
            while cursor != prefix and not cursor.exists():
                missing.append(cursor)
                cursor = cursor.parent
            for directory in reversed(missing):
                directory.mkdir(mode=0o755)
                created.append(directory)
            _copy_exact(bundle.joinpath(*relative.parts), destination, entry)
            created.append(destination)
        _copy_exact(bundle / "manifest.json", prefix / "manifest.json")
        created.append(prefix / "manifest.json")
        _copy_exact(bundle / "manifest.sha256", prefix / "manifest.sha256")
        created.append(prefix / "manifest.sha256")
        verify_bundle(str(prefix), expected_manifest_sha256, manifest["target"]["goos"] + "/" + manifest["target"]["goarch"])
        return prefix
    except Exception:
        for path in reversed(created):
            try:
                path.unlink() if path.is_file() or path.is_symlink() else path.rmdir()
            except OSError:
                pass
        if create_root:
            try:
                prefix.rmdir()
            except OSError:
                pass
        raise


def _command_env(temp_home: Path, goos: str, goarch: str, gomodcache: Path) -> dict[str, str]:
    path = os.environ.get("PATH", "/usr/bin:/bin")
    env = {
        "PATH": path,
        "HOME": str(temp_home),
        "TMPDIR": str(temp_home),
        "GOCACHE": str(temp_home / "go-cache"),
        "GOMODCACHE": str(gomodcache),
        "GOPROXY": "off",
        "GOSUMDB": "off",
        "GOTOOLCHAIN": "local",
        "GOWORK": "off",
        "GOFLAGS": "",
        "GOOS": goos,
        "GOARCH": goarch,
        "CGO_ENABLED": "0",
        "CARGO_HOME": os.environ.get("CARGO_HOME", str(Path.home() / ".cargo")),
        "RUSTUP_HOME": os.environ.get("RUSTUP_HOME", str(Path.home() / ".rustup")),
        "CARGO_NET_OFFLINE": "true",
        "CARGO_INCREMENTAL": "0",
        "SOURCE_DATE_EPOCH": os.environ.get("SOURCE_DATE_EPOCH", "0"),
        "RUSTFLAGS": "--remap-path-prefix=" + str(Path.cwd()) + "=/source -C debuginfo=0",
        "npm_config_userconfig": str(temp_home / "no-user-npmrc"),
        "npm_config_globalconfig": str(temp_home / "no-global-npmrc"),
        "npm_config_update_notifier": "false",
        "npm_config_audit": "false",
        "npm_config_fund": "false",
    }
    return env


def _tool_output(argv: list[str], cwd: Path, env: dict[str, str]) -> str:
    result = run(argv, cwd=cwd, env=env)
    return result.stdout.strip()


def _source_info(checkout: Path) -> tuple[str, bool, int, str]:
    revision_result = subprocess.run(["git", "rev-parse", "HEAD"], cwd=checkout, text=True, capture_output=True, check=False)
    revision = revision_result.stdout.strip() if revision_result.returncode == 0 else "unknown"
    status_result = subprocess.run(["git", "status", "--porcelain", "--untracked-files=normal"], cwd=checkout, text=True, capture_output=True, check=False)
    dirty = status_result.returncode != 0 or bool(status_result.stdout.strip())
    timestamp_result = subprocess.run(["git", "show", "-s", "--format=%ct", "HEAD"], cwd=checkout, text=True, capture_output=True, check=False)
    source_epoch = int(timestamp_result.stdout.strip()) if timestamp_result.returncode == 0 and timestamp_result.stdout.strip().isdigit() else 0
    build_date = dt.datetime.fromtimestamp(source_epoch, tz=dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ") if source_epoch else "unknown"
    return revision, dirty, source_epoch, build_date


def _rust_target(host_line: str) -> tuple[str, str]:
    host = host_line.strip()
    mapping = {
        "aarch64-apple-darwin": ("darwin", "arm64"),
        "x86_64-apple-darwin": ("darwin", "amd64"),
        "x86_64-unknown-linux-gnu": ("linux", "amd64"),
        "aarch64-unknown-linux-gnu": ("linux", "arm64"),
        "x86_64-pc-windows-msvc": ("windows", "amd64"),
    }
    if host not in mapping:
        raise PackageError(f"unsupported Rust host target: {host}")
    return mapping[host]


def _copy_amber_go(checkout: Path, stage: Path) -> None:
    source = checkout / "amber" / "go"
    destination = stage / "sdk" / "amber-go"
    destination.mkdir(parents=True)
    for path in sorted(source.rglob("*")):
        relative = path.relative_to(source)
        if any(part.startswith(".") for part in relative.parts):
            continue
        if path.is_symlink():
            raise PackageError(f"Amber Go SDK source contains a symlink: {relative}")
        if not path.is_file() or path.name.endswith("_test.go"):
            continue
        if path.suffix not in {".go", ".sql"} and path.name not in {"go.mod", "go.sum", "README.md"}:
            continue
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(path, target)
        target.chmod(0o644)
    license_source = checkout / "amber" / "LICENSE"
    if license_source.is_file():
        shutil.copyfile(license_source, destination / "LICENSE")


def _copy_regular_notice(source: Path, destination: Path) -> None:
    try:
        info = source.lstat()
    except FileNotFoundError as error:
        raise PackageError(f"selected toolchain notice is missing: {source.name}") from error
    if not stat.S_ISREG(info.st_mode):
        raise PackageError(f"selected toolchain notice is not a regular file: {source.name}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    destination.chmod(0o644)


def _copy_license_notices(checkout: Path, snapshot: Path, stage: Path, rust_sysroot: Path, go_root: Path) -> list[dict[str, Any]]:
    copied: list[tuple[str, Path, str, str]] = []
    _copy_regular_notice(snapshot / "LICENSE", stage / "LICENSE")
    copied.append(("LICENSE", stage / "LICENSE", "InGen", "MIT"))
    readme = snapshot / "packaging" / "licenses" / "README.md"
    _copy_regular_notice(readme, stage / "share" / "licenses" / "README.md")
    pinned_go_license = snapshot / "packaging" / "licenses" / "Go-LICENSE"
    candidates = (go_root / "LICENSE", go_root.parent / "LICENSE")
    actual_go_license = next((path for path in candidates if path.is_file()), None)
    if actual_go_license is None or sha256_file(actual_go_license) != sha256_file(pinned_go_license):
        raise PackageError("selected Go toolchain license does not match the reviewed bundle notice")
    _copy_regular_notice(pinned_go_license, stage / "share" / "licenses" / "Go-LICENSE")
    copied.append(("share/licenses/Go-LICENSE", stage / "share" / "licenses" / "Go-LICENSE", "Go standard library", "BSD-3-Clause"))
    for source_name, license_id in (("yaml.v3-LICENSE", "MIT AND Apache-2.0"), ("yaml.v3-NOTICE", "Apache-2.0")):
        source = snapshot / "packaging" / "licenses" / source_name
        destination = stage / "share" / "licenses" / source_name
        _copy_regular_notice(source, destination)
        copied.append((destination.relative_to(stage).as_posix(), destination, "gopkg.in/yaml.v3 v3.0.1", license_id))
    rust_doc = rust_sysroot / "share" / "doc" / "rust"
    copyright = rust_doc / "COPYRIGHT-library.html"
    destination = stage / "share" / "licenses" / "rust" / "COPYRIGHT-library.html"
    _copy_regular_notice(copyright, destination)
    copied.append((destination.relative_to(stage).as_posix(), destination, "Rust standard library", "Apache-2.0 OR MIT"))
    rust_licenses = rust_doc / "licenses"
    if not rust_licenses.is_dir() or rust_licenses.is_symlink():
        raise PackageError("selected Rust standard library license directory is missing")
    for source in sorted(rust_licenses.glob("*.txt")):
        if source.is_symlink() or not source.is_file():
            raise PackageError("selected Rust license entry is not a regular file")
        relative = "share/licenses/rust/licenses/" + source.name
        destination = stage / relative
        _copy_regular_notice(source, destination)
        copied.append((relative, destination, "Rust toolchain component licenses", source.stem))
    if not any(item[0].endswith(".txt") for item in copied):
        raise PackageError("selected Rust toolchain has no bundled license text files")
    return [
        {"path": relative, "component": component, "license": license_id, "size": path.stat().st_size, "sha256": sha256_file(path)}
        for relative, path, component, license_id in sorted(copied, key=lambda item: item[0])
    ]


def _pack_amber_typescript(checkout: Path, stage: Path, temp_home: Path, env: dict[str, str], compiler_source: Path | None = None) -> str:
    source = checkout / "amber" / "typescript"
    compiler_source = compiler_source or source
    compiler_path = compiler_source / "node_modules" / ".bin" / "tsc"
    if not compiler_path.is_file():
        raise PackageError("Amber TypeScript compiler is unavailable; no dependency installation is attempted")
    package = json.loads((source / "package.json").read_bytes())
    name, version = package.get("name"), package.get("version")
    if not isinstance(name, str) or not isinstance(version, str):
        raise PackageError("Amber TypeScript package metadata is invalid")
    sdk_stage = temp_home / "amber-typescript-stage"
    (sdk_stage / "dist").mkdir(parents=True)
    for filename in ("package.json", "README.md", "LICENSE"):
        shutil.copyfile(source / filename, sdk_stage / filename)
    tsc_env = env.copy()
    tsc_env["HOME"] = str(temp_home)
    # TypeScript resolves package imports from the staged source tree. Mount only
    # the compiler's existing SDK dependency directory for this build; it is
    # outside the fingerprinted tree and is never copied into the bundle.
    staged_node_modules = source / "node_modules"
    if staged_node_modules.exists() or staged_node_modules.is_symlink():
        raise PackageError("TypeScript source snapshot unexpectedly contains node_modules")
    staged_node_modules.symlink_to(compiler_source / "node_modules", target_is_directory=True)
    try:
        run([str(compiler_path), "-p", "tsconfig.json", "--outDir", str(sdk_stage / "dist")], cwd=source, env=tsc_env)
    finally:
        staged_node_modules.unlink(missing_ok=True)
    pack_dir = temp_home / "npm-package-output"
    pack_dir.mkdir()
    pack_env = env.copy()
    pack_env["HOME"] = str(temp_home)
    packed = run(["npm", "pack", "--ignore-scripts", "--json", "--pack-destination", str(pack_dir)], cwd=sdk_stage, env=pack_env)
    try:
        result = json.loads(packed.stdout)
        filename = result[0]["filename"]
    except (json.JSONDecodeError, IndexError, KeyError, TypeError) as error:
        raise PackageError("npm pack did not return valid package metadata") from error
    if not isinstance(filename, str) or Path(filename).name != filename or not filename.endswith(".tgz"):
        raise PackageError("npm pack returned an unsafe archive name")
    archive = pack_dir / filename
    if not archive.is_file():
        raise PackageError("npm pack archive is missing")
    allowed_members = {"package/package.json", "package/README.md", "package/LICENSE"}
    allowed_members.update("package/" + item for item in package.get("files", []) if isinstance(item, str))
    with gzip.open(archive, "rb") as decompressed:
        bounded = _BoundedReader(decompressed, MAX_EXPANDED_BYTES)
        with tarfile.open(fileobj=bounded, mode="r|", tarinfo=_bounded_tarinfo_type()) as tar:
            count = 0
            total = 0
            for member in tar:
                count += 1
                if count > MAX_ARCHIVE_MEMBERS:
                    raise PackageError("Amber TypeScript package has too many members")
                member_name = member.name.rstrip("/")
                if not member.isfile() and not member.isdir():
                    raise PackageError("Amber TypeScript package contains a non-regular entry")
                if member.isfile() and not (member_name in allowed_members or member_name.startswith("package/dist/")):
                    raise PackageError(f"Amber TypeScript package contains an unexpected path: {member_name}")
                _safe_relative(member_name)
                total += member.size
                if member.size < 0 or total > MAX_EXPANDED_BYTES:
                    raise PackageError("Amber TypeScript package exceeds its expanded size limit")
                if member.isfile():
                    data = tar.extractfile(member)
                    if data is None:
                        raise PackageError("Amber TypeScript package member is unreadable")
                    remaining = member.size
                    while remaining:
                        block = data.read(min(1024 * 1024, remaining))
                        if not block:
                            raise PackageError("Amber TypeScript package member ended early")
                        remaining -= len(block)
            _drain_tar_padding(tar)
    target = stage / "sdk" / "amber-typescript"
    target.mkdir(parents=True)
    shutil.copyfile(archive, target / filename)
    (target / filename).chmod(0o644)
    return "sdk/amber-typescript/" + filename


def _entry_for(path: Path, stage: Path, module: str, kind: str, executable: bool = False) -> dict[str, Any]:
    relative = path.relative_to(stage).as_posix()
    return {
        "path": relative,
        "module": module,
        "kind": kind,
        "size": path.stat().st_size,
        "sha256": sha256_file(path),
        "mode": "0755" if executable else "0644",
    }


def _probe_help(path: Path, temp_home: Path) -> int:
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(temp_home), "TMPDIR": str(temp_home), "LANG": "C", "LC_ALL": "C"}
    try:
        result = subprocess.run([str(path), "--help"], cwd=temp_home, env=env, text=True, capture_output=True, timeout=8, check=False)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise PackageError(f"CLI help probe failed: {path.name}") from error
    if result.returncode not in (0, 2):
        raise PackageError(f"CLI help probe returned unexpected status {result.returncode}: {path.name}")
    return result.returncode


def _probe_version(path: Path, temp_home: Path, expected: dict[str, str]) -> dict[str, str]:
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(temp_home), "TMPDIR": str(temp_home), "LANG": "C", "LC_ALL": "C"}
    try:
        result = subprocess.run([str(path), "version", "--format", "json"], cwd=temp_home, env=env, text=True, capture_output=True, timeout=8, check=False)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise PackageError(f"CLI version probe failed: {path.name}") from error
    if result.returncode != 0:
        raise PackageError(f"CLI version probe returned status {result.returncode}: {path.name}")
    try:
        report = json.loads(result.stdout, object_pairs_hook=_unique_object)
    except (json.JSONDecodeError, PackageError) as error:
        raise PackageError(f"CLI version probe returned invalid JSON: {path.name}") from error
    if report != expected:
        raise PackageError(f"CLI version metadata differs from selected build inputs: {path.name}")
    return report


def build_bundle(checkout_text: str, output_text: str, target_text: str | None = None, release_version: str = DEFAULT_RELEASE_VERSION) -> tuple[Path, str]:
    checkout = Path(checkout_text).resolve(strict=True)
    if not isinstance(release_version, str) or not SAFE_RELEASE_VERSION.fullmatch(release_version):
        raise PackageError("release version must be a safe 1-64 character identifier")
    output = _safe_absolute(output_text, label="output directory")
    _reject_overlap(output, checkout, label="output directory")
    if output.exists() or output.is_symlink():
        raise PackageError("output directory already exists; choose a fresh path")
    target = target_text
    if target is None:
        goenv = subprocess.run(["go", "env", "GOHOSTOS", "GOHOSTARCH"], cwd=checkout, text=True, capture_output=True, check=False)
        if goenv.returncode != 0:
            raise PackageError("could not determine native Go target")
        parts = goenv.stdout.split()
        if len(parts) != 2:
            raise PackageError("Go reported an invalid native target")
        target = parts[0] + "/" + parts[1]
    goos, goarch = _target_parts(target)
    parent = output.parent
    stage_root = Path(tempfile.mkdtemp(prefix=".ingen-package-work-", dir=parent))
    stage = stage_root / "bundle"
    stage.mkdir(mode=0o755)
    temp_home = stage_root / "home"
    temp_home.mkdir(mode=0o700)
    snapshot = stage_root / "source-snapshot"
    try:
        gomodcache = Path(os.environ.get("GOMODCACHE", str(checkout / ".cache" / "go-mod"))).resolve()
        env = _command_env(temp_home, goos, goarch, gomodcache)
        go_version = _tool_output(["go", "version"], checkout, env)
        goenv = _tool_output(["go", "env", "GOOS", "GOARCH", "GOVERSION"], checkout, env).splitlines()
        if len(goenv) != 3 or (goenv[0], goenv[1]) != (goos, goarch):
            raise PackageError("Go target does not match selected bundle target")
        rust_verbose = _tool_output(["rustc", "-vV"], checkout, env)
        rust_host = next((line.partition(":")[2].strip() for line in rust_verbose.splitlines() if line.startswith("host:")), "")
        rust_goos, rust_goarch = _rust_target(rust_host)
        if (rust_goos, rust_goarch) != (goos, goarch):
            raise PackageError("Rust host target does not match selected Go bundle target")
        go_root = Path(_tool_output(["go", "env", "GOROOT"], checkout, env)).resolve(strict=True)
        rust_sysroot = Path(_tool_output(["rustc", "--print", "sysroot"], checkout, env)).resolve(strict=True)
        cargo_version = _tool_output(["cargo", "--version"], checkout, env)
        rustc_version = _tool_output(["rustc", "--version"], checkout, env)
        node_version = _tool_output(["node", "--version"], checkout, env)
        npm_version = _tool_output(["npm", "--version"], checkout, env)
        ts_package = json.loads((checkout / "amber" / "typescript" / "package.json").read_bytes())
        ts_version_path = checkout / "amber" / "typescript" / "node_modules" / "typescript" / "package.json"
        ts_version = json.loads(ts_version_path.read_bytes()).get("version", "unknown") if ts_version_path.is_file() else str(ts_package.get("devDependencies", {}).get("typescript", "unknown"))
        revision, dirty_bool, source_epoch, build_date = _source_info(checkout)
        dirty = "dirty" if dirty_bool else "clean"
        input_rows, input_sha = build_input_inventory(checkout)
        copy_build_snapshot(checkout, snapshot, input_rows)
        env["SOURCE_DATE_EPOCH"] = str(source_epoch)
        env["RUSTFLAGS"] = "--remap-path-prefix=" + str(snapshot) + "=/source -C debuginfo=0"
        entries: list[dict[str, Any]] = []
        help_probes: list[dict[str, Any]] = []
        version_probes: list[dict[str, Any]] = []
        bin_dir = stage / "bin"
        bin_dir.mkdir()
        rust_target_dir = stage_root / "cargo-target"
        rust_env = env.copy()
        rust_env["CARGO_TARGET_DIR"] = str(rust_target_dir)
        rust_env.update({"INGEN_VERSION": release_version, "INGEN_REVISION": revision, "INGEN_BUILD_DATE": build_date, "INGEN_SOURCE_INPUTS_SHA256": input_sha, "INGEN_DIRTY": dirty, "INGEN_TOOLCHAIN": rustc_version})
        run(["cargo", "build", "--manifest-path", str(snapshot / "malcolm" / "Cargo.toml"), "--locked", "--offline", "--release"], cwd=snapshot, env=rust_env, timeout=900)
        rust_binary = rust_target_dir / "release" / "malcolm"
        if not rust_binary.is_file():
            raise PackageError("Malcolm Rust build did not produce its executable")
        destination = bin_dir / "malcolm"
        shutil.copyfile(rust_binary, destination)
        destination.chmod(0o755)
        entries.append(_entry_for(destination, stage, "malcolm", "cli", executable=True))
        help_probes.append({"name": "malcolm", "exit_code": _probe_help(destination, temp_home)})
        report = _version_report("malcolm", version=release_version, revision=revision, build_date=build_date, source_inputs_sha256=input_sha, dirty=dirty, goos=goos, goarch=goarch, toolchain=rustc_version)
        version_probes.append({"executable": "malcolm", "report": _probe_version(destination, temp_home, report)})
        for name, module, kind, package in CLI_COMMANDS[1:]:
            destination = bin_dir / name
            go_env_copy = env.copy()
            go_env_copy["GOCACHE"] = str(temp_home / "go-cache")
            go_env_copy["GOMODCACHE"] = str(gomodcache)
            linker = [
                "-buildid=",
                "-X ingen/core/cliversion.Version=" + release_version,
                "-X ingen/core/cliversion.Revision=" + revision,
                "-X ingen/core/cliversion.SourceInputsSHA256=" + input_sha,
                "-X ingen/core/cliversion.Dirty=" + dirty,
                "-X ingen/core/cliversion.BuildDate=" + build_date,
                "-X ingen/core/cliversion.Toolchain=" + goenv[2],
            ]
            if name == "sorna":
                linker.extend(["-X ingen/sorna/internal/version.Version=" + release_version, "-X ingen/sorna/internal/version.Commit=" + revision, "-X ingen/sorna/internal/version.BuildDate=" + build_date])
            if name == "paddock":
                linker.extend(["-X ingen/paddock/internal/version.Version=" + release_version, "-X ingen/paddock/internal/version.Commit=" + revision, "-X ingen/paddock/internal/version.BuildDate=" + build_date])
            go_env_copy["GOOS"] = goos
            go_env_copy["GOARCH"] = goarch
            run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags=" + " ".join(linker), "-o", str(destination), package], cwd=snapshot, env=go_env_copy, timeout=900)
            destination.chmod(0o755)
            file_kind = "bridge" if kind == "go-bridge" else "cli"
            entries.append(_entry_for(destination, stage, module, file_kind, executable=True))
            help_probes.append({"name": name, "exit_code": _probe_help(destination, temp_home)})
            report = _version_report(name, version=release_version, revision=revision, build_date=build_date, source_inputs_sha256=input_sha, dirty=dirty, goos=goos, goarch=goarch, toolchain=goenv[2])
            version_probes.append({"executable": name, "report": _probe_version(destination, temp_home, report)})
        _copy_amber_go(snapshot, stage)
        for path in sorted((stage / "sdk" / "amber-go").rglob("*")):
            if path.is_file():
                entries.append(_entry_for(path, stage, "amber", "amber-go-source"))
        typescript_archive = _pack_amber_typescript(snapshot, stage, temp_home, env, compiler_source=checkout / "amber" / "typescript")
        entries.append(_entry_for(stage / typescript_archive, stage, "amber", "amber-typescript-package"))
        distribution_tool_path = stage / "share" / "ingen-package.py"
        distribution_tool_path.parent.mkdir(parents=True)
        shutil.copyfile(snapshot / "packaging" / "ingen_package.py", distribution_tool_path)
        distribution_tool_path.chmod(0o644)
        entries.append(_entry_for(distribution_tool_path, stage, "distribution", "verifier"))
        license_inventory = _copy_license_notices(snapshot, snapshot, stage, rust_sysroot, go_root)
        entries.append(_entry_for(stage / "LICENSE", stage, "ecosystem", "license"))
        entries.append(_entry_for(stage / "share" / "licenses" / "README.md", stage, "ecosystem", "notice-readme"))
        for item in license_inventory:
            if item["path"] != "LICENSE":
                entries.append(_entry_for(stage / item["path"], stage, "ecosystem", "license"))
        entries.sort(key=lambda item: item["path"])
        modules = _expected_modules()
        source_after, input_after_sha = build_input_inventory(checkout)
        if source_after != input_rows or input_after_sha != input_sha:
            raise PackageError("selected build inputs changed while tools were being built")
        manifest = {
            "schema": SCHEMA,
            "release_status": "local-review-only",
            "license_status": "MIT; selected dependency and compiler notices are included; this is not a complete legal review",
            "release": {"version": release_version},
            "source": {"revision": revision, "dirty": dirty_bool, "build_date": build_date},
            "source_inputs": {"algorithm": SOURCE_INPUTS_SCHEMA, "files": input_rows, "sha256": input_sha},
            "target": {"goos": goos, "goarch": goarch, "rust_target": rust_host},
            "toolchains": {"go": go_version, "go_env": {"goos": goenv[0], "goarch": goenv[1], "version": goenv[2]}, "rustc": rustc_version, "cargo": cargo_version, "node": node_version, "npm": npm_version, "typescript": ts_version},
            "reproducibility": {"go": ["CGO_ENABLED=0", "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", "build from verified private source snapshot"], "rust": ["--locked", "--offline", "--release", "CARGO_INCREMENTAL=0", "SOURCE_DATE_EPOCH=git-commit-time", "--remap-path-prefix=<snapshot>=/source", "-C debuginfo=0", "build from verified private source snapshot"], "typescript": ["snapshot source and config", "tsc -p tsconfig.json --outDir <staging>/dist", "npm pack --ignore-scripts"], "native_target_only": True},
            "modules": modules,
            "files": entries,
            "licenses": license_inventory,
            "distribution_tool": "share/ingen-package.py",
            "help_probes": sorted(help_probes, key=lambda item: item["name"]),
            "version_probes": sorted(version_probes, key=lambda item: item["executable"]),
        }
        _validate_manifest(manifest)
        manifest_bytes = canonical_json(manifest)
        (stage / "manifest.json").write_bytes(manifest_bytes)
        (stage / "manifest.sha256").write_text(sha256_bytes(manifest_bytes) + "\n", encoding="ascii")
        for path in stage.rglob("*"):
            if path.is_file() and path.suffix != ".exe":
                path.chmod(0o755 if path.parent == bin_dir else 0o644)
        manifest_sha = sha256_bytes(manifest_bytes)
        # Verify the staged copy before publishing it to the fresh output path.
        verify_bundle(str(stage), manifest_sha, target)
        _publish_exclusive(stage, output)
        return output, manifest_sha
    finally:
        shutil.rmtree(stage_root, ignore_errors=True)


def _publish_exclusive(source: Path, destination: Path) -> None:
    if destination.exists() or destination.is_symlink():
        raise PackageError("output directory appeared during build; refusing to replace it")
    destination.mkdir(mode=0o755)
    created: list[Path] = []
    try:
        for path in sorted(source.rglob("*")):
            relative = path.relative_to(source)
            target = destination / relative
            if path.is_dir():
                target.mkdir(mode=0o755)
                created.append(target)
            elif path.is_file():
                target.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
                if target.parent not in created and target.parent != destination:
                    created.append(target.parent)
                _copy_exact(path, target)
                created.append(target)
            else:
                raise PackageError("staging bundle contains a non-regular path")
    except Exception:
        for path in reversed(created):
            try:
                path.unlink() if path.is_file() or path.is_symlink() else path.rmdir()
            except OSError:
                pass
        try:
            destination.rmdir()
        except OSError:
            pass
        raise


def command_main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="ingen-package", description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    build = subparsers.add_parser("build", help="build a fresh local bundle")
    build.add_argument("--checkout", default=str(Path(__file__).resolve().parents[1]))
    build.add_argument("--output-dir", required=True)
    build.add_argument("--target", help="native target in GOOS/GOARCH form")
    build.add_argument("--release-version", default=DEFAULT_RELEASE_VERSION)
    verify = subparsers.add_parser("verify", help="verify exact manifest bytes and every bundled file")
    verify.add_argument("--bundle", required=True)
    verify.add_argument("--manifest-sha256", required=True)
    verify.add_argument("--target", help="require a matching GOOS/GOARCH target")
    install = subparsers.add_parser("install", help="copy a verified bundle to an empty user-selected prefix")
    install.add_argument("--bundle", required=True)
    install.add_argument("--manifest-sha256", required=True)
    install.add_argument("--prefix", required=True)
    install.add_argument("--target", help="require a matching GOOS/GOARCH target")
    archive = subparsers.add_parser("archive", help="export a verified bundle as a reproducible tar.gz archive")
    archive.add_argument("--bundle", required=True)
    archive.add_argument("--manifest-sha256", required=True)
    archive.add_argument("--output", required=True)
    archive.add_argument("--target", help="require a matching GOOS/GOARCH target")
    verify_archive_parser = subparsers.add_parser("verify-archive", help="verify a distribution archive without executing its contents")
    verify_archive_parser.add_argument("--archive", required=True)
    verify_archive_parser.add_argument("--manifest-sha256", required=True)
    verify_archive_parser.add_argument("--target", help="require a matching GOOS/GOARCH target")
    install_archive_parser = subparsers.add_parser("install-archive", help="install a verified archive into a fresh user-selected prefix")
    install_archive_parser.add_argument("--archive", required=True)
    install_archive_parser.add_argument("--manifest-sha256", required=True)
    install_archive_parser.add_argument("--prefix", required=True)
    install_archive_parser.add_argument("--target", help="require a matching GOOS/GOARCH target")
    args = parser.parse_args(argv)
    try:
        if args.command == "build":
            output, digest = build_bundle(args.checkout, args.output_dir, args.target, args.release_version)
            print(f"bundle: {output}\nmanifest-sha256: {digest}")
        elif args.command == "verify":
            manifest = verify_bundle(args.bundle, args.manifest_sha256, args.target)
            print(f"verified: {Path(args.bundle).resolve()}\nmodules: {len(manifest['modules'])}\nfiles: {len(manifest['files'])}")
        elif args.command == "install":
            prefix = install_bundle(args.bundle, args.prefix, args.manifest_sha256, args.target)
            print(f"installed: {prefix}")
        elif args.command == "archive":
            archive_path, digest = export_archive(args.bundle, args.output, args.manifest_sha256, args.target)
            print(f"archive: {archive_path}\narchive-sha256: {digest}")
        elif args.command == "verify-archive":
            manifest = verify_archive(args.archive, args.manifest_sha256, args.target)
            print(f"verified archive: {Path(args.archive).resolve()}\nmodules: {len(manifest['modules'])}\nfiles: {len(manifest['files'])}")
        else:
            prefix = install_archive(args.archive, args.prefix, args.manifest_sha256, args.target)
            print(f"installed from archive: {prefix}")
        return 0
    except PackageError as error:
        print(f"ingen-package: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(command_main())
