#!/usr/bin/env python3
# Firecracker vsock CONNECT stdio bridge for PipeInterface.
# Stdin/stdout carry HDLC. Stdin EOF half-closes the socket write side.

import os
import select
import socket
import sys


def main() -> int:
    if len(sys.argv) < 3:
        print("usage: vsock_connect.py UDS_PATH PORT", file=sys.stderr)
        return 2
    uds_path, port = sys.argv[1], sys.argv[2]
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(uds_path)
    s.sendall(f"CONNECT {port}\n".encode("ascii"))

    buf = b""
    while b"\n" not in buf:
        chunk = s.recv(64)
        if not chunk:
            print("vsock CONNECT closed before OK", file=sys.stderr)
            return 1
        buf += chunk

    line = buf.split(b"\n", 1)[0].decode("ascii", "replace")
    if not line.startswith("OK"):
        print(f"vsock CONNECT failed: {line}", file=sys.stderr)
        return 1

    rest = buf.split(b"\n", 1)[1] if b"\n" in buf else b""
    if rest:
        os.write(sys.stdout.fileno(), rest)

    stdin_fd = sys.stdin.fileno()
    stdout_fd = sys.stdout.fileno()
    sock_fd = s.fileno()
    stdin_open = True

    while True:
        readers = [sock_fd]
        if stdin_open:
            readers.append(stdin_fd)
        readable, _, _ = select.select(readers, [], [])
        if sock_fd in readable:
            data = s.recv(65536)
            if not data:
                return 0
            os.write(stdout_fd, data)
        if stdin_open and stdin_fd in readable:
            data = os.read(stdin_fd, 65536)
            if not data:
                stdin_open = False
                try:
                    s.shutdown(socket.SHUT_WR)
                except OSError:
                    return 0
                continue
            s.sendall(data)


if __name__ == "__main__":
    raise SystemExit(main())
