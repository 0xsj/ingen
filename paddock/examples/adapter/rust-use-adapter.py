#!/usr/bin/env python3
"""Small dependency-free Rust adapter for a constrained use/module subset.

This example deliberately parses only ordinary `use path::to::item;` and
`mod name;` declarations. A production Rust adapter should use a Rust-aware
parser, but the Paddock boundary and graph shape remain the same.
"""

import argparse
import fnmatch
import json
import os
import re
import sys


ADAPTER_NAME = "paddock-rust-use"
ADAPTER_VERSION = "1.0.0"
GRAPH_SCHEMA = "paddock.graph/v1"
REQUEST_SCHEMA = "paddock.graph-request/v1"
USE_RE = re.compile(r"^\s*use\s+([^;]+);\s*$")
MOD_RE = re.compile(r"^\s*(?:pub\s+)?mod\s+([A-Za-z_][A-Za-z0-9_]*)\s*;\s*$")
STANDARD_ROOTS = {"alloc", "core", "std"}


def fail(message):
    print(f"Rust use adapter: {message}", file=sys.stderr)
    return 2


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", required=True)
    return parser.parse_args()


def slash(path):
    return path.replace(os.sep, "/")


def selected(relative, request):
    roots = request.get("roots") or []
    if roots and not any(
        relative == root or relative.startswith(root.rstrip("/") + "/")
        for root in roots
    ):
        return False

    includes = request.get("include") or []
    excludes = request.get("exclude") or []
    if includes and not any(fnmatch.fnmatchcase(relative, pattern) for pattern in includes):
        return False
    if any(fnmatch.fnmatchcase(relative, pattern) for pattern in excludes):
        return False
    return True


def discover(root, request):
    files = []
    for directory, directories, names in os.walk(root):
        directories[:] = sorted(
            name
            for name in directories
            if name not in {".git", "target"} and not name.startswith(".")
        )
        for name in sorted(names):
            if not name.endswith(".rs"):
                continue
            absolute = os.path.join(directory, name)
            relative = slash(os.path.relpath(absolute, root))
            if selected(relative, request):
                files.append((absolute, relative))
    return files


def module_name(relative):
    parts = slash(relative)[:-3].split("/")
    if parts and parts[0] == "src":
        parts.pop(0)
    if parts[-1] == "mod":
        parts.pop()
    elif parts[-1] in {"lib", "main"} and len(parts) == 1:
        return "crate" if parts[-1] == "lib" else "crate::main"
    if not parts:
        return "crate"
    return "crate::" + "::".join(parts)


def resolve_target(imported, modules):
    parts = imported.split("::") if imported else []
    for end in range(len(parts), 0, -1):
        candidate = "::".join(parts[:end])
        if candidate in modules:
            return candidate
    return None


def import_edges(absolute, relative, current, modules, local_roots):
    try:
        with open(absolute, "r", encoding="utf-8") as source:
            lines = source.readlines()
    except OSError as error:
        raise ValueError(f"read {relative}: {error}") from error

    edges = []
    for line_number, line in enumerate(lines, start=1):
        content = line.split("//", 1)[0].strip()
        use_match = USE_RE.match(content)
        mod_match = MOD_RE.match(content)
        if use_match:
            imported = use_match.group(1).strip()
            if "{" in imported:
                imported = imported.split("::{", 1)[0]
            if " as " in imported:
                imported = imported.split(" as ", 1)[0].strip()
        elif mod_match:
            imported = "::".join(part for part in [current, mod_match.group(1)] if part)
        else:
            continue

        target = resolve_target(imported, modules)
        root_name = imported.split("::", 1)[0]
        if target is not None:
            target_kind = "internal"
            to_path = modules[target]["path"]
        elif root_name in STANDARD_ROOTS:
            target_kind = "standard-library"
            imported = root_name
            to_path = ""
        elif root_name in local_roots or root_name == "crate":
            target_kind = "unresolved"
            to_path = ""
        else:
            target_kind = "external"
            to_path = ""

        edge = {
            "from": current,
            "from_path": relative,
            "to": target or imported,
            "kind": "import",
            "target_kind": target_kind,
            "file": relative,
            "line": line_number,
        }
        if to_path:
            edge["to_path"] = to_path
        edges.append(edge)
    return edges


def build_graph(root, request):
    discovered = discover(root, request)
    if not discovered:
        raise ValueError(f"no Rust files selected under {root}")

    modules = {}
    packages = []
    for absolute, relative in discovered:
        name = module_name(relative)
        package = {"import_path": name, "path": relative}
        modules[name] = package
        packages.append(package)

    local_roots = {name.split("::", 1)[0] for name in modules}
    edges = []
    for absolute, relative in discovered:
        current = module_name(relative)
        edges.extend(import_edges(absolute, relative, current, modules, local_roots))

    packages.sort(key=lambda package: (package["import_path"], package["path"]))
    edges.sort(key=lambda edge: (edge["from"], edge["file"], edge["line"], edge["to"]))
    return {
        "schema": GRAPH_SCHEMA,
        "language": "rust",
        "source_unit": "file",
        "root": request["root"],
        "roots": request.get("roots", []),
        "module_path": os.path.basename(os.path.abspath(root)) or "rust-project",
        "adapter": {
            "kind": "external",
            "name": ADAPTER_NAME,
            "version": ADAPTER_VERSION,
        },
        "capabilities": {
            "source_units": ["file"],
            "edge_kinds": ["import"],
        },
        "package_count": len(packages),
        "edge_count": len(edges),
        "packages": packages,
        "edges": edges,
    }


def main():
    try:
        arguments = parse_args()
        request = json.load(sys.stdin)
        if request.get("schema") != REQUEST_SCHEMA:
            return fail("unsupported request schema")
        if request.get("language") != "rust":
            return fail("this example supports language rust")
        if request.get("source_unit") != "file":
            return fail("this example supports source_unit file")
        if "file" not in request.get("required_capabilities", {}).get("source_units", []):
            return fail("file source capability was not negotiated")

        root = request.get("root")
        workspace = os.path.abspath(arguments.workspace)
        if not isinstance(root, str) or not root:
            return fail("request root is required")
        if os.path.abspath(root) != workspace:
            return fail("--workspace does not match the requested root")

        graph = build_graph(workspace, request)
        json.dump(graph, sys.stdout, indent=2, sort_keys=True)
        sys.stdout.write("\n")
        return 0
    except (OSError, TypeError, ValueError, json.JSONDecodeError) as error:
        return fail(str(error))


if __name__ == "__main__":
    sys.exit(main())
