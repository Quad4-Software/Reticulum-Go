// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;

/** RSG / RSM helpers. */
public final class Rsg {
    private Rsg() {}

    public static byte[] create(Identity identity, byte[] message, boolean embed) {
        byte[] payload = message == null ? new byte[0] : message;
        LongByReference needed = new LongByReference();
        int probe =
                RnsLibrary.INSTANCE.rns_rsg_create(
                        identity.handle(), payload, payload.length, embed ? 1 : 0, null, 0, needed);
        if (probe != RnsLibrary.RNS_OK && probe != RnsLibrary.RNS_ERR_TRUNCATED) {
            Rns.check(probe);
        }
        if (needed.getValue() == 0) {
            throw new RnsException(RnsException.INTERNAL);
        }
        byte[] out = new byte[(int) needed.getValue()];
        LongByReference written = new LongByReference();
        Rns.check(
                RnsLibrary.INSTANCE.rns_rsg_create(
                        identity.handle(),
                        payload,
                        payload.length,
                        embed ? 1 : 0,
                        out,
                        out.length,
                        written));
        byte[] result = new byte[(int) written.getValue()];
        System.arraycopy(out, 0, result, 0, result.length);
        return result;
    }

    public static void validate(byte[] rsg, byte[] message, byte[] requiredSignerHash) {
        byte[] r = rsg == null ? new byte[0] : rsg;
        byte[] m = message == null ? new byte[0] : message;
        byte[] s = requiredSignerHash == null ? new byte[0] : requiredSignerHash;
        Rns.check(RnsLibrary.INSTANCE.rns_rsg_validate(r, r.length, m, m.length, s, s.length));
    }

    public static byte[] signFile(Identity identity, String path) {
        if (path == null || path.isEmpty() || path.indexOf(0) >= 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        LongByReference needed = new LongByReference();
        int probe = RnsLibrary.INSTANCE.rns_rsg_sign_file(identity.handle(), path, null, 0, needed);
        if (probe != RnsLibrary.RNS_OK && probe != RnsLibrary.RNS_ERR_TRUNCATED) {
            Rns.check(probe);
        }
        byte[] out = new byte[(int) needed.getValue()];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_rsg_sign_file(identity.handle(), path, out, out.length, written));
        byte[] result = new byte[(int) written.getValue()];
        System.arraycopy(out, 0, result, 0, result.length);
        return result;
    }

    public static void verifyFile(byte[] rsg, String path, byte[] requiredSignerHash) {
        if (rsg == null || rsg.length == 0 || path == null || path.isEmpty() || path.indexOf(0) >= 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        byte[] s = requiredSignerHash == null ? new byte[0] : requiredSignerHash;
        Rns.check(RnsLibrary.INSTANCE.rns_rsg_verify_file(rsg, rsg.length, path, s, s.length));
    }

    public static byte[] rsmVerify(byte[] rsm, byte[] requiredSignerHash) {
        byte[] r = rsm == null ? new byte[0] : rsm;
        byte[] s = requiredSignerHash == null ? new byte[0] : requiredSignerHash;
        LongByReference needed = new LongByReference();
        int probe = RnsLibrary.INSTANCE.rns_rsm_verify(r, r.length, s, s.length, null, 0, needed);
        if (probe != RnsLibrary.RNS_OK && probe != RnsLibrary.RNS_ERR_TRUNCATED) {
            Rns.check(probe);
        }
        if (needed.getValue() == 0) {
            return new byte[0];
        }
        byte[] out = new byte[(int) needed.getValue()];
        LongByReference written = new LongByReference();
        Rns.check(RnsLibrary.INSTANCE.rns_rsm_verify(r, r.length, s, s.length, out, out.length, written));
        byte[] result = new byte[(int) written.getValue()];
        System.arraycopy(out, 0, result, 0, result.length);
        return result;
    }
}
