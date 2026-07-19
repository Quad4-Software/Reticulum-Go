// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.ffi;

import com.sun.jna.Library;
import com.sun.jna.Native;
import com.sun.jna.Pointer;
import com.sun.jna.Structure;
import com.sun.jna.ptr.LongByReference;
import java.io.File;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Raw JNA mapping of include/rns.h (ABI 1.5). */
public interface RnsLibrary extends Library {
    int HASH_LEN = 16;

    int RNS_OK = 0;
    int RNS_ERR_INVALID_ARG = 1;
    int RNS_ERR_INVALID_HANDLE = 2;
    int RNS_ERR_NOT_FOUND = 3;
    int RNS_ERR_STATE = 4;
    int RNS_ERR_IO = 5;
    int RNS_ERR_INTERNAL = 6;
    int RNS_ERR_TIMEOUT = 7;
    int RNS_ERR_TRUNCATED = 8;

    int RNS_EV_ANNOUNCE = 1;
    int RNS_EV_LINK_ESTABLISHED = 2;
    int RNS_EV_LINK_FAILED = 3;
    int RNS_EV_LINK_DATA = 4;
    int RNS_EV_LINK_CLOSED = 5;
    int RNS_EV_REQUEST_INCOMING = 6;
    int RNS_EV_REQUEST_RESPONSE = 7;
    int RNS_EV_REQUEST_FAILED = 8;
    int RNS_EV_RESOURCE_STARTED = 9;
    int RNS_EV_RESOURCE_CONCLUDED = 10;
    int RNS_EV_DESTINATION_DATA = 11;

    String rns_version();

    int rns_last_error(byte[] buf, long bufLen, LongByReference written);

    long rns_node_create(String configPath);

    int rns_node_start(long node);

    int rns_node_stop(long node);

    int rns_node_destroy(long node);

    int rns_node_set_identity(long node, long identity);

    int rns_node_resume(long node);

    int rns_node_pause(long node);

    long rns_identity_generate();

    long rns_identity_load(String path);

    int rns_identity_save(long identity, String path);

    int rns_identity_destroy(long identity);

    int rns_identity_hash(long identity, byte[] hexBuf, long hexBufLen, LongByReference written);

    int rns_identity_hash_bytes(long identity, byte[] out, long outLen, LongByReference written);

    int rns_identity_public_key(long identity, byte[] out, long outLen, LongByReference written);

    long rns_identity_from_public_key(byte[] pub, long pubLen);

    int rns_identity_sign(
            long identity, byte[] data, long dataLen, byte[] sigOut, long sigOutLen, LongByReference written);

    int rns_identity_verify(long identity, byte[] data, long dataLen, byte[] sig, long sigLen);

    int rns_rsg_create(
            long identity,
            byte[] message,
            long messageLen,
            int embed,
            byte[] out,
            long outLen,
            LongByReference written);

    int rns_rsg_validate(
            byte[] rsg,
            long rsgLen,
            byte[] message,
            long messageLen,
            byte[] requiredSignerHash,
            long requiredSignerHashLen);

    int rns_rsm_verify(
            byte[] rsm,
            long rsmLen,
            byte[] requiredSignerHash,
            long requiredSignerHashLen,
            byte[] messageOut,
            long messageOutLen,
            LongByReference written);

    long rns_destination_create(
            long node, long identity, String appName, Pointer aspects, long aspectCount, int acceptsLinks);

    int rns_destination_announce(long destination, byte[] appData, long appDataLen);

    int rns_destination_hash(long destination, byte[] hashOut, long hashOutLen, LongByReference written);

    int rns_destination_destroy(long destination);

    int rns_destination_register_request_handler(long destination, String path);

    int rns_path_request(long node, byte[] destHash);

    int rns_path_table(long node, RnsPathEntry[] out, long outCap, LongByReference written, int maxHops);

    int rns_interfaces(long node, RnsInterfaceEntry[] out, long outCap, LongByReference written);

    long rns_link_open(long node, byte[] destHash);

    int rns_link_send(long link, byte[] data, long dataLen);

    int rns_link_send_resource(long link, byte[] data, long dataLen, String name);

    int rns_link_close(long link);

    int rns_link_id(long link, byte[] idOut, long idOutLen, LongByReference written);

    int rns_link_request(
            long node,
            long link,
            String path,
            byte[] data,
            long dataLen,
            int timeoutMs,
            byte[] requestIdOut,
            long requestIdOutLen,
            LongByReference written);

