#!/usr/bin/env python3
"""Python git-remote-rns client for Go rngit interop tests."""

import os
import subprocess
import sys
import tempfile

_reticulum_path = os.environ.get("RETICULUM_PATH")
if _reticulum_path:
    sys.path.insert(0, os.path.abspath(_reticulum_path))


def strip_git_env() -> None:
    os.environ.pop("GIT_DIR", None)
    os.environ.pop("GIT_WORK_TREE", None)


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


def main() -> int:
    strip_git_env()
    listen_port = int(os.environ["INTEROP_LISTEN_PORT"])
    forward_port = int(os.environ["INTEROP_FORWARD_PORT"])
    identity_hex = os.environ["INTEROP_IDENTITY_HASH"]
    group = os.environ.get("INTEROP_GROUP", "public")
    repo = os.environ.get("INTEROP_REPO", "demo")

    cfg_dir = os.environ.get("INTEROP_CONFIG_DIR") or tempfile.mkdtemp(prefix="rngit_client_")
    write_config(cfg_dir, listen_port, forward_port)

    rngit_dir = os.environ.get("INTEROP_RNGIT_DIR") or tempfile.mkdtemp(prefix="rngit_client_cfg_")
    os.makedirs(rngit_dir, exist_ok=True)
    client_cfg = os.path.join(rngit_dir, "client_config")
    if not os.path.isfile(client_cfg):
        with open(client_cfg, "w", encoding="utf-8") as fh:
            fh.write("[logging]\nloglevel = 2\n")

    url = f"rns://{identity_hex}/{group}/{repo}"
    env = os.environ.copy()
    env["RNGIT_CONFIG"] = rngit_dir
    env["RNS_CONFIG"] = cfg_dir

    git_remote = os.environ.get("GIT_REMOTE_RNS", "git-remote-rns")
    fetch_mode = os.environ.get("INTEROP_FETCH", "") == "1"
    push_mode = os.environ.get("INTEROP_PUSH", "") == "1"

    lines = ["capabilities", "list", ""]

    proc = subprocess.run(
        [git_remote, "origin", url],
        input="\n".join(lines) + "\n",
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        env=env,
        check=False,
    )
    sys.stdout.write(proc.stdout)
    sys.stderr.write(proc.stderr)
    if proc.returncode != 0:
        return proc.returncode
    if "refs/heads/" not in proc.stdout:
        print("list output missing refs", file=sys.stderr)
        return 1

    if push_mode:
        work = tempfile.mkdtemp(prefix="rngit_push_work_")
        penv = env.copy()
        penv.update(
            {
                "GIT_AUTHOR_NAME": "interop",
                "GIT_AUTHOR_EMAIL": "interop@test",
                "GIT_COMMITTER_NAME": "interop",
                "GIT_COMMITTER_EMAIL": "interop@test",
            }
        )
        subprocess.run(["git", "init", "-b", "main", work], check=True, capture_output=True, env=penv)
        with open(os.path.join(work, "pushed.txt"), "w", encoding="utf-8") as fh:
            fh.write("python push interop\n")
        subprocess.run(["git", "add", "pushed.txt"], check=True, cwd=work, capture_output=True, env=penv)
        subprocess.run(
            ["git", "-c", "commit.gpgsign=false", "commit", "-m", "push interop"],
            check=True, cwd=work, capture_output=True, env=penv,
        )
        push_env = env.copy()
        push_env["GIT_DIR"] = os.path.join(work, ".git")
        push_env["GIT_WORK_TREE"] = work
        push_lines = ["capabilities", "list for-push", "push refs/heads/main:refs/heads/interop-py", ""]
        proc3 = subprocess.run(
            [git_remote, "origin", url],
            input="\n".join(push_lines) + "\n",
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            env=push_env,
            cwd=work,
            check=False,
        )
        sys.stdout.write(proc3.stdout)
        sys.stderr.write(proc3.stderr)
        if proc3.returncode != 0:
            return proc3.returncode
        if "ok refs/heads/interop-py" not in proc3.stdout:
            print("push did not return ok: " + proc3.stdout, file=sys.stderr)
            return 1
        sys.stdout.write("PUSH_OK\n")
        return 0

    if not fetch_mode:
        return 0

    sha = ""
    ref = ""
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        if line.startswith("@") and line.endswith(" HEAD"):
            ref = line[1:].rsplit(" ", 1)[0]
            continue
        if line.startswith("@"):
            continue
        parts = line.split(" ", 1)
        if len(parts) != 2 or parts[1] == "HEAD":
            continue
        if parts[1].startswith("refs/heads/"):
            sha, ref = parts[0], parts[1]
            break
    if ref and not sha:
        sha = "0" * 40
    if not ref:
        print("no branch ref for fetch", file=sys.stderr)
        return 1

    work = tempfile.mkdtemp(prefix="rngit_fetch_work_")
    subprocess.run(["git", "init", work], check=True, capture_output=True)
    fetch_lines = ["capabilities", "list", "", f"fetch {sha} {ref}", ""]
    fetch_env = env.copy()
    fetch_env["GIT_DIR"] = os.path.join(work, ".git")
    fetch_env["GIT_WORK_TREE"] = work
    proc2 = subprocess.run(
        [git_remote, "origin", url],
        input="\n".join(fetch_lines) + "\n",
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        env=fetch_env,
        cwd=work,
        check=False,
    )
    sys.stdout.write(proc2.stdout)
    sys.stderr.write(proc2.stderr)
    if proc2.returncode != 0:
        return proc2.returncode
    # Remote helpers import objects but do not update refs; the parent git
    # process writes refs during a real fetch. Verify the tip object directly.
    log = subprocess.run(
        ["git", "log", "-1", "--oneline", sha],
        cwd=work,
        capture_output=True,
        text=True,
        check=False,
    )
    if log.returncode != 0 or "init" not in log.stdout:
        print("fetch did not import commit", file=sys.stderr)
        return 1
    sys.stdout.write("FETCH_OK\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
