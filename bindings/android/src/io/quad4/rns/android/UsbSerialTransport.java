// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.android;

import android.content.Context;
import android.hardware.usb.UsbConstants;
import android.hardware.usb.UsbDevice;
import android.hardware.usb.UsbDeviceConnection;
import android.hardware.usb.UsbEndpoint;
import android.hardware.usb.UsbInterface;
import android.hardware.usb.UsbManager;
import java.io.IOException;
import java.util.HashMap;

/**
 * Android USB Host bulk serial for RNode analogous to Python usb4a and
 * usbserial4a. Opens the first bulk IN/OUT pair on the device.
 *
 * Pair with {@link RNodeTcpBridge} and Go {@code tcp://127.0.0.1:port} or feed
 * an {@code RNodeHostPipe} via JNI or FFI registration.
 */
public final class UsbSerialTransport implements RNodeByteTransport {
    public static final int VID_FTDI = 0x0403;
    public static final int VID_SILABS = 0x10C4;
    public static final int VID_QINHENG = 0x1A86;
    public static final int PID_CH9102 = 0x55D4;

    private final UsbManager usbManager;
    private UsbDevice device;
    private UsbDeviceConnection connection;
    private UsbEndpoint epIn;
    private UsbEndpoint epOut;
    private UsbInterface claimed;
    private int readTimeoutMs = 100;
    private int writeTimeoutMs = 1000;

    public UsbSerialTransport(Context context) {
        this.usbManager = (UsbManager) context.getSystemService(Context.USB_SERVICE);
    }

    public HashMap<String, UsbDevice> listDevices() {
        return usbManager.getDeviceList();
    }

    public void open(UsbDevice device) throws IOException {
        close();
        if (device == null) {
            throw new IOException("null USB device");
        }
        if (!usbManager.hasPermission(device)) {
            throw new IOException("USB permission not granted for " + device.getDeviceName());
        }
        UsbDeviceConnection conn = usbManager.openDevice(device);
        if (conn == null) {
            throw new IOException("openDevice failed for " + device.getDeviceName());
        }
        UsbInterface iface = null;
        UsbEndpoint in = null;
        UsbEndpoint out = null;
        for (int i = 0; i < device.getInterfaceCount(); i++) {
            UsbInterface candidate = device.getInterface(i);
            UsbEndpoint cin = null;
            UsbEndpoint cout = null;
            for (int e = 0; e < candidate.getEndpointCount(); e++) {
                UsbEndpoint ep = candidate.getEndpoint(e);
                if (ep.getType() != UsbConstants.USB_ENDPOINT_XFER_BULK) {
                    continue;
                }
                if (ep.getDirection() == UsbConstants.USB_DIR_IN) {
                    cin = ep;
                } else if (ep.getDirection() == UsbConstants.USB_DIR_OUT) {
                    cout = ep;
                }
            }
            if (cin != null && cout != null) {
                iface = candidate;
                in = cin;
                out = cout;
                break;
            }
        }
        if (iface == null || in == null || out == null) {
            conn.close();
            throw new IOException("no bulk IN/OUT endpoints on " + device.getDeviceName());
        }
        if (!conn.claimInterface(iface, true)) {
            conn.close();
            throw new IOException("claimInterface failed");
        }
        applyChipHints(device);
        this.device = device;
        this.connection = conn;
        this.claimed = iface;
        this.epIn = in;
        this.epOut = out;
    }

    public void openByName(String deviceName) throws IOException {
        UsbDevice match = null;
        for (UsbDevice d : usbManager.getDeviceList().values()) {
            if (deviceName.equals(d.getDeviceName())) {
                match = d;
                break;
            }
        }
        if (match == null) {
            throw new IOException("USB device not found: " + deviceName);
        }
        open(match);
    }

    private void applyChipHints(UsbDevice device) {
        int vid = device.getVendorId();
        int pid = device.getProductId();
        if (vid == VID_FTDI) {
            readTimeoutMs = 100;
        } else if (vid == VID_SILABS) {
            readTimeoutMs = 12;
        } else if (vid == VID_QINHENG && pid == PID_CH9102) {
            readTimeoutMs = 12;
        } else {
            readTimeoutMs = 100;
        }
    }

    @Override
    public int read(byte[] buffer) throws IOException {
        ensureOpen();
        int n = connection.bulkTransfer(epIn, buffer, buffer.length, readTimeoutMs);
        if (n < 0) {
            return 0;
        }
        return n;
    }

    @Override
    public int write(byte[] buffer, int offset, int length) throws IOException {
        ensureOpen();
        byte[] slice = buffer;
        if (offset != 0 || length != buffer.length) {
            slice = new byte[length];
            System.arraycopy(buffer, offset, slice, 0, length);
        }
        int n = connection.bulkTransfer(epOut, slice, slice.length, writeTimeoutMs);
        if (n < 0) {
            throw new IOException("USB bulk write failed");
        }
        return n;
    }

    @Override
    public boolean isOpen() {
        return connection != null && epIn != null && epOut != null;
    }

    public String deviceName() {
        return device == null ? null : device.getDeviceName();
    }

    private void ensureOpen() throws IOException {
        if (!isOpen()) {
            throw new IOException("USB serial not open");
        }
    }

    @Override
    public void close() {
        if (connection != null && claimed != null) {
            try {
                connection.releaseInterface(claimed);
            } catch (Exception ignored) {
            }
        }
        if (connection != null) {
            connection.close();
        }
        connection = null;
        claimed = null;
        epIn = null;
        epOut = null;
        device = null;
    }
}
