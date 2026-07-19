// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;

/** Opaque link handle. */
public final class Link implements AutoCloseable {
    private long handle;

    Link(long handle) {
        this.handle = handle;
    }

    public static Link open(Node node, byte[] destHash) {
        if (destHash == null || destHash.length != Rns.HASH_LEN) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        long h = RnsLibrary.INSTANCE.rns_link_open(node.handle(), destHash);
        if (h == 0) {
            throw new RnsException(RnsException.INTERNAL);
        }
        return new Link(h);
    }

    public void send(byte[] data) {
        if (data == null || data.length == 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        Rns.check(RnsLibrary.INSTANCE.rns_link_send(handle, data, data.length));
    }

    public void sendResource(byte[] data, String name) {
        byte[] payload = data == null ? new byte[0] : data;
        Rns.check(RnsLibrary.INSTANCE.rns_link_send_resource(handle, payload, payload.length, name));
    }

    public byte[] id() {
        byte[] out = new byte[Rns.HASH_LEN];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_link_id(handle, out, out.length, written));
        if (written.getValue() != Rns.HASH_LEN) {
            throw new RnsException(RnsException.TRUNCATED);
        }
        return out;
    }

    public byte[] request(Node node, String path, byte[] data, int timeoutMs) {
        if (path == null || path.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        byte[] payload = data == null ? new byte[0] : data;
        byte[] requestId = new byte[Rns.HASH_LEN];
        LongByReference written = new LongByReference();
        Rns.check(
                RnsLibrary.INSTANCE.rns_link_request(
                        node.handle(),
                        handle,
                        path,
                        payload,
                        payload.length,
                        timeoutMs,
                        requestId,
                        requestId.length,
                        written));
        if (written.getValue() != Rns.HASH_LEN) {
            throw new RnsException(RnsException.TRUNCATED);
        }
        return requestId;
    }

    public static void requestRespond(Node node, byte[] requestId, byte[] data) {
        if (requestId == null || requestId.length == 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        byte[] payload = data == null ? new byte[0] : data;
        Rns.check(
                RnsLibrary.INSTANCE.rns_request_respond(
                        node.handle(), requestId, requestId.length, payload, payload.length));
    }

    public static void requestRespondFile(Node node, byte[] requestId, String filename, byte[] data) {
        if (requestId == null || requestId.length == 0 || filename == null || filename.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        byte[] payload = data == null ? new byte[0] : data;
        Rns.check(
                RnsLibrary.INSTANCE.rns_request_respond_file(
                        node.handle(), requestId, requestId.length, filename, payload, payload.length));
    }

    @Override
    public void close() {
        if (handle != 0) {
            RnsLibrary.INSTANCE.rns_link_close(handle);
            handle = 0;
        }
    }
}
