// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package io.quad4.rns.android;

import java.io.Closeable;
import java.io.IOException;

/**
 * Byte stream to an RNode over USB Host, BLE UART, or classic Bluetooth RFCOMM.
 */
public interface RNodeByteTransport extends Closeable {
    int read(byte[] buffer) throws IOException;

    int write(byte[] buffer, int offset, int length) throws IOException;

    boolean isOpen();
}
