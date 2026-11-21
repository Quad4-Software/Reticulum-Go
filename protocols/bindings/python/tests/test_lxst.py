# SPDX-License-Identifier: LicenseRef-Reticulum

import unittest

import lxst


class LXSTTest(unittest.TestCase):
    def test_signalling_roundtrip(self) -> None:
        signals = [
            lxst.STATUS_AVAILABLE,
            lxst.signal_preferred_profile(lxst.PROFILE_QUALITY_MEDIUM),
            lxst.signal_preferred_mode(lxst.MODE_FULL_DUPLEX),
        ]
        data = lxst.pack_signalling(signals)
        with lxst.packet(data) as handle:
            self.assertEqual(lxst.packet_signal_count(handle), len(signals))
            for i, want in enumerate(signals):
                self.assertEqual(lxst.packet_signal_at(handle, i), want)

    def test_frame_roundtrip(self) -> None:
        payload = b"\xde\xad\xbe\xef"
        data = lxst.pack_frame(lxst.CODEC_OPUS, payload)
        with lxst.packet(data) as handle:
            self.assertEqual(lxst.packet_frame_count(handle), 1)
            frame = lxst.packet_frame_at(handle, 0)
        codec, got = lxst.split_frame(frame)
        self.assertEqual(codec, lxst.CODEC_OPUS)
        self.assertEqual(got, payload)

    def test_telephony_hash(self) -> None:
        identity = bytes(range(16))
        h = lxst.telephony_hash(identity)
        self.assertEqual(len(h), lxst.HASH_LEN)


if __name__ == "__main__":
    unittest.main()
