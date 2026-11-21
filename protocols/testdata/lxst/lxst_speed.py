# SPDX-License-Identifier: LicenseRef-Reticulum
"""CPU timing for LXST 0.5.3 pack, unpack, dest hash, and Opus."""

import hashlib
import json
import sys
import time

import numpy as np
from RNS.vendor import umsgpack as mp

from LXST import APP_NAME
from LXST.Codecs import OPUS
from LXST.Codecs.Opus import Opus
from LXST.Primitives.Telephony import PRIMITIVE_NAME, Signalling

FIELD_SIGNALLING = 0x00
FIELD_FRAMES = 0x01


def pct(sorted_vals, p):
    if not sorted_vals:
        return 0
    idx = int(round((p / 100.0) * (len(sorted_vals) - 1)))
    if idx < 0:
        idx = 0
    if idx >= len(sorted_vals):
        idx = len(sorted_vals) - 1
    return int(sorted_vals[idx])


def summarize(samples):
    if not samples:
        return {"n": 0, "min": 0, "p50": 0, "p95": 0, "p99": 0, "max": 0, "mean": 0}
    s = sorted(samples)
    mean = int(sum(s) / len(s))
    return {
        "n": len(s),
        "min": int(s[0]),
        "p50": pct(s, 50),
        "p95": pct(s, 95),
        "p99": pct(s, 99),
        "max": int(s[-1]),
        "mean": mean,
    }


def bench(fn, n, warmup):
    for _ in range(warmup):
        fn()
    out = [0] * n
    for i in range(n):
        t0 = time.perf_counter_ns()
        fn()
        out[i] = time.perf_counter_ns() - t0
    return summarize(out)


def dest_hash(identity_hash):
    name_sum = hashlib.sha256((APP_NAME + "." + PRIMITIVE_NAME).encode("utf-8")).digest()
    combined = name_sum[:10] + identity_hash
    return hashlib.sha256(combined).digest()[:16]


def main():
    n = int(sys.argv[1]) if len(sys.argv) > 1 else 20000
    warmup = int(sys.argv[2]) if len(sys.argv) > 2 else 3000
    opus_n = int(sys.argv[3]) if len(sys.argv) > 3 else 400
    opus_warmup = max(20, opus_n // 10)

    sigs = [Signalling.STATUS_AVAILABLE, Signalling.STATUS_RINGING]
    packed_sig = mp.packb({FIELD_SIGNALLING: sigs})
    payload = bytes(range(80))
    frame = bytes([OPUS]) + payload
    packed_frame = mp.packb({FIELD_FRAMES: frame})
    ident = bytes(range(1, 17))

    result = {
        "pack_signalling_ns": bench(lambda: mp.packb({FIELD_SIGNALLING: sigs}), n, warmup),
        "unpack_signalling_ns": bench(lambda: mp.unpackb(packed_sig), n, warmup),
        "pack_frame_ns": bench(lambda: mp.packb({FIELD_FRAMES: frame}), n, warmup),
        "unpack_frame_ns": bench(lambda: mp.unpackb(packed_frame), n, warmup),
        "dest_hash_ns": bench(lambda: dest_hash(ident), n, warmup),
    }

    opus_ok = False
    opus_err = ""
    try:
        codec = Opus(profile=Opus.PROFILE_VOICE_MEDIUM)
        codec.source = type("S", (), {"samplerate": 24000, "channels": 1})()
        codec.sink = type("K", (), {"samplerate": 48000, "channels": 1})()
        samples = 1440
        t = np.arange(samples, dtype=np.float32) / 24000.0
        pcm = np.zeros((samples, 1), dtype=np.float32)
        pcm[:, 0] = 0.2 * np.sin(2 * np.pi * 440.0 * t)
        encoded = codec.encode(pcm)
        _ = codec.decode(encoded)

        def do_enc():
            codec.encode(pcm)

        def do_dec():
            codec.decode(encoded)

        result["opus_encode_ns"] = bench(do_enc, opus_n, opus_warmup)
        result["opus_decode_ns"] = bench(do_dec, opus_n, opus_warmup)
        result["opus_payload_bytes"] = len(encoded)
        opus_ok = True
    except Exception as e:
        opus_err = str(e)
        result["opus_encode_ns"] = summarize([])
        result["opus_decode_ns"] = summarize([])

    result["opus_ok"] = opus_ok
    result["opus_error"] = opus_err
    json.dump(result, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
