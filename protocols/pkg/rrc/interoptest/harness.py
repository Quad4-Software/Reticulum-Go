#!/usr/bin/env python3
"""JSON stdin/stdout harness for Go <-> rrcd (Python) RRC interop tests."""

from __future__ import annotations

import json
import shutil
import sys
import tempfile
import threading
import time
import traceback
from pathlib import Path
from typing import Any

import RNS

from rrcd.codec import decode as rrc_decode
from rrcd.codec import encode as rrc_encode
from rrcd.constants import (
    B_HELLO_CAPS,
    B_HELLO_NAME,
    B_HELLO_NICK_LEGACY,
    B_HELLO_VER,
    B_LIMIT_MAX_MSG_BODY_BYTES,
    B_LIMIT_MAX_NICK_BYTES,
    B_LIMIT_MAX_ROOM_NAME_BYTES,
    B_LIMIT_MAX_ROOMS_PER_SESSION,
    B_LIMIT_RATE_LIMIT_MSGS_PER_MINUTE,
    B_RES_ENCODING,
    B_RES_ID,
    B_RES_KIND,
    B_RES_SHA256,
    B_RES_SIZE,
    B_WELCOME_CAPS,
    B_WELCOME_HUB,
    B_WELCOME_LIMITS,
    B_WELCOME_VER,
    CAP_ACTION,
    CAP_DIRECT_NOTICE,
    CAP_RESOURCE_ENVELOPE,
    HUB_DEST_NAME,
    K_BODY,
    K_DST,
    K_ID,
    K_NICK,
    K_ROOM,
    K_SRC,
    K_T,
    K_TS,
    K_V,
    RES_KIND_BLOB,
    RES_KIND_MOTD,
    RES_KIND_NOTICE,
    RRC_VERSION,
    T_ACTION,
    T_ERROR,
    T_HELLO,
    T_JOIN,
    T_JOINED,
    T_MSG,
    T_NOTICE,
    T_PART,
    T_PARTED,
    T_PING,
    T_PONG,
    T_RESOURCE_ENVELOPE,
    T_WELCOME,
)
from rrcd.envelope import make_envelope, validate_envelope
from rrcd.util import normalize_nick

from client import run_client_session, write_rns_config

RNS.loglevel = RNS.LOG_NONE


def _hex(b: bytes | None) -> str:
    if b is None:
        return ""
    return bytes(b).hex()


def _unhex(s: str) -> bytes:
    return bytes.fromhex(s)


def _jsonable(obj: Any) -> Any:
    if isinstance(obj, (bytes, bytearray)):
        return "hex:" + bytes(obj).hex()
    if isinstance(obj, dict):
        out: dict[str, Any] = {}
        for k, v in obj.items():
            key = str(int(k)) if isinstance(k, int) else str(k)
            out[key] = _jsonable(v)
        return out
    if isinstance(obj, (list, tuple)):
        return [_jsonable(x) for x in obj]
    return obj


def _from_jsonable(obj: Any) -> Any:
    if isinstance(obj, str) and obj.startswith("hex:"):
        return _unhex(obj[4:])
    if isinstance(obj, dict):
        out: dict[Any, Any] = {}
        for k, v in obj.items():
            try:
                key: Any = int(k)
            except (TypeError, ValueError):
                key = k
            out[key] = _from_jsonable(v)
        return out
    if isinstance(obj, list):
        return [_from_jsonable(x) for x in obj]
    return obj


