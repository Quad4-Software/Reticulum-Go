#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0

import os
import sys

payload = os.environ.get("INTEROP_PIPE_PAYLOAD", "")
if payload:
    print("OK pipe probe")
    sys.exit(0)
print("FAIL missing payload")
sys.exit(1)
