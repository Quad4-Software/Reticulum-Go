// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Identity implements AutoCloseable {
    private long handle;

    private Identity(long handle) {
        this.handle = handle;
    }

    public static Identity generate() {
        long h = RrcLibrary.INSTANCE.rrc_identity_generate();
        if (h == 0) {
            throw new RrcException(RrcException.INTERNAL);
        }
        return new Identity(h);
    }

    public static Identity load(String path) {
        long h = RrcLibrary.INSTANCE.rrc_identity_load(path);
        if (h == 0) {
            throw new RrcException(RrcException.IO);
        }
        return new Identity(h);
    }

    public void save(String path) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_identity_save(handle, path));
    }

    public byte[] hashBytes() {
        byte[] buf = new byte[Rrc.HASH_LEN];
        LongByReference written = new LongByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_identity_hash(handle, buf, buf.length, written));
        byte[] out = new byte[(int) written.getValue()];
        System.arraycopy(buf, 0, out, 0, out.length);
        return out;
    }

    public void seedDestination(byte[] destHash) {
        if (destHash.length != Rrc.HASH_LEN) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        Rrc.check(RrcLibrary.INSTANCE.rrc_identity_seed_destination(handle, destHash, destHash.length));
    }

    public long handle() {
        return handle;
    }

    @Override
    public void close() {
        if (handle != 0) {
            RrcLibrary.INSTANCE.rrc_identity_destroy(handle);
            handle = 0;
        }
    }
}
