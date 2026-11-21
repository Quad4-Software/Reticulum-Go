// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf;

import io.quad4.lxmf.ffi.LxmfLibrary;

public final class Identity implements AutoCloseable {
    private long handle;

    private Identity(long handle) {
        this.handle = handle;
    }

    public static Identity generate() {
        long h = LxmfLibrary.INSTANCE.lxmf_identity_generate();
        if (h == 0) {
            throw new LxmfException(LxmfException.INTERNAL);
        }
        return new Identity(h);
    }

    public static Identity load(String path) {
        if (path == null || path.isEmpty()) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        long h = LxmfLibrary.INSTANCE.lxmf_identity_load(path);
        if (h == 0) {
            throw new LxmfException(LxmfException.INTERNAL);
        }
        return new Identity(h);
    }

    public void save(String path) {
        if (path == null || path.isEmpty()) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_identity_save(handle, path));
    }

    public byte[] hashBytes() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_identity_hash(handle, buf, buf.length, written),
            Lxmf.HASH_LEN
        );
    }

    public byte[] publicKey() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_identity_public_key(handle, buf, buf.length, written),
            128
        );
    }

    public byte[] deliveryHash() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_identity_delivery_hash(handle, buf, buf.length, written),
            Lxmf.HASH_LEN
        );
    }

    public void registerRecall() {
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_identity_register_recall(handle));
    }

    public static void registerRecallSource(byte[] sourceHash, byte[] publicKey) {
        if (sourceHash == null || sourceHash.length != Lxmf.HASH_LEN) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        if (publicKey == null || publicKey.length == 0) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_identity_register_recall_source(
            sourceHash, sourceHash.length, publicKey, publicKey.length
        ));
    }

    public long handle() {
        return handle;
    }

    @Override
    public void close() {
        if (handle != 0) {
            LxmfLibrary.INSTANCE.lxmf_identity_destroy(handle);
            handle = 0;
        }
    }
}
