#!/usr/bin/env python3
"""Exercise the standalone bundle verifier with Python standard library only."""

from __future__ import annotations

import argparse
import atexit
import hashlib
import json
import os
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path, PurePosixPath


MAX_ARCHIVE_BYTES = 256 * 1024 * 1024
MAX_EXPANDED_BYTES = 2 * 1024 * 1024 * 1024
MAX_MEMBERS = 100_000


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def safe_path(name: str) -> PurePosixPath:
    if not name or "\\" in name or "\x00" in name:
        raise RuntimeError("archive contains an invalid member path")
    path = PurePosixPath(name)
    if path.is_absolute() or str(path) != name or any(part in ("", ".", "..") for part in path.parts):
        raise RuntimeError("archive contains a non-normalized or escaping member path")
    return path


def run_helper(helper: Path, env: dict[str, str], *args: str, expected: int = 0) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        [sys.executable, "-I", "-S", str(helper), *args],
        cwd=env["TMPDIR"],
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=120,
        check=False,
    )
    if result.returncode != expected:
        detail = (result.stderr or result.stdout).strip()[-800:]
        raise RuntimeError(f"standalone helper exited {result.returncode}, expected {expected}: {detail}")
    return result


def compare_archive_bytes(archive_path: Path, installed: Path, expected_manifest_sha: str) -> int:
    manifest_path = installed / "manifest.json"
    manifest_bytes = manifest_path.read_bytes()
    if hashlib.sha256(manifest_bytes).hexdigest() != expected_manifest_sha:
        raise RuntimeError("installed manifest digest changed")
    manifest = json.loads(manifest_bytes)
    expected = {row["path"]: (row["size"], row["sha256"]) for row in manifest["files"]}
    expected["manifest.json"] = (len(manifest_bytes), expected_manifest_sha)
    sidecar = installed / "manifest.sha256"
    expected["manifest.sha256"] = (sidecar.stat().st_size, sha256_file(sidecar))
    expected_dirs = {
        parent.as_posix()
        for name in expected
        for parent in PurePosixPath(name).parents
        if parent.as_posix() != "."
    }
    seen_files: set[str] = set()
    seen_dirs: set[str] = set()
    expanded = 0
    count = 0
    with tarfile.open(archive_path, mode="r|gz") as archive:
        for member in archive:
            count += 1
            if count > MAX_MEMBERS:
                raise RuntimeError("archive contains too many members during byte comparison")
            if member.name.endswith("/"):
                raise RuntimeError("archive member path is not canonical")
            relative = safe_path(member.name).as_posix()
            if member.isdir():
                if relative not in expected_dirs or relative in seen_dirs or member.size != 0:
                    raise RuntimeError("archive directory inventory changed during byte comparison")
                seen_dirs.add(relative)
                continue
            if not member.isfile() or relative not in expected or relative in seen_files:
                raise RuntimeError("archive file inventory changed during byte comparison")
            expected_size, expected_sha = expected[relative]
            if member.size != expected_size:
                raise RuntimeError(f"archive size differs from installed file: {relative}")
            archived = archive.extractfile(member)
            if archived is None:
                raise RuntimeError(f"archive file is unreadable: {relative}")
            installed_file = installed.joinpath(*PurePosixPath(relative).parts)
            if installed_file.is_symlink() or not installed_file.is_file():
                raise RuntimeError(f"installed file is missing or not regular: {relative}")
            digest = hashlib.sha256()
            remaining = member.size
            with installed_file.open("rb") as local:
                while remaining:
                    amount = min(1024 * 1024, remaining)
                    left = archived.read(amount)
                    right = local.read(amount)
                    if len(left) != amount or left != right:
                        raise RuntimeError(f"archive and installed bytes differ: {relative}")
                    digest.update(left)
                    remaining -= len(left)
                if local.read(1):
                    raise RuntimeError(f"installed file has unexpected trailing bytes: {relative}")
            if digest.hexdigest() != expected_sha:
                raise RuntimeError(f"archive digest differs from installed manifest: {relative}")
            expanded += member.size
            if expanded > MAX_EXPANDED_BYTES:
                raise RuntimeError("archive exceeds expanded size limit")
            seen_files.add(relative)
    if seen_files != set(expected) or seen_dirs != expected_dirs:
        raise RuntimeError("archive and installed inventories differ")
    return len(seen_files)


