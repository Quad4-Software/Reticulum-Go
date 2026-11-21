// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Client implements AutoCloseable {
    public static final int DEFAULT_TIMEOUT_MS = 45000;

    private long handle;

    private Client(long handle) {
        this.handle = handle;
    }

    public static Client dial(
        long node,
        long identity,
        byte[] hubHash,
        String nick,
        String name,
        String version,
        int timeoutMs
    ) {
        if (hubHash.length != Rrc.HASH_LEN) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        long h = RrcLibrary.INSTANCE.rrc_client_dial(
            node,
            identity,
            hubHash,
            hubHash.length,
            nick,
            name,
            version,
            timeoutMs
        );
        if (h == 0) {
            throw new RrcException(RrcException.INTERNAL);
        }
        return new Client(h);
    }

    public void join(String room) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_join(handle, room));
    }

    public void part(String room) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_part(handle, room));
    }

    public void sendMsg(String room, String text) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_send_msg(handle, room, text));
    }

    public void sendNotice(String room, String text) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_send_notice(handle, room, text));
    }

    public void sendAction(String room, String text) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_send_action(handle, room, text));
    }

    public void ping() {
        Rrc.check(RrcLibrary.INSTANCE.rrc_client_ping(handle));
    }

    public Event eventPoll(int timeoutMs) {
        RrcLibrary.RrcEvent raw = new RrcLibrary.RrcEvent();
        int code = RrcLibrary.INSTANCE.rrc_client_event_poll(handle, timeoutMs, raw);
        if (code == RrcLibrary.RRC_ERR_TIMEOUT) {
            throw new RrcException(RrcException.TIMEOUT);
        }
        Rrc.check(code);
        return new Event(raw);
    }

    @Override
    public void close() {
        if (handle != 0) {
            RrcLibrary.INSTANCE.rrc_client_close(handle);
            handle = 0;
        }
    }
}
