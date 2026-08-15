#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

import sys

import rns
from rns.errors import Error


def main() -> int:
    ver = rns.version()
    if ver != rns.API_VERSION:
        print(f"unexpected version: {ver}", file=sys.stderr)
        return 1

    node = rns.Node.create()
    try:
        node.start()
        try:
            node.event_poll(10)
        except Error as exc:
            if exc.code != Error.TIMEOUT:
                print(f"expected timeout poll on idle node", file=sys.stderr)
                return 1
        else:
            print("expected timeout poll on idle node", file=sys.stderr)
            return 1
        node.stop()
    finally:
        node.close()

    print("python-smoke ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
