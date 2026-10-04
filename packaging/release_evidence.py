#!/usr/bin/env python3
"""Audit a bounded release-evidence inventory without granting release approval.

The auditor verifies bytes and a small set of known local acceptance contracts.
It does not run evidence producers or binaries, contact services, or treat
user-entered assertions as attestations.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
import re
import stat
import sys
import tempfile
from pathlib import Path, PurePosixPath
from typing import Any


SCHEMA = "ingen.release-evidence-inventory/v1"
REPORT_SCHEMA = "ingen.release-evidence-audit/v1"
HEX = re.compile(r"^[0-9a-f]{64}$")
MAX_INVENTORY_BYTES = 1024 * 1024
MAX_REFERENCE_COUNT = 512
MAX_REFERENCE_BYTES = 16 * 1024 * 1024
MAX_ARCHIVE_BYTES = 256 * 1024 * 1024
MAX_SELECTED_BINARY_BYTES = 128 * 1024 * 1024
MAX_TOTAL_BYTES = 512 * 1024 * 1024
MAX_BUNDLE_ENTRIES = 10_000
MAX_BUNDLE_DEPTH = 64
PACKAGER = Path(__file__).with_name("ingen_package.py")

# Local acceptance reports whose documented structure can be checked here.
LOCAL_GATES = {
    "bundle_install": "ingen.archive-install-acceptance/v1",
    "ecosystem_workflow": "ingen.ecosystem-http-acceptance-index/v1",
    "signed_file_ingress": "ingen.sentinel-callback-auth-acceptance/v1",
    "github_protocol": "ingen.github-checks-acceptance-index/v1",
}

# These semantics require platform, host, or independent review beyond this
# integrity checker. A passed value is always reported as declared only.
REVIEW_GATES = (
    "native_governed_workflow",
    "runtime_containment",
    "linux_enforcement",
    "host_signed_callbacks_restart",
    "live_github_delivery",
    "native_install_matrix",
    "hosted_ci",
    "distribution_signature_rebuild",
)
ALL_GATES = tuple(LOCAL_GATES) + REVIEW_GATES + ("provider_test",)
GATE_STATUSES = {"passed", "failed", "missing", "deferred"}
FIXTURE_SCHEMAS = set(LOCAL_GATES.values()) | {
    "ingen.acceptance-codex-broker/v1",
    "ingen.acceptance-native-context-recovery/v1",
    "ingen.archive-install-acceptance/v1",
    "ingen.ecosystem-http-acceptance-index/v1",
    "ingen.github-checks-acceptance-index/v1",
    "ingen.sentinel-callback-auth-acceptance/v1",
}


class AuditError(Exception):
    """Invalid inventory, unsafe reference, or failed byte verification."""


def _pairs_no_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise AuditError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def _json_bytes(raw: bytes, label: str) -> Any:
    try:
        return json.loads(raw.decode("utf-8"), object_pairs_hook=_pairs_no_duplicates)
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError) as exc:
        raise AuditError(f"{label} is not valid UTF-8 JSON") from exc


def _is_str(value: Any, *, max_len: int = 4096) -> bool:
    return isinstance(value, str) and 0 < len(value) <= max_len and "\x00" not in value


def _regular_root(path: Path) -> Path:
    if not path.is_absolute() or "\x00" in str(path):
        raise AuditError("evidence_root must be absolute")
    current = Path(path.anchor)
    for part in path.parts[1:]:
        current = current / part
        try:
            mode = current.lstat().st_mode
        except OSError as exc:
            raise AuditError("evidence_root does not exist") from exc
        if stat.S_ISLNK(mode):
            raise AuditError("evidence_root contains a symlink")
    try:
        canonical = current.resolve(strict=True)
    except OSError as exc:
        raise AuditError("evidence_root cannot be resolved") from exc
    if not current.is_dir() or canonical != current:
        raise AuditError("evidence_root must be a canonical directory")
    return current


def _read_regular_bounded(path: Path, limit: int) -> bytes:
    try:
        fd = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0))
    except OSError as exc:
        raise AuditError("referenced file cannot be opened safely") from exc
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size < 0 or info.st_size > limit:
            raise AuditError("referenced file is not a bounded regular file")
        before = os.fstat(fd)
        chunks: list[bytes] = []
        remaining = limit + 1
        while remaining:
            block = os.read(fd, min(1024 * 1024, remaining))
            if not block:
                break
            chunks.append(block)
            remaining -= len(block)
        data = b"".join(chunks)
        if len(data) > limit:
            raise AuditError("referenced file exceeds its size limit")
        after = os.fstat(fd)
        if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns) or after.st_size != len(data):
            raise AuditError("referenced file changed while being read")
        return data
    finally:
        os.close(fd)


def _copy_fd_to_path(fd: int, destination: Path, limit: int) -> tuple[str, int]:
    before = os.fstat(fd)
    if not stat.S_ISREG(before.st_mode) or before.st_size < 0 or before.st_size > limit:
        raise AuditError("referenced file is not a bounded regular file")
    digest = hashlib.sha256()
    copied = 0
    os.lseek(fd, 0, os.SEEK_SET)
    with destination.open("xb") as output:
        while True:
            block = os.read(fd, 1024 * 1024)
            if not block:
                break
            copied += len(block)
            if copied > limit:
                raise AuditError("referenced file exceeds its size limit")
            digest.update(block)
            output.write(block)
        output.flush()
        os.fsync(output.fileno())
    after = os.fstat(fd)
    if copied != before.st_size or (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns):
        raise AuditError("referenced file changed while being snapshotted")
    return digest.hexdigest(), copied


def _snapshot_bundle(bundle: Path, snapshot_root: Path) -> tuple[Path, int]:
    if bundle.is_symlink() or not bundle.is_dir():
        raise AuditError("bundle root is not a real directory")
    if stat.S_IMODE(bundle.stat().st_mode) != 0o755:
        raise AuditError("bundle root has an unsupported mode")
    destination = snapshot_root / f"bundle-{len(list(snapshot_root.iterdir()))}"
    destination.mkdir(mode=0o700)
    total = 0
    entries = 1
    for current, dirs, files in os.walk(bundle, followlinks=False):
        current_path = Path(current)
        relative = current_path.relative_to(bundle)
        relative_text = relative.as_posix()
        if len(relative.parts) > MAX_BUNDLE_DEPTH or len(relative_text) > 1024 or "\\" in relative_text or relative_text.startswith("../"):
            raise AuditError("bundle snapshot path is not normalized or exceeds its depth limit")
        target_dir = destination / relative
        target_dir.mkdir(mode=0o700, exist_ok=True)
        dirs.sort()
        files.sort()
        entries += len(dirs) + len(files)
        if entries > MAX_BUNDLE_ENTRIES:
            raise AuditError("bundle snapshot has too many entries")
        for dirname in dirs:
            source = current_path / dirname
            if source.is_symlink() or not source.is_dir():
                raise AuditError("bundle snapshot contains a symlink or special directory")
            if stat.S_IMODE(source.stat().st_mode) != 0o755:
                raise AuditError("bundle snapshot directory has an unsupported mode")
            child_text = (relative / dirname).as_posix()
            if len(child_text) > 1024 or "\\" in child_text or PurePosixPath(child_text).as_posix() != child_text:
                raise AuditError("bundle snapshot directory path is not normalized")
            (target_dir / dirname).mkdir(mode=0o700, exist_ok=True)
        for filename in files:
            child_text = (relative / filename).as_posix()
            if len(child_text) > 1024 or "\\" in child_text or PurePosixPath(child_text).as_posix() != child_text:
                raise AuditError("bundle snapshot file path is not normalized")
            source = current_path / filename
            if source.is_symlink():
                raise AuditError("bundle snapshot contains a symlink")
            try:
                fd = os.open(source, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0))
            except OSError as exc:
                raise AuditError("bundle snapshot file cannot be opened safely") from exc
            try:
                info = os.fstat(fd)
                mode = stat.S_IMODE(info.st_mode)
                if mode not in {0o644, 0o755}:
                    raise AuditError("bundle snapshot file has an unsupported mode")
                size = info.st_size
                if size < 0 or total + size > MAX_TOTAL_BYTES:
                    raise AuditError("bundle snapshot exceeds the total size limit")
                file_target = target_dir / filename
                _, copied = _copy_fd_to_path(fd, file_target, MAX_TOTAL_BYTES - total)
                total += copied
                os.chmod(file_target, mode)
            finally:
                os.close(fd)
    return destination, total


def _reference(root: Path, obj: Any, seen: set[str], total: list[int], snapshot_root: Path) -> dict[str, Any]:
    if not isinstance(obj, dict) or set(obj) != {"kind", "path", "sha256"}:
        raise AuditError("evidence reference fields are invalid")
    kind, text, expected = obj["kind"], obj["path"], obj["sha256"]
    if not _is_str(kind, max_len=80) or not _is_str(text, max_len=1024) or not isinstance(expected, str) or not HEX.fullmatch(expected):
        raise AuditError("evidence reference values are invalid")
    rel = PurePosixPath(text)
    if rel.is_absolute() or str(rel) != text or any(part in ("", ".", "..") for part in rel.parts) or "\\" in text:
        raise AuditError("evidence path must be normalized and relative")
    if text in seen:
        raise AuditError("duplicate evidence path")
    seen.add(text)
    target = root.joinpath(*rel.parts)
    cursor = root
    for part in rel.parts:
        cursor = cursor / part
        try:
            mode = cursor.lstat().st_mode
        except OSError as exc:
            raise AuditError("referenced evidence file is missing") from exc
        if stat.S_ISLNK(mode):
            raise AuditError("evidence reference traverses a symlink")
    if kind in {"package-archive", "selected-cli"}:
        limit = MAX_ARCHIVE_BYTES if kind == "package-archive" else MAX_SELECTED_BINARY_BYTES
        fd = os.open(target, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0))
        try:
            suffix = ".tar.gz" if kind == "package-archive" else ".binary"
            snapshot = snapshot_root / f"ref-{len(list(snapshot_root.iterdir()))}{suffix}"
            actual, size = _copy_fd_to_path(fd, snapshot, limit)
        finally:
            os.close(fd)
        data = None
    else:
        data = _read_regular_bounded(target, MAX_REFERENCE_BYTES)
        actual = hashlib.sha256(data).hexdigest()
        size = len(data)
        snapshot = None
        if kind == "bundle-manifest":
            bundle_snapshot, bundle_size = _snapshot_bundle(target.parent, snapshot_root)
            total[0] += bundle_size
            snapshot = bundle_snapshot
    total[0] += size
    if total[0] > MAX_TOTAL_BYTES:
        raise AuditError("referenced evidence exceeds the inventory total size limit")
    if actual != expected:
        raise AuditError(f"evidence digest mismatch: {text}")
    return {"kind": kind, "path": text, "sha256": actual, "size": size, "_bytes": data, "_snapshot": snapshot}


def _check_cli_binding(
    gate_id: str,
    root: Path,
    value: dict[str, Any],
    refs: list[dict[str, Any]],
    bundle_manifest: dict[str, Any],
    target: dict[str, str],
    revision: str,
    release_version: str,
) -> None:
    executable = "sentinel" if gate_id == "signed_file_ingress" else "nublar"
    identity_key = "sentinel_binary" if gate_id == "signed_file_ingress" else "nublar_cli"
    identity = value.get(identity_key)
    if not isinstance(identity, dict) or not _is_str(identity.get("path")) or not isinstance(identity.get("sha256"), str) or not HEX.fullmatch(identity["sha256"]):
        raise AuditError(f"{gate_id} selected CLI identity is missing")
    binary_refs = [ref for ref in refs if ref["kind"] == "selected-cli"]
    if len(binary_refs) != 1:
        raise AuditError(f"{gate_id} requires exactly one selected-cli file reference")
    binary_ref = binary_refs[0]
    if identity["sha256"] != binary_ref["sha256"]:
        raise AuditError(f"{gate_id} selected CLI file differs from its acceptance index")
    selected_path = Path(identity["path"])
    if not selected_path.is_absolute() or selected_path.resolve(strict=True) != selected_path or selected_path != (root / binary_ref["path"]).resolve(strict=True):
        raise AuditError(f"{gate_id} selected CLI path differs from its evidence reference")
    entries = bundle_manifest.get("files")
    if not isinstance(entries, list):
        raise AuditError("verified bundle manifest has no artifact inventory")
    bundle_entry = next((item for item in entries if isinstance(item, dict) and item.get("path") == f"bin/{executable}"), None)
    if not isinstance(bundle_entry, dict) or identity["sha256"] != bundle_entry.get("sha256"):
        raise AuditError(f"{gate_id} selected CLI digest differs from the verified bundle")
    probes = bundle_manifest.get("version_probes")
    if not isinstance(probes, list):
        raise AuditError("verified bundle manifest has no CLI version probes")
    probe = next((item for item in probes if isinstance(item, dict) and item.get("executable") == executable), None)
    expected = probe.get("report") if isinstance(probe, dict) else None
    observed = identity.get("version")
    required_fields = {"name", "version", "revision", "goos", "goarch"}
    if not isinstance(expected, dict) or not isinstance(observed, dict) or not required_fields.issubset(observed):
        raise AuditError(f"{gate_id} version/target identity is incomplete")
    expected_identity = {
        "name": executable,
        "version": release_version,
        "revision": revision,
        "goos": target["goos"],
        "goarch": target["goarch"],
    }
    if any(observed.get(field) != expected_identity[field] or expected.get(field) != expected_identity[field] for field in required_fields):
        raise AuditError(f"{gate_id} CLI version, revision, or target differs from the verified bundle")
    for field in set(expected) & set(observed):
        if observed[field] != expected[field]:
            raise AuditError(f"{gate_id} CLI version report differs from the verified bundle: {field}")


def _check_local_gate(
    gate_id: str,
    root: Path,
    refs: list[dict[str, Any]],
    target: dict[str, str],
    revision: str,
    release_version: str,
    bundle_manifest: dict[str, Any] | None,
) -> tuple[str, dict[str, Any] | None]:
    indexes = [ref for ref in refs if ref["kind"] == "acceptance-index"]
    if len(indexes) != 1:
        raise AuditError(f"{gate_id} requires exactly one acceptance-index reference")
    value = _json_bytes(indexes[0]["_bytes"], f"{gate_id} acceptance index")
    if not isinstance(value, dict) or value.get("schema") != LOCAL_GATES[gate_id]:
        raise AuditError(f"{gate_id} evidence has an unrecognized acceptance schema")
    if gate_id == "bundle_install":
        if value.get("status") != "passed" or not isinstance(value.get("limitations"), list):
            raise AuditError("bundle_install acceptance did not pass its local contract")
        actual_target = value.get("target")
        if not isinstance(actual_target, dict) or {key: actual_target.get(key) for key in target} != target:
            raise AuditError("bundle_install target differs from inventory")
        manifest_refs = [ref for ref in refs if ref["kind"] == "bundle-manifest"]
        archive_refs = [ref for ref in refs if ref["kind"] == "package-archive"]
        if len(manifest_refs) != 1 or len(archive_refs) != 1:
            raise AuditError("bundle_install requires manifest and archive references")
        if value.get("manifest_sha256") != manifest_refs[0]["sha256"] or value.get("archive_sha256") != archive_refs[0]["sha256"]:
            raise AuditError("bundle_install identity is incomplete or mismatched")
        try:
            spec = importlib.util.spec_from_file_location("_ingen_release_audit_package", PACKAGER)
            if spec is None or spec.loader is None:
                raise AuditError("trusted bundle verifier is unavailable")
            package = importlib.util.module_from_spec(spec)
            sys.modules[spec.name] = package
            spec.loader.exec_module(package)
            bundle_path = root / manifest_refs[0]["path"]
            if bundle_path.name != "manifest.json":
                raise AuditError("bundle manifest reference must name manifest.json")
            bundle_manifest = package.verify_bundle(str(manifest_refs[0]["_snapshot"]), manifest_refs[0]["sha256"], f'{target["goos"]}/{target["goarch"]}')
            package.verify_archive(str(archive_refs[0]["_snapshot"]), manifest_refs[0]["sha256"], f'{target["goos"]}/{target["goarch"]}')
            if bundle_manifest.get("release", {}).get("version") != release_version or bundle_manifest.get("source", {}).get("revision") != revision:
                raise AuditError("bundle release version or source revision differs from project identity")
        except AuditError:
            raise
        except Exception as exc:
            raise AuditError("bundle or archive failed the standalone package verifier") from exc
        return "bundle manifest/archive identity, target, version, and source revision checked", bundle_manifest
    if gate_id == "ecosystem_workflow":
        fixture = value.get("governance_fixture")
        if not isinstance(fixture, dict) or fixture.get("explicitly_synthetic_test_only") is not True or fixture.get("operator_authorization") is not False:
            raise AuditError("ecosystem workflow does not identify its synthetic approval fixture")
        contract = value.get("contract")
        if not isinstance(contract, dict) or not _is_str(contract.get("id")) or type(contract.get("version")) is not int or contract["version"] < 1 or not HEX.fullmatch(str(contract.get("canonical_identity_sha256", ""))):
            raise AuditError("ecosystem workflow contract identity is invalid")
        ci_results = value.get("ci_results")
        if not isinstance(ci_results, dict) or not isinstance(value.get("limitations"), dict):
            raise AuditError("ecosystem workflow report is incomplete")
        expected_ci = {
            "hammond_approval": ("passed", 0, "hammond", "approved-contract"),
            "hammond_missing_governance": ("error", 2, "hammond", "approved-contract"),
            "lockwood_clean": ("passed", 0, "lockwood", "custody-verification"),
            "lockwood_complete": ("passed", 0, "lockwood", "custody-verification"),
            "lockwood_damaged_parent": ("failed", 1, "lockwood", "custody-verification"),
            "lockwood_defect": ("passed", 0, "lockwood", "custody-verification"),
            "nublar_missing_required": ("error", 2, None, None),
            "paddock_architecture": ("passed", 0, "paddock", "architecture"),
            "paddock_forbidden_edge": ("failed", 1, "paddock", "architecture"),
            "sorna_behavior_clean": ("passed", 0, "sorna", "behavioral-verification"),
            "sorna_behavior_defect": ("failed", 1, "sorna", "behavioral-verification"),
            "sorna_mutation_campaign": ("passed", 0, "sorna", "mutation-campaign"),
            "sorna_mutation_preparation": ("passed", 0, "sorna", "mutation-preparation"),
            "sorna_mutation_provider": ("passed", 0, "sorna", "mutation-provider-review"),
        }
        if set(ci_results) != set(expected_ci):
            raise AuditError("ecosystem workflow required CI result catalog differs")
        for name, expected in expected_ci.items():
            actual = ci_results[name]
            if not isinstance(actual, dict) or (actual.get("status"), actual.get("exit_code"), actual.get("tool"), actual.get("kind")) != expected:
                raise AuditError(f"ecosystem workflow CI outcome is invalid: {name}")
        campaign = value.get("counts", {}).get("mutation_campaign") if isinstance(value.get("counts"), dict) else None
        if not isinstance(campaign, dict) or campaign.get("total") != 1 or campaign.get("killed") != 1 or campaign.get("errors") != 0 or campaign.get("inconclusive") != 0:
            raise AuditError("ecosystem workflow did not demonstrate its expected killed mutation")
        nublar = value.get("nublar")
        if not isinstance(nublar, dict):
            raise AuditError("ecosystem workflow Nublar decisions are missing")
        for key, expected in {
            "clean_run": ("passed", 0),
            "clean_decision": ("passed", 0),
            "defect_run": ("failed", 1),
            "defect_decision": ("failed", 1),
        }.items():
            outcome = nublar.get(key)
            if not isinstance(outcome, dict) or (outcome.get("status"), outcome.get("exit_code")) != expected:
                raise AuditError(f"ecosystem workflow Nublar outcome is invalid: {key}")
        limitations = value["limitations"]
        if limitations.get("http_context_propagation_observed") is not False or limitations.get("native_herdr_gate") != "Not exercised by this HTTP workflow.":
            raise AuditError("ecosystem workflow limitation scope is missing")
        return "synthetic governance flags, CI outcomes, one killed mutation, Nublar transitions, and declared limitations checked", None
    if gate_id == "signed_file_ingress":
        if value.get("status") != "passed" or not isinstance(value.get("files"), list) or not value["files"]:
            raise AuditError("signed_file_ingress acceptance did not pass its local contract")
        if not any("local acceptance signer" in str(item) for item in value.get("limitations", [])):
            raise AuditError("signed_file_ingress scope limitations are missing")
        if bundle_manifest is None:
            raise AuditError("signed_file_ingress has no verified bundle reference")
        _check_cli_binding(gate_id, root, value, refs, bundle_manifest, target, revision, release_version)
        return "passed marker, local signer scope, and selected CLI/bundle identity checked", None
    if gate_id == "github_protocol":
        if value.get("status") != "passed" or value.get("head_sha") != revision:
            raise AuditError("github_protocol result is incomplete or revision-mismatched")
        if not isinstance(value.get("source_proof"), dict) or not isinstance(value.get("receipts"), dict):
            raise AuditError("github_protocol evidence is incomplete")
        if "synthetic loopback" not in str(value.get("scope", "")):
            raise AuditError("github_protocol evidence is outside the accepted local synthetic scope")
        if bundle_manifest is None:
            raise AuditError("github_protocol has no verified bundle reference")
        _check_cli_binding(gate_id, root, value, refs, bundle_manifest, target, revision, release_version)
        return "passed marker, selected revision, synthetic service scope, and selected CLI/bundle identity checked", None
    raise AssertionError("unregistered local gate")


def audit_inventory(inventory_path: Path) -> dict[str, Any]:
    try:
        parent = inventory_path.parent.resolve(strict=True)
    except OSError as exc:
        raise AuditError("inventory parent directory does not exist") from exc
    if not inventory_path.is_absolute() or "\x00" in str(inventory_path) or parent != inventory_path.parent or inventory_path.is_symlink():
        raise AuditError("inventory path must be absolute, regular, and free of symlink aliases")
    if not inventory_path.is_file():
        raise AuditError("inventory path is not a regular file")
    with tempfile.TemporaryDirectory(prefix="ingen-release-audit-") as scratch_text:
        return _audit_inventory(inventory_path, Path(scratch_text).resolve(strict=True))


def _audit_inventory(inventory_path: Path, snapshot_root: Path) -> dict[str, Any]:
    raw_inventory = _read_regular_bounded(inventory_path, MAX_INVENTORY_BYTES)
    inventory = _json_bytes(raw_inventory, "inventory")
    required = {"schema", "scope", "evidence_root", "project", "gates"}
    if not isinstance(inventory, dict) or set(inventory) != required or inventory.get("schema") != SCHEMA or inventory.get("scope") != "local-review":
        raise AuditError("inventory schema, fields, or scope are invalid")
    if not isinstance(inventory["evidence_root"], str):
        raise AuditError("evidence_root must be an absolute path string")
    root = _regular_root(Path(inventory["evidence_root"]))
    project = inventory["project"]
    if not isinstance(project, dict) or set(project) != {"revision", "release_version", "target"}:
        raise AuditError("project identity fields are invalid")
    revision = project["revision"]
    version = project["release_version"]
    target = project["target"]
    if not isinstance(revision, str) or (revision != "unknown" and not re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", revision)):
        raise AuditError("project revision is invalid")
    if not _is_str(version, max_len=64):
        raise AuditError("release version is invalid")
    if not isinstance(target, dict) or set(target) != {"goos", "goarch"} or not all(_is_str(v, max_len=32) for v in target.values()):
        raise AuditError("target identity is invalid")
    gates = inventory["gates"]
    if not isinstance(gates, dict) or set(gates) != set(ALL_GATES):
        raise AuditError("inventory must contain the closed release gate catalog")

    total = [0]
    seen: set[str] = set()
    results: dict[str, Any] = {}
    blockers: list[str] = []
    refs_seen = 0
    verified_bundle_manifest: dict[str, Any] | None = None
    for gate_id in ALL_GATES:
        gate = gates[gate_id]
        if not isinstance(gate, dict) or set(gate) != {"status", "reason", "evidence"}:
            raise AuditError(f"{gate_id} gate fields are invalid")
        status, reason, evidence = gate["status"], gate["reason"], gate["evidence"]
        if not isinstance(status, str) or status not in GATE_STATUSES or not isinstance(reason, str) or len(reason) > 2048 or not isinstance(evidence, list):
            raise AuditError(f"{gate_id} gate values are invalid")
        if status in {"missing", "deferred"} and not reason.strip():
            raise AuditError(f"{gate_id} missing/deferred state requires a reason")
        if status == "passed" and not evidence:
            raise AuditError(f"{gate_id} passed state requires evidence references")
        if status in {"missing", "deferred"} and evidence:
            raise AuditError(f"{gate_id} missing/deferred state cannot carry proof references")
        refs_seen += len(evidence)
        if refs_seen > MAX_REFERENCE_COUNT:
            raise AuditError("inventory has too many evidence references")
        refs = [_reference(root, ref, seen, total, snapshot_root) for ref in evidence]
        semantic = "integrity-checked-only"
        state = status
        if gate_id in LOCAL_GATES and status in {"passed", "failed"}:
            indexes = [ref for ref in refs if ref["kind"] == "acceptance-index"]
            if len(indexes) != 1:
                raise AuditError(f"{gate_id} requires exactly one acceptance-index reference")
            value = _json_bytes(indexes[0]["_bytes"], f"{gate_id} acceptance index")
            if not isinstance(value, dict) or value.get("schema") != LOCAL_GATES[gate_id]:
                raise AuditError(f"{gate_id} evidence has an unrecognized acceptance schema")
            if status == "passed":
                semantic, observed_manifest = _check_local_gate(gate_id, root, refs, target, revision, version, verified_bundle_manifest)
                if observed_manifest is not None:
                    verified_bundle_manifest = observed_manifest
            else:
                semantic = "recognized local failure report; gate remains blocked"
        elif status == "passed":
            state = "declared-passed"
            semantic = "integrity-checked; semantic coverage requires independent review"
            for ref in refs:
                if ref["kind"] == "acceptance-index":
                    value = _json_bytes(ref["_bytes"], f"{gate_id} acceptance index")
                    schema = value.get("schema") if isinstance(value, dict) else None
                    if isinstance(schema, str) and (schema in FIXTURE_SCHEMAS or schema.startswith("ingen.acceptance-") or schema.endswith("-acceptance-index/v1")):
                        raise AuditError(f"{gate_id} cannot use a fixture acceptance schema as live evidence")
        is_provider_deferred = gate_id == "provider_test" and status == "deferred"
        if gate_id == "provider_test" and not is_provider_deferred:
            blockers.append(gate_id)
        elif is_provider_deferred:
            semantic = "deferred by explicit scope; no provider proof claimed"
        elif gate_id in REVIEW_GATES:
            blockers.append(gate_id)
        elif gate_id in LOCAL_GATES and status != "passed":
            blockers.append(gate_id)
        results[gate_id] = {
            "declared_status": status,
            "audited_status": state,
            "reason": reason,
            "semantic_coverage": semantic,
            "evidence": [{key: ref[key] for key in ("kind", "path", "sha256", "size")} for ref in refs],
        }

    local_ready = all(results[gate]["audited_status"] == "passed" for gate in LOCAL_GATES) and results["provider_test"]["declared_status"] == "deferred"
    evidence_complete = local_ready and not blockers
    return {
        "schema": REPORT_SCHEMA,
        "evidence_status": "complete" if evidence_complete else "incomplete",
        "local_review_status": "ready-for-human-review" if local_ready else "incomplete",
        "release_status": "not-approved",
        "release_eligible": False,
        "scope": "local-review",
        "project": {"revision": revision, "release_version": version, "target": target},
        "evidence_root": str(root),
        "gates": results,
        "blockers": blockers,
        "limitations": [
            "This audit checks referenced bytes and known local report contracts only.",
            "External gate declarations are not attestations and require independent human review.",
            "No release approval, signature, publication, or real-provider success is established.",
            "The provider test is explicitly deferred and is outside this local-review scope.",
        ],
        "verified_reference_count": refs_seen,
        "verified_bytes": total[0],
    }


def write_report(path: Path, report: dict[str, Any]) -> None:
    if not path.is_absolute() or path.exists() or path.is_symlink():
        raise AuditError("output must be a fresh absolute path")
    try:
        parent = path.parent.resolve(strict=True)
    except OSError as exc:
        raise AuditError("output parent directory does not exist") from exc
    if parent != path.parent or not parent.is_dir():
        raise AuditError("output parent must be a canonical existing directory")
    encoded = (json.dumps(report, sort_keys=True, indent=2) + "\n").encode("utf-8")
    fd, tmp_name = tempfile.mkstemp(prefix=".release-evidence.", dir=parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(encoded)
            stream.flush()
            os.fsync(stream.fileno())
        os.link(tmp, path)
        dfd = os.open(parent, os.O_RDONLY)
        try:
            os.fsync(dfd)
        finally:
            os.close(dfd)
    except FileExistsError as exc:
        raise AuditError("output appeared during publication") from exc
    finally:
        try:
            tmp.unlink()
        except FileNotFoundError:
            pass


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    audit = sub.add_parser("audit")
    audit.add_argument("--inventory", required=True, type=Path)
    audit.add_argument("--output", required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        report = audit_inventory(args.inventory)
        write_report(args.output, report)
    except AuditError as exc:
        print(f"release-evidence: {exc}", file=sys.stderr)
        return 2
    except (OSError, ValueError, TypeError, RecursionError):
        print("release-evidence: malformed inventory or filesystem error", file=sys.stderr)
        return 2
    print(f"evidence status: {report['evidence_status']}; local review: {report['local_review_status']}; release approval: not granted")
    return 0 if report["evidence_status"] == "complete" else 1


if __name__ == "__main__":
    raise SystemExit(main())
