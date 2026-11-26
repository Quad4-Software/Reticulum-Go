// SPDX-License-Identifier: LicenseRef-Reticulum
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

        try (Node node = Node.create()) {
            expectCode(() -> node.reloadConfig(), RnsException.INVALID_ARG, "reload without config");
            try {
                java.nio.file.Path config = java.nio.file.Files.createTempFile("rns-java-reload", ".conf");
                java.nio.file.Files.write(config, "[reticulum]\nenable_transport = no\nin_memory_storage = yes\n".getBytes());
                try (Node configured = Node.create(config.toString())) {
                    configured.reloadConfig();
                }
                java.nio.file.Files.deleteIfExists(config);
            } catch (java.io.IOException e) {
                throw new RuntimeException(e);
            }
            expectCode(() -> node.refreshPaths(new byte[0][0]), RnsException.OK, "refresh empty");
            byte[] unknown = new byte[16];
            expectCode(() -> Link.fromId(node, unknown), RnsException.NOT_FOUND, "link from id miss");
            node.setEventCallback(event -> { });
            node.setEventCallback(null);
            try (Identity identity = Identity.generate();
                    Destination dest = Destination.create(node, identity, "javatest", java.util.Collections.emptyList(), false)) {
                dest.setProofStrategy(0);
                expectCode(() -> dest.setProofStrategy(255), RnsException.INVALID_ARG, "bad proof strategy");
                java.nio.file.Path file = java.nio.file.Files.createTempFile("rns-java", ".txt");
                java.nio.file.Files.write(file, "signed".getBytes());
                byte[] sig = Rsg.signFile(identity, file.toString());
                Rsg.verifyFile(sig, file.toString(), new byte[0]);
                java.nio.file.Files.write(file, "tampered".getBytes());
                expectCode(() -> Rsg.verifyFile(sig, file.toString(), new byte[0]), RnsException.INVALID_ARG, "tampered rsg");
                java.nio.file.Files.deleteIfExists(file);
            }
        } catch (java.io.IOException e) {
            throw new RuntimeException(e);
        }

        System.out.println("java smoke tests ok");
    }

    private interface ThrowingRunnable {
        void run();
    }

    private static void expectCode(ThrowingRunnable fn, int code, String name) {
        try {
            fn.run();
            if (code != RnsException.OK) {
                fail(name + ": expected exception");
            }
        } catch (RnsException e) {
            if (e.getCode() != code) {
                fail(name + ": code " + e.getCode());
            }
        }
    }

    private static void fail(String msg) {
        System.err.println(msg);
        System.exit(1);
    }
}
