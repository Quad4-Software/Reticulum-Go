#!/usr/bin/env python3
"""Byte shuttle between stdio and a TCP connection.

Spawned by the Go PipeInterface as its subprocess: stdin/stdout are HDLC
framed by the pipe interface, and this bridge forwards the raw bytes to a TCP
socket where a Python RNS TCPServerInterface (or any framed peer) listens.

Usage: pipe_tcp_bridge.py <host> <port>
"""

import os
import select
import socket
import sys


def main() -> int:
    host = sys.argv[1]
    port = int(sys.argv[2])

    sock = socket.create_connection((host, port), timeout=15)
    sock.setblocking(False)

    in_fd = sys.stdin.buffer.fileno()
    out_fd = sys.stdout.buffer.fileno()
    os.set_blocking(in_fd, False)
    os.set_blocking(out_fd, False)

    while True:
        readable, _, _ = select.select([in_fd, sock], [], [], 30)
        if not readable:
            continue
        for fd in readable:
            if fd == in_fd:
                try:
                    data = os.read(in_fd, 65536)
                except BlockingIOError:
                    continue
                if not data:
                    return 0
                sock.sendall(data)
            else:
                try:
                    data = sock.recv(65536)
                except BlockingIOError:
                    continue
                if not data:
                    return 0
                view = memoryview(data)
                while view:
                    try:
                        n = os.write(out_fd, view)
                        view = view[n:]
                    except BlockingIOError:
                        select.select([], [out_fd], [], 30)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (KeyboardInterrupt, BrokenPipeError, ConnectionError):
        sys.exit(0)
