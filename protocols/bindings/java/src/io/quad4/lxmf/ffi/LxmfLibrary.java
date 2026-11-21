// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf.ffi;

import com.sun.jna.Library;
import com.sun.jna.Native;
import com.sun.jna.ptr.LongByReference;

import java.io.File;

public interface LxmfLibrary extends Library {
    int HASH_LEN = 16;
    int LXMF_OK = 0;
    int LXMF_ERR_INVALID_ARG = 1;
    int LXMF_ERR_INVALID_HANDLE = 2;
    int LXMF_ERR_INTERNAL = 6;
    int LXMF_ERR_TRUNCATED = 8;

    LxmfLibrary INSTANCE = load();

    static LxmfLibrary load() {
        String env = System.getenv("LXMF_LIB_PATH");
        if (env != null && !env.isEmpty()) {
            return Native.load(new File(env).getAbsolutePath(), LxmfLibrary.class);
        }
        String root = System.getenv("LXMF_ROOT");
        if (root == null || root.isEmpty()) {
            root = System.getenv("RRC_ROOT");
        }
        if (root != null) {
            for (String name : libNames()) {
                File lib = new File(root, "bin/" + name);
                if (lib.isFile()) {
                    return Native.load(lib.getAbsolutePath(), LxmfLibrary.class);
                }
            }
        }
        return Native.load("lxmf", LxmfLibrary.class);
    }

    static String[] libNames() {
        String os = System.getProperty("os.name", "").toLowerCase();
        if (os.contains("mac")) {
            return new String[] {"liblxmf.dylib"};
        }
        if (os.contains("win")) {
            return new String[] {"lxmf.dll", "liblxmf.dll"};
        }
        return new String[] {"liblxmf.so"};
    }

    String lxmf_version();

    int lxmf_last_error(byte[] buf, int bufLen, LongByReference written);

    long lxmf_identity_generate();
    long lxmf_identity_load(String path);
    int lxmf_identity_save(long identity, String path);
    int lxmf_identity_destroy(long identity);
    int lxmf_identity_hash(long identity, byte[] out, int outLen, LongByReference written);
    int lxmf_identity_public_key(long identity, byte[] out, int outLen, LongByReference written);
    int lxmf_identity_delivery_hash(long identity, byte[] out, int outLen, LongByReference written);
    int lxmf_identity_register_recall(long identity);
    int lxmf_identity_register_recall_source(
        byte[] source, int sourceLen, byte[] publicKey, int publicKeyLen
    );

    long lxmf_message_create(
        byte[] dest, int destLen, byte[] source, int sourceLen, String title, String content
    );
    int lxmf_message_pack(long message, long identity, byte[] out, int outLen, LongByReference written);
    int lxmf_message_encrypted_payload(long message, byte[] out, int outLen, LongByReference written);
    int lxmf_message_pack_propagated(
        long message,
        long identity,
        byte[] recipientHash,
        int recipientHashLen,
        byte[] recipientPublicKey,
        int recipientPublicKeyLen,
        int pnStampCost,
        byte[] out,
        int outLen,
        LongByReference written
    );
    long lxmf_message_unpack(byte[] data, int dataLen);
    int lxmf_message_get_dest(long message, byte[] out, int outLen, LongByReference written);
    int lxmf_message_get_source(long message, byte[] out, int outLen, LongByReference written);
    int lxmf_message_get_title(long message, byte[] buf, int bufLen, LongByReference written);
    int lxmf_message_get_content(long message, byte[] buf, int bufLen, LongByReference written);
    int lxmf_message_set_fields_json(long message, String json);
    int lxmf_message_fields_json(long message, byte[] buf, int bufLen, LongByReference written);
    int lxmf_message_field_count(long message, LongByReference count);
    long lxmf_message_unpack_verified(byte[] data, int dataLen, long identity);
    int lxmf_message_destroy(long message);

    int lxmf_stamp_cost_from_app_data(byte[] appData, int appDataLen, LongByReference costOut);
    int lxmf_pn_stamp_cost_from_app_data(byte[] appData, int appDataLen, LongByReference costOut);
    int lxmf_message_apply_stamp(long message, long identity, int stampCost, int timeoutMs);
    int lxmf_encode_announce_app_data(
        String displayName,
        long stampCost,
        String iconName,
        byte[] fgRgb3,
        byte[] bgRgb3,
        byte[] out,
        int outLen,
        LongByReference written
    );
}
