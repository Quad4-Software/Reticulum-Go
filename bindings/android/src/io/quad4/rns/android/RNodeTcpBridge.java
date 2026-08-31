// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.android;

import java.io.Closeable;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Localhost TCP bridge so Go RNode can use {@code tcp://127.0.0.1:port}
 * without JNI. Accepts one client and relays bytes to a USB BLE or RFCOMM
 * transport.
 */
public final class RNodeTcpBridge implements Closeable {
    private final RNodeByteTransport transport;
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ServerSocket server;
    private Socket client;
    private Thread acceptThread;
    private Thread uplink;
    private Thread downlink;

    public RNodeTcpBridge(RNodeByteTransport transport) {
        if (transport == null) {
            throw new IllegalArgumentException("transport required");
        }
        this.transport = transport;
    }

    /**
     * Binds 127.0.0.1 on an ephemeral port and starts accept plus relay
     * threads. Returns the bound port for Go tcp://127.0.0.1:port.
     */
    public synchronized int start() throws IOException {
        if (running.get()) {
            return server.getLocalPort();
        }
        if (!transport.isOpen()) {
            throw new IOException("underlying transport is not open");
        }
        server = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"));
        server.setSoTimeout(500);
        running.set(true);
        acceptThread = new Thread(this::acceptLoop, "rnode-tcp-accept");
        acceptThread.setDaemon(true);
        acceptThread.start();
        return server.getLocalPort();
    }

    public int localPort() {
        return server == null ? -1 : server.getLocalPort();
    }

    private void acceptLoop() {
        while (running.get()) {
            try {
                Socket sock = server.accept();
                synchronized (this) {
                    if (client != null) {
                        try {
                            client.close();
                        } catch (Exception ignored) {
                        }
                    }
                    client = sock;
                    startRelay(sock);
                }
            } catch (SocketTimeoutException ignored) {
            } catch (IOException e) {
                if (running.get()) {
                    break;
                }
            }
        }
    }

    private void startRelay(Socket sock) {
        stopRelayThreads();
        uplink = new Thread(() -> tcpToRadio(sock), "rnode-tcp-up");
        downlink = new Thread(() -> radioToTcp(sock), "rnode-tcp-down");
        uplink.setDaemon(true);
        downlink.setDaemon(true);
        uplink.start();
        downlink.start();
    }

    private void tcpToRadio(Socket sock) {
        byte[] buf = new byte[4096];
        try {
            InputStream in = sock.getInputStream();
            while (running.get() && !sock.isClosed()) {
                int n = in.read(buf);
                if (n < 0) {
                    break;
                }
                if (n > 0) {
                    transport.write(buf, 0, n);
                }
            }
        } catch (IOException ignored) {
        }
    }

    private void radioToTcp(Socket sock) {
        byte[] buf = new byte[4096];
        try {
            OutputStream out = sock.getOutputStream();
            while (running.get() && !sock.isClosed() && transport.isOpen()) {
                int n = transport.read(buf);
                if (n > 0) {
                    out.write(buf, 0, n);
                    out.flush();
                }
            }
        } catch (IOException ignored) {
        }
    }

    private void stopRelayThreads() {
        Thread u = uplink;
        Thread d = downlink;
        uplink = null;
        downlink = null;
        if (u != null) {
            u.interrupt();
        }
        if (d != null) {
            d.interrupt();
        }
    }

    @Override
    public synchronized void close() {
        running.set(false);
        stopRelayThreads();
        try {
            if (client != null) {
                client.close();
            }
        } catch (Exception ignored) {
        }
        try {
            if (server != null) {
                server.close();
            }
        } catch (Exception ignored) {
        }
        client = null;
        server = null;
        try {
            transport.close();
        } catch (Exception ignored) {
        }
    }
}
