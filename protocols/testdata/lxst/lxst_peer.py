# SPDX-License-Identifier: LicenseRef-Reticulum
"""Headless LXST telephony peer for Go interop tests."""

import argparse
import json
import os
import sys
import threading
import time

import numpy as np
import RNS
from LXST.Network import LinkSource, Packetizer, SignallingReceiver
from LXST.Primitives.Telephony import PRIMITIVE_NAME, Profiles, Signalling
from LXST import APP_NAME


def emit(obj):
    obj["t_ns"] = time.perf_counter_ns()
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


class FrameOrigin:
    def __init__(self, codec):
        self.codec = codec


class FrameSink:
    def __init__(self, on_frame):
        self.channels = 1
        self.samplerate = 48000
        self.n = 0
        self.on_frame = on_frame

    def handle_frame(self, frame, source=None, decoded=False):
        self.n += 1
        if callable(self.on_frame):
            self.on_frame(self.n)


def encode_codec2(codec, tone):
    input_samples = (tone * np.iinfo(np.int16).max).astype(np.int16)
    c2 = codec.c2
    spf = c2.samples_per_frame()
    chunks = []
    for i in range(0, len(input_samples), spf):
        chunk = input_samples[i : i + spf]
        if len(chunk) < spf:
            break
        chunks.append(c2.encode(chunk))
    if not chunks:
        return None
    return codec.mode_header + b"".join(chunks)