    int rns_request_respond(long node, byte[] requestId, long requestIdLen, byte[] data, long dataLen);

    int rns_request_respond_file(
            long node, byte[] requestId, long requestIdLen, String filename, byte[] data, long dataLen);

    int rns_event_poll(long node, RnsEvent event, int timeoutMs);

    @Structure.FieldOrder({
        "kind",
        "link_id",
        "link_id_len",
        "destination_hash",
        "destination_hash_len",
        "identity_hash",
        "identity_hash_len",
        "request_id",
        "request_id_len",
        "hops",
        "path",
        "path_truncated",
        "error_message",
        "error_message_truncated",
        "app_data",
        "app_data_len",
        "app_data_cap",
        "app_data_truncated"
    })
    class RnsEvent extends Structure {
        public int kind;
        public byte[] link_id = new byte[HASH_LEN];
        public long link_id_len;
        public byte[] destination_hash = new byte[HASH_LEN];
        public long destination_hash_len;
        public byte[] identity_hash = new byte[HASH_LEN];
        public long identity_hash_len;
        public byte[] request_id = new byte[HASH_LEN];
        public long request_id_len;
        public byte hops;
        public byte[] path = new byte[256];
        public int path_truncated;
        public byte[] error_message = new byte[256];
        public int error_message_truncated;
        public Pointer app_data;
        public long app_data_len;
        public long app_data_cap;
        public int app_data_truncated;
    }

    @Structure.FieldOrder({"hash", "hash_len", "via", "via_len", "hops", "iface", "timestamp", "expires"})
    class RnsPathEntry extends Structure {
        public byte[] hash = new byte[HASH_LEN];
        public long hash_len;
        public byte[] via = new byte[HASH_LEN];
        public long via_len;
        public byte hops;
        public byte[] iface = new byte[64];
        public double timestamp;
        public double expires;

        public static class ByReference extends RnsPathEntry implements Structure.ByReference {}
    }

    @Structure.FieldOrder({
        "name", "type_name", "online", "enabled", "rx_bytes", "tx_bytes", "rx_packets", "tx_packets"
    })
    class RnsInterfaceEntry extends Structure {
        public byte[] name = new byte[96];
        public byte[] type_name = new byte[32];
        public int online;
        public int enabled;
        public long rx_bytes;
        public long tx_bytes;
        public long rx_packets;
        public long tx_packets;

        public static class ByReference extends RnsInterfaceEntry implements Structure.ByReference {}
    }

    final class Loader {
        private Loader() {}

        static RnsLibrary load() {
            String explicit = System.getenv("RNS_LIB_PATH");
            if (explicit != null && !explicit.isEmpty()) {
                return Native.load(explicit, RnsLibrary.class);
            }
            for (Path candidate : candidates()) {
                if (Files.isRegularFile(candidate)) {
                    return Native.load(candidate.toAbsolutePath().toString(), RnsLibrary.class);
                }
            }
            return Native.load("rns", RnsLibrary.class);
        }

        private static List<Path> candidates() {
            List<Path> out = new ArrayList<>();
            String root = System.getenv("RNS_ROOT");
            if (root != null && !root.isEmpty()) {
                out.add(Paths.get(root, "bin", "librns.so"));
            }
            Path here = Paths.get("").toAbsolutePath();
            out.add(here.resolve("../../bin/librns.so").normalize());
            out.add(here.resolve("../../../bin/librns.so").normalize());
            out.add(Paths.get("bin/librns.so"));
            out.add(Paths.get("../bin/librns.so"));
            out.add(Paths.get("../../bin/librns.so"));
            String classPath = RnsLibrary.class.getProtectionDomain().getCodeSource().getLocation().getPath();
            try {
                Path classDir = Paths.get(new File(classPath).getPath()).getParent();
                if (classDir != null) {
                    out.add(classDir.resolve("../../../bin/librns.so").normalize());
                    out.add(classDir.resolve("../../../../bin/librns.so").normalize());
                }
            } catch (Exception ignored) {
                // fall through
            }
            return out;
        }
    }

    RnsLibrary INSTANCE = Loader.load();

    static byte[] copyHash(byte[] src, long len) {
        int n = (int) Math.min(len, HASH_LEN);
        return Arrays.copyOf(src, n);
    }

    static String cstr(byte[] buf) {
        int n = 0;
        while (n < buf.length && buf[n] != 0) {
            n++;
        }
        return new String(buf, 0, n);
    }
}
