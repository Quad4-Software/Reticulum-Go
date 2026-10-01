#!/usr/bin/env python3
"""Python interface-management probe for shared-instance interop.

Connects as a shared-instance client to an already-running Go server and
exercises the RNS 1.5.5 manage RPC path: attach_interface, detach_interface
and reload_interface via the public RNS.Reticulum wrappers.

Prints one result token per line; the Go side asserts on them.
"""

import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

IFACE = os.environ.get("INTEROP_IFACE", "mgmt_udp")


def fmt(value):
    if value is True:
        return "TRUE"
    if value is False:
        return "FALSE"
    if value is None:
        return "NONE"
    return f"BAD:{value!r}"


def main() -> int:
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_manage_")

    reticulum = RNS.Reticulum(cfg_dir)
    if not reticulum.is_connected_to_shared_instance:
        sys.stdout.write("NOT_CLIENT\n")
        sys.stdout.flush()
        return 1

    out = []

    # The interface exists and is running: detach -> True.
    out.append("detach_running=" + fmt(reticulum.detach_interface(IFACE)))
    # Now it is gone: detach -> None.
    out.append("detach_missing=" + fmt(reticulum.detach_interface(IFACE)))
    # Attach from config (enabled=no in file, force_attach brings it up).
    out.append("attach_config=" + fmt(reticulum.attach_interface(IFACE)))
    # Attaching an already-running interface -> False.
    out.append("attach_existing=" + fmt(reticulum.attach_interface(IFACE)))
    # Reload a running interface -> True.
    out.append("reload_running=" + fmt(reticulum.reload_interface(IFACE)))
    # Attaching/detaching/reloading a name with no config entry -> None.
    out.append("attach_noconfig=" + fmt(reticulum.attach_interface("no_such_iface_xyz")))
    out.append("detach_noconfig=" + fmt(reticulum.detach_interface("no_such_iface_xyz")))
    out.append("reload_noconfig=" + fmt(reticulum.reload_interface("no_such_iface_xyz")))

    sys.stdout.write("\n".join(out) + "\n")
    sys.stdout.flush()
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        sys.stdout.write(f"ERR {exc}\n")
        sys.stdout.flush()
        sys.exit(1)