class HeadlessPhone(SignallingReceiver):
    def __init__(self, identity, auto_answer=0.2, send_frames=12):
        super().__init__()
        self.identity = identity
        self.auto_answer = auto_answer
        self.send_frames = send_frames
        self.lock = threading.Lock()
        self.active = None
        self.status = Signalling.STATUS_AVAILABLE
        self.incoming = False
        self.answered = False
        self.profile = Profiles.DEFAULT_PROFILE
        self.mode = Profiles.DEFAULT_MODE
        self.packetizer = None
        self.source = None
        self.sink = FrameSink(lambda n: emit({"event": "frame", "n": n}))
        self.codec = None
        self.should_run = True
        self.dest = RNS.Destination(
            self.identity,
            RNS.Destination.IN,
            RNS.Destination.SINGLE,
            APP_NAME,
            PRIMITIVE_NAME,
        )
        self.dest.set_proof_strategy(RNS.Destination.PROVE_NONE)
        self.dest.set_link_established_callback(self._incoming)

    def _incoming(self, link):
        with self.lock:
            if self.active is not None:
                self.signal(Signalling.STATUS_BUSY, link)
                link.teardown()
                return
            link.set_remote_identified_callback(self._identified)
            link.set_link_closed_callback(self._closed)
            self.active = link
            self.incoming = True
            self.answered = False
            self.handle_signalling_from(link)
            self.signal(Signalling.STATUS_AVAILABLE, link)

    def _identified(self, link, identity):
        with self.lock:
            if link != self.active:
                self.signal(Signalling.STATUS_BUSY, link)
                link.teardown()
                return
            self.status = Signalling.STATUS_RINGING
            self.signal(Signalling.STATUS_RINGING, link)
            emit({"event": "ringing", "from": RNS.hexrep(identity.hash, delimit=False)})
            if self.auto_answer is not None:
                threading.Thread(target=self._auto_answer, daemon=True).start()

    def _auto_answer(self):
        time.sleep(self.auto_answer)
        if self.active and self.incoming and not self.answered:
            self.answer()

    def answer(self):
        with self.lock:
            if not self.active or self.answered:
                return
            self.answered = True
            self.signal(Signalling.STATUS_CONNECTING, self.active)
            self._open(self.active)
            self.signal(Signalling.STATUS_ESTABLISHED, self.active)
            self.status = Signalling.STATUS_ESTABLISHED
            emit({"event": "established"})
            self._start_sender()

    def call(self, identity):
        dest = RNS.Destination(
            identity,
            RNS.Destination.OUT,
            RNS.Destination.SINGLE,
            APP_NAME,
            PRIMITIVE_NAME,
        )
        timeout = time.time() + 20
        if not RNS.Transport.has_path(dest.hash):
            RNS.Transport.request_path(dest.hash)
            while not RNS.Transport.has_path(dest.hash) and time.time() < timeout:
                time.sleep(0.05)
        if not RNS.Transport.has_path(dest.hash):
            emit({"event": "error", "error": "no path to remote"})
            self.should_run = False
            return
        emit({"event": "path", "destination": RNS.hexrep(dest.hash, delimit=False)})
        self.incoming = False
        self.answered = False
        self.status = Signalling.STATUS_CALLING
        emit({"event": "dial_start"})
        self.active = RNS.Link(
            dest,
            established_callback=self._outgoing_up,
            closed_callback=self._closed,
        )
        self.handle_signalling_from(self.active)

    def _outgoing_up(self, link):
        self.active = link
        self.handle_signalling_from(link)

    def _closed(self, link):
        if link == self.active:
            emit({"event": "ended", "frames": self.sink.n})
            self.active = None
            self.status = Signalling.STATUS_AVAILABLE
            self.should_run = False

    def signalling_received(self, signals, source):
        if source != self.active:
            return
        for signal in signals:
            if self.incoming and not self.answered and signal < Signalling.PREFERRED_MODE:
                continue
            if signal == Signalling.STATUS_BUSY:
                emit({"event": "busy"})
                self.active.teardown()
            elif signal == Signalling.STATUS_REJECTED:
                emit({"event": "rejected"})
                self.active.teardown()
            elif signal == Signalling.STATUS_AVAILABLE:
                self.status = signal
                source.identify(self.identity)
            elif signal == Signalling.STATUS_RINGING:
                self.status = signal
                emit({"event": "ringing"})
                self.signal(
                    [
                        Signalling.PREFERRED_PROFILE + self.profile,
                        Signalling.PREFERRED_MODE + self.mode,
                    ],
                    self.active,
                )
            elif signal == Signalling.STATUS_CONNECTING:
                self.status = signal
                self._open(self.active)
            elif signal == Signalling.STATUS_ESTABLISHED:
                if not self.incoming:
                    self._open(self.active)
                    self.status = signal
                    emit({"event": "established"})
                    self._start_sender()
            elif signal >= Signalling.PREFERRED_PROFILE:
                self.profile = signal - Signalling.PREFERRED_PROFILE
            elif signal >= Signalling.PREFERRED_MODE:
                self.mode = signal - Signalling.PREFERRED_MODE

    def _open(self, link):
        if self.packetizer is not None:
            return
        self.codec = Profiles.get_codec(self.profile)
        src_rate = 24000
        pref = getattr(self.codec, "preferred_samplerate", None)
        if callable(pref):
            src_rate = int(pref())
        elif pref:
            src_rate = int(pref)
        elif hasattr(self.codec, "INPUT_RATE"):
            src_rate = int(self.codec.INPUT_RATE)
        self.codec.source = type("S", (), {"samplerate": src_rate, "channels": 1})()
        self.packetizer = Packetizer(link)
        self.packetizer.source = FrameOrigin(self.codec)
        if self.mode == Profiles.MODE_HALF_DUPLEX:
            self.packetizer.squelch()
        self.source = LinkSource(link=link, signalling_receiver=self, sink=self.sink)

    def _start_sender(self):
        threading.Thread(target=self._send_loop, daemon=True).start()

    def _send_loop(self):
        sent = 0
        rate = 24000
        pref = getattr(self.codec, "preferred_samplerate", None)
        if callable(pref):
            rate = int(pref())
        elif pref:
            rate = int(pref)
        elif hasattr(self.codec, "INPUT_RATE"):
            rate = int(self.codec.INPUT_RATE)
        frame_ms = Profiles.get_frame_time(self.profile)
        samples = max(1, int(rate * (frame_ms / 1000.0)))
        t = np.arange(samples) / float(rate)
        while self.active and sent < self.send_frames:
            if self.packetizer is not None and self.packetizer.squelched:
                self.packetizer.unsquelch()
            tone = 0.2 * np.sin(2 * np.pi * 440.0 * t)
            try:
                if hasattr(self.codec, "INPUT_RATE"):
                    encoded = encode_codec2(self.codec, tone)
                else:
                    pcm = np.zeros((samples, 1), dtype=np.float32)
                    pcm[:, 0] = tone
                    encoded = self.codec.encode(pcm)
                if encoded is None:
                    encoded = b""
                if not encoded:
                    emit({"event": "error", "error": "codec produced empty frame"})
                    break
                self.packetizer.handle_frame(encoded)
            except Exception as e:
                emit({"event": "error", "error": str(e)})
                break
            sent += 1
            time.sleep(max(0.04, frame_ms / 1000.0))
        time.sleep(0.8)
        if self.active:
            self.active.teardown()


