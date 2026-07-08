#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# HDLC echo helper for PipeInterface interop tests.

import os
import sys

FLAG = 0x7E
ESC = 0x7D
MASK = 0x20


def escape(data: bytes) -> bytes:
    out = bytearray()
    for b in data:
        if b == ESC:
            out.extend([ESC, ESC ^ MASK])
        elif b == FLAG:
            out.extend([ESC, FLAG ^ MASK])
        else:
            out.append(b)
    return bytes(out)


def unescape(data: bytes) -> bytes:
    out = bytearray()
    i = 0
    while i < len(data):
        if data[i] == ESC and i + 1 < len(data):
            out.append(data[i + 1] ^ MASK)
            i += 2
            continue
        out.append(data[i])
        i += 1
    return bytes(out)


def main() -> None:
    buf = b""
    in_frame = False
    while True:
        chunk = os.read(0, 4096)
        if not chunk:
            break
        for b in chunk:
            if b == FLAG:
                if in_frame and buf:
                    frame = unescape(buf)
                    sys.stdout.buffer.write(bytes([FLAG]) + escape(frame) + bytes([FLAG]))
                    sys.stdout.buffer.flush()
                buf = b""
                in_frame = True
                continue
            if in_frame:
                buf += bytes([b])


if __name__ == "__main__":
    main()
