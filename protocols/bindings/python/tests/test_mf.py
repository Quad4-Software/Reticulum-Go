# SPDX-License-Identifier: LicenseRef-Reticulum

import unittest

import mf


class MFTest(unittest.TestCase):
    def test_roundtrip(self) -> None:
        sender = bytes(range(16))
        data = mf.pack(sender, "hello mf")
        got_sender, text = mf.unpack(data)
        self.assertEqual(text, "hello mf")
        self.assertEqual(got_sender, sender)


if __name__ == "__main__":
    unittest.main()
