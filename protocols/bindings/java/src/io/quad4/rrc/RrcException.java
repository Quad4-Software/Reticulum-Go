// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

public final class RrcException extends RuntimeException {
    public static final int INVALID_ARG = 1;
    public static final int INVALID_HANDLE = 2;
    public static final int NOT_FOUND = 3;
    public static final int STATE = 4;
    public static final int IO = 5;
    public static final int INTERNAL = 6;
    public static final int TIMEOUT = 7;
    public static final int TRUNCATED = 8;

    private final int code;

    public RrcException(int code) {
        super(messageFor(code));
        this.code = code;
    }

    public int getCode() {
        return code;
    }

    private static String messageFor(int code) {
        String detail = lastError();
        if (!detail.isEmpty()) {
            return detail;
        }
        return "rrc error " + code;
    }

    private static String lastError() {
        byte[] buf = new byte[512];
        com.sun.jna.ptr.LongByReference written = new com.sun.jna.ptr.LongByReference();
        int rc = io.quad4.rrc.ffi.RrcLibrary.INSTANCE.rrc_last_error(buf, buf.length, written);
        String msg = Rrc.cString(buf, (int) written.getValue());
        if (!msg.isEmpty()) {
            return msg;
        }
        if (rc == TRUNCATED) {
            return "truncated";
        }
        return "";
    }
}
