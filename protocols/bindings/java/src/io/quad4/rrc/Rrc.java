// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import io.quad4.rrc.ffi.RrcLibrary;

public final class Rrc {
    public static final String API_VERSION = "1.0";
    public static final int HASH_LEN = RrcLibrary.HASH_LEN;

    private Rrc() {}

    public static String version() {
        String raw = RrcLibrary.INSTANCE.rrc_version();
        return raw == null ? "" : raw;
    }

    public static void check(int code) {
        if (code != RrcLibrary.RRC_OK) {
            throw new RrcException(code);
        }
    }

    public static String cString(byte[] buf, int len) {
        return new String(buf, 0, len).replace("\0", "");
    }
}
