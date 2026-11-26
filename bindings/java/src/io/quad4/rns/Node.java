// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import io.quad4.rns.ffi.RnsLibrary;

/** Opaque node handle. */
public final class Node implements AutoCloseable {
    private long handle;
    private RnsLibrary.RnsEventCallback eventCallback;

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

    public void reloadConfig() {
        Rns.check(RnsLibrary.INSTANCE.rns_node_reload_config(handle));
    }

    public void refreshPaths(byte[][] destHashes) {
        if (destHashes == null || destHashes.length == 0) {
            Rns.check(RnsLibrary.INSTANCE.rns_node_refresh_paths(handle, null, 0));
            return;
        }
        byte[] flat = new byte[destHashes.length * Rns.HASH_LEN];
        for (int i = 0; i < destHashes.length; i++) {
            if (destHashes[i] == null || destHashes[i].length != Rns.HASH_LEN) {
                throw new RnsException(RnsException.INVALID_ARG);
            }
            System.arraycopy(destHashes[i], 0, flat, i * Rns.HASH_LEN, Rns.HASH_LEN);
        }
        Rns.check(RnsLibrary.INSTANCE.rns_node_refresh_paths(handle, flat, destHashes.length));
    }

    public void setEventCallback(java.util.function.Consumer<Event> handler) {
        if (handler == null) {
            Rns.check(RnsLibrary.INSTANCE.rns_set_event_callback(handle, null, null));
            eventCallback = null;
            return;
        }
        RnsLibrary.RnsEventCallback callback = (raw, user) -> handler.accept(Event.fromRaw(raw));
        eventCallback = callback;
        Rns.check(RnsLibrary.INSTANCE.rns_set_event_callback(handle, callback, null));
    }

    @Override
    public void close() {
        if (handle != 0) {
            RnsLibrary.INSTANCE.rns_set_event_callback(handle, null, null);
            eventCallback = null;
            RnsLibrary.INSTANCE.rns_node_destroy(handle);
            handle = 0;
        }
    }
}
