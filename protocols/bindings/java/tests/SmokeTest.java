// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import io.quad4.rrc.ffi.RrcLibrary;

public final class SmokeTest {
    public static void main(String[] args) {
        if (!Rrc.API_VERSION.equals(Rrc.version())) {
            fail("version mismatch: " + Rrc.version());
        }

        try (Node node = Node.create()) {
            node.start();
            node.stop();
        }

        try (Identity identity = Identity.generate()) {
            if (identity.hashBytes().length != Rrc.HASH_LEN) {
                fail("hash bytes length");
            }
        }

        byte[] sender = new byte[Rrc.HASH_LEN];
        for (int i = 0; i < sender.length; i++) {
            sender[i] = (byte) i;
        }
        try (Envelope env = Envelope.create(RrcLibrary.RRC_TYPE_MSG, sender)) {
            env.setRoom("lobby");
            env.setBodyText("hello");
            byte[] data = env.marshal();
            try (Envelope got = Envelope.unmarshal(data)) {
                if (!"hello".equals(got.bodyText())) {
                    fail("body mismatch");
                }
            }
        }

        if (!"#lobby".equals(Util.normalizeRoom("  #Lobby "))) {
            fail("normalize room");
        }
        if (!"alice".equals(Util.sanitizeNick(" alice "))) {
            fail("sanitize nick");
        }

        System.out.println("java smoke tests ok");
    }

    private static void fail(String msg) {
        System.err.println(msg);
        System.exit(1);
    }
}
