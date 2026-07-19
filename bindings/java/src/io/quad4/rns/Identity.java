// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;

/** Opaque identity handle. */
public final class Identity implements AutoCloseable {
    private long handle;

    Identity(long handle) {
        this.handle = handle;
    }

    public static Identity generate() {
        long h = RnsLibrary.INSTANCE.rns_identity_generate();
        if (h == 0) {
            throw new RnsException(RnsException.INTERNAL);
        }
        return new Identity(h);
    }

    public static Identity load(String path) {
        if (path == null || path.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        long h = RnsLibrary.INSTANCE.rns_identity_load(path);
        if (h == 0) {
            throw new RnsException(RnsException.IO);
        }
        return new Identity(h);
    }

    public static Identity fromPublicKey(byte[] pub) {
        if (pub == null || pub.length == 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        long h = RnsLibrary.INSTANCE.rns_identity_from_public_key(pub, pub.length);
        if (h == 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        return new Identity(h);
    }

    public long handle() {
        return handle;
    }

    public void save(String path) {
        if (path == null || path.isEmpty()) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        Rns.check(RnsLibrary.INSTANCE.rns_identity_save(handle, path));
    }

    public String hashHex() {
        byte[] buf = new byte[64];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_identity_hash(handle, buf, buf.length, written));
        return new String(buf, 0, (int) written.getValue());
    }

    public byte[] hashBytes() {
        byte[] out = new byte[Rns.HASH_LEN];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_identity_hash_bytes(handle, out, out.length, written));
        if (written.getValue() != Rns.HASH_LEN) {
            throw new RnsException(RnsException.TRUNCATED);
        }
        return out;
    }

    public byte[] publicKey() {
        byte[] out = new byte[64];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_identity_public_key(handle, out, out.length, written));
        byte[] result = new byte[(int) written.getValue()];
        System.arraycopy(out, 0, result, 0, result.length);
        return result;
    }

    public byte[] sign(byte[] data) {
        byte[] payload = data == null ? new byte[0] : data;
        byte[] sig = new byte[64];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_identity_sign(handle, payload, payload.length, sig, sig.length, written));
        byte[] result = new byte[(int) written.getValue()];
        System.arraycopy(sig, 0, result, 0, result.length);
        return result;
    }

    public void verify(byte[] data, byte[] signature) {
        byte[] payload = data == null ? new byte[0] : data;
        byte[] sig = signature == null ? new byte[0] : signature;
        Rns.check(RnsLibrary.INSTANCE.rns_identity_verify(handle, payload, payload.length, sig, sig.length));
    }

    @Override
    public void close() {
        if (handle != 0) {
            RnsLibrary.INSTANCE.rns_identity_destroy(handle);
            handle = 0;
        }
    }
}
