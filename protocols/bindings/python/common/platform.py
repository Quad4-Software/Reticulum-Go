# SPDX-License-Identifier: LicenseRef-Reticulum

from __future__ import annotations

import os
import sys
from pathlib import Path


def lib_names(base: str) -> list[str]:
    if sys.platform == "darwin":
        return [f"lib{base}.dylib"]
    if sys.platform == "win32":
        return [f"{base}.dll", f"lib{base}.dll"]
    return [f"lib{base}.so"]


def lib_candidates(base: str, env_var: str, here: Path) -> list[Path]:
    out: list[Path] = []
    env = os.environ.get(env_var)
    if env:
        out.append(Path(env))
    root = os.environ.get("RRC_ROOT")
    if root:
        for name in lib_names(base):
            out.append(Path(root) / "bin" / name)
    for name in lib_names(base):
        out.extend(
            [
                here.parents[3] / "bin" / name,
                here.parents[2] / "bin" / name,
                Path("bin") / name,
                Path("../bin") / name,
                Path("../../bin") / name,
            ]
        )
    return out
