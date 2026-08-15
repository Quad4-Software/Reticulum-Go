// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import io.quad4.rns.ffi.RnsLibrary;

/** Opaque node handle. */
public final class Node implements AutoCloseable {
    private long handle;

    Node(long handle) {
        this.handle = handle;
    }

    public static Node create() {
        return create("");
    }

    public static Node create(String configPath) {
        String path = configPath == null ? "" : configPath;
        long h = RnsLibrary.INSTANCE.rns_node_create(path);
        if (h == 0) {
            throw new RnsException(RnsException.INTERNAL);
        }
        return new Node(h);
    }

    public long handle() {
        return handle;
    }

    public void start() {
        Rns.check(RnsLibrary.INSTANCE.rns_node_start(handle));
    }

    public void stop() {
        Rns.check(RnsLibrary.INSTANCE.rns_node_stop(handle));
    }

    public void setIdentity(Identity identity) {
        Rns.check(RnsLibrary.INSTANCE.rns_node_set_identity(handle, identity.handle()));
    }

    public void pause() {
        Rns.check(RnsLibrary.INSTANCE.rns_node_pause(handle));
    }

    public void resume() {
        Rns.check(RnsLibrary.INSTANCE.rns_node_resume(handle));
    }

    @Override
    public void close() {
        if (handle != 0) {
            RnsLibrary.INSTANCE.rns_node_destroy(handle);
            handle = 0;
        }
    }
}
