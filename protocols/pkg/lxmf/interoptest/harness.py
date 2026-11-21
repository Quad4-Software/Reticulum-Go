#!/usr/bin/env python3
"""JSON stdin/stdout harness for Go <-> LXMF-ref interop tests."""

from __future__ import annotations

import base64
import json
import shutil
import sys
import tempfile
import threading
import time
import traceback
from pathlib import Path
from typing import Any

import RNS
import RNS.vendor.umsgpack as msgpack
from LXMF import LXMessage
import LXMF.LXStamper as LXStamper
import LXMF.LXMF as LXMF
import LXMF as LXMF_pkg

RNS.loglevel = RNS.LOG_NONE


def _hex(b: bytes | None) -> str:
    if b is None:
        return ""
    return b.hex()


def _unhex(s: str) -> bytes:
    return bytes.fromhex(s)


def _field_key(k: Any) -> int:
    if isinstance(k, int):
        return k
    if isinstance(k, str):
        if k.startswith("0x") or k.startswith("0X"):
            return int(k, 16)
        return int(k)
    raise ValueError(f"invalid field key: {k!r}")


def _normalize_fields(raw: dict[str, Any] | None) -> dict[int, Any] | None:
    if raw is None:
        return None
    out: dict[int, Any] = {}
    for k, v in raw.items():
        key = _field_key(k)
        if isinstance(v, dict):
            inner: dict[int, Any] = {}
            for ik, iv in v.items():
                inner[_field_key(ik)] = _bytesify(iv)
            out[key] = inner
        else:
            out[key] = _bytesify(v)
    return out


def _bytesify(v: Any) -> Any:
    if isinstance(v, list):
        return [_bytesify(x) for x in v]
    if isinstance(v, str) and v.startswith("hex:"):
        return _unhex(v[4:])
    return v


def _jsonify_value(v: Any) -> Any:
    if isinstance(v, (bytes, bytearray)):
        return "hex:" + bytes(v).hex()
    if isinstance(v, dict):
        out: dict[str, Any] = {}
        for ik, iv in v.items():
            if isinstance(ik, int):
                ikey = f"0x{ik:02x}"
            else:
                ikey = str(ik)
            out[ikey] = _jsonify_value(iv)
        return out
    if isinstance(v, list):
        return [_jsonify_value(x) for x in v]
    return v


def _fields_out(fields: dict[int, Any] | None) -> dict[str, Any]:
    if not fields:
        return {}
    out: dict[str, Any] = {}
    for k, v in fields.items():
        out[f"0x{k:02x}"] = _jsonify_value(v)
    return out


def _register_identity(public_key: bytes, dest_hash: bytes | None = None) -> RNS.Identity:
    ident = RNS.Identity(create_keys=False)
    ident.load_public_key(public_key)
    remember_hash = dest_hash if dest_hash is not None else ident.hash
    RNS.Identity.remember(None, remember_hash, public_key)
    return ident


def _as_text(val: Any) -> str:
    if val is None:
        return ""
    if isinstance(val, str):
        return val
    if isinstance(val, (bytes, bytearray)):
        try:
            return bytes(val).decode("utf-8")
        except UnicodeDecodeError:
            return "hex:" + bytes(val).hex()
    return str(val)


def _message_out(msg: LXMessage) -> dict[str, Any]:
    return {
        "hash": _hex(msg.hash),
        "title": _as_text(msg.title),
        "content": _as_text(msg.content),
        "fields": _fields_out(msg.fields),
        "timestamp": msg.timestamp,
        "signature_validated": bool(msg.signature_validated),
        "unverified_reason": msg.unverified_reason,
        "stamp": _hex(msg.stamp) if msg.stamp else "",
        "destination_hash": _hex(msg.destination_hash),
        "source_hash": _hex(msg.source_hash),
    }


def cmd_ping(_req: dict[str, Any]) -> dict[str, Any]:
    return {"ok": True, "lxmf_version": LXMF_pkg.__version__}