def assert_container_constraints() -> None:
    if os.getuid() == 0 or os.geteuid() == 0:
        raise RuntimeError("cleanroom process unexpectedly runs as root")
    status = Path("/proc/self/status").read_text(encoding="ascii")
    capeff = next((line.split()[1] for line in status.splitlines() if line.startswith("CapEff:")), None)
    no_new_privs = next((line.split()[1] for line in status.splitlines() if line.startswith("NoNewPrivs:")), None)
    if capeff is None or int(capeff, 16) != 0 or no_new_privs != "1":
        raise RuntimeError("container capability or no-new-privileges boundary is missing")
    mountinfo = Path("/proc/self/mountinfo").read_text(encoding="ascii")
    root_mount = next((line for line in mountinfo.splitlines() if " - " in line and line.split(" - ", 1)[0].split()[4] == "/"), None)
    if root_mount is None or "ro" not in root_mount.split(" - ", 1)[0].split()[5].split(","):
        raise RuntimeError("container root filesystem is not read-only")
    routes = Path("/proc/net/route")
    if routes.exists():
        rows = routes.read_text(encoding="ascii").splitlines()[1:]
        if any(row.split()[1] == "00000000" for row in rows if len(row.split()) > 1):
            raise RuntimeError("container unexpectedly has a default network route")


def main() -> int:
    # Do not carry CI/user credentials into any child process or helper call.
    os.environ.clear()
    parser = argparse.ArgumentParser()
    parser.add_argument("--helper", required=True, type=Path)
    parser.add_argument("--helper-sha256", required=True)
    parser.add_argument("--archive", required=True, type=Path)
    parser.add_argument("--archive-sha256", required=True)
    parser.add_argument("--manifest-sha256", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--work-parent", required=True, type=Path)
    parser.add_argument("--container-hardening", action="store_true")
    args = parser.parse_args()

    if args.container_hardening:
        assert_container_constraints()
    if not args.helper.is_file() or args.helper.is_symlink() or sha256_file(args.helper) != args.helper_sha256:
        raise RuntimeError("standalone helper bootstrap digest mismatch")
    if not args.archive.is_file() or args.archive.is_symlink() or args.archive.stat().st_size > MAX_ARCHIVE_BYTES:
        raise RuntimeError("archive input is missing, unsafe, or oversized")
    if sha256_file(args.archive) != args.archive_sha256:
        raise RuntimeError("archive input digest mismatch")
    if not all(len(value) == 64 and all(ch in "0123456789abcdef" for ch in value) for value in (args.helper_sha256, args.archive_sha256, args.manifest_sha256)):
        raise RuntimeError("expected digest argument is malformed")
    work_parent = args.work_parent.resolve(strict=True)
    state_root = Path(tempfile.mkdtemp(prefix="archive-cleanroom-state-", dir=work_parent))
    atexit.register(shutil.rmtree, state_root, True)
    python_bin = state_root / "python-bin"
    python_bin.mkdir(mode=0o700)
    python_link = python_bin / "python3"
    python_link.symlink_to(Path(sys.executable).resolve(strict=True))
    for name in ("go", "cargo", "rustc", "node", "npm", "npx"):
        if shutil.which(name, path=str(python_bin)) is not None:
            raise RuntimeError(f"cleanroom tool PATH unexpectedly contains {name}")
    env = {
        "PATH": str(python_bin),
        "HOME": str(state_root / "empty-home"),
        "TMPDIR": str(state_root),
        "PYTHONDONTWRITEBYTECODE": "1",
        "LANG": "C",
        "LC_ALL": "C",
    }
    Path(env["HOME"]).mkdir(mode=0o700)
    with tempfile.TemporaryDirectory(prefix="archive-cleanroom-", dir=state_root) as work_text:
        work = Path(work_text).resolve(strict=True)
        env["TMPDIR"] = str(work)
        helper = args.helper.resolve(strict=True)
        archive = args.archive.resolve(strict=True)
        helper_args = ("--manifest-sha256", args.manifest_sha256, "--target", args.target)
        run_helper(helper, env, "verify-archive", "--archive", str(archive), *helper_args)
        installed = work / "installed"
        run_helper(helper, env, "install-archive", "--archive", str(archive), "--prefix", str(installed), *helper_args)
        installed_helper = installed / "share" / "ingen-package.py"
        if sha256_file(installed_helper) != args.helper_sha256:
            raise RuntimeError("installed standalone helper differs from bootstrap helper")
        run_helper(installed_helper, env, "verify", "--bundle", str(installed), *helper_args)
        file_count = compare_archive_bytes(archive, installed, args.manifest_sha256)

        tampered = work / "tampered.tar.gz"
        payload = bytearray(archive.read_bytes())
        if len(payload) < 32:
            raise RuntimeError("archive is too small to tamper safely")
        payload[len(payload) // 2] ^= 0x01
        tampered.write_bytes(payload)
        tampered.chmod(0o600)
        untouched_prefix = work / "must-remain-absent"
        run_helper(
            helper,
            env,
            "install-archive",
            "--archive",
            str(tampered),
            "--prefix",
            str(untouched_prefix),
            *helper_args,
            expected=2,
        )
        if untouched_prefix.exists():
            if not untouched_prefix.is_dir() or any(untouched_prefix.iterdir()):
                raise RuntimeError("tampered archive left files in its rejected install prefix")
        print(f"archive cleanroom passed: files={file_count} target={args.target} native_tools=not-executed")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError, tarfile.TarError, subprocess.SubprocessError) as error:
        print(f"archive-cleanroom: {error}", file=sys.stderr)
        raise SystemExit(1)
