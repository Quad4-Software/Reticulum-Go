// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;

/** Shared constants, version helpers, and error mapping for librns. */
public final class Rns {
    public static final String API_VERSION = "1.5";
    public static final int HASH_LEN = RnsLibrary.HASH_LEN;

    private Rns() {}

    public static String version() {
        String raw = RnsLibrary.INSTANCE.rns_version();
        return raw == null ? "" : raw;
    }

    public static String lastError() {
        byte[] buf = new byte[512];
        LongByReference written = new LongByReference();
        int code = RnsLibrary.INSTANCE.rns_last_error(buf, buf.length, written);
        if (code != RnsLibrary.RNS_OK || written.getValue() == 0) {
            return "";
        }
        return new String(buf, 0, (int) written.getValue());
    }

    public static void check(int code) {
        if (code != RnsLibrary.RNS_OK) {
            throw new RnsException(code);
        }
    }

    public static String hashToHex(byte[] data) {
        if (data == null || data.length != HASH_LEN) {
            throw new RnsException(RnsLibrary.RNS_ERR_INVALID_ARG);
        }
        StringBuilder sb = new StringBuilder(HASH_LEN * 2);
        for (byte b : data) {
            sb.append(String.format("%02x", b & 0xff));
        }
        return sb.toString();
    }

    public static byte[] hexToHash(String text) {
        if (text == null || text.length() != 32) {
            throw new RnsException(RnsLibrary.RNS_ERR_INVALID_ARG);
        }
        byte[] out = new byte[HASH_LEN];
        for (int i = 0; i < HASH_LEN; i++) {
            int hi = Character.digit(text.charAt(i * 2), 16);
            int lo = Character.digit(text.charAt(i * 2 + 1), 16);
            if (hi < 0 || lo < 0) {
                throw new RnsException(RnsLibrary.RNS_ERR_INVALID_ARG);
            }
            out[i] = (byte) ((hi << 4) | lo);
        }
        return out;
    }

    public static boolean hashEq(byte[] a, byte[] b) {
        if (a == null || b == null || a.length != HASH_LEN || b.length != HASH_LEN) {
            return false;
        }
        for (int i = 0; i < HASH_LEN; i++) {
            if (a[i] != b[i]) {
                return false;
            }
        }
        return true;
    }
}
