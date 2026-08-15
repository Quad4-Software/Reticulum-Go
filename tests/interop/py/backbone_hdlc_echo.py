#!/usr/bin/env python3
"""Raw HDLC-over-TCP echo peer for Go backbone wire interop.

Environment:
    INTEROP_PORT          TCP listen port (server mode)
    INTEROP_MODE          server (default) or client
    INTEROP_TARGET_PORT   dial port when mode=client
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


def unescape(data: bytes) -> bytes:
    out = bytearray()
    esc = False
    for b in data:
        if esc:
            out.append(b ^ HDLC_ESC_MASK)
            esc = False
            continue
        if b == HDLC_ESC:
            esc = True
            continue
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
                    packets.append(unescape(bytes(self.buf)))
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


def serve(port: int) -> int:
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
    while True:
        chunk = conn.recv(65536)
        if not chunk:
            break
        for pkt in dec.feed(chunk):
            conn.sendall(frame(pkt))
    return 0


def client(target_port: int) -> int:
    conn = socket.create_connection(("127.0.0.1", target_port), timeout=10.0)
    # Must exceed RNS BackboneClientInterface HEADER_MINSIZE (19).
    payload = bytes([0x42, 0x43, 0x44]) * 8
    conn.sendall(frame(payload))
    dec = HDLCDecoder()
    while True:
        chunk = conn.recv(65536)
        if not chunk:
            break
        for pkt in dec.feed(chunk):
            sys.stdout.write(pkt.hex() + "\n")
            sys.stdout.flush()
            if pkt == payload:
                return 0
    return 1


def main() -> int:
    mode = os.environ.get("INTEROP_MODE", "server").strip().lower()
    if mode == "client":
        port = int(os.environ["INTEROP_TARGET_PORT"])
        return client(port)
    port = int(os.environ["INTEROP_PORT"])
    return serve(port)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
