// SPDX-License-Identifier: LicenseRef-Reticulum

import io.quad4.lxmf.Identity;
import io.quad4.lxmf.Lxmf;
import io.quad4.lxmf.Message;

public final class Main {
    public static void main(String[] args) {
        try (Identity identity = Identity.generate()) {
            byte[] hash = identity.hashBytes();
            try (Message msg = Message.create(hash, hash, "hi", "hello lxmf")) {
                byte[] packed = msg.pack(identity);
                byte[] inner = msg.encryptedPayload();
                if (inner.length != packed.length - Lxmf.HASH_LEN) {
                    System.err.println("encrypted payload length mismatch");
                    System.exit(1);
                }
                try (Message got = Message.unpack(packed)) {
                    if (!"hello lxmf".equals(got.content())) {
                        System.err.println("content mismatch");
                        System.exit(1);
                    }
                }
            }
        }
        System.out.println("java-lxmf-roundtrip ok");
    }
}
