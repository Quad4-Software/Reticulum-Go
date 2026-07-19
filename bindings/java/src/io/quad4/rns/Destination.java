// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.StringArray;
import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;
import java.util.Collections;
import java.util.List;

/** Opaque destination handle. */
public final class Destination implements AutoCloseable {
    private long handle;

    Destination(long handle) {
        this.handle = handle;
    }

    public static Destination create(
            Node node, Identity identity, String appName, List<String> aspects, boolean acceptsLinks) {
        if (appName == null || appName.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        List<String> aspectList = aspects == null ? Collections.emptyList() : aspects;
        StringArray aspectArray = null;
        if (!aspectList.isEmpty()) {
            aspectArray = new StringArray(aspectList.toArray(new String[0]));
        }
        long idHandle = identity == null ? 0 : identity.handle();
        long h =
                RnsLibrary.INSTANCE.rns_destination_create(
                        node.handle(),
                        idHandle,
                        appName,
                        aspectArray,
                        aspectList.size(),
                        acceptsLinks ? 1 : 0);
        if (h == 0) {
            throw new RnsException(RnsException.INTERNAL);
        }
        return new Destination(h);
    }

    public void announce(byte[] appData) {
        byte[] payload = appData == null ? new byte[0] : appData;
        Rns.check(RnsLibrary.INSTANCE.rns_destination_announce(handle, payload, payload.length));
    }

    public byte[] hash() {
        byte[] out = new byte[Rns.HASH_LEN];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_destination_hash(handle, out, out.length, written));
        if (written.getValue() != Rns.HASH_LEN) {
            throw new RnsException(RnsException.TRUNCATED);
        }
        return out;
    }

    public void registerRequestHandler(String path) {
        if (path == null || path.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        Rns.check(RnsLibrary.INSTANCE.rns_destination_register_request_handler(handle, path));
    }

    @Override
    public void close() {
        if (handle != 0) {
            RnsLibrary.INSTANCE.rns_destination_destroy(handle);
            handle = 0;
        }
    }
}
