from __future__ import annotations

import hashlib
import importlib.util
import json
import stat
import tempfile
import unittest
from unittest import mock
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("release_evidence.py")
SPEC = importlib.util.spec_from_file_location("release_evidence_test_module", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ReleaseEvidenceTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()
        self.inventory_path = self.root / "inventory.json"

    def tearDown(self) -> None:
        self.temp.cleanup()

    def base(self) -> dict:
        gates = {
            gate_id: {"status": "missing", "reason": "not supplied", "evidence": []}
            for gate_id in release.ALL_GATES
        }
        gates["provider_test"] = {
            "status": "deferred",
            "reason": "Real provider testing is explicitly deferred from this local-review scope.",
            "evidence": [],
        }
        return {
            "schema": release.SCHEMA,
            "scope": "local-review",
            "evidence_root": str(self.root),
            "project": {
                "revision": "a" * 40,
                "release_version": "0.0.0-local-review",
                "target": {"goos": "darwin", "goarch": "arm64"},
            },
            "gates": gates,
        }

    def save(self, value: dict) -> None:
        self.inventory_path.write_text(json.dumps(value), encoding="utf-8")

    def test_missing_release_gates_keep_report_incomplete_and_provider_deferred_is_not_blocker(self) -> None:
        self.save(self.base())
        report = release.audit_inventory(self.inventory_path)
        self.assertEqual(report["evidence_status"], "incomplete")
        self.assertEqual(report["local_review_status"], "incomplete")
        self.assertEqual(report["release_status"], "not-approved")
        self.assertFalse(report["release_eligible"])
        self.assertNotIn("provider_test", report["blockers"])
        self.assertEqual(report["gates"]["provider_test"]["audited_status"], "deferred")

    def test_exact_reference_digest_and_regular_file_are_required(self) -> None:
        value = self.base()
        data = b'{"schema":"ingen.acceptance-unreviewed/v1"}\n'
        evidence = self.root / "live-report.json"
        evidence.write_bytes(data)
        value["gates"]["linux_enforcement"] = {
            "status": "passed",
            "reason": "Declared by submitter",
            "evidence": [{
                "kind": "acceptance-index",
                "path": evidence.name,
                "sha256": hashlib.sha256(data).hexdigest(),
            }],
        }
        self.save(value)
        with self.assertRaisesRegex(release.AuditError, "fixture acceptance schema"):
            release.audit_inventory(self.inventory_path)
        evidence.write_bytes(b"changed")
        with self.assertRaisesRegex(release.AuditError, "digest mismatch"):
            release.audit_inventory(self.inventory_path)

    def test_symlinked_evidence_is_rejected(self) -> None:
        value = self.base()
        actual = self.root / "actual.json"
        actual.write_text("{}", encoding="utf-8")
        (self.root / "alias.json").symlink_to(actual)
        value["gates"]["hosted_ci"] = {
            "status": "passed",
            "reason": "Declared",
            "evidence": [{"kind": "report", "path": "alias.json", "sha256": hashlib.sha256(b"{}").hexdigest()}],
        }
        self.save(value)
        with self.assertRaisesRegex(release.AuditError, "symlink"):
            release.audit_inventory(self.inventory_path)

    def test_unknown_gate_and_duplicate_json_keys_fail_closed(self) -> None:
        value = self.base()
        value["gates"]["unrecognized"] = {"status": "missing", "reason": "x", "evidence": []}
        self.save(value)
        with self.assertRaisesRegex(release.AuditError, "closed release gate catalog"):
            release.audit_inventory(self.inventory_path)
        self.inventory_path.write_text('{"schema":"x","schema":"y"}', encoding="utf-8")
        with self.assertRaisesRegex(release.AuditError, "duplicate JSON key"):
            release.audit_inventory(self.inventory_path)

    def test_nonpassing_failure_can_retain_integrity_checked_evidence(self) -> None:
        value = self.base()
        data = b'{"schema":"reviewed.failure/v1"}\n'
        (self.root / "failed.json").write_bytes(data)
        value["gates"]["runtime_containment"] = {
            "status": "failed",
            "reason": "A required policy check failed.",
            "evidence": [{"kind": "report", "path": "failed.json", "sha256": hashlib.sha256(data).hexdigest()}],
        }
        self.save(value)
        report = release.audit_inventory(self.inventory_path)
        self.assertIn("runtime_containment", report["blockers"])
        self.assertEqual(report["gates"]["runtime_containment"]["audited_status"], "failed")
        self.assertEqual(report["verified_reference_count"], 1)

    def test_external_pass_declarations_remain_review_blockers(self) -> None:
        value = self.base()
        for gate_id in release.REVIEW_GATES:
            data = f"operator declaration only: {gate_id}\n".encode()
            relative = f"{gate_id}.txt"
            (self.root / relative).write_bytes(data)
            value["gates"][gate_id] = {
                "status": "passed",
                "reason": "Submitter declares completion.",
                "evidence": [{
                    "kind": "report",
                    "path": relative,
                    "sha256": hashlib.sha256(data).hexdigest(),
                }],
            }
        self.save(value)
        report = release.audit_inventory(self.inventory_path)
        self.assertEqual(report["evidence_status"], "incomplete")
        self.assertEqual(report["gates"]["hosted_ci"]["audited_status"], "declared-passed")
        self.assertIn("hosted_ci", report["blockers"])
        self.assertFalse(report["release_eligible"])

    def test_malformed_field_types_and_deep_json_are_safe_audit_errors(self) -> None:
        value = self.base()
        value["evidence_root"] = []
        self.save(value)
        with self.assertRaisesRegex(release.AuditError, "evidence_root"):
            release.audit_inventory(self.inventory_path)
        value = self.base()
        value["gates"]["hosted_ci"]["status"] = []
        self.save(value)
        with self.assertRaisesRegex(release.AuditError, "gate values"):
            release.audit_inventory(self.inventory_path)
        self.inventory_path.write_text('{"' + 'x":[' * 1500 + '0' + ']}' * 1500 + '}', encoding="utf-8")
        with self.assertRaisesRegex(release.AuditError, "inventory is not valid"):
            release.audit_inventory(self.inventory_path)

    def test_bundle_snapshot_bounds_entry_count_and_rejects_special_modes(self) -> None:
        bundle = self.root / "bundle"
        bundle.mkdir()
        bundle.chmod(0o755)
        (bundle / "one").write_text("1", encoding="ascii")
        (bundle / "two").write_text("2", encoding="ascii")
        snapshot_root = self.root / "snapshot"
        snapshot_root.mkdir()
        with mock.patch.object(release, "MAX_BUNDLE_ENTRIES", 2):
            with self.assertRaisesRegex(release.AuditError, "too many entries"):
                release._snapshot_bundle(bundle, snapshot_root)
        original_mode = stat.S_IMODE
        with mock.patch.object(release.stat, "S_IMODE", side_effect=lambda mode: 0o4755 if stat.S_ISREG(mode) else original_mode(mode)):
            with self.assertRaisesRegex(release.AuditError, "unsupported mode"):
                release._snapshot_bundle(bundle, snapshot_root)

    def test_callback_and_github_cli_identity_must_match_verified_bundle(self) -> None:
        data = b"selected sentinel executable"
        binary_path = self.root / "install/bin/sentinel"
        binary_path.parent.mkdir(parents=True)
        binary_path.write_bytes(data)
        digest = hashlib.sha256(data).hexdigest()
        report = {
            "schema": "ingen.tool-version/v1",
            "name": "sentinel",
            "version": "0.0.0-local-review",
            "revision": "a" * 40,
            "goos": "darwin",
            "goarch": "arm64",
            "source_inputs_sha256": "b" * 64,
        }
        manifest = {
            "release": {"version": report["version"]},
            "source": {"revision": report["revision"]},
            "files": [{"path": "bin/sentinel", "sha256": digest}],
            "version_probes": [{"executable": "sentinel", "report": report}],
        }
        value = {
            "sentinel_binary": {
                "path": str(binary_path),
                "sha256": digest,
                "version": {key: report[key] for key in ("name", "version", "revision", "goos", "goarch", "source_inputs_sha256")},
            }
        }
        refs = [{
            "kind": "selected-cli",
            "path": "install/bin/sentinel",
            "sha256": digest,
        }]
        release._check_cli_binding(
            "signed_file_ingress",
            self.root,
            value,
            refs,
            manifest,
            {"goos": "darwin", "goarch": "arm64"},
            report["revision"],
            report["version"],
        )
        value["sentinel_binary"]["sha256"] = "c" * 64
        with self.assertRaisesRegex(release.AuditError, "acceptance index"):
            release._check_cli_binding(
                "signed_file_ingress",
                self.root,
                value,
                refs,
                manifest,
                {"goos": "darwin", "goarch": "arm64"},
                report["revision"],
                report["version"],
            )
        refs[0]["sha256"] = "c" * 64
        with self.assertRaisesRegex(release.AuditError, "verified bundle"):
            release._check_cli_binding(
                "signed_file_ingress",
                self.root,
                value,
                refs,
                manifest,
                {"goos": "darwin", "goarch": "arm64"},
                report["revision"],
                report["version"],
            )


if __name__ == "__main__":
    unittest.main()
