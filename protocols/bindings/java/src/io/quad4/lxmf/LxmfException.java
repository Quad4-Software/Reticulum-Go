// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf;

import io.quad4.lxmf.ffi.LxmfLibrary;

public final class LxmfException extends RuntimeException {
    public static final int INVALID_ARG = LxmfLibrary.LXMF_ERR_INVALID_ARG;
    public static final int INVALID_HANDLE = LxmfLibrary.LXMF_ERR_INVALID_HANDLE;
    public static final int INTERNAL = LxmfLibrary.LXMF_ERR_INTERNAL;
    public static final int TRUNCATED = LxmfLibrary.LXMF_ERR_TRUNCATED;

    private final int code;

    public LxmfException(int code) {
        super(messageFor(code));
        this.code = code;
    }

    public int getCode() {
        return code;
    }

    private static String messageFor(int code) {
        String detail = Lxmf.lastError();
        if (!detail.isEmpty()) {
            return detail;
        }
        return "lxmf error " + code;
    }
}
