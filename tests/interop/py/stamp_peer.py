#!/usr/bin/env python3
"""Stamped announce / link peer for Go-Python stamp interop (RNS 1.6.0).

Modes:
  server  - stamped announce + link echo server (prints DEST/ANNOUNCED/LINK/echo)
  client  - reads stamped announce, recalls lr_cost, links and echoes
  announce- stamped announce only (for Go receive checks)
"""
import os, sys, tempfile, time

_rp = os.environ.get("RETICULUM_PATH")
if _rp:
    sys.path.insert(0, os.path.abspath(_rp))

import RNS
import RNS.Stamps as Stamps

APP, ASPECT = "interop_pygo", "linksvc"
MODE = os.environ.get("INTEROP_MODE", "server")



def write_config(cfg_dir, listen_port, forward_port):
    lines = [
        "[reticulum]", "enable_transport = true", "share_instance = no",
        "loglevel = 7", ""]
    if os.environ.get("INTEROP_ENFORCE") == "1":
        lines += ["[stamps]", "enforce_stamps = yes", ""]
    lines += ["[interfaces]", "",
        "[[interop_udp]]", "type = UDPInterface", "enabled = yes",
        "listen_ip = 127.0.0.1", f"listen_port = {listen_port}",
        "forward_ip = 127.0.0.1", f"forward_port = {forward_port}", ""]
    with open(os.path.join(cfg_dir, "config"), "w") as f:
        f.write("\n".join(lines))


def main():
    cfg_dir = tempfile.mkdtemp(prefix="rns_stamp_")
    listen = int(os.environ["INTEROP_LISTEN_PORT"])
    forward = int(os.environ["INTEROP_FORWARD_PORT"])
    write_config(cfg_dir, listen, forward)
    RNS.Reticulum(configdir=cfg_dir)
    if os.environ.get("PYLOG_STDOUT") == "1":
        RNS.logdest = RNS.LOG_STDOUT
        RNS.loglevel = int(os.environ.get("PYLOG_LEVEL", "7"))
    # Lower all stamp costs AFTER Reticulum init (apply_config resets the
    # class attrs) but BEFORE creating destinations so generation is fast.
    RNS.Transport.DESTINATION_VALUE = float(os.environ.get("DEST_VALUE", "10"))
    RNS.Transport.DESTINATION_COST = float(os.environ.get("DEST_COST", "8"))
    RNS.Transport.LR_COST = float(os.environ.get("LR_COST", "12.5"))
    RNS.Transport.STAMP_DESTINATIONS = os.environ.get("STAMP_DESTS", "yes") == "yes"
    RNS.Transport.PR_VALUE = 10
    RNS.Transport.PR_COST = float(os.environ.get("PR_COST", "8"))
    RNS.Transport.PR_STAMP_TIMEOUT = 60
    if os.environ.get("STAMP_PRS", "") == "yes":
        RNS.Transport.STAMP_PRS = True
    ident = RNS.Identity()
    dest = RNS.Destination(ident, RNS.Destination.IN, RNS.Destination.SINGLE, APP, ASPECT)
    link_cost = float(os.environ.get("INTEROP_LINK_COST", "9"))
    dest.set_link_cost(link_cost)
    print("DEST", dest.hash.hex(), flush=True)

    if MODE == "server":
        dest.set_link_established_callback(lambda l: (
            l.set_link_closed_callback(lambda _: None),
            l.set_packet_callback(lambda m, p: RNS.Packet(l, m).send()),
            print("LINK", l.stamp_cost, flush=True)))
        print("READY", flush=True)
        no_announce = os.environ.get("NO_ANNOUNCE") == "1"
        deadline = time.time() + 90
        next_announce = 0.0
        while time.time() < deadline:
            # Re-announce periodically: the first announce can race peer
            # startup, and stamped announces defer until generation ends.
            if not no_announce and time.time() >= next_announce:
                dest.announce()
                next_announce = time.time() + 3
            time.sleep(0.2)
    elif MODE == "announce":
        print("READY", flush=True)
        deadline = time.time() + 30
        next_announce = 0.0
        while time.time() < deadline:
            if time.time() >= next_announce:
                dest.announce()
                next_announce = time.time() + 3
            time.sleep(0.2)
    elif MODE == "sink":
        target = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"])
        print("READY", flush=True)
        deadline = time.time() + float(os.environ.get("INTEROP_WAIT", "30"))
        while time.time() < deadline:
            if RNS.Transport.has_path(target):
                sv = RNS.Identity.recall_stamp_value(target)
                print("PATH", "SV", sv, flush=True)
                break
            time.sleep(0.3)
        else:
            sys.exit("NO_PATH")
    elif MODE == "requestor":
        target = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"])
        print("READY", flush=True)
        deadline = time.time() + float(os.environ.get("INTEROP_WAIT", "30"))
        nxt = 0.0
        while time.time() < deadline:
            if RNS.Transport.has_path(target):
                print("PATH", flush=True)
                break
            if time.time() >= nxt:
                RNS.Transport.request_path(target)
                nxt = time.time() + 2
            time.sleep(0.3)
        else:
            sys.exit("NO_PATH")
    elif MODE == "client":
        target = bytes.fromhex(os.environ["INTEROP_GO_DEST_HASH"])
        deadline = time.time() + 60
        while time.time() < deadline and not RNS.Transport.has_path(target):
            RNS.Transport.request_path(target)
            time.sleep(0.5)
        if not RNS.Transport.has_path(target):
            sys.exit("NO_PATH")
        lrc = RNS.Identity.recall_link_cost(target)
        sv = RNS.Identity.recall_stamp_value(target)
        print("LR", lrc, "SV", sv, flush=True)
        remote = RNS.Identity.recall(target)
        if remote is None:
            sys.exit("NO_IDENTITY")
        out = RNS.Destination(remote, RNS.Destination.OUT, RNS.Destination.SINGLE, APP, ASPECT)
        link = RNS.Link(out)
        link.set_link_established_callback(lambda l: print("ESTABLISHED", l.stamp_cost, flush=True))
        deadline = time.time() + 60
        while time.time() < deadline and link.status != RNS.Link.ACTIVE:
            time.sleep(0.2)
        if link.status != RNS.Link.ACTIVE:
            sys.exit("NO_LINK")
        print("OK", flush=True)
        time.sleep(10)


main()
