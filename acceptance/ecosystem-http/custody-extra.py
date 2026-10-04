#!/usr/bin/env python3
"""Preserve acceptance inputs omitted from the main ecosystem custody loop.

This is a test-harness helper, not a producer verdict. It only snapshots bytes
into Lockwood and publishes an exclusive list of the custody IDs required by
the corresponding CI verification step.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
import tempfile


INDEX_SCHEMA = "ingen.acceptance-custody-extra/v1"


class HarnessError(RuntimeError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def beneath(root: Path, candidate: Path) -> bool:
    try:
        candidate.relative_to(root)
        return True
    except ValueError:
        return False


def project_relative(root: Path, reference: str) -> str:
    if not reference or "\\" in reference:
        raise HarnessError(f"invalid project path {reference!r}")
    if os.path.isabs(reference):
        candidate = Path(os.path.abspath(reference))
        if not beneath(root, candidate):
            raise HarnessError(f"absolute input escapes project root: {reference}")
        relative = candidate.relative_to(root).as_posix()
    else:
        parsed = PurePosixPath(reference)
        if parsed.is_absolute() or parsed.as_posix() != reference or any(part in ("", ".", "..") for part in parsed.parts):
            raise HarnessError(f"input path is not normalized and project-relative: {reference}")
        relative = parsed.as_posix()
    if relative in ("", "."):
        raise HarnessError("input path must name a file or directory")
    return relative


def checked_path(root: Path, relative: str, want: str | None = None) -> Path:
    """Walk without following symlinks and reject special files."""
    current = root
    parts = PurePosixPath(relative).parts
    for index, part in enumerate(parts):
        current = current / part
        try:
            info = os.lstat(current)
        except OSError as exc:
            raise HarnessError(f"inspect {relative}: {exc}") from exc
        if stat.S_ISLNK(info.st_mode):
            raise HarnessError(f"symlink input is not allowed: {relative}")
        final = index == len(parts) - 1
        if not final and not stat.S_ISDIR(info.st_mode):
            raise HarnessError(f"non-directory path component in {relative}")
        if final:
            if want == "file" and not stat.S_ISREG(info.st_mode):
                raise HarnessError(f"input is not a regular file: {relative}")
            if want == "dir" and not stat.S_ISDIR(info.st_mode):
                raise HarnessError(f"input is not a directory: {relative}")
            if want is None and not (stat.S_ISREG(info.st_mode) or stat.S_ISDIR(info.st_mode)):
                raise HarnessError(f"special file input is not allowed: {relative}")
    return current


def read_file(root: Path, relative: str) -> bytes:
    path = checked_path(root, relative, "file")
    flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0) | getattr(os, "O_NONBLOCK", 0)
    try:
        descriptor = os.open(path, flags)
        with os.fdopen(descriptor, "rb", closefd=True) as stream:
            if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                raise HarnessError(f"input changed to a non-regular file: {relative}")
            return stream.read()
    except OSError as exc:
        raise HarnessError(f"read {relative}: {exc}") from exc


def tree_files(root: Path, relative: str) -> list[str]:
    path = checked_path(root, relative)
    if path.is_file():
        return [relative]
    files: list[str] = []
    for entry in sorted(os.scandir(path), key=lambda item: item.name):
        child = f"{relative}/{entry.name}"
        info = entry.stat(follow_symlinks=False)
        if stat.S_ISLNK(info.st_mode):
            raise HarnessError(f"symlink input is not allowed: {child}")
        if stat.S_ISDIR(info.st_mode):
            files.extend(tree_files(root, child))
        elif stat.S_ISREG(info.st_mode):
            files.append(child)
        else:
            raise HarnessError(f"special file input is not allowed: {child}")
    return files


def prepared_source_files(root: Path, source_dir: str) -> list[str]:
    """Return every file in the already-prepared tree, as HashTree sees it."""
    return tree_files(root, source_dir)


def prepared_source_sha256(root: Path, source_dir: str, files: list[str]) -> str:
    """Match sorna/internal/campaign.HashTree over the prepared source root."""
    digest = hashlib.sha256()
    for item in sorted(files):
        relative = PurePosixPath(item).relative_to(PurePosixPath(source_dir)).as_posix()
        digest.update(relative.encode("utf-8"))
        digest.update(b"\x00")
        digest.update(read_file(root, item))
        digest.update(b"\x00")
    return digest.hexdigest()


def load_json(root: Path, relative: str) -> object:
    try:
        return json.loads(read_file(root, relative).decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise HarnessError(f"parse JSON {relative}: {exc}") from exc


def stable_id(phase: str, label: str) -> str:
    slug = re.sub(r"[^A-Za-z0-9._-]+", "-", label.replace("/", "-"))
    slug = re.sub(r"-+", "-", slug).strip("-.") or "artifact"
    if len(slug) > 56:
        slug = f"{slug[:40]}-{hashlib.sha256(label.encode()).hexdigest()[:12]}"
    return f"ecosystem-extra-{phase}-{slug}"


def atomic_exclusive(path: Path, contents: bytes, root: Path) -> None:
    if not beneath(root, path):
        raise HarnessError("index output must remain inside the acceptance project")
    parent = path.parent
    relative_parent = parent.relative_to(root).as_posix()
    if relative_parent == ".":
        raise HarnessError("index output needs an existing project subdirectory")
    checked_path(root, relative_parent, "dir")
    if path.exists() or path.is_symlink():
        raise HarnessError(f"index already exists: {path}")
    descriptor, temp_name = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".tmp", dir=parent)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(contents)
            stream.flush()
            os.fsync(stream.fileno())
        os.link(temp_name, path)
        directory = os.open(parent, os.O_RDONLY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        try:
            os.unlink(temp_name)
        except FileNotFoundError:
            pass


def prepare_index_path(project: Path, store: Path, raw: str) -> Path:
    path = Path(raw)
    if not path.is_absolute():
        path = project / path
    path = Path(os.path.abspath(path))
    if not beneath(project, path):
        raise HarnessError("index path must be inside the acceptance project")
    if beneath(store, path) or beneath(path, store):
        raise HarnessError("index path must not overlap the Lockwood store")
    relative_parent = path.parent.relative_to(project).as_posix()
    if relative_parent == ".":
        raise HarnessError("index output needs an existing project subdirectory")
    checked_path(project, relative_parent, "dir")
    if path.exists() or path.is_symlink():
        raise HarnessError(f"index already exists: {path}")
    return path


class CustodyIntake:
    def __init__(self, lockwood: Path, project: Path, store: Path, phase: str, contract_hash: str):
        self.lockwood = lockwood
        self.project = project
        self.store = store
        self.phase = phase
        self.contract_hash = contract_hash
        self.records: list[dict[str, object]] = []
        self.ids: list[str] = []
        self.by_source: dict[str, str] = {}

    def parent(self, digest: str) -> str:
        return f"references=sha256:{digest}"

    def add_file(self, source: str, producer: str, kind: str, parents: list[str] | None = None) -> str:
        relative = project_relative(self.project, source)
        source_path = self.project / relative
        if beneath(self.store, source_path) or beneath(source_path, self.store):
            raise HarnessError(f"selected source overlaps the Lockwood store: {relative}")
        existing = self.by_source.get(relative)
        if existing:
            return existing
        contents = read_file(self.project, relative)
        digest = sha256(contents)
        custody_id = stable_id(self.phase, f"{kind}-{relative}")
        # Lockwood rejects a lineage edge back to the artifact being added.
        # Preparation source copies can contain an exact copy of an upstream
        # plan, so drop any such self-edge while retaining the other context.
        parent_refs = [self.parent(self.contract_hash), *(parents or [])]
        parent_refs = [ref for ref in parent_refs if ref != self.parent(digest)]
        command = [
            str(self.lockwood), "put", "--root", str(self.store),
            "--id", custody_id, "--media-type", "application/octet-stream",
            "--producer", producer, "--kind", kind, "--name", Path(relative).name,
            "--expected-digest", f"sha256:{digest}", "--source-path", relative,
        ]
        for parent in dict.fromkeys(parent_refs):
            command.extend(["--parent", parent])
        command.append(str(self.project / relative))
        result = self.invoke(command)
        record = self.decode_record(result.stdout, custody_id)
        self.publish_record(custody_id, relative, producer, kind, record["artifact"]["digest"])
        self.by_source[relative] = custody_id
        return custody_id

    def add_sorna_bundle(self, source: str, label: str, context: list[str] | None = None) -> str:
        relative = project_relative(self.project, source)
        files = tree_files(self.project, relative)
        if "manifest.json" not in {Path(item).name for item in files} or "checksums.sha256" not in {Path(item).name for item in files}:
            raise HarnessError(f"Sorna bundle lacks manifest/checksum files: {relative}")
        custody_id = stable_id(self.phase, f"sorna-bundle-{label}")
        result = self.invoke([str(self.lockwood), "import-sorna", "--root", str(self.store), "--id", custody_id, str(self.project / relative)])
        record = self.decode_record(result.stdout, custody_id)
        digest = record["artifact"]["digest"]
        self.publish_record(custody_id, relative, "sorna", "evidence-bundle", digest)
        # import-sorna intentionally exposes no parent flags. Keep its source
        # manifest and checksums as separately pinned, context-linked records.
        for leaf, kind in (("manifest.json", "sorna-evidence-manifest"), ("checksums.sha256", "sorna-evidence-checksums")):
            self.add_file(f"{relative}/{leaf}", "sorna", kind, context)
        return custody_id

    def invoke(self, command: list[str]) -> subprocess.CompletedProcess[str]:
        result = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        if result.returncode != 0:
            sys.stderr.write(result.stderr)
            raise HarnessError(f"Lockwood exited {result.returncode}: {command!r}")
        return result

    @staticmethod
    def decode_record(output: str, expected_id: str) -> dict[str, object]:
        try:
            record = json.loads(output)
            artifact = record["artifact"]
            digest = artifact["digest"]
        except (json.JSONDecodeError, KeyError, TypeError) as exc:
            raise HarnessError(f"Lockwood returned an unreadable custody record: {exc}") from exc
        if record.get("custody_id") != expected_id or not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
            raise HarnessError("Lockwood returned a custody ID or digest that did not match the intake")
        return record

    def publish_record(self, custody_id: str, source: str, producer: str, kind: str, digest: str, related: list[str] | None = None) -> None:
        if custody_id in self.ids:
            raise HarnessError(f"duplicate generated custody ID: {custody_id}")
        if not isinstance(digest, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
            raise HarnessError(f"invalid Lockwood digest for {custody_id}")
        self.ids.append(custody_id)
        self.records.append({
            "id": custody_id,
            "source_path": source,
            "digest": digest,
            "producer": producer,
            "kind": kind,
            "related_ids": related or [],
        })


def campaign_preparation(intake: CustodyIntake) -> None:
    project = intake.project
    plan = ".ingen/artifacts/mutation-plan.json"
    provider = ".ingen/artifacts/go-provider/provider.yaml"
    preparation = ".ingen/artifacts/go-provider/preparation.json"
    provider_check = ".ingen/artifacts/checks/mutation-provider.json"
    preparation_check = ".ingen/artifacts/checks/mutation-preparation.json"
    campaign_result = ".ingen/artifacts/mutation-campaign/campaign-result.json"
    plan_id = intake.add_file(plan, "sorna", "mutation-plan")
    plan_digest = sha256(read_file(project, plan))
    provider_id = intake.add_file(provider, "sorna-go-provider", "mutation-provider-manifest", [intake.parent(plan_digest)])
    provider_digest = sha256(read_file(project, provider))
    preparation_id = intake.add_file(preparation, "sorna-go-provider", "mutation-preparation", [intake.parent(plan_digest), intake.parent(provider_digest)])
    preparation_digest = sha256(read_file(project, preparation))
    intake.add_file(provider_check, "sorna", "mutation-provider-review", [intake.parent(plan_digest), intake.parent(provider_digest)])
    intake.add_file(preparation_check, "sorna", "mutation-preparation-review", [intake.parent(plan_digest), intake.parent(provider_digest), intake.parent(preparation_digest)])
    campaign_id = intake.add_file(campaign_result, "sorna", "mutation-campaign-result", [intake.parent(plan_digest), intake.parent(provider_digest), intake.parent(preparation_digest)])
    # Use the producer's preparation and campaign records to locate their
    # declared source/binary and child-evidence artifacts; do not infer names.
    summary = load_json(project, preparation)
    variants = summary.get("variants") if isinstance(summary, dict) else None
    if not isinstance(variants, list) or not variants:
        raise HarnessError("mutation preparation has no variants")
    for variant in variants:
        if not isinstance(variant, dict):
            raise HarnessError("mutation preparation variant is not an object")
        mutation_id = str(variant.get("mutation_id", ""))
        source_dir = project_relative(project, str(variant.get("source_dir", "")))
        binary_path = project_relative(project, str(variant.get("binary_path", "")))
        source_files = prepared_source_files(project, source_dir)
        source_sha = variant.get("source_sha256")
        binary_sha = variant.get("binary_sha256")
        if not mutation_id or not isinstance(source_sha, str) or not re.fullmatch(r"[0-9a-f]{64}", source_sha):
            raise HarnessError("mutation preparation variant lacks stable identity/source digest")
        if not isinstance(binary_sha, str) or not re.fullmatch(r"[0-9a-f]{64}", binary_sha):
            raise HarnessError("mutation preparation variant lacks binary digest")
        actual_source_sha = prepared_source_sha256(project, source_dir, source_files)
        if actual_source_sha != source_sha:
            raise HarnessError(f"prepared mutation source digest changed: {source_dir}")
        for source_file in source_files:
            relative_to_source = PurePosixPath(source_file).relative_to(PurePosixPath(source_dir))
            copied_ingen_state = ".ingen" in relative_to_source.parts
            parents = None if copied_ingen_state else [intake.parent(plan_digest), intake.parent(provider_digest), intake.parent(preparation_digest)]
            intake.add_file(source_file, "sorna-go-provider", "prepared-mutation-source", parents)
        binary_contents = read_file(project, binary_path)
        if sha256(binary_contents) != binary_sha:
            raise HarnessError(f"prepared mutation binary digest changed: {binary_path}")
        intake.add_file(binary_path, "sorna-go-provider", "prepared-mutation-binary", [intake.parent(plan_digest), intake.parent(provider_digest), intake.parent(preparation_digest)])
    result = load_json(project, campaign_result)
    entries = result.get("entries") if isinstance(result, dict) else None
    if not isinstance(entries, list) or not entries:
        raise HarnessError("mutation campaign result has no entries")
    for entry in entries:
        if not isinstance(entry, dict) or not entry.get("mutation_id"):
            raise HarnessError("mutation campaign entry lacks its mutation identity")
        evidence_path = project_relative(project, str(entry.get("evidence_path", "")))
        bundle_id = intake.add_sorna_bundle(evidence_path, f"campaign-{entry['mutation_id']}", [intake.parent(plan_digest), intake.parent(provider_digest), intake.parent(preparation_digest), intake.parent(sha256(read_file(project, campaign_result)))])
        # The index records the explicit bundle-to-manifest association even
        # though the current import-sorna CLI cannot encode a parent edge.
        for item in reversed(intake.records):
            if item["id"] == bundle_id:
                item["related_ids"] = [intake.by_source[f"{evidence_path}/manifest.json"], plan_id, provider_id, preparation_id, campaign_id]
                break


def before_phase(intake: CustodyIntake) -> None:
    project = intake.project
    canonical = ".ingen/contract/sealed/canonical.json"
    canonical_hash = sha256(read_file(project, canonical))
    paddock_policy = ".ingen/acceptance/paddock.policy.yaml"
    intake.add_file(".ingen/contract/spec.malc", "malcolm", "contract-source")
    intake.add_file(".ingen/contract/document-flow.ir.json", "malcolm", "contract-ir")
    intake.add_file("go.mod", "document-pipeline-lab", "subject-go-module")
    intake.add_file(paddock_policy, "paddock", "architecture-policy", [intake.parent(canonical_hash)])
    paddock_hash = sha256(read_file(project, paddock_policy))
    intake.add_file(".ingen/artifacts/paddock-component-map.json", "paddock", "component-map", [intake.parent(paddock_hash)])
    intake.add_file(".ingen/policy/subject.yaml", "sentinel", "subject-policy")
    intake.add_file(".ingen/acceptance/mutations.catalogue.yaml", "sorna", "mutation-catalogue")
    intake.add_file(".ingen/acceptance/nublar.workflow.yaml", "nublar", "workflow-definition")
    intake.add_file(".ingen/artifacts/document-pipeline/document-pipeline", "document-pipeline-lab", "subject-binary", [intake.parent(paddock_hash)])
    for source_file in tree_files(project, "examples/document-pipeline-lab/subject"):
        intake.add_file(source_file, "document-pipeline-lab", "subject-source", [intake.parent(paddock_hash)])
    campaign_preparation(intake)
    baseline = ".ingen/artifacts/evidence"
    clean_baseline_manifest = f"{baseline}/manifest.json"
    # The mainline has already imported the baseline bundle. Preserve its
    # manifest/checksum bytes as standalone, contract-linked custody too.
    baseline_manifest_id = intake.add_file(clean_baseline_manifest, "sorna", "baseline-evidence-manifest")
    baseline_manifest_digest = sha256(read_file(project, clean_baseline_manifest))
    baseline_checksums_id = intake.add_file(f"{baseline}/checksums.sha256", "sorna", "baseline-evidence-checksums", [intake.parent(baseline_manifest_digest)])
    # Keep the previously imported archive associated in the index without
    # claiming an importer-provided lineage edge that its CLI does not expose.
    for item in intake.records:
        if item["id"] in (baseline_manifest_id, baseline_checksums_id):
            item["related_ids"] = ["ecosystem-sorna-evidence-bundle-clean"]


def after_phase(intake: CustodyIntake) -> None:
    project = intake.project
    canonical_hash = sha256(read_file(project, ".ingen/contract/sealed/canonical.json"))
    amber_root_hash = sha256(read_file(project, ".ingen/provenance/root.json"))
    child = ".ingen/provenance/sorna-defect-child.json"
    child_id = intake.add_file(child, "amber", "provenance-child", [intake.parent(canonical_hash), intake.parent(amber_root_hash)])
    child_hash = sha256(read_file(project, child))
    for path, kind in (
        (".ingen/provenance/sorna-defect-execution.json", "sentinel-execution-receipt"),
        (".ingen/provenance/sorna-defect-execution.json.stdout", "sentinel-execution-stdout"),
        (".ingen/provenance/sorna-defect-execution.json.stderr", "sentinel-execution-stderr"),
    ):
        intake.add_file(path, "sentinel", kind, [intake.parent(canonical_hash), intake.parent(child_hash)])
    evidence_dir = ".ingen/artifacts/evidence-defect"
    bundle_id = intake.add_sorna_bundle(evidence_dir, "defect", [intake.parent(canonical_hash), intake.parent(child_hash)])
    manifest_rel = f"{evidence_dir}/manifest.json"
    manifest_id = intake.by_source[manifest_rel]
    for item in reversed(intake.records):
        if item["id"] == bundle_id:
            item["related_ids"] = [manifest_id, child_id]
            break
    server = "examples/document-pipeline-lab/subject/server.go"
    intake.add_file(server, "document-pipeline-lab", "defect-subject-source", [intake.parent(canonical_hash), intake.parent(child_hash)])
    intake.add_file(".ingen/artifacts/document-pipeline/document-pipeline-defect", "document-pipeline-lab", "defect-subject-binary", [intake.parent(canonical_hash), intake.parent(child_hash), intake.parent(sha256(read_file(project, server)))])


def delivery_phase(intake: CustodyIntake, webhook_json: str | None) -> None:
    project = intake.project
    canonical_hash = sha256(read_file(project, ".ingen/contract/sealed/canonical.json"))
    workflow_hash = sha256(read_file(project, ".ingen/acceptance/nublar.workflow.yaml"))
    clean_lockwood = ".ingen/artifacts/checks/lockwood-clean.json"
    defect_lockwood = ".ingen/artifacts/checks/lockwood.json"
    clean_lockwood_hash = sha256(read_file(project, clean_lockwood))
    defect_lockwood_hash = sha256(read_file(project, defect_lockwood))
    intake.add_file(clean_lockwood, "lockwood", "clean-custody-ci-result", [intake.parent(canonical_hash)])
    intake.add_file(defect_lockwood, "lockwood", "defect-custody-ci-result", [intake.parent(canonical_hash)])
    run_store_rel = ".ingen/artifacts/nublar-runs"
    run_store_files = tree_files(project, run_store_rel)
    expected_runs = {
        "ecosystem-clean": (".ingen/artifacts/nublar-clean.json", ".ingen/artifacts/nublar-clean-decision.json", clean_lockwood_hash),
        "ecosystem-defect": (".ingen/artifacts/nublar-defect.json", ".ingen/artifacts/nublar-defect-decision.json", defect_lockwood_hash),
    }
    run_context: dict[str, tuple[str, str]] = {}
    for run_id, (output_path, decision_path, lockwood_hash) in expected_runs.items():
        output = load_json(project, output_path)
        if not isinstance(output, dict) or output.get("run_id") != run_id:
            raise HarnessError(f"Nublar run output does not identify {run_id}")
        matching = []
        for stored_path in run_store_files:
            if not stored_path.endswith(".json"):
                continue
            try:
                stored = load_json(project, stored_path)
            except HarnessError:
                continue
            if isinstance(stored, dict) and stored.get("run_id") == run_id:
                matching.append(stored_path)
        if len(matching) != 1:
            raise HarnessError(f"expected one persisted Nublar run for {run_id}, found {len(matching)}")
        output_hash = sha256(read_file(project, output_path))
        run_output_id = intake.add_file(output_path, "nublar", "run-output", [intake.parent(canonical_hash), intake.parent(lockwood_hash)])
        stored_id = intake.add_file(matching[0], "nublar", "stored-run", [intake.parent(canonical_hash), intake.parent(lockwood_hash), intake.parent(output_hash)])
        for item in intake.records:
            if item["id"] == stored_id:
                item["related_ids"] = [run_output_id]
                break
        decision_hash = sha256(read_file(project, decision_path))
        decision_id = intake.add_file(decision_path, "nublar", "run-decision", [intake.parent(canonical_hash), intake.parent(output_hash), intake.parent(workflow_hash)])
        run_context[run_id] = (output_hash, decision_hash)
        del run_output_id, decision_id, stored_id
    receipt_path = ".ingen/artifacts/nublar-defect-delivery.json"
    receipt_bytes = read_file(project, receipt_path)
    receipt_hash = sha256(receipt_bytes)
    try:
        receipt = json.loads(receipt_bytes.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise HarnessError(f"parse Nublar delivery receipt: {exc}") from exc
    if not isinstance(receipt, dict) or receipt.get("run_id") != "ecosystem-defect":
        raise HarnessError("Nublar delivery receipt does not identify the defect run")
    defect_output_hash, defect_decision_hash = run_context["ecosystem-defect"]
    receipt_id = intake.add_file(receipt_path, "nublar", "local-delivery-receipt", [intake.parent(canonical_hash), intake.parent(defect_output_hash), intake.parent(defect_decision_hash)])
    stored_receipt = f".ingen/artifacts/nublar-delivery-receipts/receipt-{receipt_hash}.json"
    stored_bytes = read_file(project, stored_receipt)
    if stored_bytes != receipt_bytes:
        raise HarnessError("stored Nublar receipt bytes differ from the local receipt")
    stored_receipt_id = intake.add_file(stored_receipt, "nublar", "stored-delivery-receipt", [intake.parent(canonical_hash), intake.parent(receipt_hash)])
    for item in intake.records:
        if item["id"] == stored_receipt_id:
            item["related_ids"] = [receipt_id]
            break
    if webhook_json:
        proof_root = project.parent
        raw = Path(webhook_json)
        if not raw.is_absolute():
            raw = proof_root / raw
        absolute = Path(os.path.abspath(raw))
        if not beneath(proof_root, absolute):
            raise HarnessError("webhook JSON must remain inside the acceptance proof root")
        relative = absolute.relative_to(proof_root).as_posix()
        contents = read_file(proof_root, relative)
        try:
            webhook = json.loads(contents.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise HarnessError(f"parse webhook request: {exc}") from exc
        if not isinstance(webhook, dict) or webhook.get("run_id") != "ecosystem-defect":
            raise HarnessError("observed webhook request does not identify the defect run")
        # Keep the project-relative path explicitly marked as an external
        # proof-root input in the test index; the bytes remain exact.
        digest = sha256(contents)
        custody_id = stable_id(intake.phase, f"webhook-{relative}")
        command = [str(intake.lockwood), "put", "--root", str(intake.store), "--id", custody_id,
                   "--media-type", "application/json", "--producer", "nublar", "--kind", "observed-webhook-request",
                   "--name", Path(relative).name, "--expected-digest", f"sha256:{digest}", "--source-path", str(absolute)]
        # The local delivery receipt may itself bind this observed webhook
        # body. Keep the webhook's contract context, but never create a
        # reverse receipt edge that would close a lineage cycle.
        for parent_ref in dict.fromkeys(ref for ref in (intake.parent(canonical_hash),) if ref != intake.parent(digest)):
            command.extend(["--parent", parent_ref])
        command.append(str(absolute))
        record = intake.decode_record(intake.invoke(command).stdout, custody_id)
        intake.publish_record(custody_id, f"proof-root:{relative}", "nublar", "observed-webhook-request", record["artifact"]["digest"], [receipt_id])


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--lockwood", required=True)
    parser.add_argument("--project", required=True)
    parser.add_argument("--phase", required=True, choices=("before", "after", "delivery"))
    parser.add_argument("--store", required=True)
    parser.add_argument("--index", required=True)
    parser.add_argument("--webhook-json")
    args = parser.parse_args()
    try:
        project_arg = Path(os.path.abspath(args.project))
        project_info = os.lstat(project_arg)
        if stat.S_ISLNK(project_info.st_mode) or not stat.S_ISDIR(project_info.st_mode):
            raise HarnessError("project root must be an existing real directory")
        project = project_arg.resolve(strict=True)
        if project != project_arg:
            raise HarnessError("project root must not use a symlink alias")
        lockwood_arg = Path(os.path.abspath(args.lockwood))
        if os.path.islink(lockwood_arg):
            raise HarnessError("Lockwood executable must not be a symlink")
        lockwood = lockwood_arg.resolve(strict=True)
        if not stat.S_ISREG(os.stat(lockwood).st_mode):
            raise HarnessError("Lockwood executable must be a regular file")
        store_arg = Path(os.path.abspath(args.store))
        store_info = os.lstat(store_arg)
        if stat.S_ISLNK(store_info.st_mode) or not stat.S_ISDIR(store_info.st_mode):
            raise HarnessError("Lockwood store must be an existing directory")
        store = store_arg.resolve(strict=True)
        if store != store_arg:
            raise HarnessError("Lockwood store must not use a symlink alias")
        index_abs = prepare_index_path(project, store, args.index)
        canonical = read_file(project, ".ingen/contract/sealed/canonical.json")
        contract_hash = sha256(canonical)
        intake = CustodyIntake(lockwood, project, store, args.phase, contract_hash)
        if args.phase == "before":
            if args.webhook_json:
                raise HarnessError("--webhook-json is only valid for --phase delivery")
            before_phase(intake)
        elif args.phase == "after":
            if args.webhook_json:
                raise HarnessError("--webhook-json is only valid for --phase delivery")
            after_phase(intake)
        else:
            delivery_phase(intake, args.webhook_json)
        index = {
            "schema": INDEX_SCHEMA,
            "phase": args.phase,
            "project_root": str(project),
            "custody_store": str(store),
            "ids": intake.ids,
            "records": intake.records,
        }
        encoded = (json.dumps(index, indent=2, sort_keys=True) + "\n").encode("utf-8")
        atomic_exclusive(index_abs, encoded, project)
        return 0
    except (HarnessError, OSError) as exc:
        print(f"custody-extra: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
