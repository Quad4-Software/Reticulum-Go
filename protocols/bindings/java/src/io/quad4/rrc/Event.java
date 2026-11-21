// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Event {
    public final int kind;
    public final byte[] sender;
    public final byte[] peer;
    public final String room;
    public final String nick;
    public final String body;
    public final long msgType;
    public final boolean roomTruncated;
    public final boolean nickTruncated;
    public final boolean bodyTruncated;

    Event(RrcLibrary.RrcEvent raw) {
        this.kind = raw.kind;
        this.sender = slice(raw.sender, (int) raw.sender_len);
        this.peer = slice(raw.peer, (int) raw.peer_len);
        this.room = Rrc.cString(raw.room, raw.room.length);
        this.nick = Rrc.cString(raw.nick, raw.nick.length);
        this.body = Rrc.cString(raw.body, raw.body.length);
        this.msgType = raw.msg_type;
        this.roomTruncated = raw.room_truncated != 0;
        this.nickTruncated = raw.nick_truncated != 0;
        this.bodyTruncated = raw.body_truncated != 0;
    }

    private static byte[] slice(byte[] in, int len) {
        byte[] out = new byte[len];
        System.arraycopy(in, 0, out, 0, len);
        return out;
    }
}
