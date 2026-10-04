#!/usr/bin/env python3
"""Run actual Nublar GitHub-check delivery against a sanitized loopback mock."""

from __future__ import annotations

import argparse
import hashlib
import http.server
import json
import os
import pathlib
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
from urllib.parse import parse_qs, urlencode, urlsplit


SYNTHETIC_TOKEN = "ingen-github-checks-acceptance-token-only"
REPOSITORY = "fixture/project"
CHECK_NAME = "Nublar / local acceptance"
RUN_IDS = {"ecosystem-clean", "ecosystem-defect"}


def digest(path: pathlib.Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def load(path: pathlib.Path):
    return json.loads(path.read_text(encoding="utf-8"))


def tree_snapshot(root: pathlib.Path):
    rows = []
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise RuntimeError(f"source Nublar store contains a symlink: {path}")
        if path.is_file():
            rows.append({"path": path.relative_to(root).as_posix(), "size": path.stat().st_size, "sha256": digest(path)})
        elif not path.is_dir():
            raise RuntimeError("source Nublar store contains a special filesystem entry")
    encoded = (json.dumps(rows, sort_keys=True, separators=(",", ":")) + "\n").encode()
    return rows, hashlib.sha256(encoded).hexdigest()


def validate_source(proof: pathlib.Path):
    proof = proof.resolve(strict=True)
    index_path = proof / "acceptance-index.json"
    if index_path.is_symlink() or not index_path.is_file():
        raise RuntimeError("ecosystem acceptance-index.json is missing")
    index = load(index_path)
    if index.get("schema") != "ingen.ecosystem-http-acceptance-index/v1" or index.get("evidence_root") != str(proof):
        raise RuntimeError("ecosystem acceptance index identity mismatch")
    project = pathlib.Path(index.get("project_root", "")).resolve(strict=True)
    store = project / ".ingen" / "artifacts" / "nublar-runs"
    if store.is_symlink() or not store.is_dir():
        raise RuntimeError("source Nublar run store is missing or unsafe")
    refs = {}
    for key, status, exit_code, run_id in (
        ("clean_run", "passed", 0, "ecosystem-clean"),
        ("clean_decision", "passed", 0, "ecosystem-clean"),
        ("defect_run", "failed", 1, "ecosystem-defect"),
        ("defect_decision", "failed", 1, "ecosystem-defect"),
    ):
        indexed = index.get("nublar", {}).get(key)
        if not isinstance(indexed, dict):
            raise RuntimeError(f"source index has invalid {key} reference")
        ref = indexed.get("path")
        if not isinstance(ref, dict) or set(ref) != {"path", "sha256"}:
            raise RuntimeError(f"source index has invalid {key} file reference")
        relative = pathlib.PurePosixPath(ref["path"])
        if relative.is_absolute() or str(relative) != ref["path"] or any(part in ("", ".", "..") for part in relative.parts):
            raise RuntimeError(f"source {key} path is unsafe")
        path = proof.joinpath(*relative.parts)
        if path.is_symlink() or not path.is_file() or digest(path) != ref["sha256"]:
            raise RuntimeError(f"source {key} bytes do not match the acceptance index")
        artifact = load(path)
        if artifact.get("status") != status or artifact.get("exit_code") != exit_code or artifact.get("run_id") != run_id:
            raise RuntimeError(f"source {key} has unexpected run identity or outcome")
        refs[key] = {"path": ref["path"], "sha256": digest(path), "status": status, "exit_code": exit_code, "run_id": run_id}
    rows, store_sha = tree_snapshot(store)
    source = {
        "schema": "ingen.github-checks-source-snapshot/v1",
        "proof_root": str(proof),
        "project_root": str(project),
        "acceptance_index": {"path": "acceptance-index.json", "sha256": digest(index_path)},
        "producer_evidence": refs,
        "nublar_store": {"path": str(store), "sha256": store_sha, "files": rows},
    }
    return source


class MockChecks(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        return

    def reply(self, status: int, value, headers=None):
        body = json.dumps(value, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        for key, value in (headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(body)
        self.wfile.flush()

    def get_body(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length < 0 or length > (8 << 20):
            raise ValueError("request body exceeds mock limit")
        body = self.rfile.read(length) if length else b""
        return json.loads(body) if body else None

    def audit(self, status: int, body, response_headers=None):
        fixture = self.server.fixture
        authorization = self.headers.get("Authorization", "")
        row = {
            "method": self.command,
            "path": self.path,
            "status": status,
            "authorization_present": bool(authorization),
            "authorization_valid": authorization == "Bearer " + SYNTHETIC_TOKEN,
            "headers": {
                key.lower(): self.headers.get(key, "")
                for key in ("Accept", "Content-Type", "X-GitHub-Api-Version")
                if self.headers.get(key) is not None
            },
            "response_link_present": bool((response_headers or {}).get("Link")),
        }
        if body is not None:
            row["body"] = body
        fixture.requests.append(row)
        data = (json.dumps({"schema": "ingen.github-checks-mock-audit/v1", "requests": fixture.requests}, sort_keys=True, indent=2) + "\n").encode()
        fd, name = tempfile.mkstemp(prefix=".mock-audit.", dir=fixture.output_root)
        try:
            with os.fdopen(fd, "wb") as stream:
                stream.write(data)
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(name, fixture.audit_path)
        finally:
            pathlib.Path(name).unlink(missing_ok=True)

    def authorized(self, body):
        valid = self.headers.get("Authorization", "") == "Bearer " + SYNTHETIC_TOKEN
        if not valid:
            self.audit(401, body)
            self.reply(401, {"message": "synthetic test authorization required"})
        return valid

    def do_GET(self):
        fixture = self.server.fixture
        try:
            body = self.get_body()
        except Exception:
            self.reply(400, {"message": "invalid fixture request"})
            return
        if not self.authorized(body):
            return
        parsed = urlsplit(self.path)
        query = parse_qs(parsed.query, strict_parsing=True)
        list_path = f"/repos/{REPOSITORY}/commits/{fixture.head_sha}/check-runs"
        if parsed.path != list_path or query.get("check_name") != [CHECK_NAME] or query.get("filter") != ["all"] or query.get("per_page") != ["100"]:
            self.audit(404, body)
            self.reply(404, {"message": "unexpected checks list endpoint"})
            return
        if fixture.deny_next_list:
            fixture.deny_next_list = False
            self.audit(403, body)
            self.reply(403, {"message": "synthetic permission denied"})
            return
        try:
            page = int(query.get("page", [""])[0])
        except ValueError:
            page = 0
        if page not in (1, 2):
            self.audit(400, body)
            self.reply(400, {"message": "unexpected pagination page"})
            return
        matches = [item for item in fixture.records if item["name"] == CHECK_NAME]
        count = len(matches)
        page_items = matches[(page - 1) * 100 : page * 100]
        response_headers = {}
        if page * 100 < count:
            next_query = {key: values[0] for key, values in query.items()}
            next_query["page"] = str(page + 1)
            next_url = f"http://127.0.0.1:{self.server.server_port}{parsed.path}?{urlencode(sorted(next_query.items()))}"
            response_headers["Link"] = f'<{next_url}>; rel="next"'
        self.audit(200, body, response_headers)
        self.reply(200, {"total_count": count, "check_runs": page_items}, response_headers)

    def expected_payload(self, body, *, create: bool):
        if not isinstance(body, dict):
            return False
        run_id = body.get("external_id")
        want_conclusion = "success" if run_id == "ecosystem-clean" else "failure"
        return (
            body.get("name") == CHECK_NAME
            and run_id in RUN_IDS
            and body.get("status") == "completed"
            and body.get("conclusion") == want_conclusion
            and bool(body.get("started_at"))
            and bool(body.get("completed_at"))
            and (not create or body.get("head_sha") == self.server.fixture.head_sha)
        )

    def do_POST(self):
        fixture = self.server.fixture
        try:
            body = self.get_body()
        except Exception:
            self.reply(400, {"message": "invalid fixture request"})
            return
        path = f"/repos/{REPOSITORY}/check-runs"
        if not self.authorized(body):
            return
        if self.path != path or not self.expected_payload(body, create=True):
            self.audit(422, body)
            self.reply(422, {"message": "unexpected create payload"})
            return
        if any(row["name"] == CHECK_NAME and row["external_id"] == body["external_id"] for row in fixture.records):
            self.audit(409, body)
            self.reply(409, {"message": "duplicate run ID"})
            return
        record = {
            "id": fixture.next_id,
            "name": body["name"],
            "external_id": body["external_id"],
            "head_sha": body["head_sha"],
            "status": body["status"],
            "conclusion": body["conclusion"],
        }
        fixture.next_id += 1
        fixture.records.append(record)
        if body["external_id"] == "ecosystem-clean" and fixture.ambiguous_create:
            fixture.ambiguous_create = False
            self.audit(503, body)
            self.reply(503, {"message": "synthetic commit followed by ambiguous response"})
            return
        self.audit(201, body)
        self.reply(201, record)

    def do_PATCH(self):
        fixture = self.server.fixture
        try:
            body = self.get_body()
        except Exception:
            self.reply(400, {"message": "invalid fixture request"})
            return
        prefix = f"/repos/{REPOSITORY}/check-runs/"
        if not self.authorized(body):
            return
        if not self.path.startswith(prefix) or not self.expected_payload(body, create=False):
            self.audit(422, body)
            self.reply(422, {"message": "unexpected update payload"})
            return
        try:
            check_id = int(self.path[len(prefix) :])
        except ValueError:
            check_id = 0
        record = next((row for row in fixture.records if row["id"] == check_id), None)
        if record is None or record["external_id"] != body["external_id"]:
            self.audit(404, body)
            self.reply(404, {"message": "unknown check ID"})
            return
        record.update(status=body["status"], conclusion=body["conclusion"], name=body["name"])
        self.audit(200, body)
        self.reply(200, record)


class FixtureServer(http.server.HTTPServer):
    def __init__(self, address, fixture):
        super().__init__(address, MockChecks)
        self.fixture = fixture


def run_delivery(binary, store, run_id, api, head_sha, output, label, expected_exit):
    receipt = output / "receipts" / f"{label}.json"
    env = {
        "PATH": "/usr/bin:/bin",
        "HOME": str(output / "empty-home"),
        "TMPDIR": str(output),
        "LANG": "C",
        "LC_ALL": "C",
        "INGEN_TEST_GITHUB_TOKEN": SYNTHETIC_TOKEN,
    }
    argv = [
        str(binary), "run", "deliver", "--transport", "github-checks",
        "--store", str(store), "--run-id", run_id,
        "--repository", REPOSITORY, "--head-sha", head_sha,
        "--check-name", CHECK_NAME, "--token-env", "INGEN_TEST_GITHUB_TOKEN",
        "--api-base-url", api, "--timeout", "15s", "--max-attempts", "1", "--retry-delay", "0s",
        "--receipt", str(receipt), "--receipt-store", str(output / "receipt-store"),
    ]
    result = subprocess.run(argv, cwd=output, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=30, check=False)
    (output / "logs" / f"{label}.log").write_text(result.stdout, encoding="utf-8")
    if result.returncode != expected_exit:
        raise RuntimeError(f"{label} returned {result.returncode}, expected {expected_exit}; inspect logs/{label}.log")
    return receipt


def validate_and_index(proof, output, head_sha, source, server, cli_identity):
    store = pathlib.Path(source["nublar_store"]["path"])
    rows, store_sha = tree_snapshot(store)
    if rows != source["nublar_store"]["files"] or store_sha != source["nublar_store"]["sha256"]:
        raise RuntimeError("source Nublar store changed during delivery")
    index_path = proof / "acceptance-index.json"
    if digest(index_path) != source["acceptance_index"]["sha256"]:
        raise RuntimeError("source ecosystem acceptance index changed during delivery")
    for key, ref in source["producer_evidence"].items():
        if digest(proof / ref["path"]) != ref["sha256"]:
            raise RuntimeError(f"source producer evidence changed during delivery: {key}")
    receipt_rows = {}
    for label, run_id, wanted in (
        ("clean-ambiguous", "ecosystem-clean", "accepted"),
        ("clean-repeat", "ecosystem-clean", "accepted"),
        ("defect-create", "ecosystem-defect", "accepted"),
        ("permission-denied", "ecosystem-defect", "failed"),
    ):
        path = output / "receipts" / f"{label}.json"
        receipt = load(path)
        if receipt.get("schema") != "ingen.nublar-delivery-receipt/v1" or receipt.get("transport") != "github-checks" or receipt.get("run_id") != run_id or receipt.get("status") != wanted:
            raise RuntimeError(f"{label} Nublar receipt has the wrong identity or outcome")
        if label == "permission-denied" and "403" not in receipt.get("error", ""):
            raise RuntimeError("permission-denied receipt does not report synthetic HTTP 403")
        receipt_rows[label] = {"path": path.relative_to(output).as_posix(), "sha256": digest(path), "status": receipt["status"], "http_status": receipt.get("http_status"), "run_id": receipt["run_id"]}
    audit_path = output / "mock-requests.json"
    audit = load(audit_path)
    requests = audit.get("requests", [])
    if audit.get("schema") != "ingen.github-checks-mock-audit/v1" or not requests:
        raise RuntimeError("synthetic API request audit is missing")
    if any("authorization" in row or row.get("authorization_valid") is not True for row in requests):
        raise RuntimeError("Authorization values leaked or authentication was incorrect")
    if any(row.get("headers", {}).get("authorization") for row in requests):
        raise RuntimeError("Authorization header value was written to evidence")
    def selected(method, run_id):
        return [row for row in requests if row.get("method") == method and isinstance(row.get("body"), dict) and row["body"].get("external_id") == run_id]
    clean_posts = selected("POST", "ecosystem-clean")
    clean_patches = selected("PATCH", "ecosystem-clean")
    defect_posts = selected("POST", "ecosystem-defect")
    defect_patches = selected("PATCH", "ecosystem-defect")
    if len(clean_posts) != 1 or clean_posts[0]["status"] != 503 or len(clean_patches) != 2:
        raise RuntimeError("ambiguous clean create was not reconciled once and updated on repeat")
    clean_operations = [row for row in requests if row.get("method") in {"POST", "PATCH"} and isinstance(row.get("body"), dict) and row["body"].get("external_id") == "ecosystem-clean"]
    if [(row["method"], row["status"]) for row in clean_operations] != [("POST", 503), ("PATCH", 200), ("PATCH", 200)]:
        raise RuntimeError("clean delivery did not reconcile the ambiguous POST before the repeat update")
    if len({row["path"].rsplit("/", 1)[-1] for row in clean_patches}) != 1:
        raise RuntimeError("repeated clean delivery did not update the same check ID")
    if len(defect_posts) != 1 or defect_posts[0]["status"] != 201 or len(defect_patches) != 0:
        raise RuntimeError("failed decision was not delivered as one failure conclusion")
    if defect_posts[0].get("body", {}).get("conclusion") != "failure":
        raise RuntimeError("failed Nublar decision did not map to a GitHub failure conclusion")
    if not any(row.get("method") == "GET" and row.get("status") == 403 for row in requests):
        raise RuntimeError("synthetic permission denial was not observed")
    if not any("page=2" in row.get("path", "") for row in requests if row.get("method") == "GET"):
        raise RuntimeError("checks-list pagination did not request the linked second page")
    if any(row.get("body", {}).get("head_sha") not in (None, head_sha) for row in requests):
        raise RuntimeError("delivery did not select the full current HEAD SHA")
    for path in output.rglob("*"):
        if path.is_file() and SYNTHETIC_TOKEN.encode() in path.read_bytes():
            raise RuntimeError("synthetic token appeared in persisted evidence")
    receipt_store = output / "receipt-store"
    receipt_store_rows, receipt_store_sha = tree_snapshot(receipt_store)
    logs = []
    for path in sorted((output / "logs").rglob("*")):
        if path.is_symlink() or not path.is_file():
            if path.is_dir():
                continue
            raise RuntimeError("acceptance log path is not a regular file")
        logs.append({"path": path.relative_to(output).as_posix(), "size": path.stat().st_size, "sha256": digest(path)})
    final = {
        "schema": "ingen.github-checks-acceptance-index/v1",
        "status": "passed",
        "scope": "actual Nublar CLI delivery to a synthetic loopback GitHub Checks API",
        "head_sha": head_sha,
        "repository": REPOSITORY,
        "check_name": CHECK_NAME,
        "nublar_cli": cli_identity,
        "source_proof": {
            "acceptance_index": source["acceptance_index"],
            "producer_evidence": source["producer_evidence"],
            "nublar_store": {"path": source["nublar_store"]["path"], "sha256": store_sha, "files": rows, "unchanged": True},
        },
        "receipts": receipt_rows,
        "receipt_store": {"path": "receipt-store", "sha256": receipt_store_sha, "files": receipt_store_rows},
        "logs": logs,
        "mock_api": {
            "requests": {"path": "mock-requests.json", "sha256": digest(audit_path), "count": len(requests)},
            "clean_ambiguous_create_http": clean_posts[0]["status"],
            "clean_reconcile_and_repeat_patch_count": len(clean_patches),
            "failed_conclusion": defect_posts[0]["body"].get("conclusion"),
            "permission_denied_http": 403,
            "authorization_values_persisted": False,
            "pagination_link_observed": any(row.get("response_link_present") for row in requests),
            "service": "synthetic local fixture; no GitHub API called",
        },
        "limitations": ["This verifies client protocol against a synthetic local server, not GitHub service acceptance."],
    }
    encoded = (json.dumps(final, sort_keys=True, indent=2) + "\n").encode()
    final_path = output / "acceptance-index.json"
    fd, temporary = tempfile.mkstemp(prefix=".acceptance-index.", dir=output)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(encoded)
            stream.flush()
            os.fsync(stream.fileno())
        os.link(temporary, final_path)
    finally:
        pathlib.Path(temporary).unlink(missing_ok=True)
    return final_path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--bin", required=True, type=pathlib.Path)
    parser.add_argument("--ecosystem-proof", required=True, type=pathlib.Path)
    parser.add_argument("--repo-root", required=True, type=pathlib.Path)
    parser.add_argument("--output-root", type=pathlib.Path)
    args = parser.parse_args()
    binary = args.bin.resolve(strict=True)
    if not binary.is_file() or not os.access(binary, os.X_OK):
        raise RuntimeError("Nublar binary is not executable")
    proof = args.ecosystem_proof.resolve(strict=True)
    repo = args.repo_root.resolve(strict=True)
    source = validate_source(proof)
    store = pathlib.Path(source["nublar_store"]["path"])
    head = subprocess.run(["git", "-C", str(repo), "rev-parse", "HEAD"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False)
    if head.returncode != 0:
        raise RuntimeError("could not determine selected full HEAD commit")
    head_sha = head.stdout.strip()
    if len(head_sha) not in (40, 64) or any(ch not in "0123456789abcdef" for ch in head_sha):
        raise RuntimeError("selected HEAD is not a full commit SHA")
    if args.output_root is None:
        output_parent = pathlib.Path("/private/tmp" if sys.platform == "darwin" else "/tmp").resolve(strict=True)
        output = pathlib.Path(tempfile.mkdtemp(prefix="ingen-github-checks.", dir=output_parent))
    else:
        requested = args.output_root
        if not requested.is_absolute() or requested != pathlib.Path(os.path.normpath(requested)) or requested.exists() or requested.is_symlink():
            raise RuntimeError("output root must be a fresh absolute normalized path")
        requested.parent.resolve(strict=True)
        output = requested
        output.mkdir(parents=False)
    output = output.resolve(strict=True)
    (output / "logs").mkdir()
    (output / "receipts").mkdir()
    (output / "empty-home").mkdir(mode=0o700)
    source_path = output / "source-before.json"
    source_path.write_text(json.dumps(source, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    version_result = subprocess.run(
        [str(binary), "version", "--format", "json"],
        cwd=output,
        env={"PATH": "/usr/bin:/bin", "HOME": str(output / "empty-home"), "LANG": "C", "LC_ALL": "C"},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=8,
        check=False,
    )
    if version_result.returncode != 0:
        raise RuntimeError("selected Nublar binary version probe failed")
    cli_version = json.loads(version_result.stdout)
    if cli_version.get("schema") != "ingen.tool-version/v1" or cli_version.get("name") != "nublar":
        raise RuntimeError("selected binary does not report the expected Nublar tool identity")
    cli_identity = {"path": str(binary), "sha256": digest(binary), "version": cli_version}
    fixture = type("FixtureState", (), {})()
    fixture.output_root = output
    fixture.audit_path = output / "mock-requests.json"
    fixture.head_sha = head_sha
    fixture.records = [
        {"id": number, "name": CHECK_NAME, "external_id": f"unrelated-{number}", "head_sha": head_sha, "status": "completed", "conclusion": "success"}
        for number in range(1, 102)
    ]
    fixture.next_id = 102
    fixture.requests = []
    fixture.ambiguous_create = True
    fixture.deny_next_list = False
    server = FixtureServer(("127.0.0.1", 0), fixture)
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.05}, daemon=True)
    thread.start()
    api = f"http://127.0.0.1:{server.server_port}"
    try:
        run_delivery(binary, store, "ecosystem-clean", api, head_sha, output, "clean-ambiguous", 0)
        run_delivery(binary, store, "ecosystem-clean", api, head_sha, output, "clean-repeat", 0)
        run_delivery(binary, store, "ecosystem-defect", api, head_sha, output, "defect-create", 0)
        fixture.deny_next_list = True
        run_delivery(binary, store, "ecosystem-defect", api, head_sha, output, "permission-denied", 2)
    finally:
        server.shutdown()
        thread.join(timeout=5)
        server.server_close()
    final_path = validate_and_index(proof, output, head_sha, source, server, cli_identity)
    print(f"GitHub Checks acceptance passed: {final_path}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError, subprocess.SubprocessError, socket.error) as error:
        print(f"github-checks acceptance: {error}", file=sys.stderr)
        raise SystemExit(1)
