// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Util {
    private Util() {}

    public static String normalizeRoom(String name) {
        return Buffers.readString(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_normalize_room(name, buf, buflen, written),
            128
        );
    }

    public static String sanitizeNick(String nick) {
        return Buffers.readString(
            (buf, buflen, written) -> RrcLibrary.INSTANCE.rrc_sanitize_nick(nick, buf, buflen, written),
            64
        );
    }
}
