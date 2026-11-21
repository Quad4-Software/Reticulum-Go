# SPDX-License-Identifier: LicenseRef-Reticulum

import unittest

import lxmf


class LXMFTest(unittest.TestCase):
    def test_roundtrip(self) -> None:
        identity = lxmf.identity_generate()
        try:
            dest = lxmf.identity_hash(identity)
            source = bytes(dest)
            msg = lxmf.message_create(dest, source, "hi", "body")
            try:
                data = lxmf.message_pack(msg, identity)
                got = lxmf.message_unpack(data)
                try:
                    content = lxmf.message_get_content(got)
                    self.assertEqual(content, "body")
                finally:
                    lxmf.message_destroy(got)
            finally:
                lxmf.message_destroy(msg)
        finally:
            lxmf.identity_destroy(identity)


if __name__ == "__main__":
    unittest.main()
