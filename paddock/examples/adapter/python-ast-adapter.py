#!/usr/bin/env python3
"""Small dependency-free Python AST adapter for Paddock.

This is an adapter-authoring example, not a production Python analyzer. It
shows the important boundary: Python owns discovery, syntax parsing, import
resolution, and edge classification; Paddock owns architecture policy.
"""

import argparse
import ast
import fnmatch
import json
import os
import sys


ADAPTER_NAME = "paddock-python-ast"
ADAPTER_VERSION = "1.0.0"
GRAPH_SCHEMA = "paddock.graph/v1"
REQUEST_SCHEMA = "paddock.graph-request/v1"


def fail(message):
    print(f"python AST adapter: {message}", file=sys.stderr)
    return 2


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--workspace", required=True)
    return parser.parse_args()


def slash(path):
    return path.replace(os.sep, "/")


def selected(rel_path, request):
    roots = request.get("roots") or []
    if roots and not any(
        rel_path == root or rel_path.startswith(root.rstrip("/") + "/")
        for root in roots
    ):
        return False

    includes = request.get("include") or []
    excludes = request.get("exclude") or []
    if includes and not any(fnmatch.fnmatchcase(rel_path, pattern) for pattern in includes):
        return False
    if any(fnmatch.fnmatchcase(rel_path, pattern) for pattern in excludes):
        return False
    return True


def discover(root, request):
    files = []
    for directory, directories, names in os.walk(root):
        directories[:] = sorted(
            name
            for name in directories
            if name not in {".git", ".venv", "__pycache__"} and not name.startswith(".")
        )
        for name in sorted(names):
            if not name.endswith(".py"):
                continue
            absolute = os.path.join(directory, name)
            relative = slash(os.path.relpath(absolute, root))
            if selected(relative, request):
                files.append((absolute, relative))
    return files


def module_name(relative):
    parts = slash(relative)[:-3].split("/")
    if parts[-1] == "__init__":
        parts.pop()
    return ".".join(parts)


def relative_base(current, package_file, level, module):
    if level == 0:
        return module or ""
    parts = current.split(".") if current else []
    package = parts if package_file else parts[:-1]
    package = package[: max(0, len(package) - level + 1)]
    if module:
        package.extend(module.split("."))
    return ".".join(part for part in package if part)


def resolve_target(imported, modules):
    parts = imported.split(".") if imported else []
    for end in range(len(parts), 0, -1):
        candidate = ".".join(parts[:end])
        if candidate in modules:
            return candidate
    return None


def standard_library_names():
    names = getattr(sys, "stdlib_module_names", None)
    if names is not None:
        return set(names)
    return {
        "abc",
        "ast",
        "collections",
        "dataclasses",
        "functools",
        "json",
        "os",
        "pathlib",
        "sys",
        "typing",
    }


def import_edges(absolute, relative, current, package_file, modules, local_roots):
    try:
        with open(absolute, "r", encoding="utf-8") as source:
            tree = ast.parse(source.read(), filename=absolute)
    except (OSError, SyntaxError) as error:
        raise ValueError(f"parse {relative}: {error}") from error

    standard = standard_library_names()
    edges = []
    for node in ast.walk(tree):
        imported_names = []
        if isinstance(node, ast.Import):
            imported_names = [alias.name for alias in node.names]
        elif isinstance(node, ast.ImportFrom):
            base = relative_base(current, package_file, node.level, node.module)
            if node.level == 0:
                # The imported module is the dependency for an absolute
                # from-import; the member is a symbol, not another module.
                imported_names = [base]
            else:
                imported_names = [
                    ".".join(part for part in [base, alias.name] if part)
                    for alias in node.names
                ]

        for imported in imported_names:
            target = resolve_target(imported, modules)
            root_name = imported.split(".", 1)[0]
            if target is not None:
                target_kind = "internal"
                to_path = modules[target]["path"]
            elif root_name in local_roots:
                target_kind = "unresolved"
                to_path = ""
            elif root_name in standard:
                target_kind = "standard-library"
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
                "line": node.lineno,
            }
            if to_path:
                edge["to_path"] = to_path
            edges.append(edge)
    return edges


def build_graph(root, request):
    discovered = discover(root, request)
    if not discovered:
        raise ValueError(f"no Python files selected under {root}")

    modules = {}
    packages = []
    for absolute, relative in discovered:
        name = module_name(relative)
        if not name:
            continue
        package = {"import_path": name, "path": relative}
        modules[name] = package
        packages.append(package)

    local_roots = {name.split(".", 1)[0] for name in modules}
    edges = []
    for absolute, relative in discovered:
        current = module_name(relative)
        if not current:
            continue
        package_file = relative.endswith("/__init__.py") or relative == "__init__.py"
        edges.extend(import_edges(absolute, relative, current, package_file, modules, local_roots))

    packages.sort(key=lambda package: (package["import_path"], package["path"]))
    edges.sort(key=lambda edge: (edge["from"], edge["file"], edge["line"], edge["to"]))
    return {
        "schema": GRAPH_SCHEMA,
        "language": "python",
        "source_unit": "file",
        "root": request["root"],
        "roots": request.get("roots", []),
        "module_path": os.path.basename(os.path.abspath(root)) or "python-project",
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
        if request.get("language") != "python":
            return fail("this example supports language python")
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
