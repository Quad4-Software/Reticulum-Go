// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rrc.ffi.RrcLibrary;

final class Buffers {
    private static final int MAX_BUFFER = 16 * 1024 * 1024;

    private Buffers() {}

    @FunctionalInterface
    interface StringFill {
        int fill(byte[] buf, int buflen, LongByReference written);
    }

    @FunctionalInterface
    interface BytesFill {
        int fill(byte[] buf, int buflen, LongByReference written);
    }

    static String readString(StringFill fill, int initial) {
        int cap = initial;
        while (cap <= MAX_BUFFER) {
            byte[] buf = new byte[cap];
            LongByReference written = new LongByReference();
            int code = fill.fill(buf, buf.length, written);
            if (code == RrcException.TRUNCATED) {
                cap *= 2;
                continue;
            }
            Rrc.check(code);
            return Rrc.cString(buf, (int) written.getValue());
        }
        throw new RrcException(RrcException.TRUNCATED);
    }

    static byte[] writeBytes(BytesFill fill, int initial) {
        int cap = initial;
        while (cap <= MAX_BUFFER) {
            byte[] buf = new byte[cap];
            LongByReference written = new LongByReference();
            int code = fill.fill(buf, buf.length, written);
            if (code == RrcException.TRUNCATED) {
                cap *= 2;
                continue;
            }
            Rrc.check(code);
            byte[] out = new byte[(int) written.getValue()];
            System.arraycopy(buf, 0, out, 0, out.length);
            return out;
        }
        throw new RrcException(RrcException.TRUNCATED);
    }
}
