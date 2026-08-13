#!/usr/bin/env python3
import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

APP_NAME = "rncp"
ASPECT = "receive"


def write_config(cfg_dir: str, listen_port: int, forward_port: int) -> None:
    config_path = os.path.join(cfg_dir, "config")
    with open(config_path, "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = false",
                    "share_instance = no",
                    "loglevel = 2",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_udp]]",
                    "type = UDPInterface",
                    "enabled = yes",
                    "listen_ip = 127.0.0.1",
                    f"listen_port = {listen_port}",
                    "forward_ip = 127.0.0.1",
                    f"forward_port = {forward_port}",
                    "",
                ],
            ),
        )


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    save_path = os.environ.get(
        "INTEROP_RNCP_SAVE", tempfile.mkdtemp(prefix="rncp_save_")
    )

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_rncp_")

    write_config(cfg_dir, listen_port, forward_port)
    RNS.Reticulum(cfg_dir)

    identity = RNS.Identity()
    destination = RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        APP_NAME,
        ASPECT,
    )

    def on_link(link):
        link.set_resource_strategy(RNS.Link.ACCEPT_ALL)

        def on_concluded(resource):
            if resource.status != RNS.Resource.COMPLETE:
                return
            try:
                if hasattr(resource.data, "read"):
                    if hasattr(resource.data, "seek"):
                        resource.data.seek(0)
                    data = resource.data.read()
                else:
                    data = resource.data
                if isinstance(data, str):
                    data = data.encode("utf-8")
            except Exception as exc:
                sys.stderr.write("resource read: " + str(exc) + "\n")
                return
            name = "received.bin"
            if resource.metadata and "name" in resource.metadata:
                try:
                    name = os.path.basename(resource.metadata["name"].decode("utf-8"))
                except Exception:
                    pass
            path = os.path.join(save_path, name)
            with open(path, "wb") as f:
                f.write(data)
            sys.stdout.write("FILE_OK\n")
            sys.stdout.flush()

        link.set_resource_concluded_callback(on_concluded)

    destination.set_link_established_callback(on_link)

    sys.stdout.write("READY\n")
    sys.stdout.write(destination.hash.hex() + "\n")
    sys.stdout.flush()
    destination.announce()

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
