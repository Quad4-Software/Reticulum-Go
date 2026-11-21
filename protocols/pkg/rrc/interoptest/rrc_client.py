#!/usr/bin/env python3
"""CLI wrapper for the Python RRC interop client."""

from __future__ import annotations

import argparse
import json
import sys

from client import run_client_session


def main() -> int:
    p = argparse.ArgumentParser(description="Python RRC client for gorrcd/rrcd interop")
    p.add_argument("--hub-hash", required=True, help="hub destination hash (hex)")
    p.add_argument("--listen-port", type=int, required=True)
    p.add_argument("--forward-port", type=int, required=True)
    p.add_argument("--room", default="#lobby")
    p.add_argument("--nick", default="py-client")
    p.add_argument("--text", default="hello from python")
    p.add_argument("--timeout", type=float, default=40)
    p.add_argument(
        "--steps",
        default="join,msg,list,who,unrecognized,ping,action,part",
        help="comma-separated session steps",
    )
    args = p.parse_args()
    steps = [s.strip() for s in args.steps.split(",") if s.strip()]
    try:
        resp = run_client_session(
            {
                "hub_hash": args.hub_hash,
                "listen_port": args.listen_port,
                "forward_port": args.forward_port,
                "room": args.room,
                "nick": args.nick,
                "text": args.text,
                "timeout_s": args.timeout,
                "steps": steps,
            }
        )
    except Exception as exc:
        sys.stdout.write(json.dumps({"ok": False, "error": str(exc)}))
        sys.stdout.write("\n")
        return 1
    sys.stdout.write(json.dumps(resp, separators=(",", ":")))
    sys.stdout.write("\n")
    return 0 if resp.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
