#!/usr/bin/env python3
"""Python RRC client for hub interop tests against gorrcd and rrcd."""

from __future__ import annotations

import shutil
import tempfile
import threading
import time
from pathlib import Path
from typing import Any

import RNS

from rrcd.codec import decode as rrc_decode
from rrcd.codec import encode as rrc_encode
from rrcd.constants import (
    B_HELLO_CAPS,
    B_HELLO_NAME,
    B_HELLO_VER,
    B_WELCOME_CAPS,
    B_WELCOME_HUB,
    B_WELCOME_LIMITS,
    CAP_ACTION,
    CAP_DIRECT_NOTICE,
    K_BODY,
    K_NICK,
    K_ROOM,
    K_T,
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
    T_WELCOME,
)
from rrcd.envelope import make_envelope, validate_envelope

RNS.loglevel = RNS.LOG_NONE


def write_rns_config(configdir: Path, listen_port: int, forward_port: int) -> None:
    configdir.mkdir(parents=True, exist_ok=True)
    (configdir / "storage").mkdir(exist_ok=True)
    (configdir / "storage" / "identities").mkdir(exist_ok=True)
    cfg = f"""[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = rrc-py-{listen_port}

[logging]
  loglevel = 0

[interfaces]

  [[UDP Interface]]
    type = UDPInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = {listen_port}
    forward_ip = 127.0.0.1
    forward_port = {forward_port}
"""
    (configdir / "config").write_text(cfg, encoding="utf-8")


def _body_text(body: Any) -> str:
    if isinstance(body, str):
        return body
    if isinstance(body, (bytes, bytearray)):
        return bytes(body).decode("utf-8", errors="replace")
    if body is None:
        return ""
    return str(body)


