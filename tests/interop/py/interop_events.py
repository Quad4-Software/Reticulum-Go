#!/usr/bin/env python3
"""Structured interop event emission for Python peers.

Emits the cross-stack timeline described in docs/en/interop-timeline.md.
Event names: ready, path_wait, path_ok, path_req, path_resp, node, link_up,
link_ok, request_ok, fail (with kind), spawn.
"""

from __future__ import annotations

import json
import os
import sys
from datetime import datetime, timezone
from typing import Any


def _events_path() -> str:
    return os.environ.get("INTEROP_EVENTS_PATH", "").strip()


def emit(event: str, **fields: Any) -> None:
    """Emit one timeline event.

    When INTEROP_EVENTS_PATH is set, append a JSON line to that file.
    Always also write INTEROP_EVENT prefixed JSON to stderr for Go capture.
    """
    payload: dict[str, Any] = {
        "ts": datetime.now(timezone.utc).isoformat(),
        "src": "py",
        "event": event,
    }
    kind = fields.pop("kind", None)
    detail = fields.pop("detail", None)
    if kind is not None:
        payload["kind"] = str(kind)
    if detail is not None:
        payload["detail"] = str(detail)
    if fields:
        payload["fields"] = fields

    line = json.dumps(payload, separators=(",", ":"), ensure_ascii=True)
    # Always emit on stderr so the Go harness can ingest into events.jsonl.
    # When INTEROP_EVENTS_PATH is set without a Go parent, also append there.
    sys.stderr.write("INTEROP_EVENT " + line + "\n")
    sys.stderr.flush()

    path = _events_path()
    if not path:
        return
    # Skip direct file writes when a Go harness owns the same path.
    # Go ingests INTEROP_EVENT lines and appends them itself.
    if os.environ.get("INTEROP_EVENTS_GO_OWNED") == "1":
        return
    try:
        with open(path, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except OSError:
        pass
