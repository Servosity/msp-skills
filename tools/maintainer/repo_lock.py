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

Semantics. ONE protocol, shared with every older copy of this file (the
O_EXCL copy release.py and release_batch.sh used before; a paused session can
restart from an old worktree, so both copies must exclude each other):
  - exclusion is an atomic create of `<name>.lock` (os.O_CREAT | os.O_EXCL)
    holding the JSON payload {pid, host, started, argv, ...} the old copy reads
  - while held, the creator also keeps an flock(2) on that open descriptor, as
    a LIVENESS signal only: the kernel drops it however the holder exits
  - on EEXIST: an flock held by someone means a live holder (refused); no
    flock means an old copy holds it or the holder died, so the old pid rule
    decides (a live pid on this host, a foreign host, or an unreadable payload
    is alive). A dead holder's file is unlinked only while holding its flock
    and only if the path still names that same inode, then the create retries
  - release unlinks the file, as the old copy does, only while the path still
    names this process's inode and only for the handle this acquisition
    returned, in the process that acquired it (an old handle, a forked child or
    a second release touch nothing); via atexit too
  - without fcntl (not macOS or Linux) the same protocol runs without the flock
  - residual, as between two old copies: an old copy's unlocked reclaim can
    race a new copy's reclaim of the same dead holder's file

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
# Each flock this process holds: (descriptor, pid that acquired it, the exact
# handle object acquire() returned). release() acts only for that handle in
# that process, so an old handle cannot unlock a later acquisition and a forked
# child cannot unlock its parent's lock (flock is shared by the open file
# description a fork duplicates).
_HELD_FDS: dict[str, tuple[int, int, Path]] = {}


def _forget_inherited() -> None:
    """In a forked child: drop the parent's locks from our bookkeeping and
    close the inherited descriptors WITHOUT unlocking (closing a duplicate
    does not release the parent's flock; LOCK_UN would)."""
    for key, (fd, _pid, _handle) in list(_HELD_FDS.items()):
        try:
            os.close(fd)
        except OSError:
            pass
        hook = _EXIT_HOOKS.pop(key, None)
        if hook is not None:
            atexit.unregister(hook)  # type: ignore[arg-type]
    _HELD_FDS.clear()


if hasattr(os, "register_at_fork"):
    os.register_at_fork(after_in_child=_forget_inherited)


def _same_file(fd: int, path: Path) -> bool:
    """True while `path` still names the inode open on `fd`."""
    try:
        a, b = os.fstat(fd), os.stat(path)
    except OSError:
        return False
    return (a.st_dev, a.st_ino) == (b.st_dev, b.st_ino)


def release(lock: Path) -> None:
    """Release a lock THIS process holds, and drop its exit hook. Pass the
    handle acquire() returned: a different Path object naming the same file
    (an old handle from an earlier acquisition) releases nothing, and so does a
    forked child or a second call. The file is unlinked (as the old copy does)
    only while the path still names this acquisition's inode, so a lock another
    process created since is never deleted; closing the descriptor then drops
    the flock."""
    held = _HELD_FDS.get(str(lock))
    if held is None or held[2] is not lock or held[1] != os.getpid():
        return  # not THIS acquisition in THIS process: touch nothing
    _HELD_FDS.pop(str(lock))
    hook = _EXIT_HOOKS.pop(str(lock), None)
    if hook is not None:
        atexit.unregister(hook)  # type: ignore[arg-type]
    fd = held[0]
    try:
        if _same_file(fd, lock):
            lock.unlink()
    except OSError:
        pass
    finally:
        os.close(fd)


def _holder_of(fd: int) -> dict:
    """The payload of the inode open on `fd` (not whatever the path names now)."""
    try:
        os.lseek(fd, 0, os.SEEK_SET)
        chunks = []
        while True:
            b = os.read(fd, 65536)
            if not b:
                break
            chunks.append(b)
        return json.loads(b"".join(chunks).decode("utf-8"))
    except (OSError, ValueError):
        return {}


def _held_message(name: str, holder: dict, lock: Path) -> str:
    return (f"lock '{name}' held by pid {holder.get('pid', '?')} on "
            f"{holder.get('host', '?')} since {holder.get('started', '?')} "
            f"(argv: {' '.join(holder.get('argv', [])) or '?'}) - "
            f"lock file: {lock}")


def _reclaim_if_dead(lock: Path, name: str) -> None:
    """The lock file exists. Raise LockHeld if its holder may be alive;
    otherwise remove it (only while holding its flock, and only if the path
    still names the inode inspected) so the caller can retry the create."""
    try:
        fd = os.open(lock, os.O_RDONLY)
    except FileNotFoundError:
        return  # released meanwhile: retry the create
    try:
        if fcntl is not None:
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except OSError as exc:
                if exc.errno not in (errno.EWOULDBLOCK, errno.EAGAIN, errno.EACCES):
                    raise
                raise LockHeld(_held_message(name, _holder_of(fd), lock))  # a live holder's liveness flock
        holder = _holder_of(fd)
        if _holder_alive(holder):  # an old copy's holder, a foreign host, or a half-written payload
            raise LockHeld(_held_message(name, holder, lock))
        if _same_file(fd, lock):
            lock.unlink()
    except FileNotFoundError:
        pass
    finally:
        os.close(fd)  # drops our flock


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

    if str(lock) in _HELD_FDS:
        raise LockHeld(f"lock '{name}' is already held by this process - lock file: {lock}")
    for _attempt in range(3):
        try:
            fd = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_RDWR, 0o644)
        except FileExistsError:
            _reclaim_if_dead(lock, name)  # raises LockHeld while the holder may be alive
            continue
        try:
            if fcntl is not None:
                # The liveness signal. Blocking: a reclaimer holds it only for
                # the moment it reads our (still empty, so "alive") payload.
                fcntl.flock(fd, fcntl.LOCK_EX)
            os.write(fd, payload.encode("utf-8"))
        except BaseException:
            if _same_file(fd, lock):
                lock.unlink()
            os.close(fd)
            raise
        lock = Path(lock)  # a fresh object: THIS acquisition's handle
        _HELD_FDS[str(lock)] = (fd, os.getpid(), lock)
        hook = functools.partial(release, lock)
        _EXIT_HOOKS[str(lock)] = hook
        atexit.register(hook)
        return lock
    raise LockHeld(f"could not acquire lock '{name}' at {lock}")
