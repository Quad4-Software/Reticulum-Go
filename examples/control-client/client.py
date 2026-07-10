#!/usr/bin/env python3
"""Minimal polyglot client for the Reticulum-Go control API (pkg/controlapi).

Demonstrates using a running reticulum-go daemon's destinations and
announces from a process that does not embed the Go stack, using only the
Python standard library (http.client for the JSON routes, a hand-rolled
RFC 6455 client for the WebSocket event stream).

Prerequisites: run reticulum-go with the following in [reticulum]:

  enable_control_api = yes
  control_api_host = 127.0.0.1
  control_api_port = 37430
  rpc_key = <hex key>

rpc_key must already be set (it also guards the shared-instance RPC
server); generate one with e.g. `python3 -c "import secrets; print(secrets.token_hex(32))"`
and pass the same hex string as --rpc-key below.

Usage:
  python3 client.py --rpc-key <hex rpc_key> [--host HOST] [--port PORT]

The script creates a session, registers a destination, subscribes to
announce events over the control API's WebSocket, sends one announce, and
prints any announce events observed for --wait seconds before tearing the
session down. Whether the script observes its own announce depends on
whether the daemon has an interface that loops it back (for example,
AutoInterface on the same host); announces from other peers on the network
will also show up while the WebSocket is subscribed.

Pass --accept-links to also accept inbound links on the destination and
register a "/ping" request handler that echoes any request payload back
prefixed with "pong:". Pass --link-to <hex destination hash> to instead (or
additionally) open an outbound link to a destination the daemon has already
seen an announce from, send it a "/ping" request, and print the response.
"""

import argparse
import base64
import hashlib
import http.client
import json
import os
import socket
import struct
import sys

WS_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


class BufferedSocket:
    """Minimal buffered reader over a raw socket, used for the WebSocket
    handshake and subsequent frame parsing."""

    def __init__(self, sock: socket.socket):
        self._sock = sock
        self._buf = b""

    def _fill(self) -> None:
        chunk = self._sock.recv(4096)
        if not chunk:
            raise ConnectionError("control API closed the connection")
        self._buf += chunk

    def read_exact(self, n: int) -> bytes:
        while len(self._buf) < n:
            self._fill()
        data, self._buf = self._buf[:n], self._buf[n:]
        return data

    def read_until(self, delimiter: bytes) -> bytes:
        while delimiter not in self._buf:
            self._fill()
        idx = self._buf.index(delimiter) + len(delimiter)
        data, self._buf = self._buf[:idx], self._buf[idx:]
        return data


def http_json(host: str, port: int, method: str, path: str, token: str, body=None):
    """Issue one JSON request against the control API and return
    (status, decoded_body_or_None)."""
    conn = http.client.HTTPConnection(host, port, timeout=10)
    headers = {"Authorization": f"Bearer {token}"}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    try:
        conn.request(method, path, body=data, headers=headers)
        resp = conn.getresponse()
        raw = resp.read()
    finally:
        conn.close()
    if resp.status >= 400:
        raise RuntimeError(f"{method} {path} -> {resp.status}: {raw.decode(errors='replace')}")
    return resp.status, (json.loads(raw.decode()) if raw else None)


def ws_connect(host: str, port: int, path: str, token: str):
    """Complete an RFC 6455 client handshake and return (socket, BufferedSocket)."""
    sock = socket.create_connection((host, port), timeout=10)
    buffered = BufferedSocket(sock)
    key = base64.b64encode(os.urandom(16)).decode()
    request = (
        f"GET {path} HTTP/1.1\r\n"
        f"Host: {host}:{port}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        f"Authorization: Bearer {token}\r\n"
        "\r\n"
    )
    sock.sendall(request.encode())
    header = buffered.read_until(b"\r\n\r\n").decode(errors="replace")
    status_line = header.split("\r\n", 1)[0]
    if " 101 " not in status_line:
        raise RuntimeError(f"websocket handshake failed: {status_line}")
    expected_accept = base64.b64encode(hashlib.sha1((key + WS_GUID).encode()).digest()).decode()
    if expected_accept not in header:
        raise RuntimeError("websocket handshake accept key mismatch")
    return sock, buffered


def ws_send_text(sock: socket.socket, payload: bytes) -> None:
    """Send payload as a single masked text frame, as RFC 6455 requires of clients."""
    mask_key = os.urandom(4)
    masked = bytes(b ^ mask_key[i % 4] for i, b in enumerate(payload))
    length = len(payload)
    header = bytearray([0x80 | 0x1])  # FIN + text opcode
    if length < 126:
        header.append(0x80 | length)
    elif length <= 0xFFFF:
        header.append(0x80 | 126)
        header += struct.pack(">H", length)
    else:
        header.append(0x80 | 127)
        header += struct.pack(">Q", length)
    sock.sendall(bytes(header) + mask_key + masked)


