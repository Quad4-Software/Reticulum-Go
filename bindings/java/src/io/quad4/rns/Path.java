// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;
import java.util.ArrayList;
import java.util.List;

/** Path table helpers. */
public final class Path {
    private Path() {}

    public static final class Info {
        public final byte[] hash;
        public final byte[] via;
        public final int hops;
        public final String iface;
        public final double timestamp;
        public final double expires;

        Info(byte[] hash, byte[] via, int hops, String iface, double timestamp, double expires) {
            this.hash = hash;
            this.via = via;
            this.hops = hops;
            this.iface = iface;
            this.timestamp = timestamp;
            this.expires = expires;
        }
    }

    public static void request(Node node, byte[] destHash) {
        if (destHash == null || destHash.length != Rns.HASH_LEN) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        Rns.check(RnsLibrary.INSTANCE.rns_path_request(node.handle(), destHash));
    }

    public static List<Info> table(Node node, int capacity, int maxHops) {
        if (capacity <= 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        RnsLibrary.RnsPathEntry[] entries =
                (RnsLibrary.RnsPathEntry[]) new RnsLibrary.RnsPathEntry().toArray(capacity);
        LongByReference written = new LongByReference();
        int code = RnsLibrary.INSTANCE.rns_path_table(node.handle(), entries, capacity, written, maxHops);
        if (code != RnsLibrary.RNS_OK && code != RnsLibrary.RNS_ERR_TRUNCATED) {
            Rns.check(code);
        }
        List<Info> out = new ArrayList<>((int) written.getValue());
        for (int i = 0; i < written.getValue(); i++) {
            RnsLibrary.RnsPathEntry e = entries[i];
            out.add(
                    new Info(
                            RnsLibrary.copyHash(e.hash, e.hash_len),
                            RnsLibrary.copyHash(e.via, e.via_len),
                            e.hops & 0xff,
                            RnsLibrary.cstr(e.iface),
                            e.timestamp,
                            e.expires));
        }
        return out;
    }

    public static boolean known(Node node, byte[] destHash) {
        if (destHash == null || destHash.length != Rns.HASH_LEN) {
            return false;
        }
        try {
            for (Info entry : table(node, 256, -1)) {
                if (Rns.hashEq(entry.hash, destHash)) {
                    return true;
                }
            }
        } catch (RnsException ignored) {
            return false;
        }
        return false;
    }
}
