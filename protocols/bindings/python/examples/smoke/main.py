#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

import sys

import rrc
from rrc.errors import Error


def main() -> int:
    ver = rrc.version()
    if ver != rrc.API_VERSION:
        print(f"unexpected version: {ver}", file=sys.stderr)
        return 1

    node = rrc.Node.create()
    try:
        node.start()
        node.stop()
    finally:
        node.close()

    with rrc.Identity.generate() as identity:
        if len(identity.hash_bytes()) != rrc.HASH_LEN:
            print("bad identity hash length", file=sys.stderr)
            return 1

    print("python-smoke ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