def cmd_pack(req: dict[str, Any]) -> dict[str, Any]:
    title = req.get("title", "")
    content = req.get("content", "")
    fields = _normalize_fields(req.get("fields"))
    timestamp = req.get("timestamp")
    stamp_hex = req.get("stamp", "")

    src_id = RNS.Identity()
    dst_id = RNS.Identity()
    src = RNS.Destination(
        src_id, RNS.Destination.OUT, RNS.Destination.SINGLE, LXMF.APP_NAME, "delivery"
    )
    dst = RNS.Destination(
        dst_id, RNS.Destination.OUT, RNS.Destination.SINGLE, LXMF.APP_NAME, "delivery"
    )

    msg = LXMessage(
        dst, src, content, title, fields=fields, desired_method=LXMessage.DIRECT
    )
    msg.defer_stamp = True
    if timestamp is not None:
        msg.timestamp = float(timestamp)
    if stamp_hex:
        msg.stamp = _unhex(stamp_hex)
        msg.defer_stamp = False
    msg.pack()

    return {
        "ok": True,
        "packed": _hex(msg.packed),
        "hash": _hex(msg.hash),
        "source_public_key": _hex(src_id.get_public_key()),
        "destination_public_key": _hex(dst_id.get_public_key()),
        "source_hash": _hex(src.hash),
        "destination_hash": _hex(dst.hash),
        "message": _message_out(msg),
    }


def cmd_unpack(req: dict[str, Any]) -> dict[str, Any]:
    packed = _unhex(req["packed"])
    for entry in req.get("identities", []):
        _register_identity(_unhex(entry["public_key"]), _unhex(entry["hash"]))
    keys = req.get("public_keys", {})
    for _label, pk_hex in keys.items():
        _register_identity(_unhex(pk_hex))

    msg = LXMessage.unpack_from_bytes(packed)
    if msg is None:
        raise ValueError("unpack_from_bytes returned None")

    return {"ok": True, "message": _message_out(msg)}


def cmd_stamp_workblock(req: dict[str, Any]) -> dict[str, Any]:
    material = _unhex(req["material"])
    rounds = int(req.get("expand_rounds", LXStamper.WORKBLOCK_EXPAND_ROUNDS))
    wb = LXStamper.stamp_workblock(material, expand_rounds=rounds)
    return {"ok": True, "workblock": _hex(wb)}


def cmd_stamp_valid(req: dict[str, Any]) -> dict[str, Any]:
    stamp = _unhex(req["stamp"])
    workblock = _unhex(req["workblock"])
    target = int(req["target_cost"])
    valid = LXStamper.stamp_valid(stamp, target, workblock)
    return {"ok": True, "valid": valid}


def cmd_stamp_value(req: dict[str, Any]) -> dict[str, Any]:
    stamp = _unhex(req["stamp"])
    workblock = _unhex(req["workblock"])
    value = LXStamper.stamp_value(workblock, stamp)
    return {"ok": True, "value": value}


def cmd_generate_stamp(req: dict[str, Any]) -> dict[str, Any]:
    message_id = _unhex(req["message_id"])
    cost = int(req["stamp_cost"])
    rounds = int(req.get("expand_rounds", LXStamper.WORKBLOCK_EXPAND_ROUNDS))
    stamp, value = LXStamper.generate_stamp(message_id, cost, expand_rounds=rounds)
    if stamp is None:
        raise ValueError("stamp generation failed")
    return {"ok": True, "stamp": _hex(stamp), "value": value}


def cmd_validate_pn_stamp(req: dict[str, Any]) -> dict[str, Any]:
    data = _unhex(req["transient_data"])
    target = int(req["target_cost"])
    tid, lxm, value, stamp = LXStamper.validate_pn_stamp(data, target)
    return {
        "ok": True,
        "transient_id": _hex(tid) if tid else "",
        "lxm_data": _hex(lxm) if lxm else "",
        "value": value if value is not None else 0,
        "stamp": _hex(stamp) if stamp else "",
        "valid": tid is not None,
    }


def cmd_packed_container(req: dict[str, Any]) -> dict[str, Any]:
    packed = _unhex(req["packed"])
    msg = LXMessage.unpack_from_bytes(packed)
    if msg is None:
        raise ValueError("unpack failed")
    if "state" in req:
        msg.state = int(req["state"])
    if "method" in req:
        msg.method = int(req["method"])
    msg.determine_transport_encryption()
    container = msg.packed_container()
    return {"ok": True, "container": _hex(container)}


