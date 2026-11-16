# SPDX-License-Identifier: LicenseRef-Reticulum
"""Dump LXST 0.6.0 constants and umsgpack wire bytes for Go reference checks."""

import hashlib
import json
import sys

from LXST._version import __version__
from RNS.vendor import umsgpack as mp
from LXST.Codecs import CODEC2, NULL, OPUS, RAW
from LXST.Codecs.Codec2 import Codec2
from LXST.Codecs.Opus import Opus
from LXST.Primitives.Telephony import PRIMITIVE_NAME, Profiles, Signalling, Telephone
from LXST import APP_NAME


def hx(b):
    return bytes(b).hex()


def dest_hash(identity_hash, app_aspect):
    name_sum = hashlib.sha256(app_aspect.encode("utf-8")).digest()
    combined = name_sum[:10] + identity_hash
    return hashlib.sha256(combined).digest()[:16]


def opus_row(profile):
    return {
        "id": profile,
        "channels": Opus.profile_channels(profile),
        "rate": Opus.profile_samplerate(profile),
        "bitrate": Opus.profile_bitrate_ceiling(profile),
        "voip": Opus.profile_application(profile) == "voip",
    }


def main():
    ident = bytes(range(1, 17))
    out = {
        "version": __version__,
        "app": APP_NAME,
        "aspect": PRIMITIVE_NAME,
        "status": {
            "busy": Signalling.STATUS_BUSY,
            "rejected": Signalling.STATUS_REJECTED,
            "calling": Signalling.STATUS_CALLING,
            "available": Signalling.STATUS_AVAILABLE,
            "ringing": Signalling.STATUS_RINGING,
            "connecting": Signalling.STATUS_CONNECTING,
            "established": Signalling.STATUS_ESTABLISHED,
        },
        "preferred_mode": Signalling.PREFERRED_MODE,
        "preferred_profile": Signalling.PREFERRED_PROFILE,
        "codecs": {
            "raw": RAW,
            "opus": OPUS,
            "codec2": CODEC2,
            "null": NULL,
        },
        "profiles": {
            "ulbw": Profiles.BANDWIDTH_ULTRA_LOW,
            "vlbw": Profiles.BANDWIDTH_VERY_LOW,
            "lbw": Profiles.BANDWIDTH_LOW,
            "mq": Profiles.QUALITY_MEDIUM,
            "hq": Profiles.QUALITY_HIGH,
            "shq": Profiles.QUALITY_MAX,
            "ull": Profiles.LATENCY_ULTRA_LOW,
            "ll": Profiles.LATENCY_LOW,
            "default": Profiles.DEFAULT_PROFILE,
        },
        "available_profiles": Profiles.available_profiles(),
        "frame_times": {str(p): Profiles.get_frame_time(p) for p in Profiles.available_profiles()},
        "buffer_frames": {str(p): Profiles.get_buffer_frames(p) for p in Profiles.available_profiles()},
        "modes": {
            "full": Profiles.MODE_FULL_DUPLEX,
            "half": Profiles.MODE_HALF_DUPLEX,
            "default": Profiles.DEFAULT_MODE,
        },
        "opus": {
            "voice_low": opus_row(Opus.PROFILE_VOICE_LOW),
            "voice_medium": opus_row(Opus.PROFILE_VOICE_MEDIUM),
            "voice_high": opus_row(Opus.PROFILE_VOICE_HIGH),
            "voice_max": opus_row(Opus.PROFILE_VOICE_MAX),
            "audio_min": opus_row(Opus.PROFILE_AUDIO_MIN),
            "audio_low": opus_row(Opus.PROFILE_AUDIO_LOW),
            "audio_medium": opus_row(Opus.PROFILE_AUDIO_MEDIUM),
            "audio_high": opus_row(Opus.PROFILE_AUDIO_HIGH),
            "audio_max": opus_row(Opus.PROFILE_AUDIO_MAX),
        },
        "telephone": {
            "ring_time": Telephone.RING_TIME,
            "wait_time": Telephone.WAIT_TIME,
            "connect_time": Telephone.CONNECT_TIME,
            "announce_interval": Telephone.ANNOUNCE_INTERVAL,
            "announce_interval_min": Telephone.ANNOUNCE_INTERVAL_MIN,
            "dial_hz": Telephone.DIAL_TONE_FREQUENCY,
        },
        "allow": {
            "all": Telephone.ALLOW_ALL,
            "none": Telephone.ALLOW_NONE,
        },
        "codec2_headers": {str(k): int(v) for k, v in Codec2.MODE_HEADERS.items()},
        "dest_hash": hx(dest_hash(ident, APP_NAME + "." + PRIMITIVE_NAME)),
        "wire": {
            "busy": hx(mp.packb({0: [Signalling.STATUS_BUSY]})),
            "rejected": hx(mp.packb({0: [Signalling.STATUS_REJECTED]})),
            "calling": hx(mp.packb({0: [Signalling.STATUS_CALLING]})),
            "available": hx(mp.packb({0: [Signalling.STATUS_AVAILABLE]})),
            "ringing": hx(mp.packb({0: [Signalling.STATUS_RINGING]})),
            "connecting": hx(mp.packb({0: [Signalling.STATUS_CONNECTING]})),
            "established": hx(mp.packb({0: [Signalling.STATUS_ESTABLISHED]})),
            "pref_mq_fd": hx(mp.packb({0: [
                Signalling.STATUS_AVAILABLE,
                Signalling.PREFERRED_PROFILE + Profiles.QUALITY_MEDIUM,
                Signalling.PREFERRED_MODE + Profiles.MODE_FULL_DUPLEX,
            ]})),
            "frame_opus": hx(mp.packb({1: bytes([1, 9, 8, 7])})),
            "frame_codec2": hx(mp.packb({1: bytes([2, 6, 1, 2, 3])})),
        },
    }
    json.dump(out, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
