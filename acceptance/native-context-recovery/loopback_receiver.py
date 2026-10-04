#!/usr/bin/env python3
"""Small loopback control endpoint for proving the sandbox blocked a live target."""

import argparse
import socket
import time
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--address-file", required=True)
    parser.add_argument("--accepted-file", required=True)
    args = parser.parse_args()
    address_file = Path(args.address_file)
    accepted_file = Path(args.accepted_file)
    with socket.socket() as server:
        server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        server.bind(("127.0.0.1", 0))
        server.listen(8)
        server.settimeout(0.2)
        address_file.write_text(f"127.0.0.1:{server.getsockname()[1]}\n", encoding="ascii")
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            try:
                connection, _ = server.accept()
            except TimeoutError:
                continue
            with connection:
                connection.settimeout(0.2)
                try:
                    payload = connection.recv(256)
                except TimeoutError:
                    payload = b""
                with accepted_file.open("ab") as output:
                    output.write(payload + b"\n")
        return 0


if __name__ == "__main__":
    raise SystemExit(main())
