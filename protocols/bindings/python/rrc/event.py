# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from dataclasses import dataclass
from enum import IntEnum

from . import ffi
from .errors import Error, map_code


class EventKind(IntEnum):
    WELCOME = ffi.RRC_EV_WELCOME
    JOINED = ffi.RRC_EV_JOINED
    PARTED = ffi.RRC_EV_PARTED
    MSG = ffi.RRC_EV_MSG
    NOTICE = ffi.RRC_EV_NOTICE
    ACTION = ffi.RRC_EV_ACTION
    ERROR = ffi.RRC_EV_ERROR
    PONG = ffi.RRC_EV_PONG
    HELLO = ffi.RRC_EV_HELLO
    JOIN = ffi.RRC_EV_JOIN
    PART = ffi.RRC_EV_PART
    CLOSE = ffi.RRC_EV_CLOSE
    TIMEOUT = ffi.RRC_EV_TIMEOUT

    @classmethod
    def _missing_(cls, value: object) -> EventKind:
        pseudo = int.__new__(cls, value)
        pseudo._name_ = f"UNKNOWN_{value}"
        pseudo._value_ = int(value)  # type: ignore[assignment]
        return pseudo


@dataclass(frozen=True, slots=True)
class Event:
    kind: EventKind
    sender: bytes
    peer: bytes
    room: str
    nick: str
    body: str
    msg_type: int
    room_truncated: bool
    nick_truncated: bool
    body_truncated: bool

    @classmethod
    def from_c(cls, ev: ffi.RrcEvent) -> Event:
        return cls(
            kind=EventKind(ev.kind),
            sender=bytes(ev.sender[: ev.sender_len]),
            peer=bytes(ev.peer[: ev.peer_len]),
            room=ev.room.decode("utf-8", errors="replace").rstrip("\x00"),
            nick=ev.nick.decode("utf-8", errors="replace").rstrip("\x00"),
            body=ev.body.decode("utf-8", errors="replace").rstrip("\x00"),
            msg_type=int(ev.msg_type),
            room_truncated=bool(ev.room_truncated),
            nick_truncated=bool(ev.nick_truncated),
            body_truncated=bool(ev.body_truncated),
        )


def poll_client(client_handle: int, timeout_ms: int) -> Event:
    ev = ffi.RrcEvent()
    code = ffi.lib.rrc_client_event_poll(client_handle, timeout_ms, ffi.ctypes.byref(ev))
    if code == Error.TIMEOUT:
        raise Error(Error.TIMEOUT)
    map_code(code)
    return Event.from_c(ev)


def poll_hub(hub_handle: int, timeout_ms: int) -> Event:
    ev = ffi.RrcEvent()
    code = ffi.lib.rrc_hub_event_poll(hub_handle, timeout_ms, ffi.ctypes.byref(ev))
    if code == Error.TIMEOUT:
        raise Error(Error.TIMEOUT)
    map_code(code)
    return Event.from_c(ev)
