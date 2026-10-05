#!/usr/bin/env python3
"""Exhaustive shared-instance RPC surface probe.

Connects to a running shared instance (Go or Python) as a client and calls
every client getter RNS exposes. Asserts the invariants Python tools rely
on:

- keys that Python servers always answer numerically must never come back
  as None (a nil msgpack reply unpacks to None and crashes callers doing
  comparisons, e.g. rngit medium_path_timeout),
- every interface_stats entry carries the full always-present key set.

Prints one line per RPC call and a final OK or FAIL summary.

Env: INTEROP_CONFIG_DIR points at the shared instance config directory.
"""

import os
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS

ZERO_DEST = b"\x00" * 16
ZERO_HASH = b"\x00" * 32
ZERO_IDENTITY = b"\x00" * 16

# Keys Python's get_interface_stats assigns unconditionally on every entry.
# Any of these missing turns into a KeyError in rnstatus and friends.
REQUIRED_IFACE_KEYS = [
    "name", "short_name", "hash", "type", "mtu",
    "rxb", "txb", "rxs", "txs",
    "arxb", "atxb", "arxc", "atxc", "arxs", "atxs",
    "prxb", "ptxb", "prxc", "ptxc", "prxs", "ptxs",
    "txdrp", "txdrb", "txstalled", "txbuffered",
    "incoming_announce_frequency", "outgoing_announce_frequency",
    "incoming_pr_frequency", "outgoing_pr_frequency",
    "announce_rate_target", "announce_rate_penalty", "announce_rate_grace",
    "held_announces",
    "burst_active", "burst_activated", "burst_count",
    "pr_burst_active", "pr_burst_activated", "pr_burst_count",
    "status", "mode", "gravity", "announces_to_internal",
    "clients", "autoconnect_source",
    "ifac_signature", "ifac_size", "ifac_netname",
    "protocol_violations", "ifac_violations", "packet_filter_hits",
]

# (label, callable, strict) - strict means the value must be a number.
CALLS_NUMERIC = [
    ("link_count", lambda r: r.get_link_count(), True),
    ("active_link_count", lambda r: r.get_active_link_count(), True),
    ("first_hop_timeout", lambda r: r.get_first_hop_timeout(ZERO_DEST), True),
    ("medium_path_timeout", lambda r: r.get_medium_path_timeout(), True),
    ("lowest_interface_bitrate", lambda r: r.get_lowest_interface_bitrate(), False),
]

CALLS_LOOSE = [
    ("next_hop", lambda r: r.get_next_hop(ZERO_DEST)),
    ("next_hop_if_name", lambda r: r.get_next_hop_if_name(ZERO_DEST)),
    ("blackholed_identities", lambda r: r.get_blackholed_identities()),
    ("is_blackholed", lambda r: r.is_blackholed(ZERO_IDENTITY)),
    ("profiling_results", lambda r: r.get_profiling_results()),
    ("packet_rssi", lambda r: r.get_packet_rssi(ZERO_HASH)),
    ("packet_snr", lambda r: r.get_packet_snr(ZERO_HASH)),
    ("packet_q", lambda r: r.get_packet_q(ZERO_HASH)),
]


def main() -> int:
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rns_rpc_")
    reticulum = RNS.Reticulum(cfg_dir)
    if not reticulum.is_connected_to_shared_instance:
        print("NOT_CLIENT")
        return 1

    failures = []

    for label, fn, strict in CALLS_NUMERIC:
        try:
            value = fn(reticulum)
        except Exception as exc:
            failures.append(f"{label}: raised {exc}")
            print(f"FAIL {label}: raised {exc}")
            continue
        if value is None:
            tag = "FAIL" if strict else "pass"
            if strict:
                failures.append(f"{label}: answered nil")
        elif strict and not isinstance(value, (int, float)):
            tag = "FAIL"
            failures.append(f"{label}: non-numeric {type(value).__name__}")
        else:
            tag = "pass"
        print(f"{tag} {label}: {value!r}")

    for label, fn in CALLS_LOOSE:
        try:
            value = fn(reticulum)
            print(f"pass {label}: {type(value).__name__}")
        except Exception as exc:
            failures.append(f"{label}: raised {exc}")
            print(f"FAIL {label}: raised {exc}")

    try:
        stats = reticulum.get_interface_stats()
        if not isinstance(stats, dict) or not isinstance(stats.get("interfaces"), list):
            failures.append("interface_stats: malformed payload")
            print("FAIL interface_stats: malformed payload")
        else:
            print(f"pass interface_stats: {len(stats['interfaces'])} interfaces")
            for entry in stats["interfaces"]:
                name = entry.get("name", "?")
                missing = [k for k in REQUIRED_IFACE_KEYS if k not in entry]
                if missing:
                    failures.append(f"interface_stats[{name}] missing: {','.join(missing)}")
                    print(f"FAIL iface {name}: missing {','.join(missing)}")
                else:
                    print(f"pass iface {name}: all required keys")
        pt = reticulum.get_path_table()
        rt = reticulum.get_rate_table()
        if pt is None:
            failures.append("path_table: nil")
        if rt is None:
            failures.append("rate_table: nil")
        print(f"pass path_table: {type(pt).__name__}")
        print(f"pass rate_table: {type(rt).__name__}")
    except Exception as exc:
        failures.append(f"interface_stats: raised {exc}")
        print(f"FAIL interface_stats: raised {exc}")

    if failures:
        print("FAIL")
        return 1
    print("OK")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        print(f"ERR {exc}")
        sys.exit(1)
