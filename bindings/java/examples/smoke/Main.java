// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

import io.quad4.rns.Event;
import io.quad4.rns.Node;
import io.quad4.rns.Rns;
import io.quad4.rns.RnsException;

public final class Main {
    public static void main(String[] args) {
        if (!Rns.API_VERSION.equals(Rns.version())) {
            System.err.println("unexpected version: " + Rns.version());
            System.exit(1);
        }
        try (Node node = Node.create()) {
            node.start();
            try {
                Event.poll(node, 10, 0);
                System.err.println("expected timeout poll on idle node");
                System.exit(1);
            } catch (RnsException e) {
                if (e.getCode() != RnsException.TIMEOUT) {
                    System.err.println("expected timeout poll on idle node");
                    System.exit(1);
                }
            }
            node.stop();
        }
        System.out.println("java-smoke ok");
    }
}
