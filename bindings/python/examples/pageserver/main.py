#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io
#
# NomadNet-style pageserver over the Python librns bindings.
# Usage: python3 main.py -c config [-i identity] [-a announce_sec] [-p page_file] [-P request_path]

from __future__ import annotations

import argparse
import sys
import time
from pathlib import Path

import rns
from rns.errors import Error

DEFAULT_ANNOUNCE_SEC = 900
DEFAULT_PAGE_PATH = "/page/index.mu"
DEFAULT_FILE_PATH = "/file/test.txt"
DEFAULT_PAGE_FILE = "pages/index.mu"
DEFAULT_FILE_FILE = "files/test.txt"
DEFAULT_IDENTITY_PATH = "identity"
REQ_DATA_CAP = 64 * 1024

FALLBACK_PAGE = (
    "> Python pageserver\n\n"
    "librns via Reticulum-Go\n\n"
    "Fallback page (file not found).\n\n"
    "`[Home`:/page/index.mu]\n"
    "`[Download Test File`:/file/test.txt]`_`f\n\n"
    "---\n"
)
FALLBACK_FILE = "Test file from Reticulum-Go node!\n"


def print_last_error(what: str) -> None:
    msg = rns.last_error()
    if msg:
        print(f"{what}: {msg}", file=sys.stderr)
    else:
        print(what, file=sys.stderr)


def load_bytes(path: str, fallback: str) -> bytes:
    try:
        return Path(path).read_bytes()
    except OSError:
        print(f"warning: could not read {path}, using built-in content", file=sys.stderr)
        return fallback.encode("utf-8")


def load_or_create_identity(path: str) -> rns.Identity:
    try:
        identity = rns.Identity.load(path)
        print(f"loaded identity from {path}", file=sys.stderr)
        return identity
    except Error:
        identity = rns.Identity.generate()
        identity.save(path)
        print(f"created and saved identity to {path}", file=sys.stderr)
        return identity


def run(
    config_path: str,
    identity_path: str,
    page_file: str,
    file_file: str,
    request_path: str,
    file_path: str,
    announce_sec: int,
) -> int:
    ver = rns.version()
    if ver != rns.API_VERSION:
        print(f"librns version mismatch: got {ver} want {rns.API_VERSION}", file=sys.stderr)
        return 1

    page_body = load_bytes(page_file, FALLBACK_PAGE)
    file_body = load_bytes(file_file, FALLBACK_FILE)

    node = rns.Node.create(config_path)
    identity = load_or_create_identity(identity_path)
    try:
        node.set_identity(identity)
        node.start()

        with rns.Destination.create(node, None, "nomadnetwork", ("node",), True) as dest:
            dest.register_request_handler(request_path)
            dest.register_request_handler(file_path)

            dest_hash = dest.hash()
            dest_hex = rns.hash_to_hex(dest_hash)

            print(f"DEST_HASH={dest_hex}")
            print(f"REQUEST_PATH={request_path}")
            print(f"FILE_PATH={file_path}")
            print(f"librns {ver} pageserver listening as nomadnetwork.node", file=sys.stderr)
            print("announce name=librns-python-pageserver interval={}s".format(announce_sec), file=sys.stderr)
            print(f"serving {len(page_body)} bytes from {page_file}", file=sys.stderr)
            print(f"serving {len(file_body)} bytes from {file_file} as {file_path}", file=sys.stderr)

            app_data = b"librns-python-pageserver"
            try:
                dest.announce(app_data)
                print("announce sent", file=sys.stderr)
            except Error:
                print_last_error("destination.announce failed")

            req_buf = bytearray(REQ_DATA_CAP)
            last_announce = time.monotonic()

            while True:
                if announce_sec > 0 and time.monotonic() - last_announce >= announce_sec:
                    try:
                        dest.announce(app_data)
                        print("announce refreshed", file=sys.stderr)
                    except Error:
                        pass
                    last_announce = time.monotonic()

                try:
                    ev = rns.Event.poll(node, 200, req_buf)
                except Error as exc:
                    if exc.code == Error.TIMEOUT:
                        continue
                    print_last_error("Event.poll failed")
                    return 1

                if ev.kind == rns.EventKind.LINK_ESTABLISHED:
                    print("inbound link established", file=sys.stderr)
                elif ev.kind == rns.EventKind.LINK_CLOSED:
                    print("link closed", file=sys.stderr)
                elif ev.kind == rns.EventKind.REQUEST_INCOMING:
                    path = ev.path()
                    print(f"request incoming path={path}", file=sys.stderr)
                    req_id = ev.request_id()
                    if path == request_path:
                        try:
                            rns.request_respond(node, req_id, page_body)
                            print(f"served {request_path} ({len(page_body)} bytes)", file=sys.stderr)
                        except Error:
                            print_last_error("request_respond failed")
                    elif path == file_path:
                        try:
                            rns.request_respond_file(node, req_id, "test.txt", file_body)
                            print(f"served {file_path} ({len(file_body)} bytes)", file=sys.stderr)
                        except Error:
                            print_last_error("request_respond_file failed")
                    else:
                        try:
                            rns.request_respond(node, req_id, b"page not found\n")
                        except Error:
                            print_last_error("request_respond failed")
    finally:
        identity.close()
        node.close()

    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Serve a NomadNet page over librns.")
    parser.add_argument("-c", required=True, help="Reticulum config file")
    parser.add_argument("-i", default=DEFAULT_IDENTITY_PATH, help="Identity file path")
    parser.add_argument("-a", type=int, default=DEFAULT_ANNOUNCE_SEC, help="Announce interval seconds")
    parser.add_argument("-p", default=DEFAULT_PAGE_FILE, help="Micron page file")
    parser.add_argument("-f", default=DEFAULT_FILE_FILE, help="Download file")
    parser.add_argument("-P", default=DEFAULT_PAGE_PATH, help="Request path to register")
    args = parser.parse_args()
    if args.a < 0:
        print("announce interval must be >= 0", file=sys.stderr)
        return 1
    return run(
        args.c,
        args.i,
        args.p,
        args.f,
        args.P,
        DEFAULT_FILE_PATH,
        args.a,
    )


if __name__ == "__main__":
    sys.exit(main())
