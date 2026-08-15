#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# NomadNet-style page fetch over the Python librns bindings.
# Usage: python3 main.py [-c config] [-t timeout_sec] <dest_hash>:<page_path>

from __future__ import annotations

import argparse
import sys
import time

import rns
from rns.errors import Error

PAGE_BUF_CAP = 512 * 1024
DEFAULT_TIMEOUT_SEC = 60
PATH_RETRY_SEC = 2


def print_last_error(what: str) -> None:
    msg = rns.last_error()
    if msg:
        print(f"{what}: {msg}", file=sys.stderr)
    else:
        print(what, file=sys.stderr)


def parse_target(target: str) -> tuple[bytes, str]:
    if ":" not in target:
        raise ValueError("missing colon")
    hex_part, page_path = target.split(":", 1)
    if not hex_part or not page_path:
        raise ValueError("empty hash or path")
    return rns.hex_to_hash(hex_part), page_path


def run(config_path: str, target: str, timeout_sec: int) -> int:
    ver = rns.version()
    if ver != rns.API_VERSION:
        print(f"librns version mismatch: got {ver} want {rns.API_VERSION}", file=sys.stderr)
        return 1

    try:
        dest_hash, page_path = parse_target(target)
    except (ValueError, Error):
        print("target must be <32-hex-dest>:<page_path>", file=sys.stderr)
        return 1

    dest_hex = rns.hash_to_hex(dest_hash)

    node = rns.Node.create(config_path)
    identity = rns.Identity.generate()
    try:
        node.set_identity(identity)
        node.start()

        print(f"librns {ver} fetching {page_path} from {dest_hex}")

        page_buf = bytearray(PAGE_BUF_CAP)
        deadline = time.monotonic() + timeout_sec
        last_path_req = 0.0
        need_path_req = True
        saw_announce = False
        link = None

        while time.monotonic() < deadline and link is None:
            now = time.monotonic()
            if need_path_req or now - last_path_req >= PATH_RETRY_SEC:
                try:
                    rns.path_request(node, dest_hash)
                except Error:
                    print_last_error("path_request failed")
                last_path_req = now
                need_path_req = False
                if rns.path_known(node, dest_hash):
                    print("path known, waiting for destination identity announce", file=sys.stderr)
                else:
                    print(f"requesting path to {dest_hex}", file=sys.stderr)

            try:
                ev = rns.Event.poll(node, 200, page_buf)
            except Error as exc:
                if exc.code == Error.TIMEOUT:
                    if saw_announce or rns.path_known(node, dest_hash):
                        try:
                            link = rns.Link.open(node, dest_hash)
                        except Error:
                            link = None
                    continue
                print_last_error("Event.poll failed")
                return 1

            if ev.kind == rns.EventKind.ANNOUNCE and rns.hash_eq(ev.destination_hash(), dest_hash):
                saw_announce = True
                print(f"announce for target (hops={ev.hops})", file=sys.stderr)
                try:
                    link = rns.Link.open(node, dest_hash)
                except Error:
                    print_last_error("Link.open after announce")
            elif ev.kind == rns.EventKind.LINK_FAILED:
                print(f"link failed while opening: {ev.error_message()}", file=sys.stderr)

        if link is None:
            print("timed out before link open", file=sys.stderr)
            return 1

        with link:
            established = False
            while time.monotonic() < deadline and not established:
                try:
                    ev = rns.Event.poll(node, 500, page_buf)
                except Error as exc:
                    if exc.code == Error.TIMEOUT:
                        continue
                    print_last_error("Event.poll failed")
                    return 1

                if ev.kind == rns.EventKind.LINK_ESTABLISHED:
                    established = True
                    print("link established", file=sys.stderr)
                elif ev.kind == rns.EventKind.LINK_FAILED:
                    print(f"link establishment failed: {ev.error_message()}", file=sys.stderr)
                    return 1
                elif ev.kind == rns.EventKind.LINK_CLOSED:
                    print("link closed before establish", file=sys.stderr)
                    return 1

            if not established:
                print("timed out waiting for link establishment", file=sys.stderr)
                return 1

            remaining_ms = int(max((deadline - time.monotonic()) * 1000, 1000))
            try:
                link.request(node, page_path, b"", remaining_ms)
            except Error:
                print_last_error("link.request failed")
                return 1
            print(f"request sent for {page_path}", file=sys.stderr)

            while time.monotonic() < deadline:
                try:
                    ev = rns.Event.poll(node, 500, page_buf)
                except Error as exc:
                    if exc.code == Error.TIMEOUT:
                        continue
                    print_last_error("Event.poll failed")
                    return 1

                if ev.kind == rns.EventKind.REQUEST_RESPONSE:
                    data = ev.app_data()
                    print(f"\n=== Page Content ({len(data)} bytes) ===")
                    if data:
                        sys.stdout.buffer.write(data)
                        if data[-1:] != b"\n":
                            sys.stdout.write("\n")
                    if ev.app_data_truncated:
                        print(f"warning: response truncated to {PAGE_BUF_CAP} bytes", file=sys.stderr)
                    print("=== End of Page ===")
                    return 0
                if ev.kind == rns.EventKind.REQUEST_FAILED:
                    print(f"request failed: {ev.error_message()}", file=sys.stderr)
                    return 1
                if ev.kind == rns.EventKind.LINK_CLOSED:
                    print("link closed before response", file=sys.stderr)
                    return 1

            print("timed out waiting for page response", file=sys.stderr)
            return 1
    finally:
        identity.close()
        node.close()


def main() -> int:
    parser = argparse.ArgumentParser(description="Fetch a NomadNet page over librns.")
    parser.add_argument("-c", required=True, help="Reticulum config file")
    parser.add_argument("-t", type=int, default=DEFAULT_TIMEOUT_SEC, help="Timeout in seconds")
    parser.add_argument("target", help="<dest_hash>:<page_path>")
    args = parser.parse_args()
    if args.t <= 0:
        print("timeout must be positive", file=sys.stderr)
        return 1
    return run(args.c, args.target, args.t)


if __name__ == "__main__":
    sys.exit(main())
