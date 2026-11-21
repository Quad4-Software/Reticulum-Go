# SPDX-License-Identifier: LicenseRef-Reticulum
import argparse
import json
import os
import sys
import time

import RNS


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--configdir", required=True)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--control-port", type=int, default=0)
    args = parser.parse_args()

    control = args.control_port if args.control_port else args.port + 1
    os.makedirs(args.configdir, exist_ok=True)
    cfg_path = os.path.join(args.configdir, "config")
    with open(cfg_path, "w", encoding="utf-8") as f:
        f.write(
            "[reticulum]\n"
            "  enable_transport = Yes\n"
            "  share_instance = Yes\n"
            f"  shared_instance_port = {args.port}\n"
            f"  instance_control_port = {control}\n"
            f"  instance_name = rgesp{args.port}\n"
            "  shared_instance_type = tcp\n"
            "  panic_on_interface_error = No\n"
            "\n"
            "[logging]\n"
            "  loglevel = 3\n"
            "\n"
            "[interfaces]\n"
            "  [[Dummy]]\n"
            "    type = AutoInterface\n"
            "    enabled = No\n"
        )

    try:
        RNS.Reticulum(configdir=args.configdir, loglevel=3)
    except OSError as e:
        print(json.dumps({"event": "error", "error": str(e)}), flush=True)
        sys.exit(1)
    print(json.dumps({"event": "ready", "port": args.port}), flush=True)
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        sys.exit(0)


if __name__ == "__main__":
    main()