def cmd_container_unpack(req: dict[str, Any]) -> dict[str, Any]:
    for entry in req.get("identities", []):
        _register_identity(_unhex(entry["public_key"]), _unhex(entry["hash"]))
    data = _unhex(req["container"])
    container = msgpack.unpackb(data)
    lxm_bytes = container["lxmf_bytes"]
    msg = LXMessage.unpack_from_bytes(lxm_bytes)
    if msg is None:
        raise ValueError("unpack failed")
    if "state" in container:
        msg.state = container["state"]
    if "method" in container:
        msg.method = container["method"]
    return {
        "ok": True,
        "state": container.get("state"),
        "method": container.get("method"),
        "transport_encrypted": container.get("transport_encrypted"),
        "transport_encryption": container.get("transport_encryption"),
        "message": _message_out(msg),
    }


def cmd_announce_encode(req: dict[str, Any]) -> dict[str, Any]:
    name = req.get("display_name", "")
    cost = req.get("stamp_cost")
    features = req.get("features")
    if features is not None:
        features = [_field_key(f) for f in features]
    if cost is None and features is None:
        app_data = name.encode("utf-8")
    else:
        payload: list[Any] = [name.encode("utf-8")]
        if cost is not None:
            payload.append(int(cost))
        if features is not None:
            payload.append(features)
        app_data = msgpack.packb(payload)
    return {"ok": True, "app_data": _hex(app_data)}


def cmd_announce_decode(req: dict[str, Any]) -> dict[str, Any]:
    app_data = _unhex(req["app_data"])
    return {
        "ok": True,
        "display_name": LXMF.display_name_from_app_data(app_data),
        "stamp_cost": LXMF.stamp_cost_from_app_data(app_data),
        "compression_support": LXMF.compression_support_from_app_data(app_data),
    }


def cmd_paper_uri(req: dict[str, Any]) -> dict[str, Any]:
    payload = _unhex(req["paper_packed"])
    encoded = base64.urlsafe_b64encode(payload).decode("utf-8").replace("=", "")
    uri = LXMessage.URI_SCHEMA + "://" + encoded
    return {"ok": True, "uri": uri}


def cmd_paper_decode(req: dict[str, Any]) -> dict[str, Any]:
    uri = req["uri"]
    prefix = LXMessage.URI_SCHEMA + "://"
    if not uri.startswith(prefix):
        raise ValueError("missing lxm:// prefix")
    encoded = uri[len(prefix) :]
    pad = (-len(encoded)) % 4
    decoded = base64.urlsafe_b64decode(encoded + ("=" * pad))
    return {"ok": True, "paper_packed": _hex(decoded)}


def cmd_ticket_stamp(req: dict[str, Any]) -> dict[str, Any]:
    ticket = _unhex(req["ticket"])
    message_id = _unhex(req["message_id"])
    stamp = RNS.Identity.truncated_hash(ticket + message_id)
    return {"ok": True, "stamp": _hex(stamp)}


def _write_rns_config(configdir: Path, listen_port: int, forward_port: int, loglevel: int = 0) -> None:
    configdir.mkdir(parents=True, exist_ok=True)
    (configdir / "storage").mkdir(exist_ok=True)
    (configdir / "storage" / "identities").mkdir(exist_ok=True)
    cfg = f"""[reticulum]
  enable_transport = Yes
  share_instance = No
  shared_instance_port = 0
  instance_name = lxmf-interop-{listen_port}

[logging]
  loglevel = {loglevel}

[interfaces]

  [[UDP Interface]]
    type = UDPInterface
    interface_enabled = True
    listen_ip = 127.0.0.1
    listen_port = {listen_port}
    forward_ip = 127.0.0.1
    forward_port = {forward_port}
"""
    (configdir / "config").write_text(cfg, encoding="utf-8")


