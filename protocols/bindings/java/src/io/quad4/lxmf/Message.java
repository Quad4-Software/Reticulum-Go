// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.lxmf;

import com.sun.jna.ptr.LongByReference;
import io.quad4.lxmf.ffi.LxmfLibrary;

public final class Message implements AutoCloseable {
    private long handle;

    private Message(long handle) {
        this.handle = handle;
    }

    public static Message create(byte[] dest, byte[] source, String title, String content) {
        if (dest == null || dest.length != Lxmf.HASH_LEN
            || source == null || source.length != Lxmf.HASH_LEN) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        long h = LxmfLibrary.INSTANCE.lxmf_message_create(
            dest, dest.length, source, source.length, title, content
        );
        if (h == 0) {
            throw new LxmfException(LxmfException.INTERNAL);
        }
        return new Message(h);
    }

    public static Message unpack(byte[] data) {
        if (data == null || data.length == 0) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        long h = LxmfLibrary.INSTANCE.lxmf_message_unpack(data, data.length);
        if (h == 0) {
            throw new LxmfException(LxmfException.INTERNAL);
        }
        return new Message(h);
    }

    public static Message unpackVerified(byte[] data, Identity identity) {
        if (data == null || data.length == 0) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        long idHandle = identity == null ? 0 : identity.handle();
        long h = LxmfLibrary.INSTANCE.lxmf_message_unpack_verified(data, data.length, idHandle);
        if (h == 0) {
            throw new LxmfException(LxmfException.INTERNAL);
        }
        return new Message(h);
    }

    public byte[] pack(Identity identity) {
        if (identity == null) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_pack(
                handle, identity.handle(), buf, buf.length, written
            ),
            65536
        );
    }

    public byte[] packPropagated(
        Identity identity, byte[] recipientHash, byte[] recipientPublicKey, int pnStampCost
    ) {
        if (identity == null) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        if (recipientHash == null || recipientHash.length != Lxmf.HASH_LEN) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        if (recipientPublicKey == null || recipientPublicKey.length == 0) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_pack_propagated(
                handle,
                identity.handle(),
                recipientHash,
                recipientHash.length,
                recipientPublicKey,
                recipientPublicKey.length,
                pnStampCost,
                buf,
                buf.length,
                written
            ),
            65536
        );
    }

    public byte[] encryptedPayload() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_encrypted_payload(
                handle, buf, buf.length, written
            ),
            65536
        );
    }

    public byte[] dest() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_get_dest(handle, buf, buf.length, written),
            Lxmf.HASH_LEN
        );
    }

    public byte[] source() {
        return Lxmf.readBytes(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_get_source(handle, buf, buf.length, written),
            Lxmf.HASH_LEN
        );
    }

    public String title() {
        return Lxmf.readUtf8(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_get_title(handle, buf, buf.length, written),
            4096
        );
    }

    public String content() {
        return Lxmf.readUtf8(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_get_content(handle, buf, buf.length, written),
            4096
        );
    }

    public void setFieldsJson(String json) {
        if (json == null) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_message_set_fields_json(handle, json));
    }

    public String fieldsJson() {
        return Lxmf.readUtf8(
            (buf, written) -> LxmfLibrary.INSTANCE.lxmf_message_fields_json(handle, buf, buf.length, written),
            262144
        );
    }

    public long fieldCount() {
        LongByReference count = new LongByReference();
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_message_field_count(handle, count));
        return count.getValue();
    }

    public void applyStamp(Identity identity, int stampCost, int timeoutMs) {
        if (identity == null) {
            throw new LxmfException(LxmfException.INVALID_ARG);
        }
        Lxmf.check(LxmfLibrary.INSTANCE.lxmf_message_apply_stamp(
            handle, identity.handle(), stampCost, timeoutMs
        ));
    }

    public long handle() {
        return handle;
    }

    @Override
    public void close() {
        if (handle != 0) {
            LxmfLibrary.INSTANCE.lxmf_message_destroy(handle);
            handle = 0;
        }
    }
}
