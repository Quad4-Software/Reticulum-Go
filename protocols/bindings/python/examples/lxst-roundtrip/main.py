#!/usr/bin/env python3
# SPDX-License-Identifier: LicenseRef-Reticulum

import sys

import lxst


def main() -> int:
    signals = [
        lxst.STATUS_AVAILABLE,
        lxst.signal_preferred_profile(lxst.PROFILE_QUALITY_MEDIUM),
        lxst.signal_preferred_mode(lxst.MODE_FULL_DUPLEX),
    ]
    data = lxst.pack_signalling(signals)
    with lxst.packet(data) as handle:
        if lxst.packet_signal_count(handle) != len(signals):
            print("signal count mismatch", file=sys.stderr)
            return 1
        for i, want in enumerate(signals):
            if lxst.packet_signal_at(handle, i) != want:
                print("signal mismatch", file=sys.stderr)
                return 1

    payload = b"\x01\x02\x03\x04"
    frame_pkt = lxst.pack_frame(lxst.CODEC_OPUS, payload)
    with lxst.packet(frame_pkt) as handle:
        frame = lxst.packet_frame_at(handle, 0)
    codec, got = lxst.split_frame(frame)
    if codec != lxst.CODEC_OPUS or got != payload:
        print("frame mismatch", file=sys.stderr)
        return 1

    print("python-lxst-roundtrip ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