def cmd_constants(_req: dict[str, Any]) -> dict[str, Any]:
    return {
        "ok": True,
        "constants": {
            "rrc_version": RRC_VERSION,
            "hub_dest": HUB_DEST_NAME,
            "envelope_keys": {
                "version": K_V,
                "type": K_T,
                "msg_id": K_ID,
                "timestamp": K_TS,
                "sender": K_SRC,
                "room": K_ROOM,
                "body": K_BODY,
                "nick": K_NICK,
                "destination": K_DST,
            },
            "message_types": {
                "hello": T_HELLO,
                "welcome": T_WELCOME,
                "join": T_JOIN,
                "joined": T_JOINED,
                "part": T_PART,
                "parted": T_PARTED,
                "msg": T_MSG,
                "notice": T_NOTICE,
                "action": T_ACTION,
                "ping": T_PING,
                "pong": T_PONG,
                "error": T_ERROR,
                "resource_envelope": T_RESOURCE_ENVELOPE,
            },
            "hello_keys": {
                "name": B_HELLO_NAME,
                "version": B_HELLO_VER,
                "capabilities": B_HELLO_CAPS,
                "nick_legacy": B_HELLO_NICK_LEGACY,
            },
            "welcome_keys": {
                "hub": B_WELCOME_HUB,
                "version": B_WELCOME_VER,
                "capabilities": B_WELCOME_CAPS,
                "limits": B_WELCOME_LIMITS,
            },
            "limit_keys": {
                "max_nick_bytes": B_LIMIT_MAX_NICK_BYTES,
                "max_room_name_bytes": B_LIMIT_MAX_ROOM_NAME_BYTES,
                "max_msg_body_bytes": B_LIMIT_MAX_MSG_BODY_BYTES,
                "max_rooms_per_session": B_LIMIT_MAX_ROOMS_PER_SESSION,
                "rate_limit_msgs_per_minute": B_LIMIT_RATE_LIMIT_MSGS_PER_MINUTE,
            },
            "capability_keys": {
                "resource_envelope": CAP_RESOURCE_ENVELOPE,
                "action": CAP_ACTION,
                "direct_notice": CAP_DIRECT_NOTICE,
            },
            "resource_keys": {
                "id": B_RES_ID,
                "kind": B_RES_KIND,
                "size": B_RES_SIZE,
                "sha256": B_RES_SHA256,
                "encoding": B_RES_ENCODING,
            },
            "resource_kinds": {
                "notice": RES_KIND_NOTICE,
                "motd": RES_KIND_MOTD,
                "blob": RES_KIND_BLOB,
            },
        },
    }


def cmd_normalize_nick(req: dict[str, Any]) -> dict[str, Any]:
    nick = str(req.get("nick", ""))
    limit = int(req.get("max_bytes", 32))
    got = normalize_nick(nick, max_bytes=limit)
    return {"ok": True, "normalized": got if got is not None else ""}


def cmd_normalize_room(req: dict[str, Any]) -> dict[str, Any]:
    room = str(req.get("room", ""))
    return {"ok": True, "normalized": room.strip().lower()}


def cmd_roundtrip(req: dict[str, Any]) -> dict[str, Any]:
    packed = _unhex(req["packed"])
    env = rrc_decode(packed)
    validate_envelope(env)
    again = rrc_encode(env)
    return {
        "ok": True,
        "roundtrip_ok": again == packed,
        "packed2": _hex(again),
    }


def validate_resource_body(body: Any) -> str:
    if body is None or not isinstance(body, dict):
        return "invalid resource envelope body"
    rid = body.get(B_RES_ID)
    kind = body.get(B_RES_KIND)
    size = body.get(B_RES_SIZE)
    sha = body.get(B_RES_SHA256)
    if rid is None or not isinstance(rid, (bytes, bytearray)) or len(rid) == 0:
        return "resource envelope missing id"
    if kind is None or not isinstance(kind, str) or kind == "":
        return "resource envelope missing kind"
    if size is None or not isinstance(size, int):
        return "resource envelope invalid size"
    if sha is not None and not isinstance(sha, (bytes, bytearray)):
        return "resource envelope invalid sha256"
    return ""


def cmd_validate_resource(req: dict[str, Any]) -> dict[str, Any]:
    body = _from_jsonable(req.get("body"))
    reason = validate_resource_body(body)
    if reason:
        return {"ok": False, "error": reason}
    env = make_envelope(
        T_RESOURCE_ENVELOPE,
        src=_unhex(req["sender"]),
        body=body,
    )
    validate_envelope(env)
    return {"ok": True}


def cmd_encode_matrix(_req: dict[str, Any]) -> dict[str, Any]:
    src = bytes([0x33] * 16)
    cases: dict[str, str] = {}
    matrix = [
        (T_HELLO, {"body": {B_HELLO_NAME: "m", B_HELLO_VER: "1"}}),
        (T_WELCOME, {"body": {B_WELCOME_HUB: "h", B_WELCOME_VER: "1"}}),
        (T_JOIN, {"room": "#lobby"}),
        (T_JOINED, {"room": "#lobby"}),
        (T_PART, {"room": "#lobby"}),
        (T_PARTED, {"room": "#lobby"}),
        (T_MSG, {"room": "#lobby", "body": "m"}),
        (T_NOTICE, {"room": "#lobby", "body": "n"}),
        (T_ACTION, {"room": "#lobby", "body": "a"}),
        (T_PING, {"body": "p"}),
        (T_PONG, {"body": "p"}),
        (T_ERROR, {"body": "e"}),
        (
            T_RESOURCE_ENVELOPE,
            {
                "body": {
                    B_RES_ID: bytes([1, 2, 3]),
                    B_RES_KIND: RES_KIND_BLOB,
                    B_RES_SIZE: 3,
                }
            },
        ),
        (T_NOTICE, {"destination": bytes([0x55] * 16), "body": "dm"}),
    ]
    for i, (typ, kw) in enumerate(matrix):
        dst = kw.pop("destination", None)
        env = make_envelope(typ, src=src, dst=dst, nick="alice", **kw)
        validate_envelope(env)
        cases[str(i)] = _hex(rrc_encode(env))
    return {"ok": True, "envelope": cases}


