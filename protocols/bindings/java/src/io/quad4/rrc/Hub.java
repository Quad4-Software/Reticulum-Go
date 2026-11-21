// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Hub implements AutoCloseable {
    private long handle;

    private Hub(long handle) {
        this.handle = handle;
    }

    public static Hub create(long node, long identity, String name, String version) {
        long h = RrcLibrary.INSTANCE.rrc_hub_create(node, identity, name, version);
        if (h == 0) {
            throw new RrcException(RrcException.INTERNAL);
        }
        return new Hub(h);
    }

    public void start() {
        Rrc.check(RrcLibrary.INSTANCE.rrc_hub_start(handle));
    }

    public void announce() {
        Rrc.check(RrcLibrary.INSTANCE.rrc_hub_announce(handle));
    }

    public byte[] hashBytes() {
        byte[] buf = new byte[Rrc.HASH_LEN];
        LongByReference written = new LongByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_hub_hash(handle, buf, buf.length, written));
        byte[] out = new byte[(int) written.getValue()];
        System.arraycopy(buf, 0, out, 0, out.length);
        return out;
    }

    public long peerCount() {
        LongByReference count = new LongByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_hub_peer_count(handle, count));
        return count.getValue();
    }

    public Event eventPoll(int timeoutMs) {
        RrcLibrary.RrcEvent raw = new RrcLibrary.RrcEvent();
        int code = RrcLibrary.INSTANCE.rrc_hub_event_poll(handle, timeoutMs, raw);
        if (code == RrcLibrary.RRC_ERR_TIMEOUT) {
            throw new RrcException(RrcException.TIMEOUT);
        }
        Rrc.check(code);
        return new Event(raw);
    }

    public long handle() {
        return handle;
    }

    @Override
    public void close() {
        if (handle != 0) {
            RrcLibrary.INSTANCE.rrc_hub_destroy(handle);
            handle = 0;
        }
    }
}
