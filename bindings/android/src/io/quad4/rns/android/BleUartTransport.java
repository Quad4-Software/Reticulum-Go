// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.android;

import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothGatt;
import android.bluetooth.BluetoothGattCallback;
import android.bluetooth.BluetoothGattCharacteristic;
import android.bluetooth.BluetoothGattDescriptor;
import android.bluetooth.BluetoothGattService;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothProfile;
import android.content.Context;
import android.os.Build;
import java.io.IOException;
import java.util.ArrayDeque;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Nordic UART Service (NUS) BLE client matching Python RNS able UART UUIDs.
 * Connect with {@code ble://AA:BB:CC:DD:EE:FF} or a paired device name.
 *
 * Host apps should bridge this transport with {@link RNodeTcpBridge} and set
 * Go RNode port to tcp://127.0.0.1:PORT or register a Go
 * RegisterRNodePortOpener("ble", ...) that feeds an RNodeHostPipe.
 */
public final class BleUartTransport implements RNodeByteTransport {
    public static final UUID UART_SERVICE_UUID =
            UUID.fromString("6e400001-b5a3-f393-e0a9-e50e24dcca9e");
    public static final UUID UART_RX_CHAR_UUID =
            UUID.fromString("6e400002-b5a3-f393-e0a9-e50e24dcca9e");
    public static final UUID UART_TX_CHAR_UUID =
            UUID.fromString("6e400003-b5a3-f393-e0a9-e50e24dcca9e");
    public static final UUID CCCD_UUID =
            UUID.fromString("00002902-0000-1000-8000-00805f9b34fb");

    private static final int BASE_MTU = 20;
    private static final int TARGET_MTU = 512;
    private static final long CONNECT_TIMEOUT_MS = 7000;
    private static final long MTU_TIMEOUT_MS = 4000;
    private static final int MAX_RX_QUEUE_BYTES = 256 * 1024;

    private final Context context;
    private final Object lock = new Object();
    private final ArrayDeque<byte[]> rxQueue = new ArrayDeque<>();
    private final AtomicBoolean open = new AtomicBoolean(false);
    private int rxQueueBytes;

    private BluetoothGatt gatt;
    private BluetoothGattCharacteristic rxChar;
    private BluetoothGattCharacteristic txChar;
    private int payloadMtu = BASE_MTU;
    private CountDownLatch connectLatch;
    private CountDownLatch servicesLatch;
    private CountDownLatch mtuLatch;
    private CountDownLatch writeLatch;
    private volatile IOException connectError;
    private volatile boolean writeOk;

    public BleUartTransport(Context context) {
        this.context = context.getApplicationContext();
    }

    public void connect(String addressOrName) throws IOException {
        close();
        BluetoothDevice device = resolveDevice(addressOrName);
        if (device == null) {
            throw new IOException("BLE device not found: " + addressOrName);
        }
        connectError = null;
        connectLatch = new CountDownLatch(1);
        servicesLatch = new CountDownLatch(1);
        mtuLatch = new CountDownLatch(1);
        payloadMtu = BASE_MTU;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            gatt = device.connectGatt(context, false, callback, BluetoothDevice.TRANSPORT_LE);
        } else {
            gatt = device.connectGatt(context, false, callback);
        }
        if (gatt == null) {
            throw new IOException("connectGatt returned null");
        }
        await(connectLatch, CONNECT_TIMEOUT_MS, "BLE connect");
        if (connectError != null) {
            close();
            throw connectError;
        }
        await(servicesLatch, CONNECT_TIMEOUT_MS, "BLE service discovery");
        if (rxChar == null || txChar == null) {
            close();
            throw new IOException("Nordic UART characteristics missing");
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            gatt.requestMtu(TARGET_MTU);
            try {
                mtuLatch.await(MTU_TIMEOUT_MS, TimeUnit.MILLISECONDS);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }
        if (!enableNotify(txChar)) {
            close();
            throw new IOException("BLE notify enable failed");
        }
        open.set(true);
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
        String want = key.toLowerCase();
        for (BluetoothDevice d : bonded) {
            String name = d.getName();
            if (name != null && name.equalsIgnoreCase(want)) {
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

    private boolean enableNotify(BluetoothGattCharacteristic characteristic) {
        if (!gatt.setCharacteristicNotification(characteristic, true)) {
            return false;
        }
        BluetoothGattDescriptor cccd = characteristic.getDescriptor(CCCD_UUID);
        if (cccd == null) {
            return true;
        }
        cccd.setValue(BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE);
        return gatt.writeDescriptor(cccd);
    }

    private final BluetoothGattCallback callback = new BluetoothGattCallback() {
        @Override
        public void onConnectionStateChange(BluetoothGatt g, int status, int newState) {
            if (newState == BluetoothProfile.STATE_CONNECTED) {
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    connectError = new IOException("BLE connect failed status=" + status);
                    CountDownLatch latch = connectLatch;
                    if (latch != null) {
                        latch.countDown();
                    }
                    return;
                }
                g.discoverServices();
                CountDownLatch latch = connectLatch;
                if (latch != null) {
                    latch.countDown();
                }
            } else if (newState == BluetoothProfile.STATE_DISCONNECTED) {
                boolean wasOpen = open.getAndSet(false);
                if (!wasOpen && connectError == null) {
                    connectError = new IOException("BLE disconnected before ready status=" + status);
                } else if (status != BluetoothGatt.GATT_SUCCESS && connectError == null) {
                    connectError = new IOException("BLE disconnect status=" + status);
                }
                CountDownLatch latch = connectLatch;
                if (latch != null) {
                    latch.countDown();
                }
                synchronized (lock) {
                    lock.notifyAll();
                }
            }
        }

        @Override
        public void onServicesDiscovered(BluetoothGatt g, int status) {
            if (status != BluetoothGatt.GATT_SUCCESS) {
                connectError = new IOException("service discovery failed status=" + status);
                CountDownLatch latch = servicesLatch;
                if (latch != null) {
                    latch.countDown();
                }
                return;
            }
            BluetoothGattService service = g.getService(UART_SERVICE_UUID);
            if (service != null) {
                rxChar = service.getCharacteristic(UART_RX_CHAR_UUID);
                txChar = service.getCharacteristic(UART_TX_CHAR_UUID);
            }
            CountDownLatch latch = servicesLatch;
            if (latch != null) {
                latch.countDown();
            }
        }

        @Override
        public void onMtuChanged(BluetoothGatt g, int mtu, int status) {
            if (status == BluetoothGatt.GATT_SUCCESS && mtu > 3) {
                payloadMtu = Math.min(TARGET_MTU, mtu - 3);
            }
            CountDownLatch latch = mtuLatch;
            if (latch != null) {
                latch.countDown();
            }
        }

        @Override
        public void onCharacteristicChanged(BluetoothGatt g, BluetoothGattCharacteristic characteristic) {
            byte[] value = characteristic.getValue();
            if (value == null || value.length == 0) {
                return;
            }
            byte[] copy = new byte[value.length];
            System.arraycopy(value, 0, copy, 0, value.length);
            synchronized (lock) {
                while (rxQueueBytes + copy.length > MAX_RX_QUEUE_BYTES && !rxQueue.isEmpty()) {
                    byte[] dropped = rxQueue.removeFirst();
                    rxQueueBytes -= dropped.length;
                }
                if (rxQueueBytes + copy.length > MAX_RX_QUEUE_BYTES) {
                    return;
                }
                rxQueue.addLast(copy);
                rxQueueBytes += copy.length;
                lock.notifyAll();
            }
        }

        @Override
        public void onCharacteristicWrite(BluetoothGatt g, BluetoothGattCharacteristic characteristic, int status) {
            writeOk = status == BluetoothGatt.GATT_SUCCESS;
            CountDownLatch latch = writeLatch;
            if (latch != null) {
                latch.countDown();
            }
        }
    };

    @Override
    public int read(byte[] buffer) throws IOException {
        ensureOpen();
        synchronized (lock) {
            while (rxQueue.isEmpty() && open.get()) {
                try {
                    lock.wait(100);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    throw new IOException("interrupted", e);
                }
            }
            if (rxQueue.isEmpty()) {
                return 0;
            }
            byte[] next = rxQueue.removeFirst();
            rxQueueBytes -= next.length;
            int n = Math.min(buffer.length, next.length);
            System.arraycopy(next, 0, buffer, 0, n);
            if (n < next.length) {
                byte[] rest = new byte[next.length - n];
                System.arraycopy(next, n, rest, 0, rest.length);
                rxQueue.addFirst(rest);
                rxQueueBytes += rest.length;
            }
            return n;
        }
    }

    @Override
    public int write(byte[] buffer, int offset, int length) throws IOException {
        ensureOpen();
        if (rxChar == null || gatt == null) {
            throw new IOException("BLE RX characteristic missing");
        }
        int written = 0;
        while (written < length) {
            int chunk = Math.min(payloadMtu, length - written);
            byte[] slice = new byte[chunk];
            System.arraycopy(buffer, offset + written, slice, 0, chunk);
            writeLatch = new CountDownLatch(1);
            writeOk = false;
            synchronized (lock) {
                rxChar.setValue(slice);
                if (!gatt.writeCharacteristic(rxChar)) {
                    throw new IOException("BLE writeCharacteristic failed");
                }
            }
            try {
                if (!writeLatch.await(2, TimeUnit.SECONDS) || !writeOk) {
                    throw new IOException("BLE write timed out or failed");
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new IOException("interrupted", e);
            }
            written += chunk;
        }
        return written;
    }

    @Override
    public boolean isOpen() {
        return open.get();
    }

    private void ensureOpen() throws IOException {
        if (!open.get()) {
            throw new IOException("BLE UART not open");
        }
    }

    private static void await(CountDownLatch latch, long timeoutMs, String label) throws IOException {
        try {
            if (!latch.await(timeoutMs, TimeUnit.MILLISECONDS)) {
                throw new IOException(label + " timed out");
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new IOException(label + " interrupted", e);
        }
    }

    @Override
    public void close() {
        open.set(false);
        synchronized (lock) {
            rxQueue.clear();
            rxQueueBytes = 0;
            lock.notifyAll();
        }
        if (gatt != null) {
            try {
                gatt.disconnect();
            } catch (Exception ignored) {
            }
            try {
                gatt.close();
            } catch (Exception ignored) {
            }
        }
        gatt = null;
        rxChar = null;
        txChar = null;
    }
}
