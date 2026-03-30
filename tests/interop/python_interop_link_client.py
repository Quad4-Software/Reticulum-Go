#!/usr/bin/env python3
import os
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

INTEROP_APP = "interop_pygo"
INTEROP_ASPECT = "linksvc"


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
                ]
            )
        )


def peer_destination(go_hash: bytes):
    identity = RNS.Identity.recall(go_hash)
    if identity is None:
        return None
    return RNS.Destination(
        identity,
        RNS.Destination.IN,
        RNS.Destination.SINGLE,
        INTEROP_APP,
        INTEROP_ASPECT,
    )


def main() -> int:
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    go_hash_hex = os.environ["INTEROP_GO_DEST_HASH"].strip()
    go_hash = bytes.fromhex(go_hash_hex)
    mode = os.environ.get("INTEROP_LINK_CLIENT_MODE", "echo")

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR")
    if not cfg_dir:
        cfg_dir = tempfile.mkdtemp(prefix="rns_interop_client_")

    write_config(cfg_dir, listen_port, forward_port)
    RNS.Reticulum(cfg_dir)

    sys.stdout.write("READY\n")
    sys.stdout.flush()

    deadline = time.time() + 60.0
    dest = None
    while time.time() < deadline:
        dest = peer_destination(go_hash)
        if dest is not None:
            break
        RNS.Transport.request_path(go_hash)
        time.sleep(0.12)

    if dest is None:
        sys.stderr.write("timeout: could not recall identity for Go destination\n")
        return 1

    def on_link_established(link):
        if mode == "echo":

            def got(message, packet):
                expect = os.environ.get("INTEROP_ECHO_EXPECT", "interop-ping").encode("utf-8")
                if message == expect:
                    sys.stdout.write("ECHO_OK\n")
                    sys.stdout.flush()

            link.set_packet_callback(got)
            send_payload = os.environ.get("INTEROP_ECHO_SEND", "interop-ping").encode("utf-8")
            RNS.Packet(link, send_payload).send()

        elif mode == "resource_send":

            def server_packet_received(message, packet):
                pass

            link.set_packet_callback(server_packet_received)
            link.set_resource_strategy(RNS.Link.ACCEPT_ALL)

            def on_res_concluded(resource):
                if resource.status == RNS.Resource.COMPLETE:
                    sys.stdout.write("RESOURCE_SENT_OK\n")
                    sys.stdout.flush()
                elif resource.status == RNS.Resource.REJECTED:
                    sys.stdout.write("RESOURCE_REJECTED\n")
                    sys.stdout.flush()
                elif resource.status == RNS.Resource.FAILED:
                    sys.stderr.write("resource send failed\n")
                    sys.stderr.flush()

            payload = os.environ.get("INTEROP_RESOURCE_SEND", "interop-resource-payload").encode("utf-8")
            auto_compress = os.environ.get("INTEROP_RESOURCE_COMPRESS", "0") == "1"
            RNS.Resource(
                payload,
                link,
                auto_compress=auto_compress,
                callback=on_res_concluded,
            )

        elif mode == "request":

            def on_resp(receipt):
                try:
                    if receipt.response == b"PONG_FROM_GO":
                        sys.stdout.write("REQUEST_OK\n")
                        sys.stdout.flush()
                except Exception as exc:
                    sys.stderr.write("request callback: " + str(exc) + "\n")
                    sys.stderr.flush()

            path = os.environ.get("INTEROP_REQUEST_PATH", "interop_req_path")
            payload = os.environ.get("INTEROP_REQUEST_PAYLOAD", "ping").encode("utf-8")
            link.request(path, payload, response_callback=on_resp)

        else:
            sys.stderr.write("unknown INTEROP_LINK_CLIENT_MODE\n")
            return 1

    RNS.Link(dest, on_link_established)

    while True:
        time.sleep(60.0)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