def ws_recv(buffered: BufferedSocket):
    """Read one unmasked server frame and return (opcode, payload)."""
    header = buffered.read_exact(2)
    opcode = header[0] & 0x0F
    length = header[1] & 0x7F
    if length == 126:
        length = struct.unpack(">H", buffered.read_exact(2))[0]
    elif length == 127:
        length = struct.unpack(">Q", buffered.read_exact(8))[0]
    payload = buffered.read_exact(length) if length else b""
    return opcode, payload


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=37430)
    parser.add_argument("--rpc-key", required=True, help="hex rpc_key configured on the daemon")
    parser.add_argument("--app-name", default="controlapi_example")
    parser.add_argument("--aspect", action="append", default=[], help="destination aspect, may be repeated")
    parser.add_argument("--wait", type=float, default=5.0, help="seconds to wait for events")
    parser.add_argument(
        "--accept-links", action="store_true",
        help='accept inbound links and register a "/ping" request handler that echoes requests back',
    )
    parser.add_argument(
        "--link-to", metavar="HEX_DESTINATION_HASH",
        help="open an outbound link to a destination already seen in an announce, then send it one /ping request",
    )
    args = parser.parse_args()

    _, health = http_json(args.host, args.port, "GET", "/v1/health", args.rpc_key)
    print("health:", health)

    _, session = http_json(args.host, args.port, "POST", "/v1/sessions", args.rpc_key, {})
    session_id = session["session_id"]
    print("session:", session)

    try:
        _, dest = http_json(
            args.host, args.port, "POST", f"/v1/sessions/{session_id}/destinations", args.rpc_key,
            {"app_name": args.app_name, "aspects": args.aspect, "accepts_links": args.accept_links},
        )
        dest_hash = dest["destination_hash"]
        print("destination:", dest)

        if args.accept_links:
            http_json(
                args.host, args.port, "POST",
                f"/v1/sessions/{session_id}/destinations/{dest_hash}/requests", args.rpc_key,
                {"path": "/ping"},
            )
            print("registered /ping request handler")

        sock, buffered = ws_connect(args.host, args.port, f"/v1/sessions/{session_id}/events", args.rpc_key)
        try:
            ws_send_text(sock, json.dumps({"type": "subscribe_announces"}).encode())

            http_json(
                args.host, args.port, "POST",
                f"/v1/sessions/{session_id}/destinations/{dest_hash}/announce", args.rpc_key, {},
            )
            print("announce sent")

            if args.link_to:
                ws_send_text(sock, json.dumps({"type": "link.open", "destination_hash": args.link_to}).encode())
                print(f"link.open sent to {args.link_to}")

            print(f"waiting {args.wait}s for events...")
            sock.settimeout(args.wait)
            try:
                while True:
                    opcode, payload = ws_recv(buffered)
                    if opcode == 0x8:  # close
                        break
                    if opcode != 0x1:  # only text frames carry JSON events
                        continue
                    event = json.loads(payload.decode())
                    print("event:", event)
                    handle_event(sock, event)
            except socket.timeout:
                print("no more events; exiting")
        finally:
            sock.close()
    finally:
        http_json(args.host, args.port, "DELETE", f"/v1/sessions/{session_id}", args.rpc_key)

    return 0


def handle_event(sock: socket.socket, event: dict) -> None:
    """React to one WebSocket event: auto-answer request.incoming with a
    canned reply, and once an outbound link is established, send it one
    demo payload over link.send."""
    event_type = event.get("type")
    if event_type == "request.incoming":
        data = base64.b64decode(event["data"]) if event.get("data") else b""
        reply = b"pong:" + data
        ws_send_text(sock, json.dumps({
            "type": "request.respond",
            "request_id": event["request_id"],
            "data": base64.b64encode(reply).decode(),
        }).encode())
        print(f"  -> replied to request.incoming on {event['path']!r}")
    elif event_type == "link.established":
        payload = base64.b64encode(b"hello over control-api link").decode()
        ws_send_text(sock, json.dumps({
            "type": "link.send",
            "link_id": event["link_id"],
            "data": payload,
        }).encode())
        print(f"  -> link.send queued on link {event['link_id']}")


if __name__ == "__main__":
    sys.exit(main())