def cmd_ping(_req: dict[str, Any]) -> dict[str, Any]:
    import rrcd

    ver = getattr(rrcd, "__version__", "unknown")
    return {"ok": True, "rrcd_version": ver, "rrc_version": RRC_VERSION}


def cmd_encode(req: dict[str, Any]) -> dict[str, Any]:
    msg_type = int(req["type"])
    src = _unhex(req["sender"])
    room = req.get("room")
    nick = req.get("nick")
    body = _from_jsonable(req.get("body")) if "body" in req else None
    dst = _unhex(req["destination"]) if req.get("destination") else None
    mid = _unhex(req["msg_id"]) if req.get("msg_id") else None
    ts = int(req["timestamp"]) if req.get("timestamp") is not None else None
    env = make_envelope(
        msg_type,
        src=src,
        dst=dst,
        room=room,
        body=body,
        nick=nick,
        mid=mid,
        ts=ts,
    )
    validate_envelope(env)
    packed = rrc_encode(env)
    return {
        "ok": True,
        "packed": _hex(packed),
        "envelope": _jsonable(env),
    }


def cmd_decode(req: dict[str, Any]) -> dict[str, Any]:
    packed = _unhex(req["packed"])
    env = rrc_decode(packed)
    validate_envelope(env)
    return {
        "ok": True,
        "envelope": _jsonable(env),
        "type": int(env[K_T]),
        "version": int(env[K_V]),
        "sender": _hex(env[K_SRC]),
        "msg_id": _hex(env[K_ID]),
        "timestamp": int(env[K_TS]),
        "room": env.get(K_ROOM),
        "nick": env.get(K_NICK),
        "destination": _hex(env.get(K_DST)) if env.get(K_DST) is not None else "",
        "body": _jsonable(env.get(K_BODY)),
    }


def cmd_validate(req: dict[str, Any]) -> dict[str, Any]:
    packed = _unhex(req["packed"])
    env = rrc_decode(packed)
    validate_envelope(env)
    return {"ok": True}


def _write_rns_config(configdir: Path, listen_port: int, forward_port: int) -> None:
    write_rns_config(configdir, listen_port, forward_port)


def _link_send(link: RNS.Link, payload: bytes) -> None:
    RNS.Packet(link, payload).send()


def cmd_live_client(req: dict[str, Any]) -> dict[str, Any]:
    """Connect as an RRC client to a Go hub over UDP and exchange JOIN/MSG."""
    req = dict(req)
    req.setdefault("steps", ["join", "msg", "part"])
    return run_client_session(req)


def cmd_client_session(req: dict[str, Any]) -> dict[str, Any]:
    """Full Python client session against a hub (commands, ping, action)."""
    return run_client_session(req)


