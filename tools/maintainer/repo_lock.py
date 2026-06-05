#!/usr/bin/env python3
"""Repo-level advisory lock for operations that write shared files.

release.py (and any future batch tool) writes two files every invocation
shares: tools/maintainer/skills.json and .claude-plugin/marketplace.json.
Two concurrent releases - even of DIFFERENT skills - race on those files
(read-modify-write, last-writer-wins). This module serializes them with a
single named lock, machine-wide.

Why the lock lives in the git COMMON dir, not the working tree: release
operations run from throwaway worktrees (see release_batch.sh), and git
worktrees have separate working trees but share one .git. A lock file under
tools/maintainer/.locks/ would exist per-worktree and never collide - two
releases in two worktrees would each see "their" lock free. Anchoring at
`git rev-parse --git-common-dir` gives every checkout of this repo the same
lock file. (The onboarding locks in msp-skills-publish's onboard.py keep the
working-tree location because they are acquired in the primary checkout
BEFORE the worktree exists; this module is a deliberate re-implementation of
that pattern adapted for run-inside-a-worktree semantics. If the two ever
converge, this repo-side copy is the canonical one.)

Semantics (mirrors onboard.py's per-slug locks):
  - atomic create via os.O_CREAT | os.O_EXCL
  - JSON payload {pid, host, started, argv, ...} so a stuck lock is
    self-documenting
  - stale-lock reclaim: if the holder PID is dead on THIS host, the lock is
    reclaimed; a lock from another host is never auto-reclaimed (a foreign
    PID is meaningless locally) - the operator removes it manually
  - released via atexit on normal or exceptional interpreter exit

Pure stdlib. Not safe across NFS (O_EXCL caveats); this repo is a local
checkout, which is the supported case.
"""

from __future__ import annotations

import atexit
import json
import os
import socket
import subprocess
import sys
from datetime import datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)


class LockHeld(RuntimeError):
    """Another process holds the lock. Message names the holder."""


def _locks_dir() -> Path:
    """Locks directory shared by every worktree of this repo.

    `git rev-parse --git-common-dir` returns the shared .git directory even
    when run inside a linked worktree. Fall back to the working-tree
    .locks/ dir (onboard.py's location) only if git itself is unavailable -
    degraded, but better than no lock at all.
    """
    try:
        p = subprocess.run(
            ["git", "-C", str(registry.ROOT), "rev-parse", "--git-common-dir"],
            capture_output=True, text=True, timeout=10,
        )
        if p.returncode == 0 and p.stdout.strip():
            common = Path(p.stdout.strip())
            if not common.is_absolute():
                common = (registry.ROOT / common).resolve()
            d = common / "msp-skills-locks"
            d.mkdir(parents=True, exist_ok=True)
            return d
    except (OSError, subprocess.SubprocessError):
        pass
    d = registry.ROOT / "tools" / "maintainer" / ".locks"
    d.mkdir(parents=True, exist_ok=True)
    gi = d / ".gitignore"
    if not gi.exists():
        gi.write_text("*\n", encoding="utf-8")
    return d


def release(lock: Path) -> None:
    try:
        lock.unlink(missing_ok=True)
    except OSError:
        pass


def _holder(lock: Path) -> dict:
    try:
        return json.loads(lock.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return {}


def _holder_alive(holder: dict) -> bool:
    """True when the lock's holder may still be running.

    A lock from another host is always treated as alive: we cannot probe a
    remote PID, and reclaiming it would defeat the lock exactly when two
    machines share a checkout.
    """
    if holder.get("host") and holder["host"] != socket.gethostname():
        return True
    pid = holder.get("pid")
    if not isinstance(pid, int):
        return True  # unreadable payload: be conservative
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True  # exists, owned by someone else
    except OSError:
        return True


def acquire(name: str = "release", holder_info: dict | None = None) -> Path:
    """Acquire the named repo lock or raise LockHeld.

    Registers an atexit hook that releases the lock when this process exits,
    so the caller has nothing to clean up on the happy path.
    """
    lock = _locks_dir() / f"{name}.lock"
    payload = json.dumps({
        "pid": os.getpid(),
        "host": socket.gethostname(),
        "started": datetime.now().isoformat(timespec="seconds"),
        "argv": sys.argv[1:],
        **(holder_info or {}),
    }, indent=2) + "\n"

    for _attempt in (1, 2):
        try:
            fd = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                fh.write(payload)
            atexit.register(release, lock)
            return lock
        except FileExistsError:
            holder = _holder(lock)
            if _holder_alive(holder):
                raise LockHeld(
                    f"lock '{name}' held by pid {holder.get('pid', '?')} on "
                    f"{holder.get('host', '?')} since {holder.get('started', '?')} "
                    f"(argv: {' '.join(holder.get('argv', [])) or '?'}) - "
                    f"lock file: {lock}"
                )
            # Holder is dead on this host: reclaim and retry once.
            release(lock)
    raise LockHeld(f"could not acquire lock '{name}' at {lock}")
