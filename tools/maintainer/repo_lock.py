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

Semantics:
  - an exclusive, non-blocking flock(2) on the lock file, held on an open
    descriptor for as long as the lock is held. The KERNEL drops it when the
    holder exits, however it exits (crash, SIGKILL), so there is no stale lock
    and no reclaim step two contenders could race through (the pid-based
    reclaim this replaced let two processes both delete-and-recreate a dead
    holder's file and both believe they held the lock)
  - JSON payload {pid, host, started, argv, ...} written into the file while
    held, so a held lock is self-documenting; release empties it and unlocks,
    and never deletes the file (deleting a lock file another process has
    already opened is how two holders appear)
  - released via atexit on normal or exceptional interpreter exit, and by an
    explicit release() that only ever touches a lock THIS process holds
  - where fcntl is unavailable (not macOS or Linux) it falls back to the old
    O_CREAT|O_EXCL file with pid-based stale reclaim, ownership-aware

Pure stdlib. Not for NFS; this repo is a local checkout, which is the supported
case.
"""

from __future__ import annotations

import atexit
import errno
import functools
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


def _locks_dir(root: Path | None = None) -> Path:
    """Locks directory shared by every worktree of this repo.

    `git rev-parse --git-common-dir` returns the shared .git directory even
    when run inside a linked worktree. Fall back to the working-tree
    .locks/ dir (onboard.py's location) only if git itself is unavailable -
    degraded, but better than no lock at all.

    `root` names the repository whose common dir anchors the lock; it defaults
    to the checkout this module lives in. merge_when_green.py passes the
    repository it is operating on, so its per-branch locks (and its self-test's
    throwaway repositories) are serialized against the right clone.
    """
    root = Path(root) if root is not None else registry.ROOT
    try:
        p = subprocess.run(
            ["git", "-C", str(root), "rev-parse", "--git-common-dir"],
            capture_output=True, text=True, timeout=10,
        )
        if p.returncode == 0 and p.stdout.strip():
            common = Path(p.stdout.strip())
            if not common.is_absolute():
                common = (root / common).resolve()
            d = common / "msp-skills-locks"
            d.mkdir(parents=True, exist_ok=True)
            return d
    except (OSError, subprocess.SubprocessError):
        pass
    d = root / "tools" / "maintainer" / ".locks"
    d.mkdir(parents=True, exist_ok=True)
    gi = d / ".gitignore"
    if not gi.exists():
        gi.write_text("*\n", encoding="utf-8")
    return d


try:
    import fcntl
except ImportError:  # not POSIX: fall back to the exclusive-create scheme
    fcntl = None  # type: ignore[assignment]

# The exit hook registered for each lock this process acquired, so an explicit
# release() can unregister exactly that one (atexit.unregister(release) would
# drop every lock's hook at once).
_EXIT_HOOKS: dict[str, object] = {}
# The open descriptor carrying each flock this process holds.
_HELD_FDS: dict[str, int] = {}


def _owned_by_me(lock: Path) -> bool:
    holder = _holder(lock)
    return holder.get("pid") == os.getpid() and holder.get("host") == socket.gethostname()


def release(lock: Path) -> None:
    """Release a lock THIS process holds, and drop its exit hook.

    Ownership-aware: once this process has released a lock, another process
    may acquire the same name, and a later release (an exit hook, or a second
    call) must not delete THEIR lock file. So the file is removed only while it
    still names this process."""
    hook = _EXIT_HOOKS.pop(str(lock), None)
    if hook is not None:
        atexit.unregister(hook)  # type: ignore[arg-type]
    fd = _HELD_FDS.pop(str(lock), None)
    if fd is not None:
        try:
            os.ftruncate(fd, 0)
            fcntl.flock(fd, fcntl.LOCK_UN)  # type: ignore[union-attr]
        except OSError:
            pass
        finally:
            os.close(fd)
        return
    if fcntl is not None:
        return  # flock mode and we hold nothing under this name: touch nothing
    try:
        if _owned_by_me(lock):
            lock.unlink(missing_ok=True)
    except OSError:
        pass


def _remove_stale(lock: Path) -> None:
    """Remove a lock whose holder is dead (reclaim)."""
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


def acquire(name: str = "release", holder_info: dict | None = None,
            root: Path | None = None) -> Path:
    """Acquire the named repo lock or raise LockHeld.

    Registers an atexit hook that releases the lock when this process exits,
    so the caller has nothing to clean up on the happy path. `root` selects the
    repository whose common dir holds the lock (default: this checkout).
    """
    lock = _locks_dir(root) / f"{name}.lock"
    payload = json.dumps({
        "pid": os.getpid(),
        "host": socket.gethostname(),
        "started": datetime.now().isoformat(timespec="seconds"),
        "argv": sys.argv[1:],
        **(holder_info or {}),
    }, indent=2) + "\n"

    if fcntl is not None:
        if str(lock) in _HELD_FDS:
            raise LockHeld(f"lock '{name}' is already held by this process - lock file: {lock}")
        fd = os.open(lock, os.O_CREAT | os.O_RDWR, 0o644)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError as exc:
            os.close(fd)
            if exc.errno not in (errno.EWOULDBLOCK, errno.EAGAIN, errno.EACCES):
                raise
            holder = _holder(lock)
            raise LockHeld(
                f"lock '{name}' held by pid {holder.get('pid', '?')} on "
                f"{holder.get('host', '?')} since {holder.get('started', '?')} "
                f"(argv: {' '.join(holder.get('argv', [])) or '?'}) - "
                f"lock file: {lock}"
            )
        os.ftruncate(fd, 0)
        os.write(fd, payload.encode("utf-8"))
        _HELD_FDS[str(lock)] = fd
        hook = functools.partial(release, lock)
        _EXIT_HOOKS[str(lock)] = hook
        atexit.register(hook)
        return lock

    for _attempt in (1, 2):
        try:
            fd = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY)
            with os.fdopen(fd, "w", encoding="utf-8") as fh:
                fh.write(payload)
            hook = functools.partial(release, lock)
            _EXIT_HOOKS[str(lock)] = hook
            atexit.register(hook)
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
            _remove_stale(lock)
    raise LockHeld(f"could not acquire lock '{name}' at {lock}")
