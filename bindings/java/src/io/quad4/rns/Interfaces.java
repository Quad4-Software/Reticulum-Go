// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns;

import com.sun.jna.ptr.LongByReference;
import io.quad4.rns.ffi.RnsLibrary;
import java.util.ArrayList;
import java.util.List;

/** Interface snapshot helpers. */
public final class Interfaces {
    private Interfaces() {}

    public static final class Info {
        public final String name;
        public final String typeName;
        public final boolean online;
        public final boolean enabled;
        public final long rxBytes;
        public final long txBytes;
        public final long rxPackets;
        public final long txPackets;

        Info(
                String name,
                String typeName,
                boolean online,
                boolean enabled,
                long rxBytes,
                long txBytes,
                long rxPackets,
                long txPackets) {
            this.name = name;
            this.typeName = typeName;
            this.online = online;
            this.enabled = enabled;
            this.rxBytes = rxBytes;
            this.txBytes = txBytes;
            this.rxPackets = rxPackets;
            this.txPackets = txPackets;
        }
    }

    public static List<Info> list(Node node, int capacity) {
        if (capacity <= 0) {
            throw new RnsException(RnsException.INVALID_ARG);
        }
        RnsLibrary.RnsInterfaceEntry[] entries =
                (RnsLibrary.RnsInterfaceEntry[]) new RnsLibrary.RnsInterfaceEntry().toArray(capacity);
        LongByReference written = new LongByReference();
        int code = RnsLibrary.INSTANCE.rns_interfaces(node.handle(), entries, capacity, written);
        if (code != RnsLibrary.RNS_OK && code != RnsLibrary.RNS_ERR_TRUNCATED) {
            Rns.check(code);
        }
        List<Info> out = new ArrayList<>((int) written.getValue());
        for (int i = 0; i < written.getValue(); i++) {
            RnsLibrary.RnsInterfaceEntry e = entries[i];
            out.add(
                    new Info(
                            RnsLibrary.cstr(e.name),
                            RnsLibrary.cstr(e.type_name),
                            e.online != 0,
                            e.enabled != 0,
                            e.rx_bytes,
                            e.tx_bytes,
                            e.rx_packets,
                            e.tx_packets));
        }
        return out;
    }
}
