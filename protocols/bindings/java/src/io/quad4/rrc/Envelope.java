// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Envelope implements AutoCloseable {
    private long handle;

    private Envelope(long handle) {
        this.handle = handle;
    }

    public static Envelope create(long msgType, byte[] sender) {
        if (sender == null || sender.length != Rrc.HASH_LEN) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        long h = RrcLibrary.INSTANCE.rrc_envelope_create(msgType, sender, sender.length);
        if (h == 0) {
            throw new RrcException(RrcException.INTERNAL);
        }
        return new Envelope(h);
    }

    public static Envelope unmarshal(byte[] data) {
        long h = RrcLibrary.INSTANCE.rrc_envelope_unmarshal(data, data.length);
        if (h == 0) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        return new Envelope(h);
    }

    public void setRoom(String room) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_set_room(handle, room));
    }

    public void setNick(String nick) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_set_nick(handle, nick));
    }

    public void setBodyText(String text) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_set_body_text(handle, text));
    }

    public void setDestination(byte[] dest) {
        if (dest == null || dest.length != Rrc.HASH_LEN) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_set_destination(handle, dest, dest.length));
    }

    public long msgType() {
        LongByReference out = new LongByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_get_type(handle, out));
        return out.getValue();
    }

    public byte[] sender() {
        byte[] buf = new byte[Rrc.HASH_LEN];
        LongByReference written = new LongByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_envelope_get_sender(handle, buf, buf.length, written));
        byte[] out = new byte[(int) written.getValue()];
        System.arraycopy(buf, 0, out, 0, out.length);
        return out;
    }

    public String room() {
        return Buffers.readString(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_envelope_get_room(handle, buf, buflen, written),
            128
        );
    }

    public String nick() {
        return Buffers.readString(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_envelope_get_nick(handle, buf, buflen, written),
            64
        );
    }

    public String bodyText() {
        return Buffers.readString(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_envelope_get_body_text(handle, buf, buflen, written),
            1024
        );
    }

    public byte[] marshal() {
        return Buffers.writeBytes(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_envelope_marshal(handle, buf, buflen, written),
            65536
        );
    }

    @Override
    public void close() {
        if (handle != 0) {
            RrcLibrary.INSTANCE.rrc_envelope_destroy(handle);
            handle = 0;
        }
    }
}
