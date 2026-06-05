#!/usr/bin/env python3
"""Deterministic content hash of a skill's vendored CLI source tree.

The hash is the single fact that distinguishes "the binary the docs describe"
from "the binary we last cut a release for". release_state.py compares the
current hash against the `cli_hash_at_release` stamped in skills.json to decide
whether a slug has a binary-level change pending a new release.

The hash is computed over the GIT-TRACKED files under cli/ (so it is identical
in every checkout of the same commit - stray gitignored files like .DS_Store or
local build junk can never poison it), folding in each file's POSIX-relative
path so a rename changes the hash, and a NUL separator so two files cannot
collide by concatenation. Files are visited in sorted order of their POSIX
relative-path string, which is identical on macOS, Linux, and Windows - the
hash is byte-for-byte reproducible across platforms and checkouts. Outside a
git work tree it falls back to a filesystem walk minus well-known junk files.

Pure stdlib (hashlib + subprocess for git). Run locally:
    python3 tools/maintainer/cli_hash.py <slug>
    python3 tools/maintainer/cli_hash.py --dir <path>
"""

from __future__ import annotations

import hashlib
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SKILLS_DIR = registry.SKILLS_DIR

# Fallback-walk exclusions (only used when the dir is not in a git work tree).
JUNK_NAMES = {".DS_Store", "Thumbs.db"}
JUNK_DIRS = {"__pycache__", ".git"}


def _tracked_files(cli_dir: Path) -> list[Path] | None:
    """Files under cli_dir that git would commit: tracked + untracked-but-not-
    ignored (so a freshly vendored tree hashes correctly BEFORE `git add`, and
    gitignored junk like .DS_Store can never poison the hash). Returns None
    when cli_dir is not in a git work tree (caller falls back to an fs walk)."""
    p = subprocess.run(
        ["git", "-C", str(cli_dir), "ls-files", "-z", "--cached", "--others",
         "--exclude-standard", "--", "."],
        capture_output=True,
    )
    if p.returncode != 0:
        return None
    rels = [r for r in p.stdout.decode("utf-8").split("\0") if r]
    return [cli_dir / r for r in rels]


def compute_cli_hash(cli_dir: Path) -> str:
    """Return 'sha256:<hex>' over cli_dir's tracked files, deterministically.

    Folding order: for each file in sorted POSIX-relpath order, update the
    digest with the relpath bytes, a NUL, the file bytes, and a NUL.
    """
    files = _tracked_files(cli_dir)
    if files is None:
        files = [
            p for p in cli_dir.rglob("*")
            if p.is_file()
            and p.name not in JUNK_NAMES
            and not (set(p.relative_to(cli_dir).parts[:-1]) & JUNK_DIRS)
        ]
    h = hashlib.sha256()
    for f in sorted(files, key=lambda p: p.relative_to(cli_dir).as_posix()):
        if not f.is_file():
            continue  # tracked but deleted in working tree
        rel = f.relative_to(cli_dir).as_posix()
        h.update(rel.encode("utf-8"))
        h.update(b"\0")
        h.update(f.read_bytes())
        h.update(b"\0")
    return "sha256:" + h.hexdigest()


def main(argv: list[str]) -> int:
    if len(argv) == 2 and argv[0] == "--dir":
        cli_dir = Path(argv[1]).expanduser().resolve()
    elif len(argv) == 1 and not argv[0].startswith("-"):
        cli_dir = SKILLS_DIR / argv[0] / "cli"
    else:
        print("usage: cli_hash.py <slug> | cli_hash.py --dir <path>", file=sys.stderr)
        return 2

    if not cli_dir.is_dir():
        print(f"cli_hash: not a directory: {cli_dir}", file=sys.stderr)
        return 2

    print(compute_cli_hash(cli_dir))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
