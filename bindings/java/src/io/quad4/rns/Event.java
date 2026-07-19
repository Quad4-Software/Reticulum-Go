// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.Memory;
import io.quad4.rns.ffi.RnsLibrary;

/** Polled librns event. */
public final class Event {
    public static final int ANNOUNCE = RnsLibrary.RNS_EV_ANNOUNCE;
    public static final int LINK_ESTABLISHED = RnsLibrary.RNS_EV_LINK_ESTABLISHED;
    public static final int LINK_FAILED = RnsLibrary.RNS_EV_LINK_FAILED;
    public static final int LINK_DATA = RnsLibrary.RNS_EV_LINK_DATA;
    public static final int LINK_CLOSED = RnsLibrary.RNS_EV_LINK_CLOSED;
    public static final int REQUEST_INCOMING = RnsLibrary.RNS_EV_REQUEST_INCOMING;
    public static final int REQUEST_RESPONSE = RnsLibrary.RNS_EV_REQUEST_RESPONSE;
    public static final int REQUEST_FAILED = RnsLibrary.RNS_EV_REQUEST_FAILED;
    public static final int RESOURCE_STARTED = RnsLibrary.RNS_EV_RESOURCE_STARTED;
    public static final int RESOURCE_CONCLUDED = RnsLibrary.RNS_EV_RESOURCE_CONCLUDED;
    public static final int DESTINATION_DATA = RnsLibrary.RNS_EV_DESTINATION_DATA;

    private final int kind;
    private final int hops;
    private final byte[] linkId;
    private final byte[] destinationHash;
    private final byte[] identityHash;
    private final byte[] requestId;
    private final String path;
    private final String errorMessage;
    private final byte[] appData;
    private final boolean appDataTruncated;

    private Event(
            int kind,
            int hops,
            byte[] linkId,
            byte[] destinationHash,
            byte[] identityHash,
            byte[] requestId,
            String path,
            String errorMessage,
            byte[] appData,
            boolean appDataTruncated) {
        this.kind = kind;
        this.hops = hops;
        this.linkId = linkId;
        this.destinationHash = destinationHash;
        this.identityHash = identityHash;
        this.requestId = requestId;
        this.path = path;
        this.errorMessage = errorMessage;
        this.appData = appData;
        this.appDataTruncated = appDataTruncated;
    }

    public static Event poll(Node node, int timeoutMs, int appDataCapacity) {
        RnsLibrary.RnsEvent raw = new RnsLibrary.RnsEvent();
        Memory storage = null;
        if (appDataCapacity > 0) {
            storage = new Memory(appDataCapacity);
            raw.app_data = storage;
            raw.app_data_cap = appDataCapacity;
        }
        int code = RnsLibrary.INSTANCE.rns_event_poll(node.handle(), raw, timeoutMs);
        Rns.check(code);

        byte[] data = new byte[0];
        if (raw.app_data_len > 0 && storage != null) {
            data = storage.getByteArray(0, (int) raw.app_data_len);
        }

        return new Event(
                raw.kind,
                raw.hops & 0xff,
                RnsLibrary.copyHash(raw.link_id, raw.link_id_len),
                RnsLibrary.copyHash(raw.destination_hash, raw.destination_hash_len),
                RnsLibrary.copyHash(raw.identity_hash, raw.identity_hash_len),
                RnsLibrary.copyHash(raw.request_id, raw.request_id_len),
                RnsLibrary.cstr(raw.path),
                RnsLibrary.cstr(raw.error_message),
                data,
                raw.app_data_truncated != 0);
    }

    public int kind() {
        return kind;
    }

    public int hops() {
        return hops;
    }

    public byte[] linkId() {
        return linkId.clone();
    }

    public byte[] destinationHash() {
        return destinationHash.clone();
    }

    public byte[] identityHash() {
        return identityHash.clone();
    }

    public byte[] requestId() {
        return requestId.clone();
    }

    public String path() {
        return path;
    }

    public String errorMessage() {
        return errorMessage;
    }

    public byte[] appData() {
        return appData.clone();
    }

    public boolean appDataTruncated() {
        return appDataTruncated;
    }
}
