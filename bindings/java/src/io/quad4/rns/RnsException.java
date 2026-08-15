// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import io.quad4.rns.ffi.RnsLibrary;

/** librns error code. */
public final class RnsException extends RuntimeException {
    public static final int OK = RnsLibrary.RNS_OK;
    public static final int INVALID_ARG = RnsLibrary.RNS_ERR_INVALID_ARG;
    public static final int INVALID_HANDLE = RnsLibrary.RNS_ERR_INVALID_HANDLE;
    public static final int NOT_FOUND = RnsLibrary.RNS_ERR_NOT_FOUND;
    public static final int STATE = RnsLibrary.RNS_ERR_STATE;
    public static final int IO = RnsLibrary.RNS_ERR_IO;
    public static final int INTERNAL = RnsLibrary.RNS_ERR_INTERNAL;
    public static final int TIMEOUT = RnsLibrary.RNS_ERR_TIMEOUT;
    public static final int TRUNCATED = RnsLibrary.RNS_ERR_TRUNCATED;

    private final int code;

    public RnsException(int code) {
        super(name(code));
        this.code = code;
    }

    public int getCode() {
        return code;
    }

    public static String name(int code) {
        switch (code) {
            case OK:
                return "ok";
            case INVALID_ARG:
                return "invalid argument";
            case INVALID_HANDLE:
                return "invalid handle";
            case NOT_FOUND:
                return "not found";
            case STATE:
                return "invalid state";
            case IO:
                return "io error";
            case INTERNAL:
                return "internal error";
            case TIMEOUT:
                return "timeout";
            case TRUNCATED:
                return "truncated";
            default:
                return "internal error";
        }
    }
}
