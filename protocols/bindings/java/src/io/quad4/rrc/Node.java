// SPDX-License-Identifier: LicenseRef-Reticulum

package io.quad4.rrc;

import com.sun.jna.ptr.IntByReference;
import io.quad4.rrc.ffi.RrcLibrary;

public final class Node implements AutoCloseable {
    private long handle;

    private Node(long handle) {
        this.handle = handle;
    }

    public static Node create() {
        return create("");
    }

    public static Node create(String configPath) {
        long h = RrcLibrary.INSTANCE.rrc_node_create(configPath);
        if (h == 0) {
            throw new RrcException(RrcException.INTERNAL);
        }
        return new Node(h);
    }

    public void start() {
        Rrc.check(RrcLibrary.INSTANCE.rrc_node_start(handle));
    }

    public void stop() {
        Rrc.check(RrcLibrary.INSTANCE.rrc_node_stop(handle));
    }

    public void setIdentity(Identity identity) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_node_set_identity(handle, identity.handle()));
    }

    public void addUdpInterface(String name, String localAddr, String peerAddr) {
        Rrc.check(RrcLibrary.INSTANCE.rrc_node_add_udp_interface(handle, name, localAddr, peerAddr));
    }

    public boolean hasPath(byte[] destHash) {
        if (destHash.length != Rrc.HASH_LEN) {
            throw new RrcException(RrcException.INVALID_ARG);
        }
        IntByReference out = new IntByReference();
        Rrc.check(RrcLibrary.INSTANCE.rrc_node_has_path(handle, destHash, destHash.length, out));
        return out.getValue() != 0;
    }

    public long handle() {
        return handle;
    }

    @Override
    public void close() {
        if (handle != 0) {
            RrcLibrary.INSTANCE.rrc_node_destroy(handle);
            handle = 0;
        }
    }
}
