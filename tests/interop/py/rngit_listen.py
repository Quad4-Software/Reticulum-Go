#!/usr/bin/env python3
"""Python rngit server for Go interop tests."""

import os
import shutil
import subprocess
import sys
import tempfile
import time

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))

import RNS
from RNS.Utilities.rngit.server import ReticulumGitNode


def write_config(cfg_dir: str, listen_port: int, forward_port: int) -> None:
    with open(os.path.join(cfg_dir, "config"), "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[reticulum]",
                    "enable_transport = yes",
                    "share_instance = no",
                    "loglevel = 2",
                    "",
                    "[interfaces]",
                    "",
                    "[[interop_udp]]",
                    "type = UDPInterface",
                    "enabled = yes",
                    "listen_ip = 127.0.0.1",
                    f"listen_port = {listen_port}",
                    "forward_ip = 127.0.0.1",
                    f"forward_port = {forward_port}",
                    "",
                ],
            ),
        )


def write_rngit_config(rngit_dir: str, repo_root: str) -> None:
    os.makedirs(rngit_dir, exist_ok=True)
    with open(os.path.join(rngit_dir, "config"), "w", encoding="utf-8") as f:
        f.write(
            "\n".join(
                [
                    "[repositories]",
                    f"public = {repo_root}",
                    "",
                    "[access]",
                    "public = rw:all",
                    "",
                ],
            ),
        )


def strip_git_env() -> None:
    os.environ.pop("GIT_DIR", None)
    os.environ.pop("GIT_WORK_TREE", None)


def is_bare_git_repo(path: str) -> bool:
    try:
        subprocess.run(
            ["git", "rev-parse", "--git-dir"],
            cwd=path,
            check=True,
            capture_output=True,
            text=True,
            env=git_env(),
        )
        out = subprocess.run(
            ["git", "config", "--bool", "core.bare"],
            cwd=path,
            check=True,
            capture_output=True,
            text=True,
            env=git_env(),
        )
        return out.stdout.strip() == "true"
    except (subprocess.CalledProcessError, OSError):
        return False


def git_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    env = {k: v for k, v in os.environ.items() if k not in ("GIT_DIR", "GIT_WORK_TREE")}
    if extra:
        env.update(extra)
    return env


def init_bare_repo_with_commit(bare_path: str) -> None:
    subprocess.run(["git", "init", "--bare", bare_path], check=True, capture_output=True, env=git_env())
    work = tempfile.mkdtemp(prefix="rngit_work_")
    env = git_env(
        {
            "GIT_AUTHOR_NAME": "interop",
            "GIT_AUTHOR_EMAIL": "interop@test",
            "GIT_COMMITTER_NAME": "interop",
            "GIT_COMMITTER_EMAIL": "interop@test",
        },
    )
    subprocess.run(["git", "clone", bare_path, work], check=True, capture_output=True, env=env)
    readme = os.path.join(work, "README")
    with open(readme, "w", encoding="utf-8") as fh:
        fh.write("rngit interop\n")
    subprocess.run(["git", "add", "README"], check=True, cwd=work, capture_output=True, env=env)
    subprocess.run(
        ["git", "commit", "-m", "init"],
        check=True,
        cwd=work,
        capture_output=True,
        env=env,
    )
    subprocess.run(["git", "push", "origin", "HEAD"], check=True, cwd=work, capture_output=True, env=env)


def main() -> int:
    strip_git_env()
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rngit_listen_")
    write_config(cfg_dir, listen_port, forward_port)

    rngit_dir = os.environ.get("INTEROP_RNGIT_DIR") or tempfile.mkdtemp(prefix="rngit_cfg_")
    repo_root = os.environ.get("INTEROP_REPO_ROOT") or os.path.join(rngit_dir, "repos", "public")
    os.makedirs(repo_root, exist_ok=True)
    write_rngit_config(rngit_dir, repo_root)
    demo = os.path.join(repo_root, "demo")
    if os.path.isdir(demo) and not is_bare_git_repo(demo):
        shutil.rmtree(demo)
    if not os.path.isdir(demo):
        init_bare_repo_with_commit(demo)

    _ = RNS.Reticulum(cfg_dir)
    git_node = ReticulumGitNode(configdir=rngit_dir, verbosity=0)
    if not git_node.ready:
        print("rngit node failed to initialize", file=sys.stderr)
        return 1
    git_node.start()
    git_node.announce()
    time.sleep(0.5)
    if os.environ.get("INTEROP_DEBUG") == "1":
        repos = git_node.groups.get("public", {}).get("repositories", {})
        print("INTEROP_REPOS " + ",".join(sorted(repos.keys())), file=sys.stderr)
        print("INTEROP_GROUP_READ " + repr(git_node.groups.get("public", {}).get("read", [])), file=sys.stderr)
    dest_hex = RNS.hexrep(git_node.destination.hash, delimit=False)
    sys.stdout.write("READY " + dest_hex + "\n")
    sys.stdout.flush()
    while True:
        time.sleep(1)


if __name__ == "__main__":
    raise SystemExit(main())
