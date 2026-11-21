#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

import sys

import lxmf


def main() -> int:
    identity = lxmf.identity_generate()
    try:
        dest = lxmf.identity_hash(identity)
        msg = lxmf.message_create(dest, dest, "hi", "hello lxmf")
        try:
            data = lxmf.message_pack(msg, identity)
            got = lxmf.message_unpack(data)
            try:
                if lxmf.message_get_content(got) != "hello lxmf":
                    print("content mismatch", file=sys.stderr)
                    return 1
            finally:
                lxmf.message_destroy(got)
        finally:
            lxmf.message_destroy(msg)
    finally:
        lxmf.identity_destroy(identity)

    print("python-lxmf-roundtrip ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