def cmd_live_hub(req: dict[str, Any]) -> dict[str, Any]:
    """Run a minimal rrc.hub over UDP and wait for a client MSG."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    ready_path = req["ready_path"]
    timeout_s = float(req.get("timeout_s", 40))
    hub_name = req.get("hub_name", "py-interop-hub")

    configdir = Path(tempfile.mkdtemp(prefix="rrc-interop-hub-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        identity = RNS.Identity()
        dest = RNS.Destination(
            identity,
            RNS.Destination.IN,
            RNS.Destination.SINGLE,
            "rrc",
            "hub",
        )

        got_text: dict[str, str] = {}
        msg_ev = threading.Event()

        def on_link(link: RNS.Link) -> None:
            def on_packet(message: bytes, packet: Any) -> None:
                try:
                    env = rrc_decode(message)
                    validate_envelope(env)
                except Exception:
                    return
                t = int(env[K_T])
                src = identity.hash
                if t == T_HELLO:
                    welcome = make_envelope(
                        T_WELCOME,
                        src=src,
                        body={
                            B_WELCOME_HUB: hub_name,
                            B_WELCOME_VER: "0.1.0",
                            B_WELCOME_CAPS: {
                                CAP_ACTION: True,
                                CAP_DIRECT_NOTICE: True,
                            },
                            B_WELCOME_LIMITS: {
                                B_LIMIT_MAX_NICK_BYTES: 32,
                                B_LIMIT_MAX_ROOM_NAME_BYTES: 64,
                                B_LIMIT_MAX_MSG_BODY_BYTES: 350,
                                B_LIMIT_MAX_ROOMS_PER_SESSION: 32,
                                B_LIMIT_RATE_LIMIT_MSGS_PER_MINUTE: 60,
                            },
                        },
                    )
                    _link_send(link, rrc_encode(welcome))
                elif t == T_JOIN:
                    room = env.get(K_ROOM) or "#lobby"
                    joined = make_envelope(T_JOINED, src=src, room=room)
                    _link_send(link, rrc_encode(joined))
                elif t == T_MSG:
                    body = env.get(K_BODY)
                    if isinstance(body, str):
                        got_text["text"] = body
                    elif isinstance(body, (bytes, bytearray)):
                        got_text["text"] = bytes(body).decode("utf-8", errors="replace")
                    else:
                        got_text["text"] = str(body)
                    msg_ev.set()

            link.set_packet_callback(on_packet)

        dest.set_link_established_callback(on_link)

        Path(ready_path).write_text(
            json.dumps(
                {
                    "hub_hash": _hex(dest.hash),
                    "public_key": _hex(identity.get_public_key()),
                }
            ),
            encoding="utf-8",
        )

        deadline = time.time() + timeout_s
        while time.time() < deadline:
            dest.announce()
            if msg_ev.wait(timeout=0.4):
                break

        if not msg_ev.is_set():
            return {"ok": False, "error": "timeout waiting for client MSG"}

        return {
            "ok": True,
            "hub_hash": _hex(dest.hash),
            "text": got_text.get("text", ""),
        }
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def cmd_live_hub_protocol(req: dict[str, Any]) -> dict[str, Any]:
    """Full rrc.hub: HELLO/WELCOME, JOIN/PART, MSG/NOTICE/ACTION relay, PING/PONG."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    ready_path = req["ready_path"]
    timeout_s = float(req.get("timeout_s", 45))
    hub_name = req.get("hub_name", "py-protocol-hub")
    want_text = str(req.get("want_text", "protocol-msg"))

    configdir = Path(tempfile.mkdtemp(prefix="rrc-interop-proto-hub-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        identity = RNS.Identity()
        dest = RNS.Destination(
            identity,
            RNS.Destination.IN,
            RNS.Destination.SINGLE,
            "rrc",
            "hub",
        )

        state: dict[str, Any] = {
            "msg": "",
            "notice": "",
            "action": "",
            "pong": False,
            "parted": False,
        }
        done = threading.Event()

        def mark_progress() -> None:
            if (
                state.get("msg") == want_text
                and state.get("notice")
                and state.get("action")
                and state.get("pong")
                and state.get("parted")
            ):
                done.set()

        def on_link(link: RNS.Link) -> None:
            rooms: dict[str, set[bytes]] = {}

            def on_packet(message: bytes, _packet: Any) -> None:
                try:
                    env = rrc_decode(message)
                    validate_envelope(env)
                except Exception:
                    return
                t = int(env[K_T])
                src = identity.hash
                peer = bytes(env[K_SRC])
                room = env.get(K_ROOM) or ""
                if t == T_HELLO:
                    welcome = make_envelope(
                        T_WELCOME,
                        src=src,
                        body={
                            B_WELCOME_HUB: hub_name,
                            B_WELCOME_VER: "0.1.0",
                            B_WELCOME_CAPS: {
                                CAP_ACTION: True,
                                CAP_DIRECT_NOTICE: True,
                                CAP_RESOURCE_ENVELOPE: True,
                            },
                            B_WELCOME_LIMITS: {
                                B_LIMIT_MAX_NICK_BYTES: 32,
                                B_LIMIT_MAX_ROOM_NAME_BYTES: 64,
                                B_LIMIT_MAX_MSG_BODY_BYTES: 350,
                                B_LIMIT_MAX_ROOMS_PER_SESSION: 32,
                                B_LIMIT_RATE_LIMIT_MSGS_PER_MINUTE: 120,
                            },
                        },
                    )
                    _link_send(link, rrc_encode(welcome))
                elif t == T_JOIN:
                    r = room or "#lobby"
                    rooms.setdefault(r, set()).add(peer)
                    joined = make_envelope(T_JOINED, src=src, room=r)
                    _link_send(link, rrc_encode(joined))
                elif t == T_PART:
                    r = room or "#lobby"
                    if r in rooms:
                        rooms[r].discard(peer)
                    parted = make_envelope(T_PARTED, src=src, room=r)
                    _link_send(link, rrc_encode(parted))
                    state["parted"] = True
                    mark_progress()
                elif t == T_MSG:
                    body = env.get(K_BODY)
                    text = body if isinstance(body, str) else str(body)
                    state["msg"] = text
                    r = room or "#lobby"
                    relay = make_envelope(
                        T_MSG,
                        src=src,
                        room=r,
                        body=text,
                        nick="py-hub",
                    )
                    _link_send(link, rrc_encode(relay))
                    if text == want_text:
                        mark_progress()
                elif t == T_NOTICE:
                    body = env.get(K_BODY)
                    text = body if isinstance(body, str) else str(body)
                    state["notice"] = text
                    r = room or "#lobby"
                    relay = make_envelope(
                        T_NOTICE,
                        src=src,
                        room=r,
                        body=text,
                        nick="py-hub",
                    )
                    _link_send(link, rrc_encode(relay))
                    mark_progress()
                elif t == T_ACTION:
                    body = env.get(K_BODY)
                    text = body if isinstance(body, str) else str(body)
                    state["action"] = text
                    r = room or "#lobby"
                    relay = make_envelope(
                        T_ACTION,
                        src=src,
                        room=r,
                        body=text,
                        nick="py-hub",
                    )
                    _link_send(link, rrc_encode(relay))
                    mark_progress()
                elif t == T_PING:
                    pong = make_envelope(T_PONG, src=src, body=env.get(K_BODY))
                    _link_send(link, rrc_encode(pong))
                    state["pong"] = True
                    mark_progress()

            link.set_packet_callback(on_packet)

        dest.set_link_established_callback(on_link)

        Path(ready_path).write_text(
            json.dumps(
                {
                    "hub_hash": _hex(dest.hash),
                    "public_key": _hex(identity.get_public_key()),
                }
            ),
            encoding="utf-8",
        )

        deadline = time.time() + timeout_s
        while time.time() < deadline:
            dest.announce()
            if done.wait(timeout=0.4):
                time.sleep(0.5)
                break

        if not done.is_set():
            return {"ok": False, "error": "timeout waiting for full protocol session"}

        return {
            "ok": True,
            "hub_hash": _hex(dest.hash),
            "text": state.get("msg", ""),
            "notice_ok": bool(state.get("notice")),
            "action_ok": bool(state.get("action")),
            "pong": bool(state.get("pong")),
            "parted": bool(state.get("parted")),
        }
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def handle(req: dict[str, Any]) -> dict[str, Any]:
    cmd = req.get("cmd")
    if cmd == "ping":
        return cmd_ping(req)
    if cmd == "constants":
        return cmd_constants(req)
    if cmd == "normalize_nick":
        return cmd_normalize_nick(req)
    if cmd == "normalize_room":
        return cmd_normalize_room(req)
    if cmd == "roundtrip":
        return cmd_roundtrip(req)
    if cmd == "validate_resource":
        return cmd_validate_resource(req)
    if cmd == "encode_matrix":
        return cmd_encode_matrix(req)
    if cmd == "encode":
        return cmd_encode(req)
    if cmd == "decode":
        return cmd_decode(req)
    if cmd == "validate":
        return cmd_validate(req)
    if cmd == "live_client":
        return cmd_live_client(req)
    if cmd == "client_session":
        return cmd_client_session(req)
    if cmd == "live_hub":
        return cmd_live_hub(req)
    if cmd == "live_hub_protocol":
        return cmd_live_hub_protocol(req)
    return {"ok": False, "error": f"unknown cmd: {cmd!r}"}


def main() -> int:
    try:
        raw = sys.stdin.read()
        req = json.loads(raw) if raw.strip() else {}
        resp = handle(req)
    except Exception as exc:
        resp = {
            "ok": False,
            "error": str(exc),
            "trace": traceback.format_exc(),
        }
    sys.stdout.write(json.dumps(resp, separators=(",", ":")))
    sys.stdout.write("\n")
    return 0 if resp.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
