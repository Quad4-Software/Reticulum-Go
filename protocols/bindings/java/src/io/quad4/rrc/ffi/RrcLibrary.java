// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc.ffi;

import com.sun.jna.Library;
import com.sun.jna.Native;
import com.sun.jna.Structure;
import com.sun.jna.ptr.IntByReference;
import com.sun.jna.ptr.LongByReference;

import java.io.File;
import java.util.Arrays;
import java.util.List;

public interface RrcLibrary extends Library {
    int HASH_LEN = 16;
    int RRC_OK = 0;
    int RRC_ERR_TIMEOUT = 7;

    int RRC_TYPE_MSG = 20;

    int RRC_EV_JOINED = 2;
    int RRC_EV_MSG = 4;

    RrcLibrary INSTANCE = load();

    static RrcLibrary load() {
        String env = System.getenv("RRC_LIB_PATH");
        if (env != null && !env.isEmpty()) {
            return Native.load(new File(env).getAbsolutePath(), RrcLibrary.class);
        }
        String root = System.getenv("RRC_ROOT");
        if (root != null) {
            for (String name : libNames()) {
                File lib = new File(root, "bin/" + name);
                if (lib.isFile()) {
                    return Native.load(lib.getAbsolutePath(), RrcLibrary.class);
                }
            }
        }
        return Native.load("rrc", RrcLibrary.class);
    }

    static String[] libNames() {
        String os = System.getProperty("os.name", "").toLowerCase();
        if (os.contains("mac")) {
            return new String[] {"librrc.dylib"};
        }
        if (os.contains("win")) {
            return new String[] {"rrc.dll", "librrc.dll"};
        }
        return new String[] {"librrc.so"};
    }

    String rrc_version();

    int rrc_last_error(byte[] buf, int bufLen, LongByReference written);

    long rrc_envelope_create(long msgType, byte[] sender, int senderLen);
    int rrc_envelope_set_room(long envelope, String room);
    int rrc_envelope_set_nick(long envelope, String nick);
    int rrc_envelope_set_body_text(long envelope, String text);
    int rrc_envelope_set_destination(long envelope, byte[] dest, int destLen);
    int rrc_envelope_get_type(long envelope, LongByReference out);
    int rrc_envelope_get_sender(long envelope, byte[] out, int outLen, LongByReference written);
    int rrc_envelope_get_room(long envelope, byte[] buf, int bufLen, LongByReference written);
    int rrc_envelope_get_nick(long envelope, byte[] buf, int bufLen, LongByReference written);
    int rrc_envelope_get_body_text(long envelope, byte[] buf, int bufLen, LongByReference written);
    int rrc_envelope_marshal(long envelope, byte[] out, int outLen, LongByReference written);
    long rrc_envelope_unmarshal(byte[] data, int dataLen);
    int rrc_envelope_destroy(long envelope);

    int rrc_normalize_room(String input, byte[] out, int outLen, LongByReference written);
    int rrc_sanitize_nick(String input, byte[] out, int outLen, LongByReference written);

    long rrc_node_create(String configPath);
    int rrc_node_start(long node);
    int rrc_node_stop(long node);
    int rrc_node_destroy(long node);
    int rrc_node_set_identity(long node, long identity);
    int rrc_node_add_udp_interface(long node, String name, String localAddr, String peerAddr);
    int rrc_node_has_path(long node, byte[] destHash, int destHashLen, IntByReference hasPath);

    long rrc_identity_generate();
    long rrc_identity_load(String path);
    int rrc_identity_save(long identity, String path);
    int rrc_identity_destroy(long identity);
    int rrc_identity_hash(long identity, byte[] out, int outLen, LongByReference written);
    int rrc_identity_seed_destination(long identity, byte[] destHash, int destHashLen);

    long rrc_hub_create(long node, long identity, String name, String version);
    int rrc_hub_start(long hub);
    int rrc_hub_announce(long hub);
    int rrc_hub_hash(long hub, byte[] out, int outLen, LongByReference written);
    int rrc_hub_peer_count(long hub, LongByReference count);
    int rrc_hub_destroy(long hub);
    int rrc_hub_event_poll(long hub, int timeoutMs, RrcEvent event);

    long rrc_client_dial(
        long node,
        long identity,
        byte[] hubHash,
        int hubHashLen,
        String nick,
        String name,
        String version,
        int timeoutMs
    );
    int rrc_client_join(long client, String room);
    int rrc_client_part(long client, String room);
    int rrc_client_send_msg(long client, String room, String text);
    int rrc_client_send_notice(long client, String room, String text);
    int rrc_client_send_action(long client, String room, String text);
    int rrc_client_ping(long client);
    int rrc_client_close(long client);
    int rrc_client_event_poll(long client, int timeoutMs, RrcEvent event);

    class RrcEvent extends Structure {
        public int kind;
        public byte[] sender = new byte[HASH_LEN];
        public long sender_len;
        public byte[] peer = new byte[HASH_LEN];
        public long peer_len;
        public byte[] room = new byte[128];
        public int room_truncated;
        public byte[] nick = new byte[64];
        public int nick_truncated;
        public byte[] body = new byte[1024];
        public int body_truncated;
        public long msg_type;

        @Override
        protected List<String> getFieldOrder() {
            return Arrays.asList(
                "kind", "sender", "sender_len", "peer", "peer_len",
                "room", "room_truncated", "nick", "nick_truncated",
                "body", "body_truncated", "msg_type"
            );
        }
    }
}
