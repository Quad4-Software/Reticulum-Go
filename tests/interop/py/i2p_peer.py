#!/usr/bin/env python3
"""
Python I2P peer for Go interop.

Modes (INTEROP_I2P_MODE):
  client  Connect outbound to INTEROP_I2P_PEER (.b32.i2p). Prints ONLINE when
          a spawned peer interface reports online.
  server  Publish a connectable I2PInterface. Prints B32=<hash> then waits.

Requires a local SAM bridge (i2pd or Java I2P).
"""
import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS


def write_config(cfg_dir: str, mode: str, peer: str) -> None:
    lines = [
        "[reticulum]",
        "enable_transport = false",
        "share_instance = no",
        "loglevel = 4",
        "",
        "[interfaces]",
        "",
        "[[interop_i2p]]",
        "type = I2PInterface",
        "enabled = yes",
    ]
    if mode == "server":
        lines.append("connectable = yes")
    else:
        lines.append("connectable = no")
        lines.append(f"peers = {peer}")
    sam = os.environ.get("I2P_SAM_ADDRESS", "").strip()
    if sam:
        lines.append(f"sam_address = {sam}")
    path = os.path.join(cfg_dir, "config")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def find_i2p_parent():
    for iface in list(RNS.Transport.interfaces):
        if hasattr(iface, "b32") and hasattr(iface, "spawned_interfaces"):
            return iface
        if getattr(iface, "name", "") == "interop_i2p":
            return iface
    return None


def main() -> int:
    mode = os.environ.get("INTEROP_I2P_MODE", "client")
    peer = os.environ.get("INTEROP_I2P_PEER", "").strip()
    if mode == "client" and not peer:
        sys.stderr.write("INTEROP_I2P_PEER required for client mode\n")
        return 2

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_i2p_")
    write_config(cfg_dir, mode, peer)

    RNS.Reticulum(cfg_dir)
    deadline = time.time() + float(os.environ.get("INTEROP_I2P_TIMEOUT", "180"))

    if mode == "server":
        parent = None
        while time.time() < deadline:
            parent = find_i2p_parent()
            if parent is not None and getattr(parent, "b32", None):
                break
            time.sleep(1)
        if parent is None or not getattr(parent, "b32", None):
            sys.stderr.write("timed out waiting for connectable b32\n")
            return 1
        sys.stdout.write(f"B32={parent.b32}\n")
        sys.stdout.flush()
        while time.time() < deadline:
            spawned = getattr(parent, "spawned_interfaces", []) or []
            online = [s for s in spawned if getattr(s, "online", False)]
            if online:
                sys.stdout.write("CLIENT_ONLINE\n")
                sys.stdout.flush()
                break
            time.sleep(1)
        while True:
            time.sleep(5)
        return 0

    while time.time() < deadline:
        for iface in list(RNS.Transport.interfaces):
            if getattr(iface, "online", False) and getattr(iface, "OUT", False):
                sys.stdout.write("ONLINE\n")
                sys.stdout.flush()
                while True:
                    time.sleep(5)
                return 0
            spawned = getattr(iface, "spawned_interfaces", None)
            if spawned:
                for s in spawned:
                    if getattr(s, "online", False):
                        sys.stdout.write("ONLINE\n")
                        sys.stdout.flush()
                        while True:
                            time.sleep(5)
                        return 0
        time.sleep(1)
    sys.stderr.write("timed out waiting for I2P peer online\n")
    return 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(0)
