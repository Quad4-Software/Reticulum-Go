// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf;

import com.sun.jna.ptr.LongByReference;
import io.quad4.lxmf.ffi.LxmfLibrary;

public final class Lxmf {
    public static final String API_VERSION = "1.0";
    public static final int HASH_LEN = LxmfLibrary.HASH_LEN;

    private Lxmf() {}

    public static String version() {
        String raw = LxmfLibrary.INSTANCE.lxmf_version();
        return raw == null ? "" : raw;
    }

    public static String lastError() {
        byte[] buf = new byte[512];
        LongByReference written = new LongByReference();
        int rc = LxmfLibrary.INSTANCE.lxmf_last_error(buf, buf.length, written);
        String msg = cString(buf, (int) written.getValue());
        if (!msg.isEmpty()) {
            return msg;
        }
        if (rc == LxmfLibrary.LXMF_ERR_TRUNCATED) {
            return "truncated";
        }
        return "";
    }

    public static void check(int code) {
        if (code != LxmfLibrary.LXMF_OK) {
            throw new LxmfException(code);
        }
    }

    public static String cString(byte[] buf, int len) {
        if (len <= 0) {
            return "";
        }
        return new String(buf, 0, len).replace("\0", "");
    }

    static byte[] readBytes(ByteReader reader, int initialCapacity) {
        int capacity = initialCapacity;
        while (capacity <= 16 * 1024 * 1024) {
            byte[] buf = new byte[capacity];
            LongByReference written = new LongByReference();
            int code = reader.read(buf, written);
            if (code == LxmfLibrary.LXMF_ERR_TRUNCATED) {
                capacity *= 2;
                continue;
            }
            check(code);
            byte[] out = new byte[(int) written.getValue()];
            System.arraycopy(buf, 0, out, 0, out.length);
            return out;
        }
        throw new LxmfException(LxmfLibrary.LXMF_ERR_TRUNCATED);
    }

    static String readUtf8(ByteReader reader, int initialCapacity) {
        byte[] data = readBytes(reader, initialCapacity);
        return cString(data, data.length);
    }

    public static long stampCostFromAppData(byte[] appData) {
        LongByReference cost = new LongByReference();
        byte[] payload = appData == null ? new byte[0] : appData;
        check(LxmfLibrary.INSTANCE.lxmf_stamp_cost_from_app_data(payload, payload.length, cost));
        return cost.getValue();
    }

    public static long pnStampCostFromAppData(byte[] appData) {
        LongByReference cost = new LongByReference();
        byte[] payload = appData == null ? new byte[0] : appData;
        check(LxmfLibrary.INSTANCE.lxmf_pn_stamp_cost_from_app_data(payload, payload.length, cost));
        return cost.getValue();
    }

    public static byte[] encodeAnnounceAppData(
        String displayName, long stampCost, String iconName, byte[] fgRgb3, byte[] bgRgb3
    ) {
        return readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_encode_announce_app_data(
                displayName,
                stampCost,
                iconName,
                fgRgb3,
                bgRgb3,
                buf,
                buf.length,
                written
            ),
            512
        );
    }

    @FunctionalInterface
    interface ByteReader {
        int read(byte[] buf, LongByReference written);
    }
}
