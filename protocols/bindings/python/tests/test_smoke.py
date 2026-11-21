# SPDX-License-Identifier: LicenseRef-Reticulum

import unittest

import rrc
from rrc.errors import Error


class SmokeTest(unittest.TestCase):
    def test_version(self) -> None:
        self.assertEqual(rrc.version(), rrc.API_VERSION)

    def test_envelope_roundtrip(self) -> None:
        sender = bytes(range(16))
        with rrc.Envelope.create(rrc.ffi.RRC_TYPE_MSG, sender) as env:
            env.set_room("lobby")
            env.set_body_text("hello")
            data = env.marshal()
        with rrc.Envelope.unmarshal(data) as got:
            self.assertEqual(got.body_text(), "hello")

    def test_node_lifecycle(self) -> None:
        node = rrc.Node.create()
        try:
            node.start()
            node.stop()
        finally:
            node.close()

    def test_identity_hash(self) -> None:
        with rrc.Identity.generate() as identity:
            self.assertEqual(len(identity.hash_bytes()), rrc.HASH_LEN)

    def test_error_invalid_handle(self) -> None:
        with self.assertRaises(Error) as ctx:
            rrc.Envelope(999999).body_text()
        self.assertIsInstance(ctx.exception, Error)
        self.assertEqual(ctx.exception.code, Error.INVALID_HANDLE)

    def test_normalize_sanitize(self) -> None:
        self.assertEqual(rrc.normalize_room("  #Lobby "), "#lobby")
        self.assertEqual(rrc.sanitize_nick(" alice "), "alice")


if __name__ == "__main__":
    unittest.main()
