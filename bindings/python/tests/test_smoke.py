# SPDX-License-Identifier: LicenseRef-Reticulum
# Copyright (c) 2024-2026 Quad4.io

import os
import tempfile
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

    def test_new_binding_surface(self) -> None:
        node = rns.Node.create()
        try:
            with self.assertRaises(Error) as ctx:
                node.reload_config()
            self.assertEqual(ctx.exception.code, Error.INVALID_ARG)
            with tempfile.TemporaryDirectory() as td:
                config_path = os.path.join(td, "config")
                with open(config_path, "w", encoding="utf-8") as f:
                    f.write("[reticulum]\nenable_transport = no\nin_memory_storage = yes\n")
                configured = rns.Node.create(config_path)
                try:
                    configured.reload_config()
                finally:
                    configured.close()
            unknown = bytes(16)
            with self.assertRaises(Error) as ctx:
                rns.destination_encrypt(unknown, b"x")
            self.assertEqual(ctx.exception.code, Error.NOT_FOUND)
            with self.assertRaises(Error) as ctx:
                node.packet_send(unknown, b"x")
            self.assertIn(ctx.exception.code, (Error.STATE, Error.NOT_FOUND))
            with self.assertRaises(Error) as ctx:
                rns.link_from_id(node, bytes(16))
            self.assertEqual(ctx.exception.code, Error.NOT_FOUND)
            with rns.Identity.generate() as identity:
                dest = rns.Destination.create(node, identity, "pytest", [], False)
                try:
                    dest.set_proof_strategy(0)
                    with self.assertRaises(Error):
                        dest.set_proof_strategy(255)
                finally:
                    dest.close()
                with tempfile.TemporaryDirectory() as td:
                    path = os.path.join(td, "msg.txt")
                    with open(path, "wb") as f:
                        f.write(b"signed")
                    blob = rns.rsg_sign_file(identity, path)
                    rns.rsg_verify_file(blob, path)
                    with open(path, "ab") as f:
                        f.write(b"x")
                    with self.assertRaises(Error):
                        rns.rsg_verify_file(blob, path)
            node.set_event_callback(lambda ev: None)
            node.set_event_callback(None)
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
