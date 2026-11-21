#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

import sys
import time

import rrc
from rrc.errors import Error
from rrc.event import EventKind

HUB_LOCAL = "127.0.0.1:42530"
HUB_PEER = "127.0.0.1:42531"
CLI_LOCAL = "127.0.0.1:42531"
CLI_PEER = "127.0.0.1:42530"


def main() -> int:
    hub_node = rrc.Node.create()
    cli_node = rrc.Node.create()
    try:
        hub_node.add_udp_interface("H1", HUB_LOCAL, HUB_PEER)
        cli_node.add_udp_interface("C1", CLI_LOCAL, CLI_PEER)

        with rrc.Identity.generate() as id_h, rrc.Identity.generate() as id_c:
            hub_node.set_identity(id_h)
            cli_node.set_identity(id_c)
            hub_node.start()
            cli_node.start()

            with rrc.Hub.create(hub_node.handle, id_h.handle, "py-hub", "1.0") as hub:
                hub.start()
                hub.announce()
                hub_hash = hub.hash_bytes()
                id_h.seed_destination(hub_hash)

                deadline = time.time() + 15
                while time.time() < deadline:
                    if cli_node.has_path(hub_hash):
                        break
                    time.sleep(0.05)
                else:
                    print("path timeout", file=sys.stderr)
                    return 1

                with rrc.Client.dial(
                    cli_node.handle,
                    id_c.handle,
                    hub_hash,
                    "alice",
                    "py-client",
                    "1.0",
                    15000,
                ) as client:
                    client.join("#lobby")
                    joined = False
                    end = time.time() + 10
                    while time.time() < end:
                        try:
                            ev = client.event_poll(500)
                            if ev.kind == EventKind.JOINED:
                                joined = True
                                break
                        except Error as exc:
                            if exc.code != Error.TIMEOUT:
                                raise
                    if not joined:
                        print("join timeout", file=sys.stderr)
                        return 1

                    want = "hello from python hub-client"
                    client.send_msg("#lobby", want)

                    end = time.time() + 10
                    while time.time() < end:
                        try:
                            ev = hub.event_poll(500)
                            if ev.kind == EventKind.MSG and ev.body == want:
                                print("python-hub-client ok")
                                return 0
                        except Error as exc:
                            if exc.code != Error.TIMEOUT:
                                raise
                    print("hub did not receive message", file=sys.stderr)
                    return 1
    finally:
        hub_node.close()
        cli_node.close()


if __name__ == "__main__":
    sys.exit(main())
