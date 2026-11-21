# SPDX-License-Identifier: LicenseRef-Reticulum

"""Python ctypes bindings for liblxst."""

from .codec import (
    LxstError,
    last_error,
    pack_frame,
    pack_signalling,
    packet,
    packet_destroy,
    packet_frame_at,
    packet_frame_count,
    packet_signal_at,
    packet_signal_count,
    profile_from_name,
    signal_preferred_mode,
    signal_preferred_profile,
    split_frame,
    telephony_hash,
    unpack,
    version,
)
from .ffi import CODEC_OPUS, HASH_LEN, MODE_FULL_DUPLEX, PROFILE_QUALITY_MEDIUM, STATUS_AVAILABLE

API_VERSION = "1.0"

__all__ = [
    "API_VERSION",
    "CODEC_OPUS",
    "HASH_LEN",
    "MODE_FULL_DUPLEX",
    "PROFILE_QUALITY_MEDIUM",
    "STATUS_AVAILABLE",
    "LxstError",
    "last_error",
    "pack_frame",
    "pack_signalling",
    "packet",
    "packet_destroy",
    "packet_frame_at",
    "packet_frame_count",
    "packet_signal_at",
    "packet_signal_count",
    "profile_from_name",
    "signal_preferred_mode",
    "signal_preferred_profile",
    "split_frame",
    "telephony_hash",
    "unpack",
    "version",
]
