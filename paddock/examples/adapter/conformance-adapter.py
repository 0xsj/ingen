#!/usr/bin/env python3
"""Small dependency-free adapter fixture for the Paddock protocol.

This is intentionally synthetic: it demonstrates that a non-Go adapter can
validate the request, preserve the requested root, and return a graph without
needing a parser dependency or Paddock internals.
"""

import json
import os
import sys


def fail(message):
    print("conformance adapter:", message, file=sys.stderr)
    return 2


def main():
    if len(sys.argv) != 3 or sys.argv[1] != "--workspace":
        return fail("usage: conformance-adapter.py --workspace <root>")

    try:
        request = json.load(sys.stdin)
    except (json.JSONDecodeError, OSError) as error:
        return fail(f"invalid request: {error}")

    if request.get("schema") != "paddock.graph-request/v1":
        return fail("unsupported request schema")
    if request.get("language") != "rust":
        return fail("this fixture expects language rust")
    if request.get("source_unit") != "file":
        return fail("this fixture expects source_unit file")
    if "file" not in request.get("required_capabilities", {}).get("source_units", []):
        return fail("file source capability was not negotiated")

    requested_root = os.path.abspath(request.get("root", ""))
    workspace = os.path.abspath(sys.argv[2])
    if not requested_root or requested_root != workspace:
        return fail("--workspace does not match the requested root")

    graph = {
        "schema": "paddock.graph/v1",
        "language": "rust",
        "source_unit": "file",
        "root": request["root"],
        "roots": request.get("roots", []),
        "module_path": "example/conformance",
        "capabilities": {
            "source_units": ["file"],
            "edge_kinds": ["import"],
        },
        "package_count": 2,
        "edge_count": 1,
        "packages": [
            {
                "import_path": "example/conformance/app",
                "path": "src/app/main.rs",
                "labels": {"role": "application", "context": "orders"},
            },
            {
                "import_path": "example/conformance/domain",
                "path": "src/domain/order.rs",
                "labels": {"role": "domain", "context": "orders"},
            },
        ],
        "edges": [
            {
                "from": "example/conformance/app",
                "from_path": "src/app/main.rs",
                "to": "example/conformance/domain",
                "to_path": "src/domain/order.rs",
                "kind": "import",
                "target_kind": "internal",
                "file": "src/app/main.rs",
                "line": 1,
            }
        ],
    }
    json.dump(graph, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
