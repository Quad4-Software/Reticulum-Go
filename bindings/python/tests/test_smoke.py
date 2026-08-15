# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

import unittest

import rns
from rns.errors import Error


class SmokeTest(unittest.TestCase):
    def test_version(self) -> None:
        self.assertEqual(rns.version(), rns.API_VERSION)

    def test_node_lifecycle(self) -> None:
        node = rns.Node.create()
        try:
            node.start()
            with self.assertRaises(Error) as ctx:
                node.event_poll(10)
            self.assertEqual(ctx.exception.code, Error.TIMEOUT)
            node.stop()
        finally:
            node.close()

    def test_identity_sign_verify(self) -> None:
        with rns.Identity.generate() as identity:
            hex_hash = identity.hash_hex()
            self.assertEqual(len(hex_hash), 32)
            self.assertEqual(len(identity.hash_bytes()), 16)
            msg = b"hello-rns"
            sig = identity.sign(msg)
            identity.verify(msg, sig)
            pub = identity.public_key()
            with rns.Identity.from_public_key(pub) as only_pub:
                only_pub.verify(msg, sig)


if __name__ == "__main__":
    unittest.main()
