// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf;

public final class LxmfSmokeTest {
    public static void main(String[] args) {
        if (!Lxmf.API_VERSION.equals(Lxmf.version())) {
            fail("version mismatch: " + Lxmf.version());
        }

        try (Identity identity = Identity.generate()) {
            byte[] hash = identity.hashBytes();
            if (hash.length != Lxmf.HASH_LEN) {
                fail("hash bytes length");
            }
            if (identity.publicKey().length == 0) {
                fail("public key empty");
            }
            if (identity.deliveryHash().length != Lxmf.HASH_LEN) {
                fail("delivery hash length");
            }

            try (Message msg = Message.create(hash, hash, "hi", "hello lxmf")) {
                msg.setFieldsJson("{\"0x01\":\"hex:61\"}");
                if (msg.fieldCount() < 1) {
                    fail("field count");
                }
                byte[] packed = msg.pack(identity);
                if (packed.length == 0) {
                    fail("pack empty");
                }
                byte[] inner = msg.encryptedPayload();
                if (inner.length != packed.length - Lxmf.HASH_LEN) {
                    fail("encrypted payload length");
                }
                try (Message got = Message.unpack(packed)) {
                    if (!"hello lxmf".equals(got.content())) {
                        fail("content mismatch: " + got.content());
                    }
                    if (!"hi".equals(got.title())) {
                        fail("title mismatch: " + got.title());
                    }
                }
            }
        }

        System.out.println("java-lxmf smoke tests ok");
    }

    private static void fail(String msg) {
        System.err.println(msg);
        System.exit(1);
    }
}
