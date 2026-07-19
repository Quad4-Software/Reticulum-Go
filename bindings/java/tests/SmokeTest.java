// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

public final class SmokeTest {
    public static void main(String[] args) {
        if (!Rns.API_VERSION.equals(Rns.version())) {
            fail("version mismatch: " + Rns.version());
        }

        try (Node node = Node.create()) {
            node.start();
            try {
                Event.poll(node, 10, 0);
                fail("expected timeout");
            } catch (RnsException e) {
                if (e.getCode() != RnsException.TIMEOUT) {
                    fail("expected timeout code");
                }
            }
            node.stop();
        }

        try (Identity identity = Identity.generate()) {
            String hex = identity.hashHex();
            if (hex.length() != 32) {
                fail("hash hex length");
            }
            if (identity.hashBytes().length != 16) {
                fail("hash bytes length");
            }
            byte[] msg = "hello-rns".getBytes();
            byte[] sig = identity.sign(msg);
            identity.verify(msg, sig);
            byte[] pub = identity.publicKey();
            try (Identity onlyPub = Identity.fromPublicKey(pub)) {
                onlyPub.verify(msg, sig);
            }
        }

        System.out.println("java smoke tests ok");
    }

    private static void fail(String msg) {
        System.err.println(msg);
        System.exit(1);
    }
}