def cmd_live_recv(req: dict[str, Any]) -> dict[str, Any]:
    """Inbound lxmf.delivery destination that waits for one packed message from Go."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    ready_path = req["ready_path"]
    timeout_s = float(req.get("timeout_s", 40))

    configdir = Path(tempfile.mkdtemp(prefix="lxmf-interop-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        identity = RNS.Identity()
        dest = RNS.Destination(
            identity,
            RNS.Destination.IN,
            RNS.Destination.SINGLE,
            LXMF.APP_NAME,
            "delivery",
        )

        got: dict[str, str] = {}
        ev = threading.Event()
        last_err = ""

        def on_packet(data: bytes, packet: Any) -> None:
            nonlocal last_err
            dest_hash = getattr(getattr(packet, "destination", None), "hash", None) or dest.hash
            blobs = []
            if dest_hash:
                blobs.append(dest_hash + data)
            blobs.append(data)
            seen: set[bytes] = set()
            for blob in blobs:
                if blob in seen:
                    continue
                seen.add(blob)
                try:
                    msg = LXMessage.unpack_from_bytes(blob)
                except Exception as e:
                    last_err = str(e)
                    continue
                if msg is None:
                    last_err = "unpack_from_bytes returned None"
                    continue
                got["text"] = _as_text(msg.content)
                ev.set()
                return

        dest.set_packet_callback(on_packet)
        Path(ready_path).write_text(
            json.dumps(
                {
                    "dest_hash": _hex(dest.hash),
                    "public_key": _hex(identity.get_public_key()),
                }
            ),
            encoding="utf-8",
        )

        deadline = time.time() + timeout_s
        while time.time() < deadline:
            dest.announce()
            if ev.wait(timeout=0.4):
                break

        if not ev.is_set():
            err = "timeout waiting for LXMF packet"
            if last_err:
                err += f": last unpack error: {last_err}"
            return {"ok": False, "error": err}

        return {
            "ok": True,
            "dest_hash": _hex(dest.hash),
            "text": got.get("text", ""),
        }
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def cmd_live_send_delivery(req: dict[str, Any]) -> dict[str, Any]:
    """Send one opportunistic LXMF message to an existing delivery destination hash."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    dest_hex = req["dest_hash"]
    title = req.get("title", "")
    text = req.get("text", "")
    timeout_s = float(req.get("timeout_s", 40))

    configdir = Path(tempfile.mkdtemp(prefix="lxmf-send-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        storage = configdir / "storage"
        storage.mkdir(parents=True, exist_ok=True)
        router = LXMF_pkg.LXMRouter(identity=RNS.Identity(), storagepath=str(storage))
        src = router.register_delivery_identity(router.identity, display_name="interop-sender")
        dest_hash = bytes.fromhex(dest_hex)
        if not RNS.Transport.has_path(dest_hash):
            RNS.Transport.request_path(dest_hash)
        deadline = time.time() + timeout_s
        while not RNS.Transport.has_path(dest_hash) and time.time() < deadline:
            src.announce()
            time.sleep(0.2)
        if not RNS.Transport.has_path(dest_hash):
            return {"ok": False, "error": "no path to destination"}

        recipient = RNS.Identity.recall(dest_hash)
        if recipient is None:
            return {"ok": False, "error": "destination identity not known"}

        out_dest = RNS.Destination(
            recipient,
            RNS.Destination.OUT,
            RNS.Destination.SINGLE,
            LXMF.APP_NAME,
            "delivery",
        )
        lxm = LXMessage(
            out_dest,
            src,
            text,
            title,
            desired_method=LXMessage.OPPORTUNISTIC,
        )
        router.handle_outbound(lxm)
        deadline = time.time() + min(timeout_s, 30.0)
        while lxm.state not in (LXMessage.SENT, LXMessage.DELIVERED) and time.time() < deadline:
            router.process_outbound()
            time.sleep(0.15)
        if lxm.state not in (LXMessage.SENT, LXMessage.DELIVERED):
            return {"ok": False, "error": f"message not sent state={lxm.state}"}
        return {"ok": True, "hash": _hex(lxm.hash)}
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def cmd_live_send_propagation(req: dict[str, Any]) -> dict[str, Any]:
    """Upload one propagated LXMF message to a propagation node destination hash."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    pn_hex = req["propagation_hash"]
    recipient_hex = req.get("recipient_hash", "")
    title = req.get("title", "")
    text = req.get("text", "")
    timeout_s = float(req.get("timeout_s", 120))

    configdir = Path(tempfile.mkdtemp(prefix="lxmf-prop-send-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        storage = configdir / "storage"
        storage.mkdir(parents=True, exist_ok=True)
        router = LXMF_pkg.LXMRouter(identity=RNS.Identity(), storagepath=str(storage))
        src = router.register_delivery_identity(router.identity, display_name="interop-prop-sender")
        pn_hash = bytes.fromhex(pn_hex)
        if not RNS.Transport.has_path(pn_hash):
            RNS.Transport.request_path(pn_hash)
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            src.announce()
            pn_app_data = RNS.Identity.recall_app_data(pn_hash)
            if RNS.Transport.has_path(pn_hash) and LXMF.pn_announce_data_is_valid(pn_app_data):
                break
            time.sleep(0.2)
        if not RNS.Transport.has_path(pn_hash):
            return {"ok": False, "error": "no path to propagation node"}
        if not LXMF.pn_announce_data_is_valid(RNS.Identity.recall_app_data(pn_hash)):
            return {"ok": False, "error": "propagation node announce data unavailable"}

        pn_identity = RNS.Identity.recall(pn_hash)
        if pn_identity is None:
            return {"ok": False, "error": "propagation node identity not known"}

        router.set_outbound_propagation_node(pn_hash)

        if recipient_hex:
            recipient_hash = bytes.fromhex(recipient_hex)
            recipient_identity = RNS.Identity.recall(recipient_hash)
            if recipient_identity is None:
                recipient_identity = RNS.Identity()
                RNS.Identity.remember(recipient_hash, recipient_identity.get_public_key())
            recipient = RNS.Destination(
                recipient_identity,
                RNS.Destination.OUT,
                RNS.Destination.SINGLE,
                LXMF.APP_NAME,
                "delivery",
            )
        else:
            recipient_id = RNS.Identity()
            recipient = RNS.Destination(
                recipient_id,
                RNS.Destination.OUT,
                RNS.Destination.SINGLE,
                LXMF.APP_NAME,
                "delivery",
            )

        lxm = LXMessage(
            recipient,
            src,
            text,
            title,
            desired_method=LXMessage.PROPAGATED,
        )
        router.handle_outbound(lxm)

        deadline = time.time() + timeout_s
        while lxm.state != LXMessage.SENT and time.time() < deadline:
            router.process_outbound()
            if lxm.progress is not None and lxm.progress >= 1.0:
                break
            time.sleep(0.15)
        if lxm.state != LXMessage.SENT and (lxm.progress is None or lxm.progress < 1.0):
            return {
                "ok": False,
                "error": f"propagation upload incomplete state={lxm.state} progress={lxm.progress}",
            }
        return {"ok": True, "hash": _hex(lxm.hash), "progress": lxm.progress}
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def cmd_live_prop_node(req: dict[str, Any]) -> dict[str, Any]:
    """Run a Python LXMRouter propagation node until stop_path appears.

    Optionally injects one stored message addressed to the given recipient
    delivery hash so a remote client can retrieve it through /get.
    """
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    ready_path = req["ready_path"]
    stop_path = req["stop_path"]
    recipient_hash_hex = req.get("recipient_hash", "")
    recipient_pubkey_hex = req.get("recipient_public_key", "")
    title = req.get("title", "pn-stored")
    text = req.get("text", "stored by python node")
    duration_s = float(req.get("duration_s", 90))

    configdir = Path(tempfile.mkdtemp(prefix="lxmf-pn-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port, int(req.get("loglevel", 0)))
        _reticulum = RNS.Reticulum(str(configdir))
        RNS.loglevel = int(req.get("loglevel", 0))
        storage = configdir / "storage"
        storage.mkdir(parents=True, exist_ok=True)
        router = LXMF_pkg.LXMRouter(identity=RNS.Identity(), storagepath=str(storage))
        router.enable_propagation()

        if recipient_hash_hex and recipient_pubkey_hex:
            rid = RNS.Identity(create_keys=False)
            rid.load_public_key(_unhex(recipient_pubkey_hex))
            recipient = RNS.Destination(
                rid,
                RNS.Destination.OUT,
                RNS.Destination.SINGLE,
                LXMF.APP_NAME,
                "delivery",
            )
            if recipient.hash != _unhex(recipient_hash_hex):
                return {"ok": False, "error": "recipient hash mismatch with public key"}
            src = router.register_delivery_identity(router.identity, display_name="pn-injector")
            lxm = LXMessage(recipient, src, text, title)
            lxm.pack()
            encrypted = recipient.encrypt(lxm.packed[LXMessage.DESTINATION_LENGTH:])
            lxmf_data = lxm.packed[:LXMessage.DESTINATION_LENGTH] + encrypted
            ok = router.lxmf_propagation(
                lxmf_data,
                stamp_value=16,
                stamp_data=b"\x00" * LXStamper.STAMP_SIZE,
            )
            if ok is False:
                return {"ok": False, "error": "lxmf_propagation rejected injected message"}

        Path(ready_path).write_text(
            json.dumps({"propagation_hash": _hex(router.propagation_destination.hash)}),
            encoding="utf-8",
        )

        initial_entries = len(router.propagation_entries)
        purged = False
        deadline = time.time() + duration_s
        while time.time() < deadline and not Path(stop_path).exists():
            router.announce_propagation_node()
            if initial_entries > 0 and len(router.propagation_entries) == 0:
                purged = True
            time.sleep(0.3)

        return {
            "ok": True,
            "propagation_hash": _hex(router.propagation_destination.hash),
            "purged": purged,
            "served": router.client_propagation_messages_served,
            "entries": len(router.propagation_entries),
        }
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


def cmd_live_prop_fetch(req: dict[str, Any]) -> dict[str, Any]:
    """Run a Python LXMF client that downloads messages from a propagation node."""
    listen_port = int(req["listen_port"])
    forward_port = int(req["forward_port"])
    ready_path = req["ready_path"]
    pn_hex = req["propagation_hash"]
    timeout_s = float(req.get("timeout_s", 90))

    configdir = Path(tempfile.mkdtemp(prefix="lxmf-fetch-"))
    try:
        _write_rns_config(configdir, listen_port, forward_port)
        _reticulum = RNS.Reticulum(str(configdir))
        storage = configdir / "storage"
        storage.mkdir(parents=True, exist_ok=True)
        router = LXMF_pkg.LXMRouter(identity=RNS.Identity(), storagepath=str(storage))

        delivered: list[str] = []
        router.register_delivery_callback(lambda msg: delivered.append(_as_text(msg.content)))
        src = router.register_delivery_identity(router.identity, display_name="fetch-client")

        Path(ready_path).write_text(
            json.dumps(
                {
                    "delivery_hash": _hex(src.hash),
                    "public_key": _hex(router.identity.get_public_key()),
                }
            ),
            encoding="utf-8",
        )

        pn_hash = _unhex(pn_hex)
        deadline = time.time() + timeout_s
        while not RNS.Transport.has_path(pn_hash) and time.time() < deadline:
            src.announce()
            RNS.Transport.request_path(pn_hash)
            time.sleep(0.3)
        if not RNS.Transport.has_path(pn_hash):
            return {"ok": False, "error": "no path to propagation node"}

        router.set_outbound_propagation_node(pn_hash)
        router.request_messages_from_propagation_node(router.identity)

        deadline = time.time() + timeout_s
        while router.propagation_transfer_state != LXMF_pkg.LXMRouter.PR_COMPLETE and time.time() < deadline:
            time.sleep(0.2)

        return {
            "ok": True,
            "state": router.propagation_transfer_state,
            "received": len(delivered),
            "texts": delivered,
            "last_result": router.propagation_transfer_last_result,
        }
    finally:
        try:
            shutil.rmtree(configdir, ignore_errors=True)
        except Exception:
            pass


HANDLERS = {
    "ping": cmd_ping,
    "pack": cmd_pack,
    "unpack": cmd_unpack,
    "stamp_workblock": cmd_stamp_workblock,
    "stamp_valid": cmd_stamp_valid,
    "stamp_value": cmd_stamp_value,
    "generate_stamp": cmd_generate_stamp,
    "validate_pn_stamp": cmd_validate_pn_stamp,
    "packed_container": cmd_packed_container,
    "container_unpack": cmd_container_unpack,
    "announce_encode": cmd_announce_encode,
    "announce_decode": cmd_announce_decode,
    "paper_uri": cmd_paper_uri,
    "paper_decode": cmd_paper_decode,
    "ticket_stamp": cmd_ticket_stamp,
    "live_recv": cmd_live_recv,
    "live_send_delivery": cmd_live_send_delivery,
    "live_send_propagation": cmd_live_send_propagation,
    "live_prop_node": cmd_live_prop_node,
    "live_prop_fetch": cmd_live_prop_fetch,
}


def main() -> int:
    try:
        raw = sys.stdin.read()
        req = json.loads(raw)
        cmd = req.get("cmd")
        if cmd not in HANDLERS:
            out = {"ok": False, "error": f"unknown command: {cmd}"}
        else:
            out = HANDLERS[cmd](req)
    except Exception as exc:
        out = {
            "ok": False,
            "error": str(exc),
            "trace": traceback.format_exc(),
        }
    sys.stdout.write(json.dumps(out))
    sys.stdout.flush()
    return 0 if out.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
