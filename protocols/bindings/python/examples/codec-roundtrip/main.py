#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

import sys

import rrc


def main() -> int:
    sender = bytes(range(16))
    with rrc.Envelope.create(rrc.ffi.RRC_TYPE_MSG, sender) as env:
        env.set_room("lobby")
        env.set_nick("alice")
        env.set_body_text("codec roundtrip")
        data = env.marshal()

    with rrc.Envelope.unmarshal(data) as got:
        if got.body_text() != "codec roundtrip":
            print("body mismatch", file=sys.stderr)
            return 1

    print("python-codec-roundtrip ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
