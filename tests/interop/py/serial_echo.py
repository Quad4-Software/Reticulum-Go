#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Serial HDLC peer matching RNS SerialInterface framing for live interop.
# Opens SERIAL_DEVICE (PTY slave) with pyserial and echoes framed packets.

import os
import sys
import time

FLAG = 0x7E
ESC = 0x7D
MASK = 0x20
HW_MTU = 564


def escape(data: bytes) -> bytes:
    out = bytearray()
    for b in data:
        if b == ESC:
            out.extend([ESC, ESC ^ MASK])
        elif b == FLAG:
            out.extend([ESC, FLAG ^ MASK])
        else:
            out.append(b)
    return bytes(out)


def main() -> None:
    device = os.environ.get("SERIAL_DEVICE", "")
    speed = int(os.environ.get("SERIAL_SPEED", "115200"))
    if not device:
        print("SERIAL_DEVICE required", file=sys.stderr)
        sys.exit(1)
    try:
        import serial
    except ImportError:
        print("pyserial required", file=sys.stderr)
        sys.exit(1)

    ser = serial.Serial(
        port=device,
        baudrate=speed,
        bytesize=8,
        parity="N",
        stopbits=1,
        timeout=0,
        xonxoff=False,
        rtscts=False,
        dsrdtr=False,
    )
    print("READY", flush=True)

    in_frame = False
    escape_b = False
    buf = bytearray()
    last_ms = int(time.time() * 1000)

    while ser.is_open:
        waiting = ser.in_waiting
        if waiting:
            chunk = ser.read(waiting)
            last_ms = int(time.time() * 1000)
            for b in chunk:
                if b == FLAG:
                    if in_frame and buf:
                        ser.write(bytes([FLAG]) + escape(bytes(buf)) + bytes([FLAG]))
                        ser.flush()
                    buf.clear()
                    in_frame = True
                    escape_b = False
                    continue
                if not in_frame:
                    continue
                if b == ESC:
                    escape_b = True
                    continue
                if escape_b:
                    b ^= MASK
                    escape_b = False
                if len(buf) < HW_MTU:
                    buf.append(b)
                else:
                    buf.clear()
                    in_frame = False
            continue
        now = int(time.time() * 1000)
        if in_frame and now - last_ms > 100:
            buf.clear()
            in_frame = False
            escape_b = False
        time.sleep(0.01)


if __name__ == "__main__":
    main()
