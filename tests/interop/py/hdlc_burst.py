#!/usr/bin/env python3
"""Burst HDLC-over-TCP peer for Go stream-read interop.

Sends or receives many HDLC frames in one TCP write/read.

Environment:
    INTEROP_PORT          listen port (server)
    INTEROP_MODE          server (default) or client
    INTEROP_TARGET_PORT   dial port when mode=client
    INTEROP_FRAMES        frame count (default 64)
    INTEROP_FAULT         none (default), corrupt, drop, reorder, or flap
"""

import os
import socket
import sys

HDLC_FLAG = 0x7E
HDLC_ESC = 0x7D
HDLC_ESC_MASK = 0x20


def escape(data: bytes) -> bytes:
    out = bytearray()
    for b in data:
        if b in (HDLC_FLAG, HDLC_ESC):
            out.append(HDLC_ESC)
            out.append(b ^ HDLC_ESC_MASK)
        else:
            out.append(b)
    return bytes(out)


def frame(payload: bytes) -> bytes:
    return bytes([HDLC_FLAG]) + escape(payload) + bytes([HDLC_FLAG])


class HDLCDecoder:
    def __init__(self):
        self.in_frame = False
        self.esc = False
        self.buf = bytearray()

    def feed(self, data: bytes):
        packets = []
        for b in data:
            if b == HDLC_FLAG:
                if self.in_frame and self.buf:
                    packets.append(bytes(self.buf))
                    self.buf.clear()
                self.in_frame = not self.in_frame
                self.esc = False
                continue
            if not self.in_frame:
                continue
            if b == HDLC_ESC:
                self.esc = True
                continue
            if self.esc:
                b ^= HDLC_ESC_MASK
                self.esc = False
            self.buf.append(b)
        return packets


def payload_for(i: int) -> bytes:
    body = bytes([i & 0xFF]) * 24
    return bytes([0x7E, 0x7D]) + body


def blob(n: int) -> bytes:
    out = bytearray()
    for i in range(n):
        out.extend(frame(payload_for(i)))
    return bytes(out)


def blob_with_fault(n: int, fault: str) -> bytes:
    fault = (fault or "none").strip().lower()
    frames = [frame(payload_for(i)) for i in range(n)]
    if fault == "drop" and n > 2:
        del frames[n // 2]
        return b"".join(frames)
    if fault == "reorder" and n > 2:
        frames[0], frames[1] = frames[1], frames[0]
        return b"".join(frames)
    if fault == "corrupt" and n > 2:
        broken = bytearray(frames[1])
        for i in range(1, len(broken) - 1):
            if broken[i] not in (HDLC_FLAG, HDLC_ESC):
                broken[i] ^= 0x01
                break
        frames[1] = bytes(broken)
        return b"".join(frames)
    return b"".join(frames)


def serve(port: int, n: int) -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(("127.0.0.1", port))
    sock.listen(1)
    sock.settimeout(30.0)
    sys.stdout.write("READY\n")
    sys.stdout.flush()
    conn, _ = sock.accept()
    conn.settimeout(10.0)
    dec = HDLCDecoder()
    got = []
    while len(got) < n:
        chunk = conn.recv(65536)
        if not chunk:
            break
        got.extend(dec.feed(chunk))
    sys.stdout.write("COUNT %d\n" % len(got))
    sys.stdout.flush()
    return 0 if len(got) == n else 1


def client(target_port: int, n: int, fault: str) -> int:
    fault = (fault or "none").strip().lower()
    if fault == "flap":
        half = max(1, n // 2)
        conn = socket.create_connection(("127.0.0.1", target_port), timeout=10.0)
        conn.sendall(blob_with_fault(half, "none"))
        conn.close()
        conn = socket.create_connection(("127.0.0.1", target_port), timeout=10.0)
        rest = bytearray()
        for i in range(half, n):
            rest.extend(frame(payload_for(i)))
        conn.sendall(bytes(rest))
        conn.close()
        return 0
    conn = socket.create_connection(("127.0.0.1", target_port), timeout=10.0)
    conn.sendall(blob_with_fault(n, fault))
    conn.close()
    return 0


def main() -> int:
    n = int(os.environ.get("INTEROP_FRAMES", "64"))
    mode = os.environ.get("INTEROP_MODE", "server").strip().lower()
    fault = os.environ.get("INTEROP_FAULT", "none")
    if mode == "client":
        port = int(os.environ["INTEROP_TARGET_PORT"])
        return client(port, n, fault)
    port = int(os.environ["INTEROP_PORT"])
    return serve(port, n)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