class RRCClient:
    """Link-level RRC client using rrcd codec over a Reticulum UDP pair."""

    def __init__(
        self,
        *,
        hub_hash: bytes,
        listen_port: int,
        forward_port: int,
        nick: str = "py-client",
        name: str = "rrc-py-client",
        version: str = "0.1.0",
        timeout_s: float = 40,
    ) -> None:
        self.hub_hash = hub_hash
        self.listen_port = listen_port
        self.forward_port = forward_port
        self.nick = nick
        self.name = name
        self.version = version
        self.timeout_s = timeout_s
        self.configdir = Path(tempfile.mkdtemp(prefix="rrc-py-client-"))
        self.identity: RNS.Identity | None = None
        self.link: RNS.Link | None = None
        self._lock = threading.Lock()
        self.events: list[dict[str, Any]] = []
        self.welcome: dict[Any, Any] | None = None
        self._welcome_ev = threading.Event()
        self._joined: dict[str, threading.Event] = {}
        self._parted: dict[str, threading.Event] = {}
        self._pong_ev = threading.Event()

    def close(self) -> None:
        if self.link is not None:
            try:
                self.link.teardown()
            except Exception:
                pass
            self.link = None
        try:
            shutil.rmtree(self.configdir, ignore_errors=True)
        except Exception:
            pass

    def connect(self) -> None:
        write_rns_config(self.configdir, self.listen_port, self.forward_port)
        RNS.Reticulum(str(self.configdir))
        self.identity = RNS.Identity()
        deadline = time.time() + self.timeout_s
        while not RNS.Transport.has_path(self.hub_hash):
            if time.time() > deadline:
                raise TimeoutError("path timeout to hub")
            RNS.Transport.request_path(self.hub_hash)
            time.sleep(0.1)
        hub_id = RNS.Identity.recall(self.hub_hash)
        if hub_id is None:
            raise RuntimeError("hub identity not recalled")
        dest = RNS.Destination(
            hub_id,
            RNS.Destination.OUT,
            RNS.Destination.SINGLE,
            "rrc",
            "hub",
        )
        if dest.hash != self.hub_hash:
            raise RuntimeError(
                f"destination hash mismatch got={bytes(dest.hash).hex()} want={self.hub_hash.hex()}"
            )
        established = threading.Event()

        def on_established(link: RNS.Link) -> None:
            link.set_packet_callback(self._on_packet)
            assert self.identity is not None
            link.identify(self.identity)
            established.set()

        self.link = RNS.Link(dest, established_callback=on_established)
        while self.link.status != RNS.Link.ACTIVE:
            if time.time() > deadline:
                raise TimeoutError("link establish timeout")
            time.sleep(0.05)
        if not established.wait(timeout=max(1.0, deadline - time.time())):
            raise TimeoutError("identify timeout")
        self.send_hello()
        if not self._welcome_ev.wait(timeout=max(1.0, deadline - time.time())):
            raise TimeoutError("welcome timeout")

    def send_hello(self) -> None:
        assert self.identity is not None
        env = make_envelope(
            T_HELLO,
            src=self.identity.hash,
            nick=self.nick,
            body={
                B_HELLO_NAME: self.name,
                B_HELLO_VER: self.version,
                B_HELLO_CAPS: {CAP_ACTION: True, CAP_DIRECT_NOTICE: True},
            },
        )
        self._send(env)

    def join(self, room: str, key: str | None = None) -> None:
        assert self.identity is not None
        with self._lock:
            ev = self._joined.setdefault(room, threading.Event())
            ev.clear()
        env = make_envelope(
            T_JOIN,
            src=self.identity.hash,
            room=room,
            nick=self.nick,
            body=key if key else None,
        )
        self._send(env)

    def wait_joined(self, room: str, timeout: float) -> None:
        with self._lock:
            ev = self._joined.setdefault(room, threading.Event())
        if not ev.wait(timeout=timeout):
            raise TimeoutError(f"joined timeout for {room}")

    def wait_parted(self, room: str, timeout: float) -> None:
        with self._lock:
            ev = self._parted.setdefault(room, threading.Event())
        if not ev.wait(timeout=timeout):
            raise TimeoutError(f"parted timeout for {room}")

    def slash(self, text: str) -> None:
        assert self.identity is not None
        env = make_envelope(
            T_NOTICE,
            src=self.identity.hash,
            body=text,
            nick=self.nick,
        )
        self._send(env)

    def part(self, room: str) -> None:
        assert self.identity is not None
        env = make_envelope(T_PART, src=self.identity.hash, room=room, nick=self.nick)
        self._send(env)

    def msg(self, room: str, text: str) -> None:
        self._typed(T_MSG, room, text)

    def notice(self, room: str, text: str) -> None:
        self._typed(T_NOTICE, room, text)

    def action(self, room: str, text: str) -> None:
        self._typed(T_ACTION, room, text)

    def ping(self, body: str = "py-ping") -> None:
        assert self.identity is not None
        self._pong_ev.clear()
        env = make_envelope(T_PING, src=self.identity.hash, body=body, nick=self.nick)
        self._send(env)

    def wait_pong(self, timeout: float) -> None:
        if not self._pong_ev.wait(timeout=timeout):
            raise TimeoutError("pong timeout")

    def command(self, room: str, text: str) -> None:
        self.msg(room, text)

    def hash_hex(self) -> str:
        assert self.identity is not None
        return bytes(self.identity.hash).hex()

    def run_session(
        self,
        *,
        room: str,
        text: str,
        steps: list[str] | None = None,
    ) -> dict[str, Any]:
        if steps is None:
            steps = [
                "join",
                "msg",
                "notice",
                "action",
                "ping",
                "slash_list",
                "part",
            ]
        leftover = self.timeout_s
        t0 = time.time()
        self.connect()
        leftover = max(1.0, leftover - (time.time() - t0))
        joined = False
        msg_echo = False
        parted = False
        notice_ok = False
        action_ok = False
        for step in steps:
            if leftover <= 0:
                raise TimeoutError(f"timeout before step {step}")
            mark = time.time()
            if step == "join":
                self.join(room)
                self.wait_joined(room, leftover)
                joined = True
            elif step == "msg":
                before = self._count("msg")
                self.msg(room, text)
                self._wait_pred(lambda: self._count("msg") > before, leftover, "msg echo")
                msg_echo = True
            elif step == "list":
                self.command(room, "/list")
                time.sleep(min(0.4, leftover))
            elif step == "who":
                self.command(room, f"/who {room}")
                time.sleep(min(0.4, leftover))
            elif step == "slash_list":
                self.slash("/list")
                time.sleep(min(0.4, leftover))
            elif step == "slash_who":
                self.slash(f"/who {room}")
                time.sleep(min(0.4, leftover))
            elif step == "unrecognized":
                self.command(room, "/not-a-real-command")
                time.sleep(min(0.3, leftover))
            elif step == "ping":
                self.ping()
                self.wait_pong(leftover)
            elif step == "notice":
                before = self._count("notice")
                self.notice(room, "interop-notice")
                self._wait_pred(lambda: self._count("notice") > before, leftover, "notice echo")
                notice_ok = True
            elif step == "action":
                before = self._count("action")
                self.action(room, "interop-action")
                self._wait_pred(lambda: self._count("action") > before, leftover, "action echo")
                action_ok = True
            elif step == "part":
                with self._lock:
                    ev = self._parted.setdefault(room, threading.Event())
                    ev.clear()
                self.part(room)
                self.wait_parted(room, leftover)
                parted = True
            else:
                raise ValueError(f"unknown step {step}")
            leftover = max(0.1, leftover - (time.time() - mark))
        notices = [e.get("text", "") for e in self.events if e.get("type") == "notice"]
        errors = [e.get("text", "") for e in self.events if e.get("type") == "error"]
        welcome_hub = ""
        welcome_caps: dict[str, Any] = {}
        welcome_limits: dict[str, Any] = {}
        if isinstance(self.welcome, dict):
            welcome_hub = str(self.welcome.get(B_WELCOME_HUB) or "")
            caps = self.welcome.get(B_WELCOME_CAPS)
            if isinstance(caps, dict):
                welcome_caps = _jsonable(caps)
            lim = self.welcome.get(B_WELCOME_LIMITS)
            if isinstance(lim, dict):
                welcome_limits = _jsonable(lim)
        return {
            "ok": True,
            "client_hash": self.hash_hex(),
            "welcome_body": _jsonable(self.welcome or {}),
            "hub_name": welcome_hub,
            "welcome_caps": welcome_caps,
            "welcome_limits": welcome_limits,
            "room": room,
            "text": text,
            "joined": joined,
            "msg_echo": msg_echo,
            "notice_ok": notice_ok,
            "action_ok": action_ok,
            "pong": self._pong_ev.is_set(),
            "parted": parted,
            "notices": notices,
            "errors": errors,
            "events": self.events,
        }

    def _typed(self, msg_type: int, room: str, text: str) -> None:
        assert self.identity is not None
        env = make_envelope(
            msg_type,
            src=self.identity.hash,
            room=room,
            body=text,
            nick=self.nick,
        )
        self._send(env)

    def _send(self, env: dict[Any, Any]) -> None:
        if self.link is None:
            raise RuntimeError("not connected")
        validate_envelope(env)
        RNS.Packet(self.link, rrc_encode(env)).send()

    def _on_packet(self, message: bytes, _packet: Any) -> None:
        try:
            env = rrc_decode(message)
            validate_envelope(env)
        except Exception:
            return
        t = int(env[K_T])
        room = env.get(K_ROOM)
        text = _body_text(env.get(K_BODY))
        rec = {
            "type": _type_name(t),
            "room": room,
            "nick": env.get(K_NICK),
            "text": text,
        }
        with self._lock:
            self.events.append(rec)
        if t == T_WELCOME:
            body = env.get(K_BODY)
            if isinstance(body, dict):
                self.welcome = body
            self._welcome_ev.set()
        elif t == T_JOINED:
            key = str(room or "")
            with self._lock:
                ev = self._joined.setdefault(key, threading.Event())
            ev.set()
        elif t == T_PARTED:
            key = str(room or "")
            with self._lock:
                ev = self._parted.setdefault(key, threading.Event())
            ev.set()
        elif t == T_PONG:
            self._pong_ev.set()

    def _count(self, type_name: str) -> int:
        with self._lock:
            return sum(1 for e in self.events if e.get("type") == type_name)

    def _wait_pred(self, pred: Any, timeout: float, what: str) -> None:
        deadline = time.time() + timeout
        while time.time() < deadline:
            if pred():
                return
            time.sleep(0.05)
        raise TimeoutError(what)


def _type_name(t: int) -> str:
    return {
        T_WELCOME: "welcome",
        T_JOINED: "joined",
        T_PARTED: "parted",
        T_MSG: "msg",
        T_NOTICE: "notice",
        T_ACTION: "action",
        T_PONG: "pong",
        T_ERROR: "error",
        T_PING: "ping",
    }.get(t, str(t))


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


def run_client_session(req: dict[str, Any]) -> dict[str, Any]:
    hub_hash = bytes.fromhex(req["hub_hash"])
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    room = str(req.get("room", "#lobby"))
    text = str(req.get("text", "hello from python"))
    nick = str(req.get("nick", "py-client"))
    timeout_s = float(req.get("timeout_s", 40))
    steps = req.get("steps")
    if steps is not None:
        steps = [str(s) for s in steps]
    client = RRCClient(
        hub_hash=hub_hash,
        listen_port=listen_port,
        forward_port=forward_port,
        nick=nick,
        timeout_s=timeout_s,
    )
    try:
        return client.run_session(room=room, text=text, steps=steps)
    finally:
        client.close()
