// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.android;

import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothSocket;
import android.content.Context;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.Set;
import java.util.UUID;

/**
 * Classic Bluetooth RFCOMM serial for RNode {@code bt://} targets.
 * Matches Python AndroidBluetoothManager RFcomm path.
 */
public final class BtRfcommTransport implements RNodeByteTransport {
    private static final UUID SPP_UUID = UUID.fromString("00001101-0000-1000-8000-00805F9B34FB");

    private final Context context;
    private BluetoothSocket socket;
    private InputStream in;
    private OutputStream out;

    public BtRfcommTransport(Context context) {
        this.context = context.getApplicationContext();
    }

    public void connect(String addressOrName) throws IOException {
        close();
        BluetoothDevice device = resolveDevice(addressOrName);
        if (device == null) {
            throw new IOException("Bluetooth device not found: " + addressOrName);
        }
        BluetoothSocket sock = device.createRfcommSocketToServiceRecord(SPP_UUID);
        BluetoothAdapter adapter = adapter();
        if (adapter != null) {
            adapter.cancelDiscovery();
        }
        sock.connect();
        this.socket = sock;
        this.in = sock.getInputStream();
        this.out = sock.getOutputStream();
    }

    private BluetoothDevice resolveDevice(String addressOrName) throws IOException {
        BluetoothAdapter adapter = adapter();
        if (adapter == null || !adapter.isEnabled()) {
            throw new IOException("Bluetooth adapter unavailable");
        }
        String key = addressOrName == null ? "" : addressOrName.trim();
        if (key.matches("(?i)^([0-9A-F]{2}:){5}[0-9A-F]{2}$")) {
            return adapter.getRemoteDevice(key.toUpperCase());
        }
        Set<BluetoothDevice> bonded = adapter.getBondedDevices();
        if (bonded == null) {
            return null;
        }
        for (BluetoothDevice d : bonded) {
            String name = d.getName();
            if (name != null && name.equalsIgnoreCase(key)) {
                return d;
            }
        }
        for (BluetoothDevice d : bonded) {
            String name = d.getName();
            if (name != null && name.startsWith("RNode ")) {
                return d;
            }
        }
        return null;
    }

    private BluetoothAdapter adapter() {
        BluetoothManager mgr = (BluetoothManager) context.getSystemService(Context.BLUETOOTH_SERVICE);
        if (mgr != null) {
            return mgr.getAdapter();
        }
        return BluetoothAdapter.getDefaultAdapter();
    }

    @Override
    public int read(byte[] buffer) throws IOException {
        ensureOpen();
        int n = in.read(buffer);
        if (n < 0) {
            close();
            throw new IOException("Bluetooth RFCOMM EOF");
        }
        return n;
    }

    @Override
    public int write(byte[] buffer, int offset, int length) throws IOException {
        ensureOpen();
        out.write(buffer, offset, length);
        out.flush();
        return length;
    }

    @Override
    public boolean isOpen() {
        return socket != null && socket.isConnected();
    }

    private void ensureOpen() throws IOException {
        if (!isOpen() || in == null || out == null) {
            throw new IOException("Bluetooth RFCOMM not open");
        }
    }

    @Override
    public void close() {
        try {
            if (in != null) {
                in.close();
            }
        } catch (Exception ignored) {
        }
        try {
            if (out != null) {
                out.close();
            }
        } catch (Exception ignored) {
        }
        try {
            if (socket != null) {
                socket.close();
            }
        } catch (Exception ignored) {
        }
        in = null;
        out = null;
        socket = null;
    }
}