def write_config(path, listen_port, target_port, name):
    os.makedirs(path, exist_ok=True)
    cfg = f"""[reticulum]
enable_transport = Yes
share_instance = No
instance_name = {name}
shared_instance_port = {37000 + (listen_port % 1000)}
instance_control_port = {38000 + (listen_port % 1000)}

[logging]
loglevel = 2

[interfaces]
  [[UDP]]
    type = UDPInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = {listen_port}
    forward_ip = 127.0.0.1
    forward_port = {target_port}
"""
    with open(os.path.join(path, "config"), "w") as f:
        f.write(cfg)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--configdir", required=True)
    p.add_argument("--listen-port", type=int, required=True)
    p.add_argument("--target-port", type=int, required=True)
    p.add_argument("--mode", choices=["listen", "dial"], default="listen")
    p.add_argument("--dial", default="")
    p.add_argument("--auto-answer", type=float, default=0.2)
    p.add_argument("--frames", type=int, default=12)
    p.add_argument("--name", default="lxstpeer")
    p.add_argument("--profile", default="mq")
    p.add_argument("--call-mode", choices=["full", "half"], default="full")
    args = p.parse_args()

    write_config(args.configdir, args.listen_port, args.target_port, args.name)
    RNS.Reticulum(configdir=args.configdir, loglevel=2)
    identity = RNS.Identity()
    phone = HeadlessPhone(identity, auto_answer=args.auto_answer, send_frames=args.frames)
    if args.call_mode == "half":
        phone.mode = Profiles.MODE_HALF_DUPLEX
    if args.profile == "lbw":
        phone.profile = Profiles.BANDWIDTH_LOW
    elif args.profile == "vlbw":
        phone.profile = Profiles.BANDWIDTH_VERY_LOW
    elif args.profile == "ulbw":
        phone.profile = Profiles.BANDWIDTH_ULTRA_LOW
    phone.dest.announce()
    emit({
        "event": "ready",
        "identity": RNS.hexrep(identity.hash, delimit=False),
        "destination": RNS.hexrep(phone.dest.hash, delimit=False),
    })

    if args.mode == "dial":
        if len(args.dial) != 32:
            emit({"event": "error", "error": "dial hash must be 32 hex chars"})
            return
        ident_hash = bytes.fromhex(args.dial)
        dest_hash = RNS.Destination.hash_from_name_and_identity("lxst.telephony", ident_hash)
        timeout = time.time() + 20
        if not RNS.Transport.has_path(dest_hash):
            RNS.Transport.request_path(dest_hash)
            while not RNS.Transport.has_path(dest_hash) and time.time() < timeout:
                time.sleep(0.05)
        remote = RNS.Identity.recall(dest_hash)
        if remote is None:
            emit({"event": "error", "error": "could not recall remote identity"})
            return
        emit({"event": "calling", "destination": RNS.hexrep(dest_hash, delimit=False)})
        phone.call(remote)

    while phone.should_run:
        time.sleep(0.05)


if __name__ == "__main__":
    main()
