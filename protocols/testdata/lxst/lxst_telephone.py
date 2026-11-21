# SPDX-License-Identifier: LicenseRef-Reticulum
"""Official LXST Telephone peer for Go interop tests.

Audio devices are stubbed so signalling and packetizer paths run headless.
"""

import argparse
import json
import os
import sys
import threading
import time

import numpy as np
import RNS
from LXST.Primitives import Telephony as tel
from LXST.Primitives.Telephony import Profiles, Signalling, Telephone
from LXST.Sinks import LocalSink
from LXST.Sources import LocalSource

PROFILES = {
    "ulbw": Profiles.BANDWIDTH_ULTRA_LOW,
    "vlbw": Profiles.BANDWIDTH_VERY_LOW,
    "lbw": Profiles.BANDWIDTH_LOW,
    "mq": Profiles.QUALITY_MEDIUM,
    "hq": Profiles.QUALITY_HIGH,
    "shq": Profiles.QUALITY_MAX,
    "ll": Profiles.LATENCY_LOW,
    "ull": Profiles.LATENCY_ULTRA_LOW,
}


def emit(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


class StubLineSink(LocalSink):
    def __init__(self, preferred_device=None, autodigest=True, low_latency=False):
        self.preferred_device = preferred_device
        self.should_run = False
        self.buffer_max_height = 3
        self.autostart_min = 1
        self.streaming = False
        self.channels = 1
        self.samplerate = 24000
        self.frames = 0
        self.lock = threading.Lock()

    def start(self):
        self.should_run = True

    def stop(self):
        self.should_run = False

    def wait_for_frames(self):
        return None

    def enable_low_latency(self):
        return None

    def can_receive(self, from_source=None):
        return True

    def handle_frame(self, frame, source=None):
        with self.lock:
            self.frames += 1
            n = self.frames
        if n == 1 or n % 5 == 0:
            emit({"event": "frame", "n": n})
        return None

    def flush(self):
        return None


class StubLineSource(LocalSource):
    def __init__(self, preferred_device=None, target_frame_ms=60, codec=None, sink=None,
                 filters=None, gain=0.0, ease_in=0.0, skip=0.0, profile=None):
        self.preferred_device = preferred_device
        self.target_frame_ms = target_frame_ms
        self.profile = profile
        self.codec = codec
        self.sink = sink
        self.filters = filters
        self.should_run = False
        self.channels = 1
        self.samplerate = 24000
        self._thread = None
        self.sent = 0

    def start(self):
        if self.should_run:
            return
        self.should_run = True
        self._thread = threading.Thread(target=self._loop, daemon=True)
        self._thread.start()

    def stop(self):
        self.should_run = False

    def _loop(self):
        from LXST.Primitives.Telephony import Profiles
        frame_ms = self.target_frame_ms
        if self.profile is not None:
            frame_ms = Profiles.get_frame_time(self.profile)
        rate = self.samplerate
        if self.codec is not None:
            pref = getattr(self.codec, "preferred_samplerate", None)
            if callable(pref):
                rate = int(pref())
            elif pref:
                rate = int(pref)
            elif hasattr(self.codec, "INPUT_RATE"):
                rate = int(self.codec.INPUT_RATE)
        samples = max(1, int(rate * (frame_ms / 1000.0)))
        t = np.arange(samples) / float(rate)
        while self.should_run:
            pcm = np.zeros((samples, 1), dtype=np.float32)
            pcm[:, 0] = 0.2 * np.sin(2 * np.pi * 440.0 * t)
            if self.sink is not None:
                try:
                    self.sink.handle_frame(pcm, self, decoded=True)
                    self.sent += 1
                except Exception as e:
                    emit({"event": "error", "error": str(e)})
                    break
            time.sleep(frame_ms / 1000.0)


tel.LineSink = StubLineSink
tel.LineSource = StubLineSource


def write_udp_config(path, listen_port, target_port, name):
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


def write_shared_config(path, name, shared_port, control_port):
    os.makedirs(path, exist_ok=True)
    cfg = f"""[reticulum]
enable_transport = No
share_instance = Yes
instance_name = {name}
shared_instance_type = tcp
shared_instance_port = {shared_port}
instance_control_port = {control_port}

[logging]
loglevel = 2
"""
    with open(os.path.join(path, "config"), "w") as f:
        f.write(cfg)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--configdir", required=True)
    p.add_argument("--listen-port", type=int, default=0)
    p.add_argument("--target-port", type=int, default=0)
    p.add_argument("--shared-port", type=int, default=0)
    p.add_argument("--control-port", type=int, default=0)
    p.add_argument("--mode", choices=["listen", "dial"], default="listen")
    p.add_argument("--dial", default="")
    p.add_argument("--auto-answer", type=float, default=0.3)
    p.add_argument("--name", default="lxsttel")
    p.add_argument("--profile", default="mq")
    p.add_argument("--allowed", choices=["all", "none"], default="all")
    p.add_argument("--reject", action="store_true")
    p.add_argument("--hold", type=float, default=4.0)
    args = p.parse_args()

    if args.profile not in PROFILES:
        emit({"event": "error", "error": "unknown profile " + args.profile})
        return
    profile = PROFILES[args.profile]
    allowed = Telephone.ALLOW_ALL if args.allowed == "all" else Telephone.ALLOW_NONE
    auto = None if args.reject else args.auto_answer

    if args.shared_port:
        control = args.control_port if args.control_port else args.shared_port + 1
        write_shared_config(args.configdir, args.name, args.shared_port, control)
    else:
        write_udp_config(args.configdir, args.listen_port, args.target_port, args.name)

    RNS.Reticulum(configdir=args.configdir, loglevel=2)
    identity = RNS.Identity()
    phone = Telephone(identity, auto_answer=auto, allowed=allowed)
    established = threading.Event()
    ended = threading.Event()
    busy = threading.Event()
    rejected = threading.Event()

    def on_ringing(ident):
        emit({"event": "ringing", "from": RNS.hexrep(ident.hash, delimit=False)})
        if args.reject:
            threading.Thread(target=lambda: (time.sleep(0.15), phone.hangup()), daemon=True).start()

    def on_established(ident):
        established.set()
        emit({"event": "established"})
        if args.hold > 0:
            threading.Thread(target=lambda: (time.sleep(args.hold), phone.hangup()), daemon=True).start()

    def on_ended(ident):
        frames = 0
        sink = getattr(phone, "audio_output", None)
        if sink is not None:
            frames = getattr(sink, "frames", 0)
        ended.set()
        emit({"event": "ended", "frames": frames})

    def on_busy(ident):
        busy.set()
        emit({"event": "busy"})

    def on_rejected(ident):
        rejected.set()
        emit({"event": "rejected"})

    phone.set_ringing_callback(on_ringing)
    phone.set_established_callback(on_established)
    phone.set_ended_callback(on_ended)
    phone.set_busy_callback(on_busy)
    phone.set_rejected_callback(on_rejected)
    phone.announce()
    emit({
        "event": "ready",
        "identity": RNS.hexrep(identity.hash, delimit=False),
        "destination": RNS.hexrep(phone.destination.hash, delimit=False),
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
        phone.call(remote, profile=profile, mode=Profiles.DEFAULT_MODE)

    deadline = time.time() + 40
    while time.time() < deadline:
        if ended.is_set() or busy.is_set() or rejected.is_set():
            time.sleep(0.2)
            return
        time.sleep(0.05)


if __name__ == "__main__":
    main()
