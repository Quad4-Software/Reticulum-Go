# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

from . import ffi
from .errors import Error, map_code
from .event import poll_client


class Client:
    DEFAULT_TIMEOUT_MS = 45000

    def __init__(self, handle: int) -> None:
        self._handle = int(handle)

    @classmethod
    def dial(
        cls,
        node_handle: int,
        identity_handle: int,
        hub_hash: bytes,
        nick: str,
        name: str,
        version: str,
        timeout_ms: int = DEFAULT_TIMEOUT_MS,
    ) -> Client:
        if len(hub_hash) != ffi.HASH_LEN:
            raise Error(Error.INVALID_ARG)
        arr = (ffi.ctypes.c_uint8 * ffi.HASH_LEN).from_buffer_copy(hub_hash)
        h = ffi.lib.rrc_client_dial(
            node_handle,
            identity_handle,
            arr,
            ffi.HASH_LEN,
            nick.encode("utf-8"),
            name.encode("utf-8"),
            version.encode("utf-8"),
            timeout_ms,
        )
        if h == 0:
            raise Error(Error.INTERNAL)
        return cls(h)

    def join(self, room: str) -> None:
        map_code(ffi.lib.rrc_client_join(self._handle, room.encode("utf-8")))

    def part(self, room: str) -> None:
        map_code(ffi.lib.rrc_client_part(self._handle, room.encode("utf-8")))

    def send_msg(self, room: str, text: str) -> None:
        map_code(
            ffi.lib.rrc_client_send_msg(
                self._handle,
                room.encode("utf-8"),
                text.encode("utf-8"),
            )
        )

    def send_notice(self, room: str, text: str) -> None:
        map_code(
            ffi.lib.rrc_client_send_notice(
                self._handle,
                room.encode("utf-8"),
                text.encode("utf-8"),
            )
        )

    def send_action(self, room: str, text: str) -> None:
        map_code(
            ffi.lib.rrc_client_send_action(
                self._handle,
                room.encode("utf-8"),
                text.encode("utf-8"),
            )
        )

    def ping(self) -> None:
        map_code(ffi.lib.rrc_client_ping(self._handle))

    def event_poll(self, timeout_ms: int = 0):
        return poll_client(self._handle, timeout_ms)

    @property
    def handle(self) -> int:
        return self._handle

    def close(self) -> None:
        if self._handle:
            ffi.lib.rrc_client_close(self._handle)
            self._handle = 0

    def __enter__(self) -> Client:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass

    def __repr__(self) -> str:
        return f"Client(handle={self._handle})"
