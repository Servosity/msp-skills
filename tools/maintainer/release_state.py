#!/usr/bin/env python3
"""Report each skill's release state: is a new tag pending, and why?

For every slug in the registry this answers one question - "does this skill need
a release, and what kind?" - by comparing three facts:

  current_hash   the deterministic content hash of skills/<slug>/cli today
  released_hash  skills.json "cli_hash_at_release" (the hash at the last cut)
  latest_tag     the newest local git tag matching "<slug>-v*" (semver-sorted)

Classification:
  never-released   released_hash is missing/None (no release has ever been cut)
  binary-pending   current_hash != released_hash (the CLI source changed)
  version-pending  hashes match but no tag "<slug>-v<manifest version>" exists
                   (locally; with --remote also consults `git ls-remote`)
  up-to-date       hashes match and the manifest-version tag exists

This is a reporter, not a gate: it always exits 0. Feed --pending to a release
loop, or --json to another tool.

Pure stdlib. Run locally:
    python3 tools/maintainer/release_state.py
    python3 tools/maintainer/release_state.py --json
    python3 tools/maintainer/release_state.py --pending
    python3 tools/maintainer/release_state.py --remote
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cli_hash  # noqa: E402  (local tools/ module)
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR

PENDING_STATES = ("never-released", "binary-pending", "version-pending")


def manifest_version(slug: str) -> str:
    """Read the "version" field from skills/<slug>/manifest.json (or '0.0.0')."""
    mf = SKILLS_DIR / slug / "manifest.json"
    try:
        return json.loads(mf.read_text(encoding="utf-8")).get("version", "0.0.0")
    except (OSError, json.JSONDecodeError):
        return "0.0.0"


def _version_tuple(v: str) -> tuple[int, ...]:
    """Parse a dotted version into a comparable int tuple; non-numeric -> 0."""
    parts = []
    for chunk in v.split("."):
        num = ""
        for ch in chunk:
            if ch.isdigit():
                num += ch
            else:
                break
        parts.append(int(num) if num else 0)
    return tuple(parts)


def _git(args: list[str]) -> str:
    """Run a git command from ROOT; return stdout, or '' on any failure."""
    try:
        out = subprocess.run(
            ["git", *args],
            cwd=str(ROOT),
            capture_output=True,
            text=True,
            timeout=15,
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return out.stdout if out.returncode == 0 else ""


def latest_tag(slug: str) -> tuple[str | None, str | None]:
    """Return (tag, version) for the highest-semver local tag '<slug>-v*'.

    Sorts by parsed version tuple in pure Python (not `sort -V`)."""
    prefix = f"{slug}-v"
    raw = _git(["tag", "--list", f"{prefix}*"])
    tags = [t.strip() for t in raw.splitlines() if t.strip().startswith(prefix)]
    if not tags:
        return None, None
    best = max(tags, key=lambda t: _version_tuple(t[len(prefix):]))
    return best, best[len(prefix):]


def tag_exists_local(tag: str) -> bool:
    raw = _git(["tag", "--list", tag])
    return any(line.strip() == tag for line in raw.splitlines())


def tag_exists_remote(tag: str) -> bool:
    raw = _git(["ls-remote", "--tags", "origin", tag])
    return bool(raw.strip())


def classify(slug: str, remote: bool) -> dict:
    cli_dir = SKILLS_DIR / slug / "cli"
    current = cli_hash.compute_cli_hash(cli_dir) if cli_dir.is_dir() else None
    entry = registry.skills().get(slug, {})
    released = entry.get("cli_hash_at_release")
    version = manifest_version(slug)
    lt, _lt_ver = latest_tag(slug)

    version_tag = f"{slug}-v{version}"
    if released is None:
        state = "never-released"
    elif current != released:
        state = "binary-pending"
    else:
        have_tag = tag_exists_local(version_tag)
        if not have_tag and remote:
            have_tag = tag_exists_remote(version_tag)
        state = "up-to-date" if have_tag else "version-pending"

    return {
        "state": state,
        "version": version,
        "latest_tag": lt,
        "current_hash": current,
        "released_hash": released,
    }


def main(argv: list[str]) -> int:
    as_json = "--json" in argv
    pending_only = "--pending" in argv
    remote = "--remote" in argv

    slugs = sorted(registry.skills())
    results = {slug: classify(slug, remote) for slug in slugs}

    if pending_only:
        for slug in slugs:
            if results[slug]["state"] in PENDING_STATES:
                print(slug)
        return 0

    if as_json:
        print(json.dumps(results, indent=2))
        return 0

    # Aligned table.
    headers = ("slug", "state", "version", "latest tag", "hash-match")
    rows = []
    for slug in slugs:
        r = results[slug]
        match = "n/a" if r["released_hash"] is None else (
            "yes" if r["current_hash"] == r["released_hash"] else "no"
        )
        rows.append((
            slug,
            r["state"],
            r["version"],
            r["latest_tag"] or "-",
            match,
        ))

    widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(str(cell)))

    def fmt(cells: tuple[str, ...]) -> str:
        return "  ".join(str(c).ljust(widths[i]) for i, c in enumerate(cells))

    print(fmt(headers))
    print(fmt(tuple("-" * w for w in widths)))
    for row in rows:
        print(fmt(row))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
